// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A function defined inside `emulate … -c` is sticky: the emulation is
// entered again whenever it is later called, and left when it returns.
//
// Every row reads the state from inside the body and from the top level, so a
// row cannot pass on a shell that is simply in the emulation the whole time —
// the top-level reads before and after are the control, and they were already
// right before this was built (#4477).
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26.
const stickyShow = "showopts() { print -r -- \"ws=${options[shwordsplit]} " +
	"eg=${options[extendedglob]} em=$(emulate)\"; }\n"

func TestAFunctionDefinedInAnEmulationReentersIt(t *testing.T) {
	src := stickyShow + `showopts
emulate -R sh -c "s1() { showopts; }"
s1
showopts`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=off eg=off em=zsh\n" +
		"ws=on eg=off em=sh\n" +
		"ws=off eg=off em=zsh\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// The mode and the strictness travel with the *definition* and not with the
// call, which is the row an implementation keying on the caller's state would
// get wrong: `extendedglob` is turned on between the definition and the call
// and the `-R` reset still takes it off inside the body.
func TestAStickyCallCarriesTheDefinitionsMode(t *testing.T) {
	src := stickyShow + `emulate -R sh -c "s1() { showopts; }"
emulate -R ksh -c "s2() { showopts; }"
setopt extendedglob
s1
s2
showopts`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=on eg=off em=sh\n" +
		"ws=on eg=off em=ksh\n" +
		"ws=off eg=on em=zsh\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `-R` is not what makes a function sticky — the plain form marks it too —
// and whatever the body then moves goes back at the return.
func TestAPlainEmulationIsStickyAndTheTableGoesBack(t *testing.T) {
	src := stickyShow + `emulate sh -c "n1() { setopt errexit; showopts; }"
n1
print -r -- "after: ee=${options[errexit]}"`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=on eg=off em=sh\nafter: ee=off\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// The mark is the definition's and the last definition wins, in both
// directions. These two rows are why the mark is taken at the definition
// rather than from a difference between the function tables around the `-c`:
// the first adds no name, and the second happens where no emulation is
// running at all.
func TestTheStickyMarkIsTheLastDefinitions(t *testing.T) {
	src := stickyShow + `f1() { showopts; }
emulate -R sh -c "f1() { showopts; }"
f1
emulate -R sh -c "f2() { showopts; }"
f2() { showopts; }
f2`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=on eg=off em=sh\nws=off eg=off em=zsh\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// What a sticky function calls is not itself sticky and needs no mark: an
// ordinary callee runs under the emulation its caller is in, and goes back to
// the shell's own when called from the top level.
func TestWhatAStickyFunctionCallsIsNotSticky(t *testing.T) {
	src := stickyShow + `plain() { showopts; }
emulate -R sh -c "s1() { plain; }"
s1
plain`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=on eg=off em=sh\nws=off eg=off em=zsh\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// And a sticky call nests: a function the emulation defined that calls
// another one it defined stays in the emulation throughout, and a `-c` inside
// a sticky body marks with its own mode rather than the enclosing one.
func TestStickyCallsNest(t *testing.T) {
	src := stickyShow + `emulate -R sh -c "a1() { showopts; a2; }; a2() { showopts; }"
a1
emulate -R ksh -c "b1() { emulate -R sh -c 'b2() { showopts; }'; }"
b1
b2
showopts`
	out, st := runZsh(t, t.TempDir(), src)
	want := "ws=on eg=off em=sh\n" +
		"ws=on eg=off em=sh\n" +
		"ws=on eg=off em=sh\n" +
		"ws=off eg=off em=zsh\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
