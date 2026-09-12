// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/secret"
)

// The history that outlives the session.
//
// A shell that forgets everything the moment it exits is one nobody keeps
// using, and this is the cheapest part of it: a file of lines, read at the
// start and appended to at the end.
//
// Appended rather than rewritten. Two shells open at once both want to add to
// it, and a rewrite makes the last one to exit the only one that happened —
// which is the bug every shell's history has had at some point.

// defaultHistorySize is how many lines are kept when neither HISTSIZE nor
// HISTFILESIZE says anything. Enough to be worth searching and small enough to
// read at startup without thinking about it.
const defaultHistorySize = 1000

// historyFile is where the session's lines are kept, and how many.
type historyFile struct {
	path string

	// size is HISTSIZE: how many lines the session can recall. It bounds the
	// list the up arrow walks and the search looks through, which is a
	// different question from how large the file is allowed to get, and is
	// answered by a different variable.
	//
	// Measured on 2026-09-05: bash 5.3.15 started with `HISTSIZE=2` and four
	// lines in the file lets the up arrow reach two of them and stops. Zero
	// means the session remembers nothing, and then there is nothing to write
	// either.
	size int

	// file is HISTFILESIZE: how many lines the file keeps. Its default is
	// HISTSIZE's value, which is measured — bash's manual says so and bash
	// with only HISTSIZE set trims to it.
	file int
	// bound is the session's gate and event sink, because this file is
	// inside the boundary rather than beside it: HISTFILE is a shell
	// variable, so the path is one a line typed at the prompt can change,
	// and an open a script can aim is an open a policy is entitled to refuse.
	// A zero Boundary allows and records nothing, which is every session that
	// was never given a policy.
	bound boundary.Boundary
}

// historyFrom reads the settings a session should use.
//
// HISTFILE names the file and an empty one turns the history off, which is how
// a shell is told not to record anything — a session in a directory someone
// does not want remembered. So does having no HOME to put a default under.
func historyFrom(get func(string) (string, bool), home string) historyFile {
	path, ok := get("HISTFILE")
	if !ok {
		if home == "" {
			return historyFile{}
		}
		path = filepath.Join(home, ".sh_history")
	}
	// An empty path is what turns the history off, and both load and save
	// check for it — so there is nothing to do here but carry it through.
	size := countFrom(get, "HISTSIZE", defaultHistorySize)
	return historyFile{path: path, size: size, file: countFrom(get, "HISTFILESIZE", size)}
}

// countFrom reads one of the two size variables, or leaves the default alone.
//
// A value that is not a whole number is not a bound: bash given
// `HISTSIZE=lots` keeps its previous answer rather than treating the setting
// as zero, and zero is the one value that turns the history off — so reading
// nonsense as zero would silently be the destructive reading.
func countFrom(get func(string) (string, bool), name string, fallback int) int {
	v, ok := get(name)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

// Lines is the file as a HistorySource: what previous sessions left.
//
// The substrate's own implementation of the public seam, so the shipped
// history and a front end's are the same kind of thing and are composed by
// one rule rather than one being a special case in the loop.
func (h historyFile) Lines(ctx context.Context) []string { return h.load(ctx) }

// load reads the lines a previous session left.
func (h historyFile) load(ctx context.Context) []string {
	if h.path == "" || h.size == 0 {
		// Nothing to open, so nothing to ask a policy about: a history that
		// is turned off is not an access that was refused.
		return nil
	}
	// A refused history reads as no history, and so does a missing file:
	// the first session a person runs has no history, a history turned off
	// has no path, and a policy that hides the file meant for it not to be
	// read. The open fails in all three cases and there is nothing to read.
	// Complaining would be the first thing they saw.
	//
	// The open is the boundary's rather than the os package's, which is what
	// makes HISTFILE a verified access: it is a shell variable, so a typed
	// line can point it at a link into somewhere the policy hides, and until
	// #942 the gate was asked about the name and the read was done on the
	// object.
	f, err := h.bound.OpenFile(ctx, boundary.File{Path: h.path})
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	// A line longer than the scanner's default is not a reason to lose the
	// file; a pasted command can be very long.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if line := sc.Text(); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > h.size {
		lines = lines[len(lines)-h.size:]
	}
	return lines
}

// save appends what this session added.
//
// Only what it added: the lines it read at the start are already in the file,
// and writing them again would double it every time a shell is opened.
func (h historyFile) save(ctx context.Context, added []string) error {
	added = withoutCredentials(added)
	if h.path == "" || h.size == 0 || h.file == 0 || len(added) == 0 {
		return nil
	}
	// 0600: a shell history is a record of what someone typed, which is not
	// something to leave readable by everyone on the machine. Parents,
	// because the directory may not exist on a first run — a HISTFILE
	// somewhere deliberate rather than in a home that is already there — and
	// the boundary makes it only once the gate has allowed the file, so a
	// refused HISTFILE does not leave a directory tree behind where it
	// pointed.
	f, err := h.bound.OpenFile(ctx, boundary.File{
		Path:    h.path,
		Flags:   os.O_APPEND | os.O_CREATE | os.O_WRONLY,
		Perm:    0o600,
		Parents: true,
	})
	if errors.Is(err, boundary.ErrRefused) {
		// Refused, and silently: the session is ending, there is nobody left
		// to tell, and a policy that hid the file meant for it not to be
		// written. The sink has the refusal. A HISTFILE that is a link into
		// a denied place is refused here too, on the object rather than on
		// the name, and reads the same way.
		return nil
	}
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, line := range added {
		_, _ = w.WriteString(line)
		_ = w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return h.trim()
}

// trim brings the file back under HISTFILESIZE.
//
// This is the one time the file is rewritten rather than appended to, and the
// exception is what makes the bound a bound: a limit that is never enforced is
// a number in a variable. It happens only when the file is over it, so the
// ordinary exit is still an append and two shells closing at once still both
// keep their lines.
//
// A temporary file and a rename, so that a shell killed in the middle of this
// leaves the old history rather than half of it. The temporary is made in the
// same directory because a rename across filesystems is not one, and it is
// created 0600 for the reason the history itself is.
//
// The bound is enforced when this shell writes, which is the only moment it
// touches the file. bash instead rewrites the whole file from its in-memory
// list at exit, so a bash that ran with a small HISTSIZE throws away what
// earlier sessions wrote; that is a divergence and it is on purpose, because
// silently deleting somebody's history is the worse of the two failures.
func (h historyFile) trim() error {
	f, err := os.Open(h.path)
	if err != nil {
		return nil
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	_ = f.Close()
	if len(lines) <= h.file {
		return nil
	}
	lines = lines[len(lines)-h.file:]

	dir := filepath.Dir(h.path)
	tmp, err := os.CreateTemp(dir, ".sh_history-")
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmp)
	for _, line := range lines {
		_, _ = w.WriteString(line)
		_ = w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), h.path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// withoutCredentials drops the lines that carry a secret.
//
// This is the write path, and putting the rule here rather than only where a
// line is typed is the point: the file is the thing that outlives the
// session, gets copied into a backup, gets read by whoever ends up with the
// machine. Everything that reaches disk goes through this function, so
// "the history file never held a credential" is a property of the file rather
// than a property of one loop remembering to ask.
//
// Rejected rather than redacted, which is the split the rules are built for.
// A command line carrying a secret usually *is* the secret — `export
// TOKEN=…` is nothing else — so a redacted skeleton of it recalls nothing
// anyone wanted, and dropping the line whole is proportionate. Output is the
// opposite case and is redacted instead; the same table answers both.
//
// Silent here, deliberately. A history that quietly loses lines is its own
// confusion, so the person is told at the moment they type the line — see
// Shell.recording — where the notice lands next to the thing it is about
// instead of arriving in a rush as the shell exits.
func withoutCredentials(added []string) []string {
	kept := added[:0:0]
	for _, line := range added {
		if _, found := secret.Default().Match(line); found {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

// matchingWalk is what a run of these keys remembers.
//
// **A run of them is one walk, not a search per keystroke**, and that is
// measured rather than an optimization. On an empty line the first press walks
// plainly; by the second press the line holds a recalled entry with a first
// word of its own, and recomputing from the line would turn the plain walk
// into a search for that word and stop dead. Measured against zsh 5.9.2: empty
// line, Up, Up gives `print hello` then `echo two`, where recomputing gives
// `print hello` twice.
//
// before says the previous keystroke was one of these, which is the same
// shape lastTab and lastArg.walking use and is set in the same place — see
// readLine's bookkeeping.
type matchingWalk struct {
	// word is what entries must begin with, and plain says this walk is not
	// searching for anything: the ordinary Up and Down, which is what an empty
	// line gets. The two are separate because an empty word is *not* the same
	// question as a plain walk — see browseMatching.
	word   string
	plain  bool
	now    bool
	before bool
}

// browseMatching walks the history the way browse does, but only to entries
// that begin with the line's first word. -1 is older, +1 is newer.
//
// See WidgetPreviousHistoryMatching in widgets.go, which carries what was
// measured. Four things shape it, and each is a row of that table:
//
//   - **The first word, not the whole line and not the text before the
//     cursor.** `echo zz` finds `echo two` although nothing begins with `echo
//     zz`, and `echo` with the cursor moved to the start finds it too.
//   - **An empty line is a plain walk, and a line beginning with a blank is
//     not.** Both have an empty first word and they are different questions —
//     see the switch below.
//   - **Nothing to walk to leaves the line exactly as it is**, rather than
//     stopping at the nearest entry or clearing the line.
//   - **A run of these keys is one walk**, which is matchingWalk above.
//
// The draft bookkeeping is browse's, unchanged: the line being typed comes
// back when the walk reaches the bottom, and an entry recalled and then edited
// keeps the edit. That is why this hands off to browse rather than reaching
// into the walk — the two must not come to disagree about what a walk leaves
// behind.
//
// **One measured case is deliberately not reproduced.** Recall an entry, edit
// it, then press the other direction: zsh leaves the line alone and `$HISTNO`
// does not move, though the entry it would reach is still a match. Probing
// from outside did not explain why — the first word is unchanged and the
// target still matches — so this walks, as it does from an unedited line.
// Written down rather than guessed at, so the next person has the observation
// rather than a surprise.
func (e *editor) browseMatching(dir int, prompt drawnPrompt) {
	if !e.search.before {
		// A new walk. What it looks for is fixed here and not asked again,
		// because the line changes under it on every step.
		e.search.plain = len(e.line) == 0
		e.search.word = firstWord(e.line)
	}
	e.search.now = true
	switch {
	case e.search.plain:
		// An empty line is the "or-history" half of these keys: the ordinary
		// walk, which is what makes the arrows behave normally in the case a
		// person notices.
		e.browse(dir, prompt)
		return
	case e.search.word == "":
		// A line that is not empty and whose first word is — it begins with a
		// blank — searches for nothing and finds nothing. Measured, and it is
		// the row that separates this from an empty line: ` echo`, `  echo`
		// and a lone space all leave the line alone with `$HISTNO` unmoved,
		// where an empty line walks. An empty search read as "matches
		// everything" would have recalled the newest entry for all three.
		return
	}
	for to := e.browsing + dir; to >= 0 && to <= len(e.history); to += dir {
		if !e.entryStartsWith(to, e.search.word) {
			continue
		}
		// Reached through browse's own step count so that the drafts, the
		// cursor and the redraw are the walk's and not a second copy of it.
		e.browse(to-e.browsing, prompt)
		return
	}
	// Nothing older, or newer, begins with it. The line stays as it is.
}

// entryStartsWith reports whether the history at this step begins with word.
//
// len(e.history) is the line being typed rather than an entry, and it is a
// candidate like any other: walking back down to the bottom brings back what
// was being typed — measured, `echo` then Up, Up, Down, Down ends on `echo`
// with the cursor where it was — and a walk that skipped it would strand a
// person on the oldest match with no way back to their own line.
//
// Everywhere else the *entry* is what is searched, not whatever a draft left
// at that step. An entry recalled and then edited still comes back edited,
// because that is browse's doing and this does not touch it; what an edit does
// not do is change which entries a later search can reach. That is the simple
// reading rather than a measured one — see browseMatching's note about the
// edit-then-reverse case zsh answers in a way probing did not explain, which
// is the same corner from the other side.
func (e *editor) entryStartsWith(to int, word string) bool {
	if to == len(e.history) {
		draft, kept := e.drafts[to]
		return kept && strings.HasPrefix(string(draft), word)
	}
	return strings.HasPrefix(e.history[to], word)
}

// firstWord is the run of non-blank characters the line *starts* with, and is
// empty when the line starts with a blank.
//
// Not "the first word after any leading blanks", which is the reading that
// looks right and is measured wrong: ` echo` finds nothing in zsh, where
// skipping the blank would have found `echo two`. See browseMatching, which is
// where that distinction is spent.
func firstWord(line []rune) string {
	end := 0
	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		end++
	}
	return string(line[:end])
}
