package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

func main() {
	log.SetFlags(0)

	images := flag.Bool("images", false, "extract embedded images")
	flag.BoolVar(images, "i", false, "extract embedded images (shorthand)")

	imagesDir := flag.String("images-dir", "", "directory for extracted images (default: <output>_images)")

	outputDir := flag.String("output-dir", "", "output directory for batch conversion")
	flag.StringVar(outputDir, "o", "", "output directory for batch conversion (shorthand)")

	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `fb2md — convert FB2/EPUB ebooks to Markdown

Usage:
  fb2md book.fb2                  convert to book.md in current directory
  fb2md book.fb2 output.md        convert to explicit output path
  fb2md books/                    convert all fb2/epub files in directory
  fb2md -o out/ books/            batch convert to specified directory
  fb2md -i book.fb2               convert and extract images

Flags must come before file arguments.

Flags:
`)
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Println("fb2md", version)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	input := args[0]

	info, err := os.Stat(input)
	if err != nil {
		log.Fatalf("error: %s: %v", input, err)
	}

	if info.IsDir() {
		dir := *outputDir
		if dir == "" {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("error: cannot create output directory: %v", err)
		}
		n, err := convertDirectory(input, dir, *images, *imagesDir)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		fmt.Printf("converted %d file(s)\n", n)
		return
	}

	// Single file conversion
	output := ""
	if len(args) >= 2 {
		output = args[1]
	} else {
		base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		if *outputDir != "" {
			if err := os.MkdirAll(*outputDir, 0755); err != nil {
				log.Fatalf("error: cannot create output directory: %v", err)
			}
			output = filepath.Join(*outputDir, base+".md")
		} else {
			output = base + ".md"
		}
	}

	if err := convertFile(input, output, *images, *imagesDir); err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("%s -> %s\n", input, output)
}

func convertFile(input, output string, extractImages bool, imagesDir string) error {
	ext := strings.ToLower(filepath.Ext(input))

	switch ext {
	case ".fb2":
		converter := NewConverter()
		if extractImages && imagesDir == "" {
			imagesDir = strings.TrimSuffix(output, filepath.Ext(output)) + "_images"
		}
		return converter.Convert(input, output, extractImages, imagesDir)
	case ".epub":
		converter := NewEpubConverter()
		return converter.Convert(input, output)
	default:
		return fmt.Errorf("unsupported format: %s", ext)
	}
}

func convertDirectory(dir, outputDir string, extractImages bool, imagesDir string) (int, error) {
	var count int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".fb2" && ext != ".epub" {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		base := strings.TrimSuffix(rel, filepath.Ext(rel))
		safeName := strings.ReplaceAll(base, string(filepath.Separator), "_")
		outPath := filepath.Join(outputDir, safeName+".md")

		if err := convertFile(path, outPath, extractImages, imagesDir); err != nil {
			log.Printf("warning: %s: %v", path, err)
			return nil
		}
		fmt.Printf("%s -> %s\n", path, outPath)
		count++
		return nil
	})

	return count, err
}
