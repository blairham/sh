// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `$signals` against **this shell's own `kill -l`, in the same run**.
//
// The roster is the machine's — this kernel takes 31 signals and names all of
// them, and the Linux runner's does neither — so a test asserting the darwin
// list would pass on one runner and fail on the other. The assertion that is
// true on both is the one #4906 is filed on: the middle of `$signals` *is*
// the bare listing, and that listing was measured against the reference's
// byte for byte before this parameter existed. Grading the two against each
// other is therefore also what catches them drifting apart, which a literal
// would not.
func TestTheSignalRosterIsTheOneTheListingWrites(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		print -r -- "P ${(j: :)signals}"
		print -r -- "L $(kill -l)"
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	var param, listing string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "P "):
			param = strings.TrimPrefix(line, "P ")
		case strings.HasPrefix(line, "L "):
			listing = strings.TrimPrefix(line, "L ")
		}
	}
	// The control, and it is the positive this whole test rests on: a
	// listing that came back empty would make every assertion below true of
	// two empty words. `kill -l` names at least the signals POSIX requires a
	// shell to have.
	for _, want := range []string{"HUP", "INT", "TERM", "KILL"} {
		if !strings.Contains(listing, want) {
			t.Fatalf("kill -l = %q, want %s in it — the instrument is not firing", listing, want)
		}
	}
	want := "EXIT " + listing + " ZERR DEBUG"
	if param != want {
		t.Errorf("$signals = %q, want %q — EXIT, this shell's own listing, then the two it traps and no kernel sends", param, want)
	}
}

// The three parts of the array, by position rather than by name, so that a
// change putting the roster in without `EXIT` in front of it — which would
// move every kernel signal down one and still contain every word — fails
// here.
func TestTheSignalRosterIsExitThenTheKernelThenTheTwoPseudoSignals(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		print -r -- "first=$signals[1]"
		print -r -- "second=$signals[2]"
		print -r -- "count=${#signals}"
		print -r -- "last=$signals[-1]"
		print -r -- "before=$signals[-2]"
		print -r -- "hup=$(kill -l | { read -r first rest; print -r -- $first; })"
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			fields[name] = value
		}
	}
	if fields["first"] != "EXIT" {
		t.Errorf("$signals[1] = %q, want EXIT", fields["first"])
	}
	// Position two is signal one, whatever this kernel calls it — read off
	// the listing rather than written down, for the same reason as above.
	if fields["second"] == "" || fields["second"] != fields["hup"] {
		t.Errorf("$signals[2] = %q, want the first name `kill -l` writes, which is %q", fields["second"], fields["hup"])
	}
	if fields["last"] != "DEBUG" || fields["before"] != "ZERR" {
		t.Errorf("the tail of $signals is %q then %q, want ZERR then DEBUG", fields["before"], fields["last"])
	}
	// Both pseudo-signals are names this shell's `trap` takes, which is what
	// makes them entries rather than decoration. A roster naming a word no
	// trap accepts would be a list a script cannot use.
	out, st = runZsh(t, t.TempDir(), `trap 'print zerr' ZERR; false; trap - ZERR; print done`)
	if st != 0 || !strings.Contains(out, "zerr") {
		t.Errorf("trap … ZERR = %q at %d, want the handler to fire", out, st)
	}
}

// `array`, with neither `special` nor `readonly`, and a script's own write
// wins — so this is not the frozen produced seam `$ARGC` and `$status` use.
//
// `$path` is in the same run as the control: it is the shell's own array and
// says so, which is what separates "this name is ordinary" from "this shell
// has stopped saying `special` about anything".
func TestTheSignalRosterIsAnOrdinaryArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		print -r -- "signals=${(t)signals}"
		print -r -- "path=${(t)path}"
		print -r -- "there=${+signals}"
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "signals=array", "path=array-tied-special", "there=1")

	out, st = runZsh(t, t.TempDir(), `signals=(x y); print -r -- "${(t)signals} ${signals}"`)
	if st != 0 || out != "array x y\n" {
		t.Errorf("signals=(x y) then a read = %q at %d, want the script's own array back", out, st)
	}
}

// And the row a listing writes for it, which is an ordinary array's.
func TestTheSignalRosterListsAsAnOrdinaryArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -p signals`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	if !strings.HasPrefix(out, "typeset -a signals=( EXIT ") || !strings.HasSuffix(out, " ZERR DEBUG )\n") {
		t.Errorf("typeset -p signals = %q, want `typeset -a signals=( EXIT … ZERR DEBUG )`", out)
	}
}
