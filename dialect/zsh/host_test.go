// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The machine's name is a *parameter* in this shell, and the prompt codes read
// that parameter rather than asking the system each time.
//
// Measured 2026-09-13 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin`, over `-c`:
//
//	printf '%s' "$HOST"            Blairs-MacBook-Pro-5.local
//	HOST=a.b.c.d; print -rP '%m'   a
//	HOST=a.b.c.d; print -rP '%2m'  a.b
//	HOST=a.b.c.d; print -rP '%M'   a.b.c.d
//
// Two facts and not one, which is why they are two tests. This shell had
// neither: `$HOST` was empty, and `%m` asked the operating system, so an
// assignment moved nothing (#2576). An implementation that set the parameter
// and left the prompt asking would pass the first and fail the second — it
// would draw the *build machine's* name for all four rows above.
//
// No test here names a host name that belongs to a machine. The startup value
// is checked against interp.MachineName, which is the question this dialect
// asks, and every behavior below is checked on a name the test assigned.
func TestTheMachineNameIsAParameterAtStartup(t *testing.T) {
	r := zshRunnerForTest(t)
	got, ok := r.GetVar("HOST")
	if !ok {
		t.Fatalf("HOST is not set: a real `.zshrc` branches on `[[ $HOST == … ]]`, and unset takes the wrong arm silently")
	}
	if want := interp.MachineName(); got != want {
		t.Errorf("HOST = %q, want %q — the name this dialect asks the system for", got, want)
	}
}

// And it is an *ordinary* scalar, not one of the produced or special names.
//
// Measured on the same run: `${(t)HOST}` is a bare `scalar`, where `$UID` is
// `integer-special` and `$IFS` is `scalar-special`; `typeset -p HOST` writes
// `typeset HOST=…`; and `unset HOST` removes it outright, after which
// `typeset -p HOST` is `no such variable: HOST` at 1.
func TestTheMachineNameParameterIsAnOrdinaryScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${(t)HOST}`, "scalar\n"},
		{`unset HOST; print -r -- "[${HOST-no such parameter}]"`, "[no such parameter]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// An inherited `HOST` wins over the one this dialect would supply.
//
// Measured: `env HOST=injected.example zsh -c 'print -r -- "$HOST"'` writes
// `injected.example`, and `typeset -p HOST` writes `export HOST=…` where the
// shell's own answer is an unexported `typeset HOST=…`. That is the half a
// guard on the *stored* table alone does not give — the environment is a layer
// under it, so a name found unstored may still be a name the shell was handed,
// and this shell overwrote it until the guard read both.
//
// It is the opposite answer from `$UID` beside it, which is why it is measured
// rather than assumed: `env UID=999 zsh -c 'print $UID'` writes the real uid.
func TestAnInheritedMachineNameWins(t *testing.T) {
	base := dialecttest.Base{Dir: t.TempDir(), Env: []string{"HOST=injected.example"}}
	out, _, err := preset.Combined(t, base, `print -r -- "[$HOST]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "[injected.example]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	r := preset.Runner(base)
	got, ok := r.PromptField(interp.FieldHostFull, "", false)
	if !ok || got != "injected.example" {
		t.Errorf("%%M = %q (ok=%v), want %q — the escape reads the parameter, inherited or not", got, ok, "injected.example")
	}
}

// Assigning to the parameter moves both prompt codes, on the next draw.
//
// The test the deferred form could not pass: SetPromptHostFunc keeps its first
// answer, which is right for a fact about the process and wrong for a name a
// script may assign. `HOST=a.b.c.d` on the line before is a *different* answer
// from the one drawn a moment earlier, so the pair below is asked twice on one
// runner rather than once each on two.
func TestAssigningTheMachineNameMovesThePromptCodes(t *testing.T) {
	r := zshRunnerForTest(t)
	r.SetVar("HOST", "first.example.test")
	if got, ok := r.PromptField(interp.FieldHost, "", false); !ok || got != "first" {
		t.Fatalf("%%m before = %q (ok=%v), want %q", got, ok, "first")
	}
	r.SetVar("HOST", "a.b.c.d")
	for _, tc := range []struct {
		field interp.PromptField
		code  string
		arg   string
		want  string
	}{
		{interp.FieldHost, "%m", "", "a"},
		{interp.FieldHost, "%2m", "2", "a.b"},
		{interp.FieldHostFull, "%M", "", "a.b.c.d"},
	} {
		got, ok := r.PromptField(tc.field, tc.arg, false)
		if !ok || got != tc.want {
			t.Errorf("%s after the second assignment = %q (ok=%v), want %q", tc.code, got, ok, tc.want)
		}
	}
}

// Unsetting it draws nothing, and does not report the escape as missing.
//
// Measured: `unset HOST; print -rP "[%m][%M]"; echo "st=$?"` writes `[][]` and
// `st=0`. That is the line between the two ways a Runner can have no host —
// this one has been told where to look and a script emptied it, where a Runner
// nobody told at all still refuses `%m` by name. See SetPromptHostParameter.
func TestUnsettingTheMachineNameDrawsNothingRatherThanRefusing(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`unset HOST; print -rP -- "[%m][%M]"; print -r -- "st=$?"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "[][]\nst=0\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
