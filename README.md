# fb2md

Convert FB2/EPUB ebooks to Markdown for use as AI/LLM context.

## Install

```
go build -o fb2md .
```

## Usage

```
fb2md book.fb2                  # → book.md
fb2md book.fb2 output.md        # explicit output path
fb2md books/                    # convert all files in directory
fb2md -o out/ books/            # batch to specified directory
fb2md -i book.fb2               # extract embedded images
```

Flags go before file arguments.

| Flag | Short | Description |
|------|-------|-------------|
| `--output-dir` | `-o` | Output directory (batch mode) |
| `--images` | `-i` | Extract embedded images |
| `--images-dir` | | Custom images directory |
| `--version` | `-v` | Print version |

## Supported formats

- **FB2** (FictionBook 2.x) — including windows-1251 / koi8-r encoded files
- **EPUB**

## Credits

Based on [fb2md](https://github.com/rocketmandrey/fb2md) by rocketmandrey — extended with footnotes, poems, citations, tables, encoding detection, and simplified CLI.
