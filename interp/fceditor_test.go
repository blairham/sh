// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `fc`'s editor road: the entries go to a temporary file, a program is run
// over it, and what comes back is echoed and run. #4017.
//
// Nothing here names a shell. The subject is the builtin's own machinery —
// which words become the editor's command line, where the file goes and that
// it goes away, what each way out reports — and the one place the panel
// disagrees is reached by moving [Semantics.FcEmptyEditIsAnError] rather than
// by choosing a dialect. What bash and zsh were measured to do is written
// down in docs/spec/semantics.md and beside fcHistory.edit.
//
// The editors are ordinary programs off `PATH` rather than scripts this test
// writes, because a test that writes a program has to make it executable, has
// to choose an interpreter for it, and has to keep it off the tree. `cat`
// leaves the file alone and prints it, `cp /dev/null` empties it, `false`
// fails without touching it, and `echo` says what it was handed — which is
// four of the five roads out between them, with no file of our own anywhere.
//
// **No editor here waits for input.** That is the hazard this road has and no
// other: an editor is the one program a shell runs expecting a person, and a
// test that reached a real one would hang until the run timed out.

// fcEdit runs src against a list the test owns, with a temporary directory of
// its own for the spool file, and hands back what the shell wrote, what the
// list ended up holding, and that directory.
//
// The directory is the script's `TMPDIR` — assigned in the script rather than
// in the environment, because that is the variable the builtin reads and the
// only way a test can find the file again is to have chosen where it goes.
func fcEdit(t *testing.T, list *fcList, src string) (out string, status int, tmp string) {
	t.Helper()
	tmp = t.TempDir()
	out, status = run(t, "TMPDIR="+tmp+"\n"+src, list.install)
	return out, status, tmp
}

// leftBehind is what is still in the spool directory. Empty is the answer on
// every road, including the ones where the editor failed.
func leftBehind(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the spool directory: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// The whole road in one case: `cat` prints the file it was handed, so the
// first line is what was *written* for the editor, the second is the shell
// echoing what came back, and the third is that line running.
//
// A whole-string comparison rather than three Contains checks, because the
// two things this most easily gets wrong — writing the history number in
// front of the entry, and echoing nothing at all — are both invisible to a
// check that only asks whether `two` appears somewhere.
func TestFcEditorRoadWritesTheEntryRunsTheEditorAndRunsWhatComesBack(t *testing.T) {
	list := &fcList{entries: []string{"echo one", "echo two"}}
	out, st, tmp := fcEdit(t, list, "fc -e cat")
	if want := "echo two\necho two\ntwo\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	if names := leftBehind(t, tmp); len(names) != 0 {
		t.Errorf("the spool file was left behind: %v", names)
	}
}

// The edited line takes the `fc` call's own place in the list, which is the
// pair `-s` already does: the line the front end recorded for `fc` itself
// goes, and what ran goes in after it.
func TestFcEditorRoadPutsWhatRanWhereTheFcCallWas(t *testing.T) {
	list := &fcList{entries: []string{"echo one", "echo two", "fc -e cat"}, own: true}
	_, st, _ := fcEdit(t, list, "fc -e cat")
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	want := []string{"echo one", "echo two", "echo two"}
	if strings.Join(list.entries, "\n") != strings.Join(want, "\n") {
		t.Errorf("list = %v, want %v", list.entries, want)
	}
}

// A range goes in whole and unnumbered, one entry per line, in the order the
// two operands describe — and `-r` turns that order around rather than
// swapping the ends.
// The list is five entries and the operands name the first three, which is
// not padding: an absolute number is in range only up to `cur-2` — #4016's
// measurement — so a three-entry list cannot be asked about its third entry
// by number at all, and a shorter list here would be testing that rule
// instead of this one.
func TestFcEditorRoadWritesARangeInTheOrderItWasAskedFor(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Written forwards, and `cat` echoes the file before the shell
		// reads it back. The echo and the run interleave because the text
		// is read a line at a time — see fceditorlines_test.go, which is
		// where that is the subject rather than a consequence.
		{"fc -e cat 1 3", "echo one\necho two\necho three\n" +
			"echo one\none\necho two\ntwo\necho three\nthree\n"},
		// Written backwards, which runs backwards.
		{"fc -e cat 3 1", "echo three\necho two\necho one\n" +
			"echo three\nthree\necho two\ntwo\necho one\none\n"},
		// And `-r` on top of the forward range gives the backward one.
		{"fc -r -e cat 1 3", "echo three\necho two\necho one\n" +
			"echo three\nthree\necho two\ntwo\necho one\none\n"},
		// An absent `last` is `first` and not the end of the list, which is
		// where this road parts company with `-l`.
		{"fc -e cat 1", "echo one\necho one\none\n"},
	} {
		list := &fcList{entries: []string{
			"echo one", "echo two", "echo three", "echo four", "echo five",
		}}
		out, st, _ := fcEdit(t, list, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s: out = %q status = %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The editor is a command **line**: it is split into words and the file is
// appended after the split, so an editor with an option keeps it.
//
// `echo` is the editor, so the output is the whole argv after the program
// name — which is how this asserts both halves at once: the option survived
// and the file came last.
func TestFcEditorIsSplitIntoWordsWithTheFileLast(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, st, tmp := fcEdit(t, list, "fc -e 'echo seen'")
	if st != 0 {
		t.Fatalf("out = %q status = %d, want 0", out, st)
	}
	// `echo` leaves the file alone, so the entry is echoed and run after it.
	line, rest, found := strings.Cut(out, "\n")
	if !found || rest != "echo one\none\n" {
		t.Errorf("out = %q, want the entry echoed and run after the editor's line", out)
	}
	word, path, ok := strings.Cut(line, " ")
	if !ok || word != "seen" {
		t.Errorf("editor argv = %q, want the option word kept in front of the file", line)
	}
	if !strings.HasPrefix(path, tmp+string(os.PathSeparator)) {
		t.Errorf("the file was %q, want it under the script's own TMPDIR %q", path, tmp)
	}
}

// An editor that exits non-zero stops the road: nothing is echoed, nothing
// runs, and `fc` answers 1 — not the status the editor exited with.
func TestAnEditorThatFailsRunsNothing(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, st, tmp := fcEdit(t, list, "fc -e false")
	if out != "" || st != 1 {
		t.Errorf("out = %q status = %d, want silence at 1", out, st)
	}
	if names := leftBehind(t, tmp); len(names) != 0 {
		t.Errorf("a failed editor left the spool file behind: %v", names)
	}
}

// An editor that is not there is the shell's ordinary `command not found`,
// and `fc` answers 1 rather than the 127 the search produced.
func TestAnEditorThatIsNotThereIsReportedAndRunsNothing(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, st, tmp := fcEdit(t, list, "fc -e no-such-editor-4017")
	if st != 1 || !strings.Contains(out, "no-such-editor-4017") {
		t.Errorf("out = %q status = %d, want the name reported at 1", out, st)
	}
	if strings.Contains(out, "one\n") {
		t.Errorf("out = %q, the entry ran without an editor", out)
	}
	if names := leftBehind(t, tmp); len(names) != 0 {
		t.Errorf("an unrunnable editor left the spool file behind: %v", names)
	}
}

// The order the editor is chosen in: the `-e` operand, then `FCEDIT`, then
// `EDITOR`, each only while it is non-empty.
//
// `false` and `cat` stand in for "this one was chosen" and "this one was
// not", so each row is a status rather than a name that has to be matched.
func TestTheEditorIsTheOperandThenFceditThenEditor(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      int
	}{
		{"the operand wins over both", "FCEDIT=false EDITOR=false fc -e cat", 0},
		{"FCEDIT wins over EDITOR", "FCEDIT=false EDITOR=cat fc", 1},
		{"EDITOR is taken when FCEDIT is unset", "EDITOR=cat fc", 0},
		{"an empty FCEDIT falls through to EDITOR", "FCEDIT= EDITOR=cat fc", 0},
		{"an empty EDITOR falls through too", "FCEDIT=cat EDITOR= fc", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			list := &fcList{entries: []string{"echo one"}}
			out, st, _ := fcEdit(t, list, c.src)
			if st != c.want {
				t.Errorf("out = %q status = %d, want %d", out, st, c.want)
			}
		})
	}
}

// The fallback, when nothing named an editor: `vi`.
//
// Asserted through a `PATH` that holds nothing, so the name is *reported*
// rather than run. That is not a way around the assertion but the only safe
// shape it has: `vi` is on this machine, an editor waits for a person, and a
// test that started one would hang until the run timed out.
//
// `ed` under `set -o posix` is bash's alone — no other column in the panel
// has the option — so it is asserted in dialect/bash beside the rest of that
// shell's measurements.
func TestTheFallbackEditorIsVi(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, st, _ := fcEdit(t, list, "PATH=/nonexistent-4017\nfc")
	if st != 1 || !strings.Contains(out, "vi:") {
		t.Errorf("out = %q status = %d, want vi reported at 1", out, st)
	}
	if strings.Contains(out, "one\n") {
		t.Errorf("out = %q, the entry ran without an editor", out)
	}
}

// An editor that emptied the file, which is the one answer the panel splits
// on — see Semantics.FcEmptyEditIsAnError.
//
// The base answer is silence at 0: an editor quit without saving is not an
// error the shell invented. `cp /dev/null` is the editor, which is also the
// second proof that the command line is split — it is two words and the file
// is the third.
func TestAnEmptiedEditorFileCanBeSilent(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	out, st, tmp := fcEdit(t, list, "fc -e 'cp /dev/null'")
	if out != "" || st != 0 {
		t.Errorf("out = %q status = %d, want silence at 0", out, st)
	}
	if names := leftBehind(t, tmp); len(names) != 0 {
		t.Errorf("the spool file was left behind: %v", names)
	}
}

// And the other answer: a complaint naming the file, at 1, which **ends the
// shell** rather than leaving a status behind.
//
// The second command is what says so. A refusal that only set a status would
// let `echo after` run, and `out` would hold it — which is the difference a
// status-only assertion cannot see.
func TestAnEmptiedEditorFileCanEndTheShell(t *testing.T) {
	list := &fcList{entries: []string{"echo one"}}
	tmp := t.TempDir()
	out, st := run(t, "TMPDIR="+tmp+"\nfc -e 'cp /dev/null'\necho after", func(r *Runner) {
		list.install(r)
		sem := testSemantics()
		sem.FcEmptyEditIsAnError = Yes
		r.Semantics = &sem
		dg := Diagnostics{FcEmptyEdit: "read error on %[1]s"}
		r.Diagnostics = &dg
	})
	// 2 is the core preset's fatal status — Semantics.FatalErrorStatusIsOne
	// is what moves it, and the 1 the shell that answers this axis `yes`
	// actually reports is asserted in dialect/zsh.
	if st != 2 {
		t.Errorf("out = %q status = %d, want the fatal status 2", out, st)
	}
	if !strings.HasPrefix(out, "sh: read error on "+tmp+string(os.PathSeparator)) ||
		!strings.HasSuffix(out, "\n") {
		t.Errorf("out = %q, want the refusal naming the file under %q", out, tmp)
	}
	if strings.Contains(out, "after") {
		t.Errorf("out = %q, the shell carried on past a refusal that ends it", out)
	}
	if names := leftBehind(t, tmp); len(names) != 0 {
		t.Errorf("the spool file was left behind: %v", names)
	}
}

// The refusals that come *before* an editor is chosen are still the ones
// #4016 measured, and naming an editor does not turn them into something
// else: an operand nothing begins with is `no command found`, and this very
// command is `out of range`.
//
// Here because the editor road is now where both are reached from, and a fix
// that resolved the range after spooling would pass every case above while
// running `vi` on a file it should never have written.
func TestTheEditorRoadStillRefusesBeforeItSpools(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"fc -e cat zzz", "no command found"},
		{"fc -e cat 1 -0", "out of range"},
	} {
		list := &fcList{entries: []string{"echo one", "echo two"}}
		out, st, tmp := fcEdit(t, list, c.src)
		if st != 1 || !strings.Contains(out, c.want) {
			t.Errorf("%s: out = %q status = %d, want %q at 1", c.src, out, st, c.want)
		}
		if names := leftBehind(t, tmp); len(names) != 0 {
			t.Errorf("%s: a refused call spooled a file anyway: %v", c.src, names)
		}
	}
}
