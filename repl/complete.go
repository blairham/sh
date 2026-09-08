// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/internal/boundary"
)

// Completing the word under the cursor.
//
// Two kinds, decided by where the word sits rather than by what is in it. The
// first word of a command is a *command*, so it completes against the builtins,
// the functions and what is on PATH; anything after it is a filename. That
// distinction is the whole of what makes completion feel like it understands
// the line — offering `/etc/passwd` where a command belongs is worse than
// offering nothing.

// complete replaces the word under the cursor with what it could become, and
// reports the matches when it cannot decide.
//
// Returning the matches rather than printing them: the editor owns the screen,
// and deciding *when* to list is its business — the second Tab, not the first.
func (e *editor) complete(c Completer) []string {
	if c == nil {
		return nil
	}
	start := wordStart(e.line, e.pos)
	word := string(e.line[start:e.pos])
	matches := c.Complete(e.completion(start))
	if len(matches) == 0 {
		return nil
	}
	if len(matches) == 1 {
		e.replaceWord(start, matches[0]+completionSuffix(word, matches[0]))
		return nil
	}
	// Several. Fill in as far as they agree, which is what makes a second Tab
	// worth pressing rather than a repeat of the first.
	if common := commonPrefix(matches); len(common) > len(word) {
		e.replaceWord(start, common)
		return nil
	}
	return displayNames(matches, word)
}

// completion is the question this keystroke asks, as the seam states it.
//
// Built here rather than in each completer because every fact in it is one the
// editor already holds and nobody outside can recompute: where a word begins
// is this package's own escaping rule, and whether it is a command is decided
// by the operator before it. A completer that had to work those out from the
// line would be a second copy of two rules that have to agree.
//
// The rune-to-byte conversion happens once, here. Line is a string because
// that is what a caller wants to slice, and the editor holds runes because a
// cursor sits between characters.
func (e *editor) completion(start int) Completion {
	return Completion{
		Line:    string(e.line),
		Point:   len(string(e.line[:e.pos])),
		Start:   len(string(e.line[:start])),
		Word:    string(e.line[start:e.pos]),
		Command: commandPosition(e.line, start),
		Dir:     e.dir(),
	}
}

// dir is the shell's working directory, or empty when there is nothing to ask.
func (e *editor) dir() string {
	if e.workingDir == nil {
		return ""
	}
	return e.workingDir()
}

// displayNames are the matches as a listing shows them: without the directory
// already typed, which every one of them carries and none of them is about.
//
// Measured in both shells — `: sub/` lists `nested.txt`, not `sub/nested.txt`.
// The opening quote goes the same way, for the same reason.
func displayNames(matches []string, word string) []string {
	prefix := wordPrefix(word)
	if prefix == "" {
		return matches
	}
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = strings.TrimPrefix(m, prefix)
	}
	return out
}

// replaceWord swaps the word that began at start for the completion.
func (e *editor) replaceWord(start int, with string) {
	rest := append([]rune(nil), e.line[e.pos:]...)
	e.line = append(e.line[:start], []rune(with)...)
	e.pos = len(e.line)
	e.line = append(e.line, rest...)
}

// completionSuffix is what follows a single match.
//
// A directory already carries its slash and gets nothing more, because the
// next thing typed is usually what is inside it. Anything else is a finished
// word: the quotation it was typed inside is closed, and a space follows.
//
// Measured in both shells: `: "file o` completes to `: "file one.txt" ` with
// the quote closed, and `: "only` to `: "onlydir/` with it still open.
func completionSuffix(word, match string) string {
	if strings.HasSuffix(match, "/") {
		return ""
	}
	if q := wordQuote(word); q != 0 {
		return string(q) + " "
	}
	return " "
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

// commonPrefix is how much of the matches agree.
//
// In escaped units rather than in bytes or in runes: a backslash and what it
// escapes are one thing, and a prefix that ended between them would escape
// whatever was typed next instead.
func commonPrefix(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	prefix := escapeUnits(matches[0])
	for _, m := range matches[1:] {
		u := escapeUnits(m)
		if len(u) < len(prefix) {
			prefix = prefix[:len(u)]
		}
		for i := range prefix {
			if u[i] != prefix[i] {
				prefix = prefix[:i]
				break
			}
		}
	}
	return strings.Join(prefix, "")
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

	// home is what a bare `~` names, taken from the shell's own HOME rather
	// than from the process's, for the same reason dir is.
	home string

	// hidden offers names beginning with a dot to a word that does not begin
	// with one. See EditorStyle.CompletionMatchesHiddenFiles.
	hidden bool

	// emptyWordOffersNothing withholds the command list from a command word
	// that is empty — bash's `no_empty_cmd_completion`, read from the shell
	// rather than settled once, because the option is one a person turns on
	// at the prompt.
	//
	// The deviation and not the state, so the zero value of this struct is
	// the completer this package has always had: Tab on an empty line offers
	// every builtin, function, reserved word and executable on PATH, which is
	// what the option's *off* state means and what makes turning it on a
	// change in behavior rather than a confirmation of one.
	emptyWordOffersNothing bool

	// passwd is where account names are read from; empty is /etc/passwd. A
	// field so a test can ask about names that are not on the machine.
	passwd string

	// bound is the session's gate and sink, so a directory listed to answer
	// Tab is asked about the way one listed by a glob is. The zero value
	// allows everything and records nothing, which is what a shell without a
	// policy is.
	bound boundary.Boundary

	// ctx is the session's, because the seam a completer answers through
	// cannot carry one: Completer.Complete takes a Completion and nothing
	// else, and widening a published interface so that this package can reach
	// its own gate would make every caller's completer pay for it. So the
	// context is captured where the session begins — Shell.Run has it — and
	// held here for the length of the session, which is exactly the lifetime
	// of the editor that asks.
	//
	// Nil is normal: a completer built without one is a completer with no gate
	// and no sink, and context() is where that is turned into a usable value
	// rather than at each of the two call sites.
	ctx context.Context
}

// context is the session's context, or a background one where a completer was
// built without a session — which is every completer with nothing watching it.
func (s shellCompleter) context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

// commands are the names that could run.
//
// A word with a slash in it is a path rather than a name, and PATH has
// nothing to say about it: what is offered is what could actually run from
// there, which is the directories and the files with an execute bit.
// Measured — `./pl` offers nothing where `plain.txt` is not executable, and
// `./onl` offers `./onlydir/`.
func (s shellCompleter) commands(word string) []string {
	if hasPathSeparator(word) {
		return s.paths(word, s.runnable)
	}
	quote := wordQuote(word)
	base := dequote(word)
	seen := map[string]bool{}
	for _, n := range s.names {
		if strings.HasPrefix(n, base) {
			seen[n] = true
		}
	}
	for _, dir := range filepath.SplitList(s.path) {
		if dir == "" {
			// An empty entry is the current directory, which is the same
			// rule PATH lookup follows.
			dir = "."
		}
		// The same listing through the same boundary. A directory a policy
		// hides contributes no names, exactly as one that is not on the disk
		// contributes none — which is already what this loop does with every
		// other reason a PATH entry cannot be read.
		entries, err := s.bound.ReadDir(s.context(), s.resolve(dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, base) || e.IsDir() {
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
	for i, n := range out {
		out[i] = wordPrefix(word) + escapeName(n, quote, false)
	}
	return out
}

// files are the paths that could follow.
func (s shellCompleter) files(word string) []string { return s.paths(word, nil) }

// Complete is this shell's own answer, and the substrate's implementation of
// the public seam.
//
// The two kinds are chosen by where the word sits, which the request already
// says. One place decides it — the editor, when it builds the Completion — so
// a caller's completer and this one are looking at the same fact rather than
// each deciding for itself.
func (s shellCompleter) Complete(c Completion) []string {
	if c.Command {
		if c.Word == "" && s.emptyWordOffersNothing {
			// Nothing, which is the whole of the option: with it on, bash
			// 5.3.15 through a pseudo-terminal answers two Tabs on an empty
			// line with no listing and no bell-and-list, and answers the same
			// after `true; ` — so it is the empty *word* in command position
			// that is withheld, not only a line with nothing on it. With it
			// off the same two Tabs offer 2102 names.
			//
			// Only the empty word. A word with a letter in it is completed
			// exactly as before, which is what keeps this an option about
			// listing everything rather than an option that turns command
			// completion off.
			return nil
		}
		return s.commands(c.Word)
	}
	return s.files(c.Word)
}

// resolve reads a path the way the shell would, against its working directory
// rather than the process's — which are not the same once `cd` has run.
func (s shellCompleter) resolve(path string) string {
	if s.dir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.dir, path)
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
