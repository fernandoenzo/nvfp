package ngx

// This file is the only place in the package that knows the manifest's format:
// line endings, and which lines can be rewritten. A rewrite is safe because a
// line the parser does not recognise is kept verbatim, and a recognised line
// keeps the exact text that preceded its value — editing one key never
// reformats the others. The manifest is UTF-8 text, which NVIDIA has always
// written, and the output is canonical: UTF-8, CRLF, every line terminated.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// manifest is a parsed nvngx_config.txt.
type manifest struct {
	items []item
}

// item is one top-level line: a section header, or a line kept verbatim
// (comments and anything before the first header).
type item struct {
	block *block // nil for an opaque top-level line
	raw   string
}

// block is one [name] section with the lines under it.
type block struct {
	name  string
	lines []line
}

// line is one line inside a block: a key/value pair, or an opaque line kept
// verbatim (a comment, a blank line, a stray token).
type line struct {
	key    string // empty for an opaque line
	value  string
	prefix string // literal text before the value: indentation, key, "=", spaces
	raw    string // the line as read, for opaque lines
}

// parseManifest decodes a manifest and splits it into blocks and lines. It
// never fails on content: lines it does not recognise are preserved verbatim.
func parseManifest(data []byte) (*manifest, error) {
	text, err := decode(data)
	if err != nil {
		return nil, err
	}
	m := new(manifest)
	var current *block
	for _, raw := range splitLines(text) {
		if name, ok := sectionHeader(raw); ok {
			current = &block{name: name}
			m.items = append(m.items, item{block: current})
			continue
		}
		if current != nil {
			current.lines = append(current.lines, parseLine(raw))
			continue
		}
		m.items = append(m.items, item{raw: raw})
	}
	return m, nil
}

// splitLines normalises every tolerated line ending to "\n" and returns the
// lines without the trailing empty one.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // terminator of the last line
	}
	return lines
}

// parseFile reads and parses a manifest from disk.
func parseFile(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	m, err := parseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// bytes renders the manifest in canonical form: CRLF line endings, every line
// terminated, UTF-8, no BOM.
func (m *manifest) bytes() []byte {
	var b strings.Builder
	for _, it := range m.items {
		if it.block == nil {
			b.WriteString(it.raw)
			b.WriteString("\r\n")
			continue
		}
		b.WriteString("[" + it.block.name + "]\r\n")
		for _, line := range it.block.lines {
			if line.key == "" {
				b.WriteString(line.raw)
			} else {
				b.WriteString(line.prefix + line.value)
			}
			b.WriteString("\r\n")
		}
	}
	return []byte(b.String())
}

// section returns the first block with that name, or nil.
func (m *manifest) section(name string) *block {
	for _, it := range m.items {
		if it.block != nil && it.block.name == name {
			return it.block
		}
	}
	return nil
}

// appendSection adds a section at the end, separated from the previous one by a
// blank line like the manifest's own sections, and returns it.
func (m *manifest) appendSection(name string) *block {
	if !m.endsWithBlank() {
		m.items = append(m.items, item{raw: ""})
	}
	b := &block{name: name}
	m.items = append(m.items, item{block: b})
	return b
}

// endsWithBlank reports whether the last line of the manifest is already blank,
// so appendSection does not add a second one.
func (m *manifest) endsWithBlank() bool {
	if len(m.items) == 0 {
		return true
	}
	last := m.items[len(m.items)-1]
	if last.block == nil {
		return strings.TrimSpace(last.raw) == ""
	}
	lines := last.block.lines
	return len(lines) == 0 || (lines[len(lines)-1].key == "" && strings.TrimSpace(lines[len(lines)-1].raw) == "")
}

// get returns the value of a key, ignoring surrounding whitespace, and whether
// it was found.
func (b *block) get(key string) (string, bool) {
	for _, line := range b.lines {
		if line.key == key {
			return strings.TrimSpace(line.value), true
		}
	}
	return "", false
}

// set writes key = value, replacing the value in place when the key exists or
// adding the line at the end when it does not. It reports whether anything
// changed, so applying an already-correct manifest writes nothing. Comparison
// ignores surrounding whitespace, and the trailing whitespace of the existing
// line is kept.
func (b *block) set(key, value string) bool {
	for i := range b.lines {
		line := &b.lines[i]
		if line.key != key {
			continue
		}
		if strings.TrimSpace(line.value) == strings.TrimSpace(value) {
			return false
		}
		trailing := line.value[len(strings.TrimRight(line.value, " \t")):]
		line.value = value + trailing
		return true
	}
	b.lines = append(b.lines, line{key: key, prefix: key + " = ", value: value})
	return true
}

// sectionHeader recognises [name], tolerating surrounding whitespace; decode
// has already stripped the file-level BOM. Anything else is not a header.
func sectionHeader(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 3 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return "", false
	}
	name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if name == "" || strings.ContainsAny(name, "[]") {
		return "", false
	}
	return name, true
}

// parseLine turns a line into a key/value pair, or keeps it opaque. The prefix
// keeps the literal text before the value, so an edited key is the only byte
// range that changes in the file.
func parseLine(text string) line {
	if strings.HasPrefix(strings.TrimLeft(text, " \t"), "[") {
		return line{raw: text} // a header inside a block is not ours to parse
	}
	i := strings.Index(text, "=")
	if i <= 0 {
		return line{raw: text}
	}
	key := strings.TrimRight(text[:i], " \t")
	if key == "" {
		return line{raw: text}
	}
	rest := text[i+1:]
	spaces := len(rest) - len(strings.TrimLeft(rest, " \t"))
	return line{key: key, prefix: text[:i+1+spaces], value: rest[spaces:]}
}

// decode reads the input as UTF-8 text, stripping a BOM when present. NUL
// bytes mean the file is not the text the interposer reads, so they are
// refused rather than guessed.
func decode(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("manifest contains NUL bytes; not a UTF-8 text file")
	}
	return string(data), nil
}
