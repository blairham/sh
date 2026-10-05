// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

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
// and deciding *when* to list is its business — see completeKey, which is
// where the dialect's answer to that is read.
//
// The second result is what the keystroke did, which is a **third** thing
// beyond the line and the matches: the two shells with a line editor ask
// different questions about it, so neither "the line was written to" nor
// "there were matches" is enough on its own. See completionOutcome.
func (e *editor) complete(c Completer) (matches []Candidate, did completionOutcome) {
	start, word, candidates := e.candidates(c)
	if len(candidates) == 0 {
		return nil, completionFoundNothing
	}
	if e.typesTheKey(candidates) {
		return nil, completionSettledTheWord
	}
	// The words are what is inserted and what the prefix is computed over;
	// a candidate with no word is a row of a listing and nothing else, so
	// two of those and one word is a lone match rather than three.
	words := insertableWords(candidates)
	if len(words) == 1 {
		e.replaceWord(start, words[0]+loneSuffix(candidates, word))
		return nil, completionSettledTheWord
	}
	// Several. Fill in as far as they agree, which is what makes a second Tab
	// worth pressing rather than a repeat of the first.
	if common := commonPrefix(words); len(words) > 1 && len(common) > len(word) && keepsWhatWasTyped(word, common) {
		e.replaceWord(start, common)
		return nil, completionFilledInWhatTheyAgreeOn
	}
	return displayCandidates(candidates, word), completionHadNothingToInsert
}

// keepsWhatWasTyped answers whether filling in common keeps every character
// of the word that was typed, in order and allowing a change of case.
//
// The candidates usually begin with the word, and then this is that. Under a
// completion system's match specification they need not: `l:|=* r:|=*`
// matches `pl` in the middle of `xapple` and `xaple`, whose common prefix is
// `xap`, and putting that on the line would throw the typed `pl` away. zsh
// never does — measured on zsh 5.9.2, 2026-10-05, a fill under a match
// specification keeps what was typed: `re` against `README.md READY` under
// `m:{a-z}={A-Z}` becomes `READ`, and `f.b` against `foo.bar.baz foo.bar.qux`
// under `r:|[._-]=*` becomes `foo.bar.`, while `pl` against `abcpl abcqpl`
// leaves the line alone and lists. What zsh puts on the line where the
// candidates agree on something other than a prefix (`ple`, with the cursor
// in front of it) is not drawn here; listing instead loses nothing.
func keepsWhatWasTyped(word, common string) bool {
	typed := []rune(word)
	k := 0
	for _, c := range common {
		if k < len(typed) && unicode.ToLower(c) == unicode.ToLower(typed[k]) {
			k++
		}
	}
	return k == len(typed)
}

// completionOutcome is what one completion keystroke did, in the three states
// the bell is decided by.
//
// Three and not two, because the two shells with a line editor split them
// differently. Measured 2026-09-19 through a pseudo-terminal against twelve
// directories agreeing on a prefix, one unique name, and a word agreeing on
// nothing more:
//
//	bash 5.3.20   \a and the prefix   the name and a space   \a
//	zsh  5.9.2    the prefix          the name and a space   \a
//
// So a settled word is silent in both and an unsettled one rings in both; the
// middle state — several matches and a prefix put in — is bash's alone. A
// pair of booleans could not say that without one of them meaning two things.
type completionOutcome uint8

const (
	// completionFoundNothing: no match at all, and the line is as it was.
	completionFoundNothing completionOutcome = iota
	// completionSettledTheWord: one match, put in whole with whatever
	// follows a finished word.
	completionSettledTheWord
	// completionFilledInWhatTheyAgreeOn: several matches, and more of the
	// word than was typed is now on the line.
	completionFilledInWhatTheyAgreeOn
	// completionHadNothingToInsert: several matches agreeing on nothing past
	// what is typed, so the line is as it was and the matches are the answer.
	completionHadNothingToInsert
)

// candidates is what the word under the cursor could become, and where that
// word begins.
//
// One place asks, because four keys now ask it — Tab, the listing, the menu in
// both directions — and every one of them has to be asking about the same
// word. Splitting the question out is what keeps a second copy of "where does
// this word start" from appearing beside the one in completion() below.
//
// A nil completer answers nothing, which is a session with no completion at
// all; the cursor stands in for the start so that a caller which goes on to
// replace a word has a position rather than a zero.
func (e *editor) candidates(c Completer) (start int, word string, matches []Candidate) {
	if c == nil {
		return e.pos, "", nil
	}
	start = wordStart(e.line, e.pos)
	return start, string(e.line[start:e.pos]), c.Complete(e.completion(start))
}

// insertableWords are the replacement words among the candidates: everything a
// listing-only row is not.
func insertableWords(candidates []Candidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Word != "" {
			out = append(out, c.Word)
		}
	}
	return out
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

// displayCandidates are the candidates as a listing shows them: each one's row
// settled, and the directory already typed taken off the rows that are nothing
// but a word.
//
// Measured in both shells — `: sub/` lists `nested.txt`, not `sub/nested.txt`.
// The opening quote goes the same way, for the same reason. A candidate that
// brought its own Display is left exactly as it is: the completion system that
// built that string has already decided what the row says, and trimming a
// path off the front of a padded `name  -- sentence` would cut it in the
// middle of the padding.
func displayCandidates(candidates []Candidate, word string) []Candidate {
	prefix := wordPrefix(word)
	out := make([]Candidate, len(candidates))
	for i, c := range candidates {
		if c.Display == "" {
			c.Display = strings.TrimPrefix(c.Word, prefix)
		}
		out[i] = c
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

// loneSuffix is completionSuffix for the one insertable candidate among
// candidates, which may say that nothing follows it at all. See
// Candidate.Open.
func loneSuffix(candidates []Candidate, word string) string {
	for _, c := range candidates {
		if c.Word == "" {
			continue
		}
		if c.Open {
			return ""
		}
		return completionSuffix(word, c.Word)
	}
	return ""
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

	// symlinkMarkedWhenWhole withholds a symlinked directory's slash until
	// the word names it whole. See
	// EditorStyle.SymlinkedDirectoryMarkedWhenNamedWhole.
	symlinkMarkedWhenWhole bool

	// listTypes is whether a listing of files draws `ls -F`'s marks, asked at
	// the keystroke because it is an option a person sets at the prompt. Nil
	// is a dialect with no such option. See EditorStyle.ListTypesOption.
	listTypes func() bool

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

	// correctDir retries a directory that would not read against the closest
	// name that would — bash's `dirspell`. Nil where nothing has asked for
	// the correction, which is what the zero value of this struct is and what
	// a session that has not set the option is.
	//
	// A function rather than a bool because the correction is the core's:
	// interp.Runner.CorrectedDirectory is the same corrector `cdspell`
	// reaches, and a completer that walked the directory itself would be the
	// second copy this repository has paid for before. It reads the option
	// itself, so it answers empty while the option is off.
	correctDir func(string) string

	// expandsDirectory writes the directory that was *read* back into the
	// line rather than the text that was typed — bash's `direxpand`. Read
	// from the shell per keystroke, for the reason emptyWordOffersNothing is.
	//
	// It is the half that makes a correction visible; see
	// shellCompleter.corrected for the measured pair.
	expandsDirectory bool

	// expandParams expands the parameters in a directory portion before it is
	// read — `$HOME/docum` is a word under somebody's home directory and not
	// one under a directory called `$HOME`. Nil where nothing has supplied
	// one, which is what the zero value of this struct is and what a completer
	// built for a test without a shell behind it is: the directory is then
	// read as typed, which is what this package did everywhere before #1574.
	//
	// A function rather than a bool for the reason correctDir is one, and the
	// second return is the part that makes it safe to hold: it is false for a
	// word whose expansion would have to *run* something, and a completer
	// cannot run a command on a keystroke. See
	// interp.Runner.ExpandParametersOnly, which carries that argument and
	// enforces it structurally.
	expandParams func(string) (string, bool)

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
func (s shellCompleter) Complete(c Completion) []Candidate {
	if !c.Command || hasPathSeparator(c.Word) {
		return s.pathCandidates(c.Word, s.keepFor(c))
	}
	return Words(s.words(c)...)
}

// keepFor is the narrowing a path in this position gets: what could run, for
// a command word, and everything otherwise.
func (s shellCompleter) keepFor(c Completion) func(string, os.DirEntry) bool {
	if c.Command {
		return s.runnable
	}
	return nil
}

// words is this completer's answer before it is dressed as candidates. A
// separate method because everything below it hands back names, and a name is
// the whole of what this completer knows about a match — no shell state here
// carries a sentence to draw beside one.
func (s shellCompleter) words(c Completion) []string {
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
// than printing, where the dialect keeps no parameter for it.
//
// A hundred, measured in both shells that ask, and it is the count reaching
// it rather than passing it: ninety-nine print and a hundred ask. The same
// number in both, so it is not a dialect's answer — what to *say* is, and in
// one of the two **how many** is a parameter a script can set. See
// editor.listQueryAsks.
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
func (e *editor) confirmList(matches []Candidate, prompt drawnPrompt) bool {
	if e.listQuery == "" || !e.listQueryAsks(matches) {
		return true
	}
	e.endLine(prompt, "")
	e.write(fmt.Sprintf(e.listQuery, len(matches), len(listingRows(matches, e.cols(), e.listLayout()))))
	for {
		// A shell's watched descriptors are served while the question
		// waits, as they are while the line does. See serveWhileAsking.
		e.serveWhileAsking()
		var buf [1]byte
		// Through nextByte and not the reader: this editor buffers what the
		// terminal delivered, so a read that went straight to the descriptor
		// steps over input already in hand. It reported end-of-input while
		// the answer sat in the buffer, and the `n` was then typed into the
		// line — which is what `line = "an"` in the listing test was.
		n, err := e.nextByte(buf[:])
		if err != nil {
			// Nothing more is coming, so there is nobody to print for.
			return false
		}
		if n == 0 {
			continue
		}
		c := buf[0]
		yes := c == 'y' || c == 'Y'
		if e.listQueryEchoes {
			// The key, where it can be drawn, and `n` for one that cannot:
			// measured 2026-10-05 against zsh 5.9.2, `x` is echoed as `x`,
			// and `^X` and the Escape an arrow key starts with as `n`. This
			// wrote the raw byte (#6119).
			if c >= 0x20 && c < del || yes {
				e.write(string(rune(c)))
			} else {
				e.write("n")
			}
		}
		switch {
		case yes:
			if e.listQueryTakesItsRow {
				// The listing goes where the question was. See
				// EditorStyle.ListQueryAnswerTakesTheQuestionsRow.
				e.write("\r\x1b[J")
				e.listHere = true
				return true
			}
			e.write("\r\n")
			return true
		case c == 'n' || c == 'N', !e.listQueryStrict:
			// Anything at all declines where a shell takes the first key it
			// is given; only `n` does where one waits for an answer.
			if e.listQueryTakesItsRow {
				e.backToTheLine(prompt)
				return false
			}
			e.write("\r\n")
			return false
		default:
			// Not an answer. The bell, and ask again — which is what waiting
			// for one of two keys means.
			e.write(e.bell())
		}
	}
}

// completerFor is what answers this Tab: the shell's own completion system
// first where the key's binding named one, and this package's completion
// after it.
//
// Before rather than instead, which is the rule Binding.Candidates states and
// the reason a startup file's completion system can be wired here at all. A
// shell action that offers nothing — because it is not defined, because it
// failed, or because it had nothing to say about this word — leaves the
// answer this editor would have given on its own standing, so the worst a
// broken one can do is cost a call. #2770 is the failure that makes this the
// only acceptable ordering: a real `~/.zshrc` replaced a working Tab with one
// that diagnosed on every keystroke and completed nothing.
//
// The name is per keystroke and the chain is not cached, because the two
// facts it is built from move independently: a key's binding is read fresh on
// every key, and the session's own completer is built once.
// ownCompletion reports whether a key bound to complete with the shell
// completion named name is answered by this editor's own completion first —
// which it is where the binding names none or the session has no way to ask.
// See tabOnABlankLine, which is asked only then.
func (e *editor) ownCompletion(name string) bool {
	return name == "" || e.shellComplete == nil
}

func (e *editor) completerFor(name string) Completer {
	if name == "" || e.shellComplete == nil {
		return e.comp
	}
	return CompleterFunc(func(c Completion) []Candidate {
		matches := e.shellComplete(name, c)
		if len(matches) == 1 && matches[0].stop {
			// The shell's function stopped on an error, which is no
			// completion at all: this editor's own is not asked, and the
			// line is left as it was. Measured 2026-10-05 through a
			// pseudo-terminal against zsh 5.9.2, `cf() { BUFFER=zz }` behind
			// `zle -C` on a key prints the refusal and a bell, and `x` stays
			// `x` where this went on to list every command beginning with
			// it (#6068).
			//
			// And the diagnostic it printed ended its row, so the screen
			// the last draw described is gone: the next draw starts from
			// the row the cursor is on, and writes the prompt and the line
			// there — what zsh draws under the diagnostic (#6062).
			e.drawn = drawnLine{}
			e.row = 0
			return nil
		}
		if len(matches) > 0 {
			return matches
		}
		if e.comp == nil {
			return nil
		}
		return e.comp.Complete(c)
	})
}

// completeKey is the whole of what a completion key does, including the part
// that takes two of them.
//
// One copy for the two callers — the Tab this editor has by default, and a key
// a shell rebound to completion — because everything either of them needs is
// the same, and because the two had drifted: a rebound key ran the completion
// and dropped the matches on the floor, so it could never list. See the case
// in editor.go that calls this, and lastTab for why the listing is the second
// keystroke's and not the first's.
func (e *editor) completeKey(c Completer, builtin, wasTab bool, prompt drawnPrompt) {
	if builtin && e.tabOnABlankLine(prompt) {
		return
	}
	var matches []Candidate
	var did completionOutcome
	e.change(false, func() { matches, did = e.complete(c) })
	// Whether the matches are drawn now or left for a second key is the
	// dialect's answer; see EditorStyle.ListMatchesWithoutASecondKeyOption.
	listing := len(matches) > 0 && (wasTab || e.listsMatches)
	if e.ringsFor(did, listing) {
		e.ring()
	}
	if listing && e.confirmList(matches, prompt) {
		e.list(matches, prompt)
	}
	e.redraw(prompt)
	// Set after the redraw, and the only key that leaves it set: two
	// completions in a row are a request to see the matches, and anything
	// between them is not.
	e.lastTab = true
}

// ringsFor reports whether this keystroke sounds the bell, given what it did
// and whether it is drawing the matches.
//
// A word it settled is silent in both shells with a line editor, and a word
// it could not move rings in both. The middle case is the disagreement: bash
// asks whether the word is **settled** and rings for an ambiguous completion
// even while it puts a prefix in, and zsh asks whether the keystroke **did
// anything** and stays silent. See
// EditorStyle.BellRingsOnAnAmbiguousCompletionThatInserts, which is measured
// and identical in bash 5.3.20 and bash 3.2.57.
//
// The listing is the other half, and only for the shells that keep the
// matches back: where a second key exists to draw them, that key draws them
// and is silent — measured, one bell over two Tabs — so what is asked is
// whether this keystroke is that one. Where they are drawn on the first key
// the bell rings beside them.
func (e *editor) ringsFor(did completionOutcome, listing bool) bool {
	switch did {
	case completionSettledTheWord:
		return false
	case completionFilledInWhatTheyAgreeOn:
		return e.bellsOnAPartialCompletion
	default:
		return e.listsMatches || !listing
	}
}

// listQueryAsks reports whether this many matches is enough to ask about.
//
// Where the dialect keeps the threshold in a parameter — zsh's `$LISTMAX` —
// it is read **live**, and it has three readings rather than one. Measured
// 2026-09-28 through a pseudo-terminal against zsh 5.9.2, `zsh -f -i` on a
// terminal of 80 columns, completing `ls alpha<TAB>` in a directory of five
// files that list in one row, and `ls beta<TAB>` in one of three hundred that
// list in thirty-eight:
//
//	LISTMAX   5 matches, 1 row      300 matches, 38 rows
//	2         asks                  —
//	5         asks                  —
//	6         silent                —
//	0         silent                asks
//	-1        asks                  —
//	-2        asks                  —
//	-10       asks                  —
//	400       —                     silent
//
// So a positive number is a count and the comparison is **at** it rather than
// past it — five matches and `LISTMAX=5` asks, `LISTMAX=6` does not, which is
// the same "reaching it rather than passing it" the constant above records.
// Zero asks only when the listing would not fit the screen. And **every**
// negative asks, whatever the listing's size: `-10` asks about a one-row
// listing, which is what rules out the reading where a negative is a number
// of rows to compare against.
//
// The boundary of the zero reading is not measured and is not claimed: what
// the two rows above bracket is one row on a twenty-four row terminal not
// asking and thirty-eight rows on the same terminal asking. "More rows than
// the terminal has" is the shape taken; the exact cell it turns over at would
// need a listing built to the terminal's height.
func (e *editor) listQueryAsks(matches []Candidate) bool {
	n, ok := 0, false
	if e.listThreshold != nil {
		n, ok = e.listThreshold()
	}
	if !ok {
		// No parameter, or one holding something that is not a number: the
		// built-in count, which is what a dialect with no such parameter
		// always gets.
		return len(matches) >= listQueryThreshold
	}
	switch {
	case n < 0:
		return true
	case n == 0:
		return len(listingRows(matches, e.cols(), e.listLayout())) > e.rows()
	default:
		return len(matches) >= n
	}
}

// typesTheKey types the key that asked for a completion, where the completion
// answered that it should be typed instead — see CompletionInsertsTheKey —
// and reports whether it did. The last character of the key, the way a
// self-insert types one.
func (e *editor) typesTheKey(matches []Candidate) bool {
	if len(matches) != 1 || !matches[0].insertKey {
		return false
	}
	e.adoptKeys(string(e.keyBytes))
	if e.typedKey != 0 {
		e.insert(e.typedKey)
	}
	return true
}

// tabOnABlankLine types a Tab that asked this editor's own completion for a
// word with nothing but blanks before the cursor, where the dialect says so,
// and reports whether it did.
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2 with no
// completion system loaded: Tab on an empty line, on `  `, and with the
// cursor at the start of `ls` or after the one blank of ` ls` puts a tab in
// the line, under `expand-or-complete`, `complete-word` and `menu-complete`
// alike; the same widgets on `^T` complete — so it is the key and not the
// widget — and `list-choices` on Tab lists. The completion system, where one
// is loaded, makes the same decision itself (its `insert-tab` style) and
// answers with CompletionInsertsTheKey, so this is asked only of the
// editor's own completion. In a real configuration whose completion system
// was not found this editor's completion is the one answering, and a Tab at
// an empty prompt asked whether to list every command there is (#6119).
// See EditorStyle.TabOnABlankLineTypesItself.
func (e *editor) tabOnABlankLine(prompt drawnPrompt) bool {
	if !e.tabOnBlank || string(e.keyBytes) != "\t" {
		return false
	}
	for _, r := range e.line[:e.pos] {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	e.change(false, func() { e.insert('\t') })
	e.redraw(prompt)
	e.lastTab = false
	return true
}

// backToTheLine takes a declined question off the screen and puts the cursor
// back on the line it was asked under, which is where zsh draws the line
// again: measured 2026-10-05 against zsh 5.9.2, a declined question is
// followed by `\r\e[J\e[A` and the line redrawn on its own row, where this
// drew a fresh prompt under the question and left the question on the screen
// (#6119). See EditorStyle.ListQueryAnswerTakesTheQuestionsRow.
func (e *editor) backToTheLine(prompt drawnPrompt) {
	e.write("\r\x1b[J\x1b[A")
	cols := e.cols()
	if cols <= 0 {
		e.row = 0
		return
	}
	// The row endLine left the line from, counted the way toLastRow counts
	// it, which is the row the question was written under.
	_, _, endRow, endCol := place(prompt.cells, e.displayed(), e.pos, cols)
	if endCol == cols {
		endRow++
	}
	e.row = endRow
}
