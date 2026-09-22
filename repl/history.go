// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"context"
	"errors"
	"io"
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

	// encoding is how the file spells an entry — see decodeEntries, and
	// HistoryStyle, where the two facts it holds were measured. The zero value
	// is one entry per line, which is what every reader of this file did
	// before the encoding was a question.
	encoding historyEncoding
}

// historyEncoding is the pair of answers HistoryStyle gives about the file.
//
// A struct of its own rather than two fields on historyFile, so that the thing
// passed to decodeEntries is the whole of what decides the answer and a third
// fact added later has one place to go.
type historyEncoding struct {
	continuesOnABackslash bool
	mayCarryATimestamp    bool
	mayCarryAHashLine     bool
	emptyIsAnEntry        bool
}

// historyEncodingFrom reads the encoding off the style a dialect stated.
//
// One place rather than one per reader. A history file is read by more than
// one thing in this repository — this package reads the session's, and a
// dialect's own `history -r` and `fc -R` read a script's — and the encoding is
// a property of the *file*, so a reader that answered the question for itself
// would be a second answer to it.
func historyEncodingFrom(style HistoryStyle) historyEncoding {
	return historyEncoding{
		continuesOnABackslash: style.EntriesContinueOnABackslash,
		mayCarryATimestamp:    style.EntriesMayCarryATimestampHeader,
		mayCarryAHashLine:     style.EntriesMayCarryAHashTimestampLine,
		emptyIsAnEntry:        style.EmptyLinesAreEntries,
	}
}

// HistoryEntries turns a history file's physical lines into the entries it
// holds, under the encoding the dialect stated.
//
// The public half of the decoder, for a front end or a dialect that reads a
// history file of its own — `history -r`, `history -n`, `fc -R`, and the read
// a script's first `set -o history` does. They pass the same HistoryStyle the
// session is built from, so a file this shell's prompt can read is a file its
// builtins read the same way, and an encoding learned for one is not a rule
// the other is missing.
//
// Whether an empty line in the file is an entry is part of that decoding and
// not a policy left to whoever called — see EmptyLinesAreEntries, where
// the panel's disagreement is. It used to be the caller's, which meant the
// session's reader dropped a blank and a script's `history -r` kept one, out
// of the same file (#4024).
func HistoryEntries(style HistoryStyle, lines []string) []string {
	return decodeEntries(lines, historyEncodingFrom(style), true)
}

// HistoryEntriesIn turns a history file's **text** into the entries it holds.
//
// The text rather than the lines a reader already split, because one of the
// answers is about where the file *stops*. A style that joins entries on a
// backslash has to know whether the last line had a newline after it, and a
// reader that split the text first has thrown that away: `echo b\` with a
// newline after it is a promise of another line that never came and the
// entry goes, and the same bytes with no newline are a command ending in a
// backslash somebody typed. Measured 2026-09-21, zsh 5.9.2, one file at a
// time under `env -i` with a scratch `HOME`:
//
//	echo a ⏎ echo b\ ⏎          echo a — the second entry is dropped
//	echo b\ ⏎                   nothing at all
//	echo b\  (no newline)       echo b\ — the backslash is a character
//	one newline and nothing else an entry, empty
//
// [HistoryEntries] is the same decoder for a caller that has only lines, and
// it reads them as a file that ended with a newline. A style stating no
// continuation cannot tell the two apart, so that caller loses nothing.
func HistoryEntriesIn(style HistoryStyle, text string) []string {
	return decodeText(text, historyEncodingFrom(style))
}

// decodeText is HistoryEntriesIn against an encoding already read off a
// style, and is where a file's text becomes its physical lines.
func decodeText(text string, enc historyEncoding) []string {
	if text == "" {
		// No file at all, which is not the same as a file holding a newline
		// — that one is an entry. See the table above.
		return nil
	}
	terminated := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	return decodeEntries(lines, enc, terminated)
}

// HistoryText is the **encoder**: the text a history file holds for these
// entries, under the encoding the dialect stated.
//
// The mirror of [HistoryEntriesIn], and it exists for the same reason the
// decoder does — one answer per file rather than one per writer. Every
// writer in this tree put each entry down followed by a newline, so an entry
// holding a newline became two physical lines and reading the file back gave
// two entries, in the one dialect whose own file closes that round trip
// (#4034).
//
// Measured 2026-09-21, zsh 5.9.2 under a pseudo-terminal, entries planted
// and written with `fc -W`:
//
//   - an entry's newline is written as a backslash and a newline, which is
//     exactly what the decoder reads back;
//   - an entry whose last character is a backslash gets a **space** after
//     it, so that the newline ending it is not read as a promise of another
//     line — and the reader takes that one space back off, measured a
//     variant at a time: `echo x\ ` reads as `echo x\` and `echo x\  `
//     keeps one of the two;
//   - a tab, a carriage return and a backslash in the middle of an entry are
//     all written as themselves.
//
// One shape is written differently here on purpose. A backslash immediately
// *before* an embedded newline is one character in the real shell's file —
// the same backslash doing both jobs — so the character somebody typed is
// gone when the entry is read back. This writes both, which is the exact
// mirror of a decoder that strips one, and it is safe rather than only
// nicer: measured 2026-09-21, the real shell reads the two-backslash file to
// the same entry this does, so a file written here is one it understands.
//
// Nothing writes a timestamp header, with the option or without, because
// nothing in this shell keeps the times an entry would need. When something
// does, the header belongs here beside the continuation and not in a second
// writer — which is the shape this change exists to stop happening again.
func HistoryText(style HistoryStyle, entries []string) string {
	return encodeEntries(entries, historyEncodingFrom(style))
}

// encodeEntries is HistoryText against an encoding already read off a style.
func encodeEntries(entries []string, enc historyEncoding) string {
	var b strings.Builder
	for _, entry := range entries {
		if enc.continuesOnABackslash {
			entry = strings.ReplaceAll(entry, "\n", "\\\n")
			if strings.HasSuffix(entry, `\`) {
				// The guard, so the newline below is not a continuation.
				entry += " "
			}
		}
		b.WriteString(entry)
		b.WriteByte('\n')
	}
	return b.String()
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

	// The whole text rather than a line at a time, because whether the file
	// ended with a newline is one of the things the decoder asks — and a
	// scanner cannot say. It used to be a bufio.Scanner with a four-megabyte
	// line bound, which answered a different worry (a pasted command can be
	// very long) and threw this one away before the decoder could reach it.
	// See HistoryEntriesIn. A read failure part-way through reads as the
	// bytes that did arrive, for the reason a missing file reads as no
	// history: there is nobody to tell at startup.
	data, _ := io.ReadAll(f)
	// One decoder, and it is the same call `history -r` makes — whether an
	// empty line is an entry included. This used to drop the blanks itself,
	// which made the session's reading of a file differ from a script's
	// reading of the very same file (#4024).
	entries := decodeText(string(data), h.encoding)
	if len(entries) > h.size {
		entries = entries[len(entries)-h.size:]
	}
	return entries
}

// decodeEntries turns the file's physical lines into the entries a person
// typed. See HistoryStyle, where both facts were measured.
//
// The order is the file's own: an entry is gathered across its continuation
// lines *first*, and the timestamp header comes off the front of what that
// produced. Doing it the other way round would strip a header, then join, and
// a continuation line that happened to begin `: 1:0;` would lose its text.
//
// terminated is whether the file ended with a newline, which only a style
// that continues on a backslash can ask about — see HistoryEntriesIn, where
// the four rows are, and where the reason it is the *text* and not the lines
// that is decoded is written down.
func decodeEntries(lines []string, enc historyEncoding, terminated bool) []string {
	var out []string
	var held strings.Builder
	continuing := false
	// Whether this read is one of the files that puts its times on lines of
	// their own, which is decided once by the first line and not per line.
	// See EntriesMayCarryAHashTimestampLine, where the measurement is.
	hashed := enc.mayCarryAHashLine && len(lines) > 0 && isHashTimestampLine(lines[0])
	for i, line := range lines {
		if hashed && isHashTimestampLine(line) {
			// The header is not part of any entry and there is nothing here
			// that keeps a time, so it goes. A dangling one at the end of the
			// file goes with the rest.
			continue
		}
		if enc.continuesOnABackslash && strings.HasSuffix(line, `\`) && (terminated || i < len(lines)-1) {
			// The backslash is the mark and not part of the command: what was
			// typed had a newline there.
			//
			// Not on the file's last line where the file did not end with a
			// newline: there is no line after it to join to, and the shell
			// reads the backslash as a character somebody typed rather than
			// as a mark. That row and the one under it are what say the rule
			// is about where the **file** stops.
			held.WriteString(strings.TrimSuffix(line, `\`))
			held.WriteByte('\n')
			continuing = true
			continue
		}
		if enc.continuesOnABackslash {
			// A line whose trailing run of spaces has a backslash in front
			// of it loses exactly one of them: that space is the encoder's
			// guard around an entry whose own last character is a
			// backslash, and taking it off is what closes the round trip.
			// Measured a variant at a time: `echo x\ ` reads as `echo x\`,
			// `echo x\  ` keeps one of the two, `echo x \ ` reads as
			// `echo x \`, and `echo a  ` keeps both — so it is the
			// backslash and not the space that makes the rule. See
			// HistoryText.
			if bare := strings.TrimRight(line, " "); len(bare) < len(line) && strings.HasSuffix(bare, `\`) {
				line = line[:len(line)-1]
			}
		}
		if continuing {
			held.WriteString(line)
			out = append(out, withoutTimestamp(held.String(), enc))
			held.Reset()
			continuing = false
			continue
		}
		out = append(out, withoutTimestamp(line, enc))
	}
	if continuing {
		// A file whose last line promised another line and did not have one
		// — a session killed mid-write, or a trim that cut inside an entry.
		// The entry goes, and what there was of it goes with it: measured, a
		// file holding `echo a` and `echo b\` reads as `echo a` alone, and
		// one holding `echo b\` by itself reads as nothing at all.
		//
		// The other reading — keep what there is — was this decoder's before
		// the file's end was a question it could ask, and it is reachable by
		// exactly one dialect, so it is stated here rather than carried as a
		// field nothing else would ever set. A fifth dialect that wanted the
		// salvage is where the field belongs.
		_ = held
	}
	if !enc.emptyIsAnEntry {
		// Last, and the ordering is the point: an empty line at the end of a
		// multi-line command is part of that command, so a file whose
		// encoding joins on a backslash would lose the join if the empties
		// went first. See EmptyLinesAreEntries.
		out = withoutEmpty(out)
	}
	return out
}

// withoutTimestamp takes the `: <start>:<elapsed>;` off the front of an entry,
// where the file is one that may carry it and this entry does.
//
// A line that does not match is the command itself, which is the whole reason
// this is a match and not a split: the same file holds both kinds, because the
// option can be turned on part-way through its life.
func withoutTimestamp(entry string, enc historyEncoding) string {
	if !enc.mayCarryATimestamp {
		return entry
	}
	rest, ok := strings.CutPrefix(entry, ": ")
	if !ok {
		return entry
	}
	start, rest, ok := strings.Cut(rest, ":")
	if !ok || !allDigits(start) {
		return entry
	}
	// Only the first `;` after the numbers ends the header, measured: an entry
	// whose command contains a `;` keeps it.
	elapsed, command, ok := strings.Cut(rest, ";")
	if !ok || !allDigits(elapsed) {
		return entry
	}
	return command
}

// isHashTimestampLine reports whether a physical line is one of the `#` time
// lines, which is `#` as the first character and a digit straight after it.
//
// Narrow on purpose, and measured that way: `#1abc` is one and `#-5`, `#`,
// `# 1700000000`, `#comment here` and an indented `  #1700000000` are not. A
// wider test would eat a comment somebody typed at a prompt.
func isHashTimestampLine(line string) bool {
	rest, ok := strings.CutPrefix(line, "#")
	return ok && rest != "" && rest[0] >= '0' && rest[0] <= '9'
}

// allDigits reports whether s is a run of at least one digit. A header with an
// empty or non-numeric field is not a header, and the line stands as it is.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// withoutEmpty drops the entries with nothing in them at all.
//
// Nothing at all, rather than nothing but whitespace: a line of spaces is a
// command as far as every shell measured is concerned, and this used to trim
// before testing, which dropped an entry bash keeps. See
// EmptyLinesAreEntries.
func withoutEmpty(entries []string) []string {
	out := entries[:0]
	for _, e := range entries {
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// save writes what this session added.
//
// Appending it, almost always: the lines it read at the start are already in
// the file, and writing them again would double it every time a shell is
// opened.
//
// rewrite is interp.Runner.RewritesTheHistoryFile — bash's `shopt histappend`
// read the other way round — and it asks for the other answer in the one case
// the two differ. Measured against bash 5.3.20; the table is on that method,
// and the condition below is the whole of what it says: the file is written
// from the session's list where the list no longer holds every line the
// session added, which is what HISTSIZE trimming it below that count does.
// Everywhere else bash appends with the option off exactly as it does with it
// on, so this shell's ordinary exit is the append it has always been.
func (h historyFile) save(ctx context.Context, earlier, added []string, rewrite bool) error {
	added = withoutCredentials(added)
	if h.path == "" || h.size == 0 || h.file == 0 || len(added) == 0 {
		return nil
	}
	if rewrite && len(added) > h.size {
		// The list rather than what was added, because that is what the
		// rewrite is: the file becomes the session's list, and the lines the
		// list no longer holds are the ones that go. A line another process
		// appended to the file while this session ran goes with them, which
		// is the cost the option names and the reason bash's own default
		// only pays it here.
		if err := h.writeOver(ctx, h.list(earlier, added)); err != nil {
			return err
		}
		return h.trim(ctx)
	}
	// The same bound on the append, which is HISTSIZE's and not this
	// branch's: what bash writes is the tail of its list, so a session that
	// typed more lines than the list holds appends only the lines still in
	// it. Measured 2026-09-22 on bash 5.3.20 — `HISTSIZE=2`,
	// `HISTFILESIZE=100`, `shopt -s histappend` and four lines typed leaves
	// the eight already in the file and the last **two** after them.
	if len(added) > h.size {
		added = added[len(added)-h.size:]
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
	// One encoder, and it is the same one a dialect's `history -w` and
	// `fc -W` reach: an entry holding a newline is the file's business and
	// not each writer's. See HistoryText.
	w := bufio.NewWriter(f)
	_, _ = w.WriteString(encodeEntries(added, h.encoding))
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return h.trim(ctx)
}

// trim brings the file back under HISTFILESIZE.
//
// The bound is what makes a bound: a limit that is never enforced is a number
// in a variable. It happens only when the file is over it, so the ordinary
// exit is still an append and two shells closing at once still both keep
// their lines.
//
// Measured 2026-09-22 against bash 5.3.20, which truncates here with
// `histappend` on as well as off: a session under `HISTFILESIZE=3` leaves
// three lines behind either way. So this runs after both of save's two
// writes, not only after the append.
//
// The bound is enforced when this shell writes, which is the only moment it
// touches the file. The temporary beside it is this shell's own name, but the
// two verbs that
// finish the rewrite — the rename over HISTFILE and the removal of the
// temporary — are changes to a path a *script* chose, since HISTFILE is a
// variable a line at the prompt can set. So they go through
// boundary.Boundary.Modify, which is the seam #1824 added: the open of this
// same file has passed the gate since this package existed, and the rewrite
// that replaces it was exempt as scaffolding. A refused rename leaves the old
// history where it is, which is what a shell killed in the middle of this
// leaves too.
func (h historyFile) trim(ctx context.Context) error {
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
	return h.writeOver(ctx, lines[len(lines)-h.file:])
}

// list is the entries the session can still recall: what it read at the start
// and what it added, bounded by HISTSIZE the way the file's own read is.
//
// The same list a rewrite writes and the same length the condition in save is
// read against, named once so the two cannot disagree about what "the list"
// is.
func (h historyFile) list(earlier, added []string) []string {
	all := make([]string, 0, len(earlier)+len(added))
	all = append(all, earlier...)
	all = append(all, added...)
	if len(all) > h.size {
		all = all[len(all)-h.size:]
	}
	return all
}

// writeOver writes these entries over the history file.
//
// Two callers and one rewrite: the bound below, which drops the oldest lines
// when the file is over HISTFILESIZE, and save above, which writes the
// session's list when the option asks it to. They were one function once, and
// the bound's own temporary-and-rename is the part neither can be written
// without.
//
// A temporary file and a rename, so that a shell killed in the middle of this
// leaves the old history rather than half of it. The temporary is made in the
// same directory because a rename across filesystems is not one, and it is
// created 0600 for the reason the history itself is.
//
// The entry encoder rather than a line per entry, for the reason save's
// append uses it: an entry holding a newline is the file's business and not
// each writer's, and a rewrite that wrote raw newlines would turn one entry
// into several the next session reads back. See HistoryText.
func (h historyFile) writeOver(ctx context.Context, entries []string) error {
	dir := filepath.Dir(h.path)
	// dir is the history file's own directory and is never empty, which is
	// the whole of what this pattern guards against: os.TempDir is
	// os.Getenv("TMPDIR"), and CreateTemp consults it only for an empty
	// first argument. The rewrite has to land beside the file it replaces
	// anyway, or the rename across filesystems fails.
	tmp, err := os.CreateTemp(dir, ".sh_history-") //nolint:forbidigo // dir is never empty, so TMPDIR is not consulted
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmp)
	_, _ = w.WriteString(encodeEntries(entries, h.encoding))
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if !h.bound.Modify(ctx, h.path) {
		// Refused, and the temporary goes with it: a policy that will not
		// have the file replaced must not be left a half-written neighbor of
		// it either. Silent for the reason save's refusal is — the session
		// is ending and the sink has the record.
		//
		// The temporary's own removal asks nothing, and that is the rule
		// rather than an omission: its name is one *this shell* composed and
		// no script can aim, which is what puts it outside the boundary —
		// the same sentence interp.ActionOpen makes about a process
		// substitution's pipes. Gating the removal and not the creation
		// beside it would be a check that could refuse the cleanup of a file
		// it had already allowed into existence.
		_ = os.Remove(tmp.Name())
		return nil
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
