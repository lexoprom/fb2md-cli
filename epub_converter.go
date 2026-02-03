package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/beevik/etree"
)

type EpubConverter struct {
	files map[string]*zip.File
}

func NewEpubConverter() *EpubConverter {
	return &EpubConverter{
		files: make(map[string]*zip.File),
	}
}

func (e *EpubConverter) Convert(inputFile, outputFile string) error {
	reader, err := zip.OpenReader(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open EPUB: %w", err)
	}
	defer reader.Close()

	e.files = make(map[string]*zip.File)
	for _, f := range reader.File {
		e.files[f.Name] = f
	}

	rootFile, err := e.findRootFile()
	if err != nil {
		return err
	}

	spineDocs, err := e.getSpineDocuments(rootFile)
	if err != nil {
		return err
	}

	var output strings.Builder
	for _, docPath := range spineDocs {
		content, err := e.readFile(docPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to read %s: %v\n", docPath, err)
			continue
		}

		markdown := e.xhtmlToMarkdown(content)
		if strings.TrimSpace(markdown) == "" {
			// fmt.Printf("Warning: empty markdown for %s\n", docPath)
			continue
		}

		// fmt.Printf("Successfully converted %s (%d bytes)\n", docPath, len(markdown))

		output.WriteString(markdown)
		if !strings.HasSuffix(markdown, "\n\n") {
			output.WriteString("\n\n")
		}
	}

	if err := os.WriteFile(outputFile, []byte(output.String()), 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

func (e *EpubConverter) findRootFile() (string, error) {
	container, err := e.readFile("META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("failed to read container.xml: %w", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(container); err != nil {
		return "", fmt.Errorf("failed to parse container.xml: %w", err)
	}

	rootFileElem := doc.FindElement(".//rootfile")
	if rootFileElem == nil {
		return "", fmt.Errorf("invalid EPUB: rootfile not found")
	}

	rootPath := rootFileElem.SelectAttrValue("full-path", "")
	if rootPath == "" {
		return "", fmt.Errorf("invalid EPUB: rootfile path missing")
	}

	return rootPath, nil
}

func (e *EpubConverter) getSpineDocuments(rootFile string) ([]string, error) {
	data, err := e.readFile(rootFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read root file %s: %w", rootFile, err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", rootFile, err)
	}

	manifest := doc.FindElement(".//manifest")
	spine := doc.FindElement(".//spine")
	if manifest == nil || spine == nil {
		return nil, fmt.Errorf("invalid EPUB: manifest or spine missing")
	}

	hrefByID := make(map[string]string)
	baseDir := path.Dir(rootFile)

	for _, item := range manifest.SelectElements("item") {
		id := item.SelectAttrValue("id", "")
		href := item.SelectAttrValue("href", "")
		if id == "" || href == "" {
			continue
		}

		hrefByID[id] = path.Clean(path.Join(baseDir, href))
	}

	var docs []string
	for _, itemRef := range spine.SelectElements("itemref") {
		idRef := itemRef.SelectAttrValue("idref", "")
		if idRef == "" {
			continue
		}

		if href, ok := hrefByID[idRef]; ok {
			docs = append(docs, href)
		}
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no spine documents found in EPUB")
	}

	return docs, nil
}

func (e *EpubConverter) readFile(name string) ([]byte, error) {
	file, ok := e.files[name]
	if !ok {
		return nil, fmt.Errorf("file %s not found in EPUB", name)
	}

	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", name, err)
	}

	return data, nil
}

func (e *EpubConverter) xhtmlToMarkdown(content []byte) string {
	// Replace incompatible entities
	contentStr := string(content)
	contentStr = strings.ReplaceAll(contentStr, "&nbsp;", "&#160;")

	doc := etree.NewDocument()
	if err := doc.ReadFromString(contentStr); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse XHTML: %v\n", err)
		return ""
	}

	// Debug structure
	// fmt.Printf("Root element: %s (Space: %s)\n", doc.Root().Tag, doc.Root().Space)
	// Debug structure
	// for _, child := range doc.Root().ChildElements() {
	// 	// fmt.Printf("  Child: %s (Space: %s)\n", child.Tag, child.Space)
	// }

	body := doc.FindElement(".//body")
	if body == nil {
		// Try with namespace
		body = doc.FindElement(".//{http://www.w3.org/1999/xhtml}body")
	}
	if body == nil {
		// fmt.Printf("Error: body not found in content (len: %d)\n", len(content))
		return ""
	}

	var output strings.Builder
	for _, child := range body.ChildElements() {
		e.renderBlock(child, &output)
	}

	return strings.TrimSpace(output.String()) + "\n"
}

func (e *EpubConverter) renderBlock(elem *etree.Element, output *strings.Builder) {
	tag := strings.ToLower(elem.Tag)

	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := tag[1] - '0'
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		output.WriteString(strings.Repeat("#", int(level)))
		output.WriteString(" ")
		output.WriteString(e.extractText(elem))
		output.WriteString("\n\n")
	case "p", "div":
		e.renderInline(elem, output)
		output.WriteString("\n\n")
	case "blockquote":
		var inner strings.Builder
		for _, child := range elem.ChildElements() {
			e.renderInline(child, &inner)
			inner.WriteString("\n")
		}
		lines := strings.Split(strings.TrimSpace(inner.String()), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			output.WriteString("> ")
			output.WriteString(strings.TrimSpace(line))
			output.WriteString("\n")
		}
		output.WriteString("\n")
	case "ul":
		e.renderList(elem, output, false)
	case "ol":
		e.renderList(elem, output, true)
	case "img":
		src := elem.SelectAttrValue("src", "")
		alt := elem.SelectAttrValue("alt", "")
		output.WriteString(fmt.Sprintf("![%s](%s)\n\n", alt, src))
	case "br":
		output.WriteString("  \n")
	case "hr":
		output.WriteString("\n---\n\n")
	default:
		e.renderInline(elem, output)
		output.WriteString("\n\n")
	}
}

func (e *EpubConverter) renderList(list *etree.Element, output *strings.Builder, ordered bool) {
	items := list.SelectElements("li")
	for i, item := range items {
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", i+1)
		}
		output.WriteString(prefix)
		e.renderInline(item, output)
		output.WriteString("\n")
	}
	output.WriteString("\n")
}

func (e *EpubConverter) renderInline(elem *etree.Element, output *strings.Builder) {
	appendInlineText(output, normalizeInlineWhitespace(elem.Text(), false, true))

	for _, child := range elem.ChildElements() {
		switch strings.ToLower(child.Tag) {
		case "em", "i":
			output.WriteString("*")
			e.renderInline(child, output)
			output.WriteString("*")
		case "strong", "b":
			output.WriteString("**")
			e.renderInline(child, output)
			output.WriteString("**")
		case "code":
			output.WriteString("`")
			e.renderInline(child, output)
			output.WriteString("`")
		case "a":
			href := child.SelectAttrValue("href", "")
			linkText := e.extractText(child)
			if linkText == "" {
				linkText = href
			}
			output.WriteString("[")
			output.WriteString(linkText)
			output.WriteString("]")
			output.WriteString("(")
			output.WriteString(href)
			output.WriteString(")")
		case "img":
			src := child.SelectAttrValue("src", "")
			alt := child.SelectAttrValue("alt", "")
			output.WriteString(fmt.Sprintf("![%s](%s)", alt, src))
		case "br":
			output.WriteString("  \n")
		default:
			e.renderInline(child, output)
		}

		appendInlineText(output, normalizeInlineWhitespace(child.Tail(), true, true))
	}
}

func (e *EpubConverter) extractText(elem *etree.Element) string {
	var text strings.Builder

	if elem.Text() != "" {
		text.WriteString(elem.Text())
	}

	for _, child := range elem.ChildElements() {
		text.WriteString(e.extractText(child))
		if child.Tail() != "" {
			text.WriteString(child.Tail())
		}
	}

	return strings.TrimSpace(text.String())
}

func normalizeInlineWhitespace(s string, preserveLeading, preserveTrailing bool) string {
	if s == "" {
		return ""
	}

	hasLeading := preserveLeading && strings.TrimLeftFunc(s, unicode.IsSpace) != s
	hasTrailing := preserveTrailing && strings.TrimRightFunc(s, unicode.IsSpace) != s

	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		if hasLeading || hasTrailing {
			return " "
		}
		return ""
	}

	collapsed := strings.Join(strings.Fields(trimmed), " ")
	if hasLeading {
		collapsed = " " + collapsed
	}
	if hasTrailing {
		collapsed += " "
	}
	return collapsed
}

func appendInlineText(output *strings.Builder, s string) {
	if s == "" {
		return
	}

	if output.Len() == 0 {
		if strings.TrimSpace(s) == "" {
			return
		}
		s = strings.TrimLeft(s, " ")
	} else if strings.HasPrefix(s, " ") {
		outStr := output.String()
		if len(outStr) > 0 {
			last := outStr[len(outStr)-1]
			if last == ' ' || last == '\n' {
				s = strings.TrimLeft(s, " ")
				if s == "" {
					return
				}
			}
		}
	}

	output.WriteString(s)
}
