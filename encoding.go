package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

var xmlEncodingRe = regexp.MustCompile(`(?i)<\?xml[^?]*encoding=["']([^"']+)["']`)

// detectAndConvertEncoding reads raw bytes, detects encoding from XML declaration,
// and converts to UTF-8 if necessary. Returns UTF-8 bytes with encoding declaration
// removed or replaced.
func detectAndConvertEncoding(data []byte) ([]byte, error) {
	match := xmlEncodingRe.FindSubmatch(data)
	if match == nil {
		return data, nil
	}

	enc := strings.ToLower(string(match[1]))

	switch enc {
	case "utf-8", "utf8":
		return data, nil
	case "windows-1251", "win-1251", "cp1251":
		decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode windows-1251: %w", err)
		}
		return fixXMLDeclarationEncoding(decoded), nil
	case "koi8-r", "koi8r":
		decoded, err := charmap.KOI8R.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode koi8-r: %w", err)
		}
		return fixXMLDeclarationEncoding(decoded), nil
	case "koi8-u", "koi8u":
		decoded, err := charmap.KOI8U.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode koi8-u: %w", err)
		}
		return fixXMLDeclarationEncoding(decoded), nil
	case "iso-8859-1", "latin1":
		decoded, err := charmap.ISO8859_1.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode iso-8859-1: %w", err)
		}
		return fixXMLDeclarationEncoding(decoded), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", enc)
	}
}

// fixXMLDeclarationEncoding replaces the encoding in XML declaration with utf-8
// so the XML parser doesn't complain.
func fixXMLDeclarationEncoding(data []byte) []byte {
	return xmlEncodingRe.ReplaceAll(data, bytes.Replace(
		xmlEncodingRe.Find(data),
		xmlEncodingRe.FindSubmatch(data)[1],
		[]byte("utf-8"),
		1,
	))
}
