// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Completing the word under the cursor.
//
// Two kinds, decided by where the word sits rather than by what is in it. The
// first word of a command is a *command*, so it completes against the builtins,
// the functions and what is on PATH; anything after it is a filename. That
// distinction is the whole of what makes completion feel like it understands
// the line — offering `/etc/passwd` where a command belongs is worse than
// offering nothing.

// completer answers what a prefix could become.
//
// An interface rather than the Runner directly, so the editor can be tested
// without one and so a caller can offer its own — which is what a shell built
// on this would want anyway.
type completer interface {
	// commands are the names that could run, given a prefix.
	commands(prefix string) []string
	// files are the paths that could follow, given a prefix.
	files(prefix string) []string
}

// complete replaces the word under the cursor with what it could become, and
// reports the matches when it cannot decide.
//
// Returning the matches rather than printing them: the editor owns the screen,
// and deciding *when* to list is its business — the second Tab, not the first.
func (e *editor) complete(c completer) []string {
	if c == nil {
		return nil
	}
	start := wordStart(e.line, e.pos)
	word := string(e.line[start:e.pos])

	var matches []string
	if commandPosition(e.line, start) {
		matches = c.commands(word)
	} else {
		matches = c.files(word)
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) == 1 {
		e.replaceWord(start, matches[0]+completionSuffix(matches[0]))
		return nil
	}
	// Several. Fill in as far as they agree, which is what makes a second Tab
	// worth pressing rather than a repeat of the first.
	if common := commonPrefix(matches); len(common) > len(word) {
		e.replaceWord(start, common)
		return nil
	}
	return matches
}

// replaceWord swaps the word that began at start for the completion.
func (e *editor) replaceWord(start int, with string) {
	rest := append([]rune(nil), e.line[e.pos:]...)
	e.line = append(e.line[:start], []rune(with)...)
	e.pos = len(e.line)
	e.line = append(e.line, rest...)
}

// completionSuffix is what follows a single match: a slash for a directory,
// because the next thing typed is usually what is inside it, and a space for
// anything else, because the word is finished.
func completionSuffix(match string) string {
	if strings.HasSuffix(match, "/") {
		return ""
	}
	return " "
}

// wordStart finds where the word under the cursor begins.
//
// Whitespace is the boundary, and a backslash before it is not: `a\ b` is one
// word with a space in it, and completing the `b` alone would replace half of
// a filename someone escaped deliberately.
func wordStart(line []rune, pos int) int {
	i := pos
	for i > 0 {
		if line[i-1] == ' ' || line[i-1] == '\t' {
			if i >= 2 && line[i-2] == '\\' {
				i -= 2
				continue
			}
			break
		}
		i--
	}
	return i
}

// commandPosition reports whether a word beginning at start is the first of a
// command.
//
// What comes before it decides: the beginning of the line, or an operator that
// ends the previous command. `ls |` and `ls;` are both followed by a command,
// and `ls ` is followed by an argument.
func commandPosition(line []rune, start int) bool {
	i := start
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i == 0 {
		return true
	}
	switch line[i-1] {
	case ';', '|', '&', '(', '\n', '{':
		return true
	}
	return false
}

// commonPrefix is how much of the matches agree, in runes rather than bytes.
func commonPrefix(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	prefix := []rune(matches[0])
	for _, m := range matches[1:] {
		r := []rune(m)
		if len(r) < len(prefix) {
			prefix = prefix[:len(r)]
		}
		for i := range prefix {
			if r[i] != prefix[i] {
				prefix = prefix[:i]
				break
			}
		}
	}
	return string(prefix)
}

// shellCompleter answers from a running shell: its builtins, its functions,
// what is on its PATH, and the files in its working directory.
type shellCompleter struct {
	// names supplies the builtins and functions, and path the directories to
	// search. Held as values rather than as a Runner so the completer can be
	// tested without starting one.
	names []string
	path  string
	dir   string
}

func (s shellCompleter) commands(prefix string) []string {
	seen := map[string]bool{}
	for _, n := range s.names {
		if strings.HasPrefix(n, prefix) {
			seen[n] = true
		}
	}
	for _, dir := range filepath.SplitList(s.path) {
		if dir == "" {
			// An empty entry is the current directory, which is the same
			// rule PATH lookup follows.
			dir = "."
		}
		entries, err := os.ReadDir(s.resolve(dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, prefix) || e.IsDir() {
				continue
			}
			if info, err := e.Info(); err == nil && info.Mode()&0o111 != 0 {
				seen[name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (s shellCompleter) files(prefix string) []string {
	dir, base := filepath.Split(prefix)
	entries, err := os.ReadDir(s.resolve(dirOrDot(dir)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		// A name that was not asked for by its dot is not offered: a bare Tab
		// listing every dotfile is what makes completion unusable in a home
		// directory.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		out = append(out, dir+name)
	}
	sort.Strings(out)
	return out
}

// resolve reads a path the way the shell would, against its working directory
// rather than the process's — which are not the same once `cd` has run.
func (s shellCompleter) resolve(path string) string {
	if s.dir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.dir, path)
}

func dirOrDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// listQueryThreshold is how many matches it takes before a shell asks rather
// than printing.
//
// A hundred, measured in both shells that ask, and it is the count reaching
// it rather than passing it: ninety-nine print and a hundred ask. The same
// number in both, so it is not a dialect's answer — what to *say* is.
const listQueryThreshold = 100

// confirmList asks before printing a large number of matches, and reports
// whether to print.
//
// Columns made a long listing four times shorter and did not stop it pushing
// the prompt off the screen: a directory of a thousand files is still
// hundreds of rows. Every shell with a line editor stops and asks first.
//
// The question is asked with the line still half-drawn — the answer is a
// single key read here rather than at the top of the loop, and the line is
// redrawn afterwards either way. It is the one place the editor reads a key
// in the middle of drawing.
func (e *editor) confirmList(matches []string, prompt drawnPrompt) bool {
	if e.listQuery == "" || len(matches) < listQueryThreshold {
		return true
	}
	e.endLine(prompt, "")
	e.write(fmt.Sprintf(e.listQuery, len(matches), len(columns(matches, e.cols()))))
	for {
		var buf [1]byte
		n, err := e.in.Read(buf[:])
		if err != nil {
			// Nothing more is coming, so there is nobody to print for.
			return false
		}
		if n == 0 {
			continue
		}
		c := buf[0]
		if e.listQueryEchoes {
			e.write(string(rune(c)))
		}
		switch {
		case c == 'y' || c == 'Y':
			e.write("\r\n")
			return true
		case c == 'n' || c == 'N', !e.listQueryStrict:
			// Anything at all declines where a shell takes the first key it
			// is given; only `n` does where one waits for an answer.
			e.write("\r\n")
			return false
		default:
			// Not an answer. The bell, and ask again — which is what waiting
			// for one of two keys means.
			e.write("\a")
		}
	}
}
