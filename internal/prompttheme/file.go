// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// The configuration file.
//
// Named by SH_PROMPT_CONFIG, named or nowhere: an unset variable is a person
// who has not asked for a file, and inventing a location for them is what the
// block store deliberately stopped doing.
//
// The format is the namespace written down, one setting per line, so that
// what a configuration prints and what a file holds are the same vocabulary:
//
//	LEFT_ELEMENTS = dir vcs newline prompt_char
//	DIR_FOREGROUND = 31
//	TRANSIENT = always
//
// The SH_PROMPT_ prefix is accepted on input and stripped, so a line copied
// out of a session works in the file and back again.

// ParseFile reads settings from r into a store named layer.
//
// A line that is not an assignment is recorded as a problem rather than
// failing the load. A configuration file is read on the way to drawing a
// prompt, and a prompt is not the place to report a typo by not drawing —
// but a setting that was read and not honored is named, because the silent
// half of that is the failure this repository treats as its worst.
func ParseFile(layer string, r io.Reader) (*Store, error) {
	store := NewStore(layer)
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		name, value, ok := strings.Cut(text, "=")
		if !ok {
			store.Note(position(layer, line) + ": not an assignment")
			continue
		}
		key := Key(name)
		if !settingName(key) {
			store.Note(position(layer, line) + ": " + strings.TrimSpace(name) + " is not a setting name")
			continue
		}
		store.SetText(key, unquote(strings.TrimSpace(value)))
	}
	if err := scanner.Err(); err != nil {
		return store, err
	}
	return store, nil
}

func position(layer string, line int) string {
	if layer == "" {
		return "line " + strconv.Itoa(line)
	}
	return layer + ":" + strconv.Itoa(line)
}

// settingName reports whether a key is spelled the way this namespace spells
// one. Rejecting the rest is what keeps a line of prose in the middle of a
// file from becoming a setting nothing will ever read.
func settingName(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if !isNameRune(r) {
			return false
		}
	}
	return true
}

// unquote removes one layer of matching quotes.
//
// Quoting exists for exactly one reason: a value whose whitespace matters. A
// separator of a single space and a suffix of two are both real settings, and
// without quotes the line that holds them is indistinguishable from an empty
// one. Everything else is written unquoted.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if quote != '"' && quote != '\'' {
		return value
	}
	if value[len(value)-1] != quote {
		return value
	}
	return value[1 : len(value)-1]
}

// File is a configuration file as a Layer: read once, and re-read when its
// mtime changes, so that editing it takes effect on the next prompt and there
// is no reload command.
type File struct {
	path     string
	layer    string
	store    *Store
	modTime  time.Time
	size     int64
	loaded   bool
	lastErr  error
	notFound bool
}

// OpenFile returns a File for path. Nothing is read until Refresh.
func OpenFile(path string) *File {
	layer := "file:" + path
	return &File{path: path, layer: layer, store: NewStore(layer)}
}

// Path is the file's name, as it was given.
func (f *File) Path() string { return f.path }

// Name identifies the layer.
func (f *File) Name() string { return f.layer }

// Lookup answers a canonical key out of what was last read.
func (f *File) Lookup(key string) (Value, bool) { return f.store.Lookup(key) }

// Keys returns what was last read, sorted.
func (f *File) Keys() []string { return f.store.Keys() }

// Problems names what was read and not honored, and the file's own trouble —
// a named file that is not there is a problem, because somebody named it.
func (f *File) Problems() []string {
	problems := f.store.Problems()
	if f.lastErr != nil {
		problems = append(problems, f.path+": "+f.lastErr.Error())
	}
	return problems
}

// Refresh re-reads the file when its mtime or size has changed, and reports
// whether the settings it holds changed as a result.
//
// The engine calls this once per prompt rather than per lookup: a stat per
// prompt is the cost of an edit taking effect without a command, and a stat
// per setting would be that cost times the size of the namespace.
func (f *File) Refresh() bool {
	info, err := os.Stat(f.path)
	if err != nil {
		changed := f.loaded || !f.notFound
		f.store = NewStore(f.layer)
		f.lastErr = err
		f.loaded = false
		f.notFound = true
		f.modTime, f.size = time.Time{}, 0
		return changed
	}
	f.notFound = false
	if f.loaded && info.ModTime().Equal(f.modTime) && info.Size() == f.size {
		return false
	}
	text, err := os.ReadFile(f.path)
	if err != nil {
		f.lastErr = err
		return false
	}
	store, err := ParseFile(f.layer, bytes.NewReader(text))
	f.store, f.lastErr = store, err
	f.modTime, f.size, f.loaded = info.ModTime(), info.Size(), true
	return true
}
