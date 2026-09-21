// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"errors"
	"fmt"
	"strings"
)

// Enough TOML to read a starship configuration, and no more.
//
// # Why this is written and not imported
//
// `internal/depsurface` pins this module's runtime dependencies at **zero**,
// and `go list -deps ./cmd/sh` naming nothing outside the standard library
// and this module is an argument several decisions rest on — the plugin
// transport's, among them. A TOML library for a one-way converter would be
// the first runtime dependency this repository has ever had, bought for a
// command a person runs once.
//
// It is also the habit `AGENTS.md` states as **generate, do not import**, and
// the same call `internal/eastasian`, `internal/unorm` and `internal/fswatch`
// each made: the question is small, the answer is small, and what a general
// package would add is the parts nobody here asks for.
//
// # What it reads, and what it refuses
//
// A **subset**, stated rather than discovered: comments, tables, basic and
// literal strings, multi-line basic strings, integers, floats read as their
// text, booleans, and flat arrays of those. That is the whole of what a
// starship configuration is made of.
//
// Everything outside it is an **error** rather than a silent skip. A reader
// that quietly ignored a construct it did not know would drop a setting and
// produce a converted configuration missing something, which is the
// silent-wrong-answer class the whole import is written against. An
// unreadable file is a refusal a person can act on.
//
// Not a TOML implementation and not offered as one: no dotted keys, no
// inline tables, no arrays of tables, no dates, and no escape beyond the
// handful below. Each is absent because starship's configuration format does
// not use it, and each would be an error rather than a wrong answer if one
// turned up.

// tomlValue is one value: text, and whether it was written as a list.
type tomlValue struct {
	Text string
	List []string
	Is   bool
}

// tomlDoc is a parsed document: a table name to its keys, in the order they
// were written, so a converter walking it does not change its answer between
// runs.
type tomlDoc struct {
	// order is the table names as the file wrote them, with the root table
	// first and spelled "".
	order  []string
	tables map[string]map[string]tomlValue
	// keyOrder is each table's keys, as written.
	keyOrder map[string][]string
}

func newTOMLDoc() *tomlDoc {
	return &tomlDoc{
		tables:   map[string]map[string]tomlValue{},
		keyOrder: map[string][]string{},
	}
}

func (d *tomlDoc) put(table, key string, value tomlValue) {
	if _, ok := d.tables[table]; !ok {
		d.tables[table] = map[string]tomlValue{}
		d.order = append(d.order, table)
	}
	if _, seen := d.tables[table][key]; !seen {
		d.keyOrder[table] = append(d.keyOrder[table], key)
	}
	d.tables[table][key] = value
}

// get reads one key of one table.
func (d *tomlDoc) get(table, key string) (tomlValue, bool) {
	v, ok := d.tables[table][key]
	return v, ok
}

// tables is every table name, the root first, in the order they were written.
func (d *tomlDoc) names() []string { return d.order }

// keys is one table's keys, in the order they were written.
func (d *tomlDoc) keys(table string) []string { return d.keyOrder[table] }

// parseTOML reads the subset above.
func parseTOML(text string) (*tomlDoc, error) {
	doc := newTOMLDoc()
	table := ""
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(stripComment(lines[i]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			return nil, fmt.Errorf("line %d: an array of tables is outside what this reader takes", i+1)
		}
		if name, ok := strings.CutPrefix(line, "["); ok {
			name, closed := strings.CutSuffix(name, "]")
			if !closed {
				return nil, fmt.Errorf("line %d: a table header that does not close", i+1)
			}
			table = strings.TrimSpace(name)
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: not an assignment and not a table", i+1)
		}
		key = strings.TrimSpace(key)
		rest = strings.TrimSpace(rest)
		// A multi-line basic string runs on past this line, and it is the one
		// construct here that does. starship's own `format` is written that
		// way, so this is not an optional corner.
		if strings.HasPrefix(rest, `"""`) {
			value, consumed, err := multiline(lines, i, rest)
			if err != nil {
				return nil, err
			}
			doc.put(table, key, tomlValue{Text: value})
			i = consumed
			continue
		}
		value, err := tomlScalar(rest)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		doc.put(table, key, value)
	}
	return doc, nil
}

// multiline reads a `"""…"""` string, which may end on the line it started.
func multiline(lines []string, start int, first string) (string, int, error) {
	body := strings.TrimPrefix(first, `"""`)
	if rest, closed := strings.CutSuffix(body, `"""`); closed {
		return unescapeBasic(rest), start, nil
	}
	// TOML trims a newline immediately after the opening delimiter, which is
	// how a value is written starting on the next line. starship's `format`
	// is written exactly that way.
	var b strings.Builder
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	for i := start + 1; i < len(lines); i++ {
		if rest, closed := strings.CutSuffix(lines[i], `"""`); closed {
			b.WriteString(rest)
			return unescapeBasic(b.String()), i, nil
		}
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	return "", 0, fmt.Errorf("line %d: a multi-line string that never closes", start+1)
}

// tomlScalar reads one value: a string, a number read as its text, a
// boolean, or a flat array of those.
func tomlScalar(text string) (tomlValue, error) {
	if text == "" {
		return tomlValue{}, errors.New("an assignment with no value")
	}
	if strings.HasPrefix(text, "[") {
		return tomlArray(text)
	}
	s, err := tomlString(text)
	if err != nil {
		return tomlValue{}, err
	}
	return tomlValue{Text: s}, nil
}

// tomlString reads one scalar as text: quotes removed where there are any,
// and a bare word — a number or a boolean — kept as it stands.
//
// A number kept as *text* rather than converted, because every setting this
// converter writes is text in the end and a round trip through a float is a
// way to turn `3000` into `3000.0000001` for no gain.
func tomlString(text string) (string, error) {
	switch {
	case strings.HasPrefix(text, `'`):
		rest, closed := strings.CutSuffix(strings.TrimPrefix(text, `'`), `'`)
		if !closed {
			return "", errors.New("a literal string that does not close")
		}
		// Literal, so nothing in it is an escape. That is the whole point of
		// the spelling and it is what a configuration reaches for when the
		// value is full of backslashes.
		return rest, nil
	case strings.HasPrefix(text, `"`):
		rest, closed := strings.CutSuffix(strings.TrimPrefix(text, `"`), `"`)
		if !closed {
			return "", errors.New("a basic string that does not close")
		}
		return unescapeBasic(rest), nil
	}
	if strings.ContainsAny(text, " \t") {
		return "", fmt.Errorf("%q is neither a quoted string nor one word", text)
	}
	return text, nil
}

// tomlArray reads a flat array on one line.
func tomlArray(text string) (tomlValue, error) {
	inner, closed := strings.CutSuffix(strings.TrimPrefix(text, "["), "]")
	if !closed {
		return tomlValue{}, errors.New("an array that does not close on its own line")
	}
	value := tomlValue{Is: true}
	for _, field := range splitOutsideQuotes(inner, ',') {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		s, err := tomlString(field)
		if err != nil {
			return tomlValue{}, err
		}
		value.List = append(value.List, s)
	}
	value.Text = strings.Join(value.List, " ")
	return value, nil
}

// splitOutsideQuotes splits on a separator that is not inside a string.
func splitOutsideQuotes(text string, sep byte) []string {
	var out []string
	var quote byte
	start := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == sep:
			out = append(out, text[start:i])
			start = i + 1
		}
	}
	return append(out, text[start:])
}

// stripComment removes a `#` comment that is not inside a string.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}

// unescapeBasic decodes the escapes a basic string may carry.
//
// The handful a configuration uses, and an unknown one is **kept as it was
// written** rather than guessed at — the same rule the theme's own value
// markup follows for a sequence outside its vocabulary.
func unescapeBasic(text string) string {
	if !strings.Contains(text, `\`) {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) {
			b.WriteByte(text[i])
			continue
		}
		i++
		switch text[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(text[i])
		}
	}
	return b.String()
}
