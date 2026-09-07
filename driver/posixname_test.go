// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A shell invoked under the standard's own name starts in POSIX mode, and it
// is the front end that reads the name — #733, the half #691 left behind.
//
// The observable used throughout is the axis POSIX mode moves: a redirection
// that cannot be made, written on a *special* builtin, ends a non-interactive
// shell wherever the standard's rule is being kept. So a dialect answering
// `No` prints `after` and the same dialect in POSIX mode does not, which is
// exactly the difference between the two columns the same binary produces
// under two names.
const redirProbe = "exec 3>/nope/x\necho after\n"

// notPosix is a dialect that carries on past that redirection — the answer
// POSIX mode has something to change. Measured on a shell that has both
// answers under two names, so the starting one must be the permissive half or
// the test would pass with the wire cut.
func notPosix() interp.Semantics {
	s := interp.PosixSemantics()
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	return s
}

// namedShell is that dialect as a front end, with the `posix` option name
// declared so a script can leave the mode again. Whether a shell *has* the
// name is a dialect's answer; the mode is the core's, which is the split this
// change rests on.
func namedShell() driver.Shell {
	sh := shell()
	sh.Semantics = notPosix()
	sh.Register = func(r *interp.Runner) { r.AddSetOptions("posix") }
	return sh
}

// TestPosixNamed pins which words are the name.
//
// The last element of the path, with one leading dash removed. Measured on
// two binaries that both move: bash answers to `sh`, `/bin/sh`, `./sh` and
// `-sh` and not to `shx`, `SH` or `--sh`, so the reduction is a basename and
// the dash is stripped exactly once.
//
// zsh's own rule is wider — it reads the first letter of the name, so `bash`,
// `shx` and even `s` all put it in `sh` emulation — and this deliberately
// takes neither shell's extras. `sh` is the word both of them agree on and
// the only one anything is really invoked by.
func TestPosixNamed(t *testing.T) {
	for _, tc := range []struct {
		argv0 string
		want  bool
	}{
		{"sh", true},
		{"/bin/sh", true},
		{"/usr/bin/sh", true},
		{"./sh", true},
		{"a/b/sh", true},
		{"//sh", true},
		// The login convention prepends exactly one dash, and `login` may do
		// it to a path.
		{"-sh", true},
		{"-/bin/sh", true},
		// Two dashes is not the convention, and bash agrees: `--sh` carries
		// on where `-sh` stops.
		{"--sh", false},
		{"shx", false},
		{"bsh", false},
		{"SH", false},
		{"bash", false},
		{"testsh", false},
		{"", false},
		{"sh/", false},
	} {
		t.Run(tc.argv0, func(t *testing.T) {
			if got := driver.PosixNamed([]string{tc.argv0}); got != tc.want {
				t.Errorf("PosixNamed(%q) = %v, want %v", tc.argv0, got, tc.want)
			}
		})
	}
	if driver.PosixNamed(nil) {
		t.Error("PosixNamed(nil) = true, want false — an argv with nothing in it names nothing")
	}
}

// TestCalledShStartsInPosixMode on every route.
//
// It is a fact about the name and not about how the program arrived: measured
// 2026-09-05, bash 5.3.15 invoked as `sh` stops on all three of a command
// string, a script operand and a program on standard input, and the same
// binary invoked as `bash` prints `after` on all three.
func TestCalledShStartsInPosixMode(t *testing.T) {
	script := writeScript(t, redirProbe)
	for _, r := range []struct {
		name  string
		argv  func(argv0 string) []string
		typed string
	}{
		{"a command string", func(a string) []string { return []string{a, "-c", redirProbe} }, ""},
		{"a script operand", func(a string) []string { return []string{a, script} }, ""},
		{"standard input", func(a string) []string { return []string{a} }, redirProbe},
	} {
		for _, c := range []struct {
			name  string
			argv0 string
			want  string
		}{
			{"called sh", "sh", ""},
			{"called something else", "testsh", "after\n"},
		} {
			t.Run(r.name+"/"+c.name, func(t *testing.T) {
				out, _, _ := runPipedShell(t, namedShell(), r.typed, r.argv(c.argv0)...)
				if out != c.want {
					t.Errorf("out = %q, want %q", out, c.want)
				}
			})
		}
	}
}

// TestLeavingPosixModeReachesTheDialectsOwnAnswer is the detail #733 names,
// and the one a startup override gets wrong by writing the axis directly.
//
// Leaving the mode restores the answer that was saved on the way in rather
// than asserting the standard's opposite. A front end that set the axis
// without entering the mode would leave `set +o posix` with nothing to put
// back — and, worse, with nothing even to notice, since the mode would read
// as off and the request would be granted by doing nothing at all.
//
// Measured: `sh -c 'set +o posix; exec 3>/nope/x; echo after'` prints `after`
// at 0 in bash 5.3.15 invoked as `sh`, and re-entering the mode stops it
// again.
func TestLeavingPosixModeReachesTheDialectsOwnAnswer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		snippet string
		want    string
	}{
		{"left", "set +o posix\n" + redirProbe, "after\n"},
		{"left and entered again", "set +o posix\nset -o posix\n" + redirProbe, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runArgs(t, namedShell(), "sh", "-c", tc.snippet)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestTheNameOutranksTheInvocationsOwnOption.
//
// Measured, and it is the one option the name overrides: `sh +o posix -c
// 'exec 3>/nope/x; echo after'` stops in bash 5.3.15 where `bash +o posix -c`
// the same string carries on. The option loop is not being ignored —
// `+o errexit` on the same invocation is honored — so the name is read after
// the options rather than instead of them.
func TestTheNameOutranksTheInvocationsOwnOption(t *testing.T) {
	out, _, _ := runArgs(t, namedShell(), "sh", "+o", "posix", "-c", redirProbe)
	if out != "" {
		t.Errorf("out = %q, want the name to win over `+o posix`", out)
	}
	// The other half, which is what makes the row above about this option
	// rather than about the loop: an unrelated option written the same way
	// still takes effect.
	out, _, _ = runArgs(t, namedShell(), "sh", "-o", "errexit", "-c", `case $- in *e*) echo has-e ;; *) echo no-e ;; esac`)
	if out != "has-e\n" {
		t.Errorf("out = %q, want `-o errexit` honored under the same name", out)
	}
}

// TestTheProfileRunsBeforePosixMode.
//
// Measured 2026-09-05: a `~/.profile` read by bash 5.3.15 as `-sh -l` reports
// `posix off`, and the script that follows it stops on the failed redirection
// all the same. So the mode is the last thing startup does, after the
// invocation's options and after the files.
func TestTheProfileRunsBeforePosixMode(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("exec 3>/nope/x\necho profile-after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// A login shell that reads the profile with a script to run, which is
	// three of the panel's answer and the one that lets the file be seen at
	// all on this route.
	sh := namedShell()
	sem := notPosix()
	sem.LoginProfileWhenNonInteractive = true
	sh.Semantics = sem
	out, _, _ := runArgs(t, sh, "-sh", "-c", redirProbe)
	if want := "profile-after\n"; out != want {
		t.Errorf("out = %q, want %q — the profile runs with the mode still off, and the script then stops", out, want)
	}
}

// modeProbe asks the shell what mode it is in, in the words a person would
// use.
const modeProbe = "set -o\n"

// TestThePromptReadsTheNameFromItsOwnArgv.
//
// The prompt route never reaches the place the script routes read argv[0], so
// a fact read in one of them and not the other is a shell that answers two
// ways depending on whether it was given work — which is exactly what
// happened to `-i` (#472) and to the login profile (#482).
//
// **The mode itself is the observable and the failed redirection is not**,
// which is a correction rather than a preference. POSIX mode moves three
// answers here: whether a redirection failure on a special builtin is fatal,
// whether `unset` of a readonly name is, and whether aliases expand. The
// first two are *abandonments*, and at an interactive prompt an abandonment
// is caught — measured through a pseudo-terminal, `exec 3>/nope/x` draws the
// next prompt in bash 5.3, in bash invoked as `sh`, in dash, in ksh93 and in
// zsh, and so does `: 3>/nope/x` (#1124). So the redirection probe stopped
// being able to see the mode at a prompt the moment a prompt became a
// boundary, and it was never the thing under test: what is under test is
// whether the route read argv[0], which `set -o` answers directly and in the
// words a person would use.
func TestThePromptReadsTheNameFromItsOwnArgv(t *testing.T) {
	for _, tc := range []struct {
		name, argv0, want string
	}{
		{"called sh", "sh", "posix          on"},
		{"called something else", "testsh", "posix          off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runPipedShell(t, namedShell(), modeProbe, tc.argv0, "-i")
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %q in it", out, tc.want)
			}
		})
	}
}

// TestAFailedRedirectionAtAPromptIsNotTheModesObservable is the other half of
// that correction, kept as a test of its own so the fact is pinned rather
// than only explained: in POSIX mode *and* out of it, the prompt survives the
// failure it would abandon a script for. Both names are run, because the
// finding is that the two answers are the same here.
func TestAFailedRedirectionAtAPromptIsNotTheModesObservable(t *testing.T) {
	for _, argv0 := range []string{"sh", "testsh"} {
		out, _, _ := runPipedShell(t, namedShell(), redirProbe, argv0, "-i")
		if !strings.Contains(out, "after") {
			t.Errorf("argv0 %q: out = %q, want the prompt to have survived", argv0, out)
		}
	}
}

// TestTheExportedEntryPointsAreNotNamedShell. Run, RunCommand and the rest
// take no argument vector, so nothing they are handed can name the shell —
// the same reasoning that keeps them from being login shells. A program
// embedding a runner has an invocation of its own and this shell is not it.
func TestTheExportedEntryPointsAreNotNamedShell(t *testing.T) {
	sh := namedShell()
	sh.Name = "sh"
	var out, errs strings.Builder
	sh.Stdout, sh.Stderr = &out, &errs
	if code := driver.Run(sh, redirProbe, "sh"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if want := "after\n"; out.String() != want {
		t.Errorf("out = %q, want %q — an embedder's Name is not an argv[0]", out.String(), want)
	}
}
