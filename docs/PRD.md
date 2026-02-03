# fb2md — FB2/EPUB to Markdown Converter

## Problem

There is a large amount of literature in FB2 (FictionBook) format — an XML-based ebook standard popular in Russian-speaking countries. Modern AI/LLM systems (Claude, ChatGPT, Google NotebookLM, etc.) work well with Markdown but cannot process FB2 files directly. A reliable conversion tool is needed to make this literature accessible as AI context.

## Goal

A CLI utility that converts FB2 and EPUB ebooks to clean Markdown, optimized for consumption by AI/LLM agents. The primary goal is **semantic preservation** — no meaning should be lost during conversion. Visual formatting is secondary since the reader is an AI model, not a human.

## Non-Goals

- Web UI or GUI
- PDF conversion
- AI-powered translation or text improvement
- Publishing to external platforms

## Usage

```
fb2md book.fb2                     # → book.md in current directory
fb2md book.fb2 output.md           # → explicit output path
fb2md books/                       # → convert all fb2/epub files in directory
fb2md books/ --output-dir out/     # → batch convert to specified directory
fb2md book.fb2 --images            # → extract embedded images
```

## Supported Input Formats

| Format | Extension | Description |
|--------|-----------|-------------|
| FictionBook 2.x | `.fb2` | XML-based ebook format (FictionBook 2.0/2.1/2.2) |
| EPUB | `.epub` | ZIP-based ebook format with XHTML content |

## FB2 Element Coverage

Based on the [FictionBook 2.1 XSD schema](https://github.com/gribuser/fb2/blob/master/FictionBook.xsd):

### Metadata
- Book title, authors (first/middle/last name, nickname)
- Genres, annotation, date
- Series (sequence name + number)

### Block Elements
- `section` — hierarchical chapters → Markdown headings (h1–h6)
- `p` — paragraphs with inline formatting
- `poem` → `stanza` → `v` (verse lines) — poetry with stanza separation
- `cite` — citations with author attribution → blockquotes
- `epigraph` — epigraphs with author → blockquotes
- `table` → `tr` → `td`/`th` — tables → Markdown tables
- `subtitle` — subheadings → bold text
- `empty-line` — vertical spacing
- `annotation` — section-level annotations
- `image` — block-level images

### Inline Elements
- `emphasis` → `*italic*`
- `strong` → `**bold**`
- `strikethrough` → `~~strikethrough~~`
- `code` → `` `code` ``
- `a` — hyperlinks → `[text](url)`
- `a[type="note"]` — footnote references → `[^N]`
- `sub`, `sup` — subscript/superscript → plain text
- `image` — inline images

### Footnotes
- `body[name="notes"]` sections → Markdown footnotes `[^N]: text`
- Inline note references `<a type="note">` → `[^N]`

### Binary Data
- Base64-encoded images → extracted to files (with `--images` flag)
- Supported formats: JPEG, PNG, GIF

### Encoding
- UTF-8 (native)
- Windows-1251, KOI8-R (auto-detected from XML declaration, converted to UTF-8)

## CLI Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output-dir` | `-o` | `.` | Output directory for batch conversion |
| `--images` | `-i` | `false` | Extract embedded images |
| `--images-dir` | | `<output>_images` | Directory for extracted images |

## Technical Details

- **Language:** Go
- **Dependencies:** `github.com/beevik/etree` (XML), `golang.org/x/text` (encoding)
- **Distribution:** Single static binary
- **Platforms:** macOS, Linux, Windows
