// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

func TestLocalIsDynamicallyScoped(t *testing.T) {
	// A local is visible to everything the function calls and gone when it
	// returns. Testing only the second half would pass against a scheme that
	// hid the variable from callees, which is not what shells do.
	const src = `
inner() { echo "inner sees $v"; }
outer() { local v=in; inner; }
v=out; outer; echo "after $v"`
	got, _ := run(t, src, nil)
	want := "inner sees in\nafter out\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLocalRestoresAbsenceNotEmptiness(t *testing.T) {
	// The outer variable did not exist, so it must not exist afterwards.
	// Restoring "" would leave a set-but-empty variable, which `${v-unset}`
	// can tell apart and a careless implementation cannot.
	const src = `f() { local v=in; }; f; echo "[${v-unset}]"`
	if got, _ := run(t, src, nil); got != "[unset]\n" {
		t.Errorf("got %q, want %q", got, "[unset]\n")
	}
}

func TestLocalOutsideAFunctionFails(t *testing.T) {
	_, st := run(t, `local v=1`, nil)
	if st == 0 {
		t.Error("local outside a function reported success")
	}
}

func TestCaseContinueKeepsTestingLaterPatterns(t *testing.T) {
	// `;;&` re-tests; `;&` falls through without testing. The difference is
	// the whole reason they are separate operators, so both are asserted.
	// `;;&` is not core grammar, so the flag is enabled by name.
	got, _ := runGrammar(t, `case ab in a*) echo one;;& *b) echo two;;& zz) echo three;; esac`,
		func(d *syntax.Dialect) { d.CaseContinue = true }, nil)
	if want := "one\ntwo\n"; got != want {
		t.Errorf(";;&: got %q, want %q", got, want)
	}
	got, _ = run(t, `case ab in a*) echo one;& zz) echo two;; esac`, nil)
	if want := "one\ntwo\n"; got != want {
		t.Errorf(";&: got %q, want %q", got, want)
	}
}

func TestNoclobberRefusesAndPipeOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("pre\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The refusal must stop the command. Letting it run would send the
	// output to the inherited stream — the loudest possible wrong answer,
	// and the one this originally had.
	out, st := run(t, `set -C; echo two > `+path, nil)
	if st != 1 {
		t.Errorf("noclobber: status = %d, want 1", st)
	}
	if strings.Contains(out, "two") {
		t.Errorf("the command ran anyway: %q", out)
	}
	if b, _ := os.ReadFile(path); string(b) != "pre\n" {
		t.Errorf("file was overwritten: %q", b)
	}
	if _, st := run(t, `set -C; echo two >| `+path, nil); st != 0 {
		t.Errorf(">| override: status = %d, want 0", st)
	}
	if b, _ := os.ReadFile(path); string(b) != "two\n" {
		t.Errorf(">| did not write: %q", b)
	}
}

func TestSetCDoesNotClobberPositionalParameters(t *testing.T) {
	// `set -C` sets an option; only `--` or operands replace the parameters.
	if got, _ := run(t, `set -- a b c; set -C; echo $#`, nil); got != "3\n" {
		t.Errorf("got %q, want %q", got, "3\n")
	}
}

func TestIndirectionReadsTheNamedVariable(t *testing.T) {
	// `${!x}` is grammar the core does not have, and what it means is the
	// IndirectionYieldsName axis — both named here.
	indirect := func(d *syntax.Dialect) { d.ParamIndirection = true }
	sem := permissive()
	sem.IndirectionYieldsName = No
	if got, _ := runGrammar(t, `x=y; y=V; printf "[%s]" "${!x}"`, indirect, withSem(sem)); got != "[V]" {
		t.Errorf("got %q, want %q", got, "[V]")
	}
	// An unset middle name yields empty rather than the name itself, which
	// is the reading the other answer gives and this one must not.
	if got, _ := runGrammar(t, `x=nope; printf "[%s]" "${!x}"`, indirect, withSem(sem)); got != "[]" {
		t.Errorf("unset target: got %q, want %q", got, "[]")
	}
}

func TestCaseChangeOperators(t *testing.T) {
	got, _ := runGrammar(t, `x=aBc; printf "[%s]" "${x^^}" "${x,,}"`,
		func(d *syntax.Dialect) { d.ParamCaseChange = true }, nil)
	if got != "[ABC][abc]" {
		t.Errorf("got %q, want %q", got, "[ABC][abc]")
	}
}

func TestArithmeticErrorAbandonsTheScript(t *testing.T) {
	// Fatality is not an axis: every measured vector stops. Only the status
	// differs, and that is the FatalErrorStatusIsOne axis asserted in
	// eval_test.go — so both of its answers are exercised here.
	one := permissive()
	one.FatalErrorStatusIsOne = Yes
	for _, sem := range []Semantics{PosixSemantics(), permissive(), one} {
		out, _ := run(t, `echo $((1/0)); echo reached`, withSem(sem))
		if strings.Contains(out, "reached") {
			t.Errorf("the script continued past a fatal expansion: %q", out)
		}
	}
}

func TestFailedRedirectSkipsTheCommand(t *testing.T) {
	// Not noclobber-specific: any open that fails must stop the command.
	out, st := run(t, `echo hi > /nope/nowhere/x`, nil)
	if st == 0 {
		t.Error("a failed redirect reported success")
	}
	if strings.Contains(out, "hi") {
		t.Errorf("the command ran anyway: %q", out)
	}
}

func TestBackticksNestWithEscaping(t *testing.T) {
	// The older form nests only through backslash escaping, which is the
	// reason $( ) exists. The escape removal happens in the lexer, so the
	// inner substitution is found by re-lexing rather than by a special case.
	src := "echo \"[`echo \\`echo deep\\``]\""
	if got, _ := run(t, src, nil); got != "[deep]\n" {
		t.Errorf("got %q, want %q", got, "[deep]\n")
	}
	// A backslash before anything else keeps its literal meaning inside
	// backquotes, which is what stops the removal from being a blanket one.
	if got, _ := run(t, "echo \"[`printf '%s' 'a\\tb'`]\"", nil); got != "[a\\tb]\n" {
		t.Errorf("literal backslash: got %q", got)
	}
}
