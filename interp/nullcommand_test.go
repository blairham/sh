// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// hooked is a semantics vector with the null-command hook on and the two
// parameters named, which is what a dialect that has it supplies.
func hooked(t *testing.T) Semantics {
	t.Helper()
	s := CoreSemantics()
	s.NullCommandVariable = "NULLCMD"
	s.ReadNullCommandVariable = "READNULLCMD"
	s.RedirectsUseEveryTarget = No
	return s
}

// Every assertion here points a parameter at a *marker* rather than leaving it
// at a value that copies the file, and that is the point rather than the
// style.
//
// The two defaults a real shell ships — `cat` for the writing side and `more`
// for the reading side — both end up putting the file on standard output, so
// a probe that ran `<f` with them in place cannot tell "the hook fired and
// ran the writer" from "the hook fired and ran the reader" from "the file was
// copied by something else entirely". Three readings, one output. Pointing
// each parameter at a function that prints its own name is the only shape
// that separates them, and it is how the behavior was measured in the first
// place (#1779).
func TestACommandThatIsOnlyRedirectionsRunsTheNullCommand(t *testing.T) {
	const markers = `R(){ printf R; }; N(){ printf N; }; READNULLCMD=R; NULLCMD=N; `
	for _, tc := range []struct{ name, src, want string }{
		// One plain input redirection, and the descriptor it names does not
		// change the answer.
		{"a lone input redirection", `<f`, "R"},
		{"a numbered input redirection", `3<f`, "R"},
		{"standard input written out", `0<f`, "R"},
		// Anything else is the writing parameter's.
		{"a second input redirection", `<f <f`, "N"},
		{"an output redirection", `>g`, ""},
		{"an output redirection beside it", `<f 2>g`, "N"},
		{"a here-string", `<<<hi`, "N"},
		{"a read-write redirection", `<>g`, "N"},
		{"a duplication", `<&0`, "N"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := hooked(t)
			out, st := run(t, `printf 'hello\n' > f; `+markers+tc.src,
				func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The writing parameter's command runs with the redirection already in place,
// so `>g` puts its output in `g` rather than on the terminal — which is the
// half a standard-output comparison cannot see.
func TestTheNullCommandRunsUnderItsOwnRedirections(t *testing.T) {
	sem := hooked(t)
	dir := t.TempDir()
	out, st := run(t, `N(){ printf N; }; NULLCMD=N; >g`, func(r *Runner) {
		r.Semantics, r.Dir = &sem, dir
	})
	if st != 0 || out != "" {
		t.Errorf("out = %q, status %d, want nothing on standard output", out, st)
	}
	if got := readFile(t, dir, "g"); got != "N" {
		t.Errorf("g = %q, want the null command's output in the file it named", got)
	}
}

// A dialect with no hook runs nothing, which is the core's answer and five of
// the six panel columns'. Asserted from the same source as the row above, so
// that the two readings are told apart by the vector alone.
func TestWithoutTheHookOnlyTheRedirectionHappens(t *testing.T) {
	sem := CoreSemantics()
	dir := t.TempDir()
	out, st := run(t, `N(){ printf N; }; NULLCMD=N; >g`, func(r *Runner) {
		r.Semantics, r.Dir = &sem, dir
	})
	if st != 0 || out != "" {
		t.Errorf("out = %q, status %d, want a silent success", out, st)
	}
	if got := readFile(t, dir, "g"); got != "" {
		t.Errorf("g = %q, want the file made and nothing written to it", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "g")); err != nil {
		t.Errorf("the file was never made: %v", err)
	}
}

// An assignment prefix takes the command off the hook: `x=1 <f` assigns,
// opens and runs nothing, in the shell that has the hook as well as in the
// five that do not.
func TestAnAssignmentPrefixIsNotANullCommand(t *testing.T) {
	sem := hooked(t)
	out, st := run(t, `printf 'hello\n' > f; R(){ printf R; }; READNULLCMD=R; x=1 <f; printf "[x=%s]" "$x"`,
		func(r *Runner) { r.Semantics = &sem })
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if out != "[x=1]" {
		t.Errorf("out = %q, want the assignment and no command", out)
	}
}

// The reading parameter falls back rather than refusing when it is empty, so
// a script that clears it gets the writing one.
func TestAnEmptyReadingParameterFallsBackToTheWritingOne(t *testing.T) {
	sem := hooked(t)
	for _, src := range []string{
		`N(){ printf N; }; NULLCMD=N; READNULLCMD=; <f`,
		`N(){ printf N; }; NULLCMD=N; unset READNULLCMD; <f`,
	} {
		out, st := run(t, `printf 'hello\n' > f; `+src, func(r *Runner) { r.Semantics = &sem })
		if st != 0 || out != "N" {
			t.Errorf("%s: out = %q, status %d, want N at 0", src, out, st)
		}
	}
}

// A hook with nothing in it is a third shell, and not the same as no hook: the
// command is refused by name, the refusal abandons the script, and the
// redirection never happens.
func TestAnEmptyNullCommandIsRefused(t *testing.T) {
	sem := hooked(t)
	dg := PosixDiagnostics()
	dg.RedirectionWithNoCommand = "redirection with no command"
	dir := t.TempDir()
	out, st := run(t, `NULLCMD=; >g; printf after`, func(r *Runner) {
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
	})
	if st == 0 {
		t.Errorf("status 0, want the refusal to report a failure")
	}
	if !strings.Contains(out, "redirection with no command") {
		t.Errorf("out = %q, want the dialect's wording", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("out = %q, want the refusal to abandon the script", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "g")); err == nil {
		t.Errorf("the file was made, so the refusal came after the redirection rather than before it")
	}
}
