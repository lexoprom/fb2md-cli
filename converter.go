package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/beevik/etree"
)

type Converter struct {
	doc           *etree.Document
	output        *strings.Builder
	outputMain    strings.Builder
	sectionLevel  int
	extractImages bool
	imagesDir     string
	imageCounter  int
	imageFiles    map[string]string
	// Footnotes: map from note ID to note text
	footnotes     map[string]string
	footnoteSeen  map[string]bool
	footnoteOrder []string
}

func stripBase64Whitespace(s string) string {
	if s == "" {
		return ""
	}

	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Normalize separators and strip any path components.
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)

	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	out := strings.Trim(b.String(), "._-")
	if out == "" || out == "." || out == ".." {
		return ""
	}
	return out
}

func NewConverter() *Converter {
	c := &Converter{
		footnotes:    make(map[string]string),
		footnoteSeen: make(map[string]bool),
		imageFiles:   make(map[string]string),
	}
	c.output = &c.outputMain
	return c
}

func (c *Converter) Convert(inputFile, outputFile string, extractImages bool, imagesDir string) error {
	c.extractImages = extractImages
	c.imagesDir = imagesDir

	// Read file as bytes for encoding detection
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read FB2 file: %w", err)
	}

	// Detect and convert encoding to UTF-8
	data, err = detectAndConvertEncoding(data)
	if err != nil {
		return fmt.Errorf("encoding conversion failed: %w", err)
	}

	// Parse FB2 XML
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return fmt.Errorf("failed to parse FB2 file: %w", err)
	}
	c.doc = doc

	// Create images directory if needed
	if c.extractImages && c.imagesDir != "" {
		if err := os.MkdirAll(c.imagesDir, 0755); err != nil {
			return fmt.Errorf("failed to create images directory: %w", err)
		}
	}

	// Find root element
	root := doc.SelectElement("FictionBook")
	if root == nil {
		return fmt.Errorf("invalid FB2 file: FictionBook element not found")
	}

	// Collect image filenames before rendering so Markdown links match written files.
	if c.extractImages {
		c.collectBinaryImageFilenames(root)
	}

	// First pass: collect footnotes from notes bodies
	for _, body := range root.SelectElements("body") {
		name := body.SelectAttrValue("name", "")
		if name == "notes" || name == "footnotes" || name == "comments" {
			c.collectFootnotes(body)
		}
	}

	// Process description (metadata)
	if desc := root.SelectElement("description"); desc != nil {
		c.processDescription(desc)
	}

	// Process main body (skip notes bodies)
	for _, body := range root.SelectElements("body") {
		name := body.SelectAttrValue("name", "")
		if name == "notes" || name == "footnotes" || name == "comments" {
			continue
		}
		c.processBody(body)
	}

	// Append footnotes at the end
	c.writeFootnotes()

	// Extract embedded images
	if c.extractImages {
		c.extractBinaryImages(root)
	}

	// Write output
	if err := os.WriteFile(outputFile, []byte(c.output.String()), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

// collectFootnotes extracts footnote text from notes body sections.
// It recurses into nested sections since notes can be wrapped in a container section.
func (c *Converter) collectFootnotes(elem *etree.Element) {
	for _, section := range elem.SelectElements("section") {
		id := section.SelectAttrValue("id", "")
		if id == "" {
			// Container section without ID — recurse into it
			c.collectFootnotes(section)
			continue
		}
		var noteText strings.Builder
		for _, child := range section.ChildElements() {
			switch child.Tag {
			case "title":
				// Skip title in footnotes — it's usually just the number
			case "p":
				text := c.extractInlineText(child)
				if text != "" {
					if noteText.Len() > 0 {
						noteText.WriteString(" ")
					}
					noteText.WriteString(text)
				}
			case "section":
				// Nested sections inside a note — recurse
				c.collectFootnotes(child)
			default:
				text := c.extractAllText(child)
				if text != "" {
					if noteText.Len() > 0 {
						noteText.WriteString(" ")
					}
					noteText.WriteString(text)
				}
			}
		}
		if noteText.Len() > 0 {
			c.footnotes[id] = noteText.String()
		}
	}
}

// writeFootnotes appends collected footnotes at the end of the document.
func (c *Converter) writeFootnotes() {
	if len(c.footnoteOrder) == 0 {
		return
	}
	c.output.WriteString("\n---\n\n")
	for _, id := range c.footnoteOrder {
		if text, ok := c.footnotes[id]; ok {
			c.output.WriteString(fmt.Sprintf("[^%s]: %s\n\n", id, text))
		}
	}
}

func (c *Converter) processDescription(desc *etree.Element) {
	titleInfo := desc.SelectElement("title-info")
	if titleInfo == nil {
		return
	}

	// Book title
	if title := titleInfo.SelectElement("book-title"); title != nil {
		c.output.WriteString("# ")
		c.output.WriteString(title.Text())
		c.output.WriteString("\n\n")
	}

	// Authors
	authors := titleInfo.SelectElements("author")
	if len(authors) > 0 {
		c.output.WriteString("**Authors:** ")
		authorNames := []string{}
		for _, author := range authors {
			name := c.getAuthorName(author)
			if name != "" {
				authorNames = append(authorNames, name)
			}
		}
		c.output.WriteString(strings.Join(authorNames, ", "))
		c.output.WriteString("\n\n")
	}

	// Genres
	genres := titleInfo.SelectElements("genre")
	if len(genres) > 0 {
		c.output.WriteString("**Genres:** ")
		genreNames := []string{}
		for _, genre := range genres {
			if text := genre.Text(); text != "" {
				genreNames = append(genreNames, text)
			}
		}
		c.output.WriteString(strings.Join(genreNames, ", "))
		c.output.WriteString("\n\n")
	}

	// Series (sequence)
	sequences := titleInfo.SelectElements("sequence")
	for _, seq := range sequences {
		name := seq.SelectAttrValue("name", "")
		number := seq.SelectAttrValue("number", "")
		if name != "" {
			c.output.WriteString("**Series:** ")
			c.output.WriteString(name)
			if number != "" {
				c.output.WriteString(", #")
				c.output.WriteString(number)
			}
			c.output.WriteString("\n\n")
		}
	}

	// Annotation
	if annotation := titleInfo.SelectElement("annotation"); annotation != nil {
		c.output.WriteString("## Annotation\n\n")
		c.processBlockContent(annotation)
		c.output.WriteString("\n")
	}

	// Date
	if date := titleInfo.SelectElement("date"); date != nil {
		if text := date.Text(); text != "" {
			c.output.WriteString("**Date:** ")
			c.output.WriteString(text)
			c.output.WriteString("\n\n")
		}
	}

	// Separator
	c.output.WriteString("---\n\n")
}

func (c *Converter) getAuthorName(author *etree.Element) string {
	parts := []string{}

	if firstName := author.SelectElement("first-name"); firstName != nil {
		parts = append(parts, firstName.Text())
	}
	if middleName := author.SelectElement("middle-name"); middleName != nil {
		parts = append(parts, middleName.Text())
	}
	if lastName := author.SelectElement("last-name"); lastName != nil {
		parts = append(parts, lastName.Text())
	}
	if nickname := author.SelectElement("nickname"); nickname != nil && len(parts) == 0 {
		parts = append(parts, nickname.Text())
	}

	return strings.Join(parts, " ")
}

func (c *Converter) processBody(body *etree.Element) {
	for _, child := range body.ChildElements() {
		switch child.Tag {
		case "title":
			c.output.WriteString("\n## ")
			titleText := c.extractAllText(child)
			c.output.WriteString(titleText)
			c.output.WriteString("\n\n")
		case "epigraph":
			c.processEpigraph(child)
		case "section":
			c.processSection(child)
		case "p":
			c.processParagraph(child)
		case "subtitle":
			c.processSubtitle(child)
		case "empty-line":
			c.output.WriteString("\n")
		case "image":
			c.processImage(child)
		case "poem":
			c.processPoem(child)
		case "cite":
			c.processCite(child)
		case "table":
			c.processTable(child)
		default:
			c.processBlockContent(child)
		}
	}
}

func (c *Converter) processSection(section *etree.Element) {
	c.sectionLevel++
	defer func() { c.sectionLevel-- }()

	// Process section title
	if title := section.SelectElement("title"); title != nil {
		level := c.sectionLevel + 1
		if level > 6 {
			level = 6
		}
		c.output.WriteString(strings.Repeat("#", level))
		c.output.WriteString(" ")
		titleText := c.extractAllText(title)
		c.output.WriteString(titleText)
		c.output.WriteString("\n\n")
	}

	// Process epigraphs
	for _, epigraph := range section.SelectElements("epigraph") {
		c.processEpigraph(epigraph)
	}

	// Process annotation if present in section
	if annotation := section.SelectElement("annotation"); annotation != nil {
		c.processBlockContent(annotation)
	}

	// Process all child elements
	for _, child := range section.ChildElements() {
		switch child.Tag {
		case "title", "epigraph", "annotation":
			// Already processed above
		case "section":
			c.processSection(child)
		case "p":
			c.processParagraph(child)
		case "subtitle":
			c.processSubtitle(child)
		case "empty-line":
			c.output.WriteString("\n")
		case "image":
			c.processImage(child)
		case "poem":
			c.processPoem(child)
		case "cite":
			c.processCite(child)
		case "table":
			c.processTable(child)
		default:
			c.processBlockContent(child)
		}
	}
}

func (c *Converter) processEpigraph(epigraph *etree.Element) {
	for _, child := range epigraph.ChildElements() {
		switch child.Tag {
		case "p":
			c.output.WriteString("> ")
			c.processInlineElement(child)
			c.output.WriteString("\n")
		case "poem":
			c.processQuotedPoem(child)
		case "cite":
			// Nested cite in epigraph — process as quoted
			for _, cc := range child.ChildElements() {
				switch cc.Tag {
				case "p":
					c.output.WriteString("> ")
					c.processInlineElement(cc)
					c.output.WriteString("\n")
				case "text-author":
					c.output.WriteString(">\n> — ")
					c.processInlineElement(cc)
					c.output.WriteString("\n")
				case "empty-line":
					c.output.WriteString(">\n")
				}
			}
		case "text-author":
			c.output.WriteString(">\n> — ")
			c.processInlineElement(child)
			c.output.WriteString("\n")
		case "empty-line":
			c.output.WriteString(">\n")
		}
	}
	c.output.WriteString("\n")
}

// processPoem handles <poem> elements with stanzas and verses.
func (c *Converter) processPoem(poem *etree.Element) {
	// Poem title
	if title := poem.SelectElement("title"); title != nil {
		titleText := c.extractAllText(title)
		if titleText != "" {
			c.output.WriteString("**")
			c.output.WriteString(titleText)
			c.output.WriteString("**\n\n")
		}
	}

	// Epigraphs
	for _, epigraph := range poem.SelectElements("epigraph") {
		c.processEpigraph(epigraph)
	}

	// Process stanzas and subtitles
	for _, child := range poem.ChildElements() {
		switch child.Tag {
		case "title", "epigraph":
			// Already processed
		case "stanza":
			c.processStanza(child)
			c.output.WriteString("\n")
		case "subtitle":
			c.processSubtitle(child)
		}
	}

	// Text author
	for _, author := range poem.SelectElements("text-author") {
		c.output.WriteString("*— ")
		c.processInlineElement(author)
		c.output.WriteString("*\n\n")
	}

	// Date
	if date := poem.SelectElement("date"); date != nil {
		if text := date.Text(); text != "" {
			c.output.WriteString("*")
			c.output.WriteString(text)
			c.output.WriteString("*\n\n")
		}
	}
}

// processQuotedPoem handles poem inside blockquotes (epigraph, cite).
func (c *Converter) processQuotedPoem(poem *etree.Element) {
	if title := poem.SelectElement("title"); title != nil {
		titleText := c.extractAllText(title)
		if titleText != "" {
			c.output.WriteString("> **")
			c.output.WriteString(titleText)
			c.output.WriteString("**\n>\n")
		}
	}

	for _, child := range poem.ChildElements() {
		switch child.Tag {
		case "title", "epigraph":
			// Skip
		case "stanza":
			for _, v := range child.SelectElements("v") {
				c.output.WriteString("> ")
				c.processInlineElement(v)
				c.output.WriteString("\n")
			}
			c.output.WriteString(">\n")
		case "subtitle":
			c.output.WriteString("> **")
			c.processInlineElement(child)
			c.output.WriteString("**\n")
		}
	}

	for _, author := range poem.SelectElements("text-author") {
		c.output.WriteString("> *— ")
		c.processInlineElement(author)
		c.output.WriteString("*\n")
	}
}

// processStanza handles <stanza> elements with verse lines.
func (c *Converter) processStanza(stanza *etree.Element) {
	// Stanza title
	if title := stanza.SelectElement("title"); title != nil {
		titleText := c.extractAllText(title)
		if titleText != "" {
			c.output.WriteString("**")
			c.output.WriteString(titleText)
			c.output.WriteString("**\n")
		}
	}

	// Subtitle
	if subtitle := stanza.SelectElement("subtitle"); subtitle != nil {
		c.output.WriteString("**")
		c.processInlineElement(subtitle)
		c.output.WriteString("**\n")
	}

	// Verse lines — each on its own line with trailing double-space for MD line break
	verses := stanza.SelectElements("v")
	for i, v := range verses {
		c.processInlineElement(v)
		if i < len(verses)-1 {
			c.output.WriteString("  \n") // MD line break
		} else {
			c.output.WriteString("\n")
		}
	}
}

// processCite handles <cite> elements as blockquotes.
func (c *Converter) processCite(cite *etree.Element) {
	for _, child := range cite.ChildElements() {
		switch child.Tag {
		case "p":
			c.output.WriteString("> ")
			c.processInlineElement(child)
			c.output.WriteString("\n>\n")
		case "poem":
			c.processQuotedPoem(child)
		case "subtitle":
			c.output.WriteString("> **")
			c.processInlineElement(child)
			c.output.WriteString("**\n>\n")
		case "empty-line":
			c.output.WriteString(">\n")
		case "table":
			// Tables inside quotes — process inline, not ideal but preserves content
			c.processTable(child)
		case "text-author":
			c.output.WriteString(">\n> — ")
			c.processInlineElement(child)
			c.output.WriteString("\n")
		}
	}
	c.output.WriteString("\n")
}

// processTable handles <table> elements as Markdown tables.
func (c *Converter) processTable(table *etree.Element) {
	rows := table.SelectElements("tr")
	if len(rows) == 0 {
		return
	}

	// Determine column count from first row
	firstRow := rows[0]
	cells := firstRow.SelectElements("th")
	if len(cells) == 0 {
		cells = firstRow.SelectElements("td")
	}
	// Also check mixed th/td
	if len(cells) == 0 {
		cells = firstRow.ChildElements()
	}
	colCount := len(cells)
	if colCount == 0 {
		return
	}

	// Check if first row is a header (has <th> elements)
	hasHeader := len(firstRow.SelectElements("th")) > 0

	if hasHeader {
		// Render header row
		c.output.WriteString("| ")
		for _, cell := range firstRow.ChildElements() {
			if cell.Tag == "th" || cell.Tag == "td" {
				text := c.extractInlineText(cell)
				c.output.WriteString(text)
				c.output.WriteString(" | ")
			}
		}
		c.output.WriteString("\n")

		// Separator row
		c.output.WriteString("|")
		for i := 0; i < colCount; i++ {
			c.output.WriteString(" --- |")
		}
		c.output.WriteString("\n")

		// Data rows (skip first)
		for _, row := range rows[1:] {
			c.renderTableRow(row)
		}
	} else {
		// No header — create empty header for valid MD table
		c.output.WriteString("|")
		for i := 0; i < colCount; i++ {
			c.output.WriteString("  |")
		}
		c.output.WriteString("\n|")
		for i := 0; i < colCount; i++ {
			c.output.WriteString(" --- |")
		}
		c.output.WriteString("\n")

		// All rows as data
		for _, row := range rows {
			c.renderTableRow(row)
		}
	}

	c.output.WriteString("\n")
}

func (c *Converter) renderTableRow(row *etree.Element) {
	c.output.WriteString("| ")
	for _, cell := range row.ChildElements() {
		if cell.Tag == "th" || cell.Tag == "td" {
			text := c.extractInlineText(cell)
			c.output.WriteString(text)
			c.output.WriteString(" | ")
		}
	}
	c.output.WriteString("\n")
}

func (c *Converter) processSubtitle(subtitle *etree.Element) {
	c.output.WriteString("**")
	c.processInlineElement(subtitle)
	c.output.WriteString("**\n\n")
}

func (c *Converter) processParagraph(p *etree.Element) {
	c.processInlineElement(p)
	c.output.WriteString("\n\n")
}

// processBlockContent handles a generic container with block-level children.
func (c *Converter) processBlockContent(elem *etree.Element) {
	for _, child := range elem.ChildElements() {
		switch child.Tag {
		case "p":
			c.processParagraph(child)
		case "empty-line":
			c.output.WriteString("\n")
		case "section":
			c.processSection(child)
		case "subtitle":
			c.processSubtitle(child)
		case "epigraph":
			c.processEpigraph(child)
		case "image":
			c.processImage(child)
		case "poem":
			c.processPoem(child)
		case "cite":
			c.processCite(child)
		case "table":
			c.processTable(child)
		default:
			c.processInlineElement(child)
		}
	}
}

func (c *Converter) processInlineElement(elem *etree.Element) {
	// Process direct text content of this element
	if text := elem.Text(); text != "" {
		c.output.WriteString(text)
	}

	// Process child elements
	for _, child := range elem.ChildElements() {
		switch child.Tag {
		case "emphasis":
			c.output.WriteString("*")
			c.processInlineElement(child)
			c.output.WriteString("*")
		case "strong":
			c.output.WriteString("**")
			c.processInlineElement(child)
			c.output.WriteString("**")
		case "strikethrough":
			c.output.WriteString("~~")
			c.processInlineElement(child)
			c.output.WriteString("~~")
		case "code":
			c.output.WriteString("`")
			c.processInlineElement(child)
			c.output.WriteString("`")
		case "sup":
			c.processInlineElement(child)
		case "sub":
			c.processInlineElement(child)
		case "a":
			c.processLink(child)
		case "image":
			c.processImage(child)
		case "empty-line":
			c.output.WriteString("\n")
		case "style":
			// Named style — just extract text content
			c.processInlineElement(child)
		default:
			c.processInlineElement(child)
		}

		// Process tail text after element
		if tail := child.Tail(); tail != "" {
			c.output.WriteString(tail)
		}
	}
}

// extractInlineText extracts formatted text from an inline element (for table cells etc.)
func (c *Converter) extractInlineText(elem *etree.Element) string {
	var buf strings.Builder
	old := c.output
	c.output = &buf
	c.processInlineElement(elem)
	result := buf.String()
	c.output = old
	return result
}

func (c *Converter) processLink(link *etree.Element) {
	href := link.SelectAttrValue("l:href", "")
	if href == "" {
		href = link.SelectAttrValue("href", "")
	}

	linkType := link.SelectAttrValue("type", "")

	// Handle footnote references
	if linkType == "note" && strings.HasPrefix(href, "#") {
		noteID := strings.TrimPrefix(href, "#")
		if _, exists := c.footnotes[noteID]; exists {
			c.output.WriteString("[^")
			c.output.WriteString(noteID)
			c.output.WriteString("]")
			if !c.footnoteSeen[noteID] {
				c.footnoteSeen[noteID] = true
				c.footnoteOrder = append(c.footnoteOrder, noteID)
			}
			return
		}
	}

	// Regular link
	linkText := c.extractAllText(link)
	if linkText == "" {
		linkText = "Link"
	}

	c.output.WriteString("[")
	c.output.WriteString(linkText)
	c.output.WriteString("](")
	c.output.WriteString(href)
	c.output.WriteString(")")
}

// extractAllText recursively extracts all text from an element and its children.
func (c *Converter) extractAllText(elem *etree.Element) string {
	var text strings.Builder

	if elem.Text() != "" {
		text.WriteString(elem.Text())
	}

	for _, child := range elem.ChildElements() {
		text.WriteString(c.extractAllText(child))
		if child.Tail() != "" {
			text.WriteString(child.Tail())
		}
	}

	return strings.TrimSpace(text.String())
}

func (c *Converter) processImage(img *etree.Element) {
	href := img.SelectAttrValue("l:href", "")
	if href == "" {
		href = img.SelectAttrValue("href", "")
	}

	if strings.HasPrefix(href, "#") {
		imageID := strings.TrimPrefix(href, "#")

		if c.extractImages {
			filename := imageID
			if v, ok := c.imageFiles[imageID]; ok && v != "" {
				filename = v
			} else {
				if safe := sanitizeFilename(imageID); safe != "" {
					filename = safe
				}
			}
			imagePath := filepath.Join(c.imagesDir, filename)
			c.output.WriteString(fmt.Sprintf("![%s](%s)", imageID, filepath.ToSlash(imagePath)))
		} else {
			c.output.WriteString(fmt.Sprintf("![Image: %s]", imageID))
		}
	} else {
		c.output.WriteString(fmt.Sprintf("![Image](%s)", href))
	}
	c.output.WriteString("\n\n")
}

func (c *Converter) extractBinaryImages(root *etree.Element) error {
	for _, binary := range root.SelectElements("binary") {
		id := binary.SelectAttrValue("id", "")
		contentType := binary.SelectAttrValue("content-type", "image/jpeg")

		if id == "" {
			continue
		}

		imageData := stripBase64Whitespace(strings.TrimSpace(binary.Text()))

		decoded, err := base64.StdEncoding.DecodeString(imageData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to decode image %s: %v\n", id, err)
			continue
		}

		filename := c.imageFiles[id]
		if filename == "" {
			ext := ".jpg"
			if strings.Contains(contentType, "png") {
				ext = ".png"
			} else if strings.Contains(contentType, "gif") {
				ext = ".gif"
			}
			filename = id
			if !strings.HasSuffix(filename, ext) {
				filename = filename + ext
			}
		}

		imagePath := filepath.Join(c.imagesDir, filename)
		if err := os.WriteFile(imagePath, decoded, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write image %s: %v\n", id, err)
			continue
		}
	}

	return nil
}

func (c *Converter) collectBinaryImageFilenames(root *etree.Element) {
	used := make(map[string]bool)
	for _, binary := range root.SelectElements("binary") {
		id := binary.SelectAttrValue("id", "")
		contentType := binary.SelectAttrValue("content-type", "image/jpeg")
		if id == "" {
			continue
		}

		ext := ".jpg"
		if strings.Contains(contentType, "png") {
			ext = ".png"
		} else if strings.Contains(contentType, "gif") {
			ext = ".gif"
		}

		base := sanitizeFilename(id)
		if base == "" {
			base = "image"
		}

		filename := base
		if !strings.HasSuffix(strings.ToLower(filename), ext) {
			filename += ext
		}

		if used[filename] {
			for n := 2; ; n++ {
				alt := fmt.Sprintf("%s_%d%s", base, n, ext)
				if !used[alt] {
					filename = alt
					break
				}
			}
		}

		used[filename] = true
		c.imageFiles[id] = filename
	}
}
