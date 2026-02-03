# fb2md — заметки по проекту

## Постановка задачи

CLI утилита для конвертации книг из FB2/EPUB в Markdown. Результат предназначен как контекст для AI/LLM агентов (Claude, ChatGPT, Google NotebookLM и т.д.), которые хорошо понимают Markdown, но не работают с FB2. Имеется большое количество литературы в FB2 формате.

Главный приоритет — **сохранение смысла**. Форматирование вторично, читатель — AI модель, не человек.

## Источники и референсы

### Существующие реализации (изучены как референс)

- **fb2-to-md-converter** (Python, BS4 + lxml) — базовый конвертер, обрабатывает ~5 FB2 элементов (p, subtitle, emphasis, empty-line, metadata). Много теряет.
- **fb2md** (Go, etree + urfave/cli) — зрелый конвертер, 20+ элементов, EPUB поддержка, извлечение картинок. Взят как основа.

Оба проекта лежат в `/Users/boreas/Code/repos/` для справки.

### Спецификация FB2

- [FictionBook 2.1 XSD](https://github.com/gribuser/fb2/blob/master/FictionBook.xsd) — официальная XML-схема
- [FB2 — MobileRead Wiki](https://wiki.mobileread.com/wiki/FB2) — описание формата
- [FictionBook2.21.xsd](https://github.com/larin/librusec7/blob/master/schema/FictionBook2.21.xsd) — расширенная схема
- [fictionbook.org](http://www.fictionbook.org/index.php/Eng:XML_Schema_Fictionbook_2.1) — официальный сайт (часто недоступен)

## Решение

Форк Go-проекта `fb2md` с доработками:
- Убраны Python-скрипты (improve_translation, publish_telegraph, create_epub)
- Убрана зависимость от `urfave/cli` — CLI на stdlib `flag`
- Добавлена поддержка кодировок (windows-1251, koi8-r/u)
- Добавлены недостающие FB2 элементы

## Что добавлено по сравнению с оригинальным fb2md

| Функция | Было | Стало |
|---|---|---|
| Сноски (`body[name="notes"]`) | Обрабатывались как обычный body | Markdown footnotes `[^id]` с определениями в конце |
| Стихи (`poem/stanza/v`) | Не обрабатывались | Строки стиха с MD line breaks, автор курсивом |
| Цитаты (`cite`) | Не обрабатывались | Blockquotes `>` с атрибуцией автора |
| Таблицы (`table/tr/td/th`) | Не обрабатывались | Markdown-таблицы `\| col \|` |
| `sub`/`sup` | Игнорировались | Извлекается текст (plain text) |
| `annotation` в секциях | Только в metadata | Обрабатывается как блочный контент |
| `sequence` (серия) | Не извлекалась | `**Series:** Название, #N` в metadata |
| Кодировка | Только UTF-8 | Авто: windows-1251, koi8-r/u, iso-8859-1 |
| Inline whitespace | `TrimSpace` склеивал слова | Сохраняет пробелы внутри элементов |
| CLI | `urfave/cli` с флагами | stdlib `flag`, позиционные аргументы |

## Покрытие FB2 элементов (по XSD спецификации)

| Элемент | Статус | Маппинг в Markdown |
|---|---|---|
| `body` | ✅ | — |
| `body[name="notes"]` | ✅ | Footnotes `[^id]: text` |
| `section` | ✅ | `## ... ######` (h1–h6) |
| `title` | ✅ | Heading уровня секции |
| `p` | ✅ | Параграф с двойным newline |
| `emphasis` | ✅ | `*italic*` |
| `strong` | ✅ | `**bold**` |
| `strikethrough` | ✅ | `~~text~~` |
| `code` | ✅ | `` `code` `` |
| `a` | ✅ | `[text](url)` |
| `a[type="note"]` | ✅ | `[^id]` |
| `image` | ✅ | `![alt](path)` |
| `subtitle` | ✅ | `**text**` |
| `epigraph` | ✅ | `> text` + `> — author` |
| `empty-line` | ✅ | `\n` |
| `binary` | ✅ | Base64 → файлы (с `-i`) |
| `poem` | ✅ | Заголовок + строфы + автор |
| `stanza` | ✅ | Блок строк с MD line breaks |
| `v` | ✅ | Строка стиха + `  \n` |
| `cite` | ✅ | `> text` + `> — author` |
| `table/tr/td/th` | ✅ | Markdown table `\| \|` |
| `sub`/`sup` | ✅ | Plain text |
| `text-author` | ✅ | `*— Author*` (в epigraph, cite, poem) |
| `annotation` | ✅ | Блочный контент |
| `sequence` | ✅ | `**Series:** name, #N` |
| `style` | ⚠️ | Извлекается текст (редкий элемент) |
| `coverpage` | ❌ | Пропускается (не нужна для AI) |

## Использование

```
fb2md book.fb2                  # → book.md
fb2md book.fb2 output.md        # → явный путь
fb2md books/                    # → все fb2/epub в директории
fb2md -o out/ books/            # → batch в указанную директорию
fb2md -i book.fb2               # → с извлечением картинок
```

Флаги ставятся перед файловыми аргументами (стандартное поведение Go `flag`).

## Зависимости

- `github.com/beevik/etree` — XML-парсер (FB2 и EPUB)
- `golang.org/x/text` — конвертация кодировок

## Структура файлов

```
fb2md-cli/
├── main.go           (133 строки) CLI entry point
├── converter.go      (798 строк) FB2 → Markdown
├── epub_converter.go (336 строк) EPUB → Markdown
├── encoding.go       (65 строк)  Определение кодировок
├── go.mod / go.sum
├── PRD.md                        Описание продукта
└── NOTES.md                      Этот файл
```

## Тестирование

Проверено на реальных файлах:
- `504451.fb2` — нон-фикшн, 204 сноски (все корректно конвертированы в footnotes)
- `Almanack_ru.fb2` — перевод, ссылки, эпиграфы
- `Leyhi_Sem-preobrazuyushchih-yazykov.554589.epub` — EPUB с 27 spine-документами

## Возможные улучшения

- Поддержка `colspan`/`rowspan` в таблицах (сейчас игнорируется)
- Unicode-замена для `sub`/`sup` (H₂O вместо H2O)
- Поддержка `.fb2.zip` архивов
- Тесты (unit tests для каждого типа элемента)
