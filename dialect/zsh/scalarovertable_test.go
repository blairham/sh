// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

const scalarOverTableRefusal = "attempt to set associative array to scalar"

// A scalar store over a name holding a **table** is refused under
// `setopt ksharrays`, and the refusal ends the shell.
//
// Measured 2026-09-26 from a script file under `zsh -f f.sh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), every row with `setopt ksharrays` in front of
// it:
//
//	typeset -A h=(one 1); h=string    f.sh:1: h: attempt to set associative
//	                                  array to scalar, status 1, nothing
//	                                  after it
//	typeset -A h=(one 1); h+=string   the same, so the operator does not
//	                                  decide
//	typeset -A h=(one 1 two 2); …     the same, so it is not an emptiness
//	typeset -A h; h=string            the same, so an empty table is a table
//
// bash 5.3.20 and ksh93u+ 2012-08-01 write the key `0` and keep the table, and
// so does this shell with the option off — where the name becomes a plain
// scalar instead. That is what makes this an axis of its own rather than a
// value of ScalarAssignedOverACompoundReplacesTheName (#4617).
func TestAScalarStoredOverATableIsRefusedUnderKshArrays(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a plain assignment", "typeset -A h=(one 1)\nh=string\nprint after"},
		{"an append", "typeset -A h=(one 1)\nh+=string\nprint after"},
		{"a table of two", "typeset -A h=(one 1 two 2)\nh=string\nprint after"},
		{"an empty table", "typeset -A h\nh=string\nprint after"},
		{"an empty table appended to", "typeset -A h\nh+=string\nprint after"},
		{"a value the base key already has", "typeset -A h=(0 pre)\nh+=x\nprint after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, "setopt ksharrays\n"+tc.src)
			if !strings.Contains(out, "h: "+scalarOverTableRefusal) {
				t.Errorf("= %q, want the refusal, with the name in it", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("= %q, want the shell left before the next command", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// And with the option **off** the same six lines are the replacing answer:
// the name becomes a plain scalar and the table is gone. This is the pair that
// says the option is what reaches the refusal, and it is also the row
// `A06assign.ztst` stops on — its *add scalar to association* chunk is
// `typeset -A hash; hash=(one 1); hash+=string; [[ $hash[@] == string ]]`,
// which is status 0 there because the table really has become the scalar.
//
// The append joins **nothing**, which is the half that is not the plain
// store's: `typeset -A h=(0 pre); h+=x` is `typeset h=x` and not `prex`, where
// a bare `$h` reads `pre`. A replacement that had gone through the string
// append would have written `prex`.
func TestWithoutKshArraysTheSameStoreReplacesTheName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a plain assignment", "typeset -A h=(one 1)\nh=string\ntypeset -p h", "typeset h=string"},
		{"an append", "typeset -A h=(one 1)\nh+=string\ntypeset -p h", "typeset h=string"},
		{"a table of two", "typeset -A h=(one 1 two 2)\nh=string\ntypeset -p h", "typeset h=string"},
		{"an empty table", "typeset -A h\nh=string\ntypeset -p h", "typeset h=string"},
		{"an empty table appended to", "typeset -A h\nh+=string\ntypeset -p h", "typeset h=string"},
		{"the append joins nothing", "typeset -A h=(0 pre)\nh+=x\ntypeset -p h", "typeset h=x"},
		{
			"the suite's own chunk",
			"typeset -A hash\nhash=(one 1)\nhash+=string\n[[ $hash[@] == string ]]\nprint \"st=$?\"",
			"st=0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// **The noun is the name holding a table**, and these hold the option on and
// move one other thing. Each is taken, and each is a line a rule written on a
// wider noun refuses.
//
// The ordinary **array** is the pair that makes this an axis of its own and
// it has a test to itself below, because only half of its row is ours yet.
// The rest here say it is the bare name's kind and not the declaration, not
// the operator and not the parentheses.
func TestOnlyABareScalarStoreOverATableIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an ordinary array appended to", "typeset -a a=(x y)\na+=string\ntypeset -p a", "typeset -a a=( xstring y )"},
		{"a subscript", "typeset -A h=(one 1)\nh[one]=Q\ntypeset -p h", "typeset -A h=( [one]=Q )"},
		{"a name no longer holding one", "typeset -A h=(one 1)\nunset h\nh=string\ntypeset -p h", "typeset h=string"},
		{"a name that never held one", "s=plain\ns=string\ntypeset -p s", "typeset s=string"},
		{"a scalar appended to", "s=plain\ns+=more\ntypeset -p s", "typeset s=plainmore"},
		{"an emptying store", "typeset -A h=(one 1)\nh=()\ntypeset -p h", "typeset -A h=( )"},
		{"a literal append", "typeset -A h=(one 1)\nh+=()\ntypeset -p h", "typeset -A h=( [one]=1 )"},
		{"a keyed literal", "typeset -A h=(one 1)\nh=(a 1 b 2)\ntypeset -p h", "typeset -A h=( [a]=1 [b]=2 )"},
		{"a keyed literal appended", "typeset -A h=(one 1)\nh+=(b 2)\ntypeset -p h", "typeset -A h=( [b]=2 [one]=1 )"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, "setopt ksharrays\n"+tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// **An ordinary array under the same option is not refused**, which is the
// pair that makes this an axis of its own rather than a third value on
// ScalarAssignedOverACompoundReplacesTheName: a single three-valued field
// would have to refuse for both kinds of compound. "A compound name" and "a
// table name" agree on every other row in the panel, and they part exactly
// here.
//
// What is asserted is that the array store is **taken** — status 0 and no
// refusal — rather than the whole listing, because half that listing is a
// different issue's. Measured, `setopt ksharrays; typeset -a a=(x y);
// a=string` is `typeset -a a=( string y )` in the reference and
// `typeset a=string` here: the base is written there and the name replaced
// here, which is ScalarAssignedOverACompoundReplacesTheName not yet moving
// with the option (#4618). That row is deliberately left standing — flipping
// it before this axis existed would have made a *table* write an invented key
// `0` where the reference refuses — and it is unblocked by this file: once the
// refusal is asked first, a table never reaches the base-writing reading. The
// append half of the same row moved already (#4619) and is asserted whole
// above.
func TestAnOrdinaryArrayUnderTheOptionIsNotRefused(t *testing.T) {
	for _, src := range []string{
		"typeset -a a=(x y)\na=string\nprint after",
		"typeset -a a=(x y)\na+=string\nprint after",
		"a=(x y)\na=string\nprint after",
	} {
		out, st := answersRun(t, "setopt ksharrays\n"+src)
		if strings.Contains(out, scalarOverTableRefusal) {
			t.Errorf("%q = %q, want an array store taken and not refused", src, out)
		}
		if !strings.Contains(out, "after") || st != 0 {
			t.Errorf("%q = %q status %d, want the next command run at 0", src, out, st)
		}
	}
}

// It is asked wherever a scalar is **stored** and not only at an assignment
// statement, which is the reach ScalarAssignedOverACompoundReplacesTheName
// already has and the reason both live at the store rather than at the
// statement. Measured the same day under the option: each of these earns the
// sentence and the shell leaves.
//
// The **location** is the store's and not the builtin's, which `read` and
// `printf` are what show: this shell writes `f.sh:1:` here and not
// `f.sh:read:1:`, exactly as it does for its own `read-only variable` refusal
// from inside the same builtins.
func TestEveryScalarStoreOverATableIsRefusedHere(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a for loop's variable", "typeset -A h=(one 1)\nfor h in x y; do :; done\nprint after"},
		{"read", "typeset -A h=(one 1)\nprint x | read h\nprint after"},
		{"printf -v", "typeset -A h=(one 1)\nprintf -v h x\nprint after"},
		{"an assigning expansion", "typeset -A h=(one 1)\n: ${h::=x}\nprint after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, "setopt ksharrays\n"+tc.src)
			if !strings.Contains(out, "h: "+scalarOverTableRefusal) {
				t.Errorf("= %q, want the refusal", out)
			}
			if strings.Contains(out, ":read:") || strings.Contains(out, ":printf:") {
				t.Errorf("= %q, want the store's location and not the builtin's", out)
			}
			if strings.Contains(out, "after") || st != 1 {
				t.Errorf("= %q status %d, want the shell left at 1", out, st)
			}
		})
	}
}

// What the refusal reaches, measured against the odd-pair refusal's grid row
// for row — see Semantics.BareElementsInATableLiteralMustPairOff, which is
// the same shape. `||` does **not** catch it, an `always` block still runs,
// `eval` and a subshell contain it, and a function's name goes in front of
// the sentence where the file and line otherwise do.
//
// And in every contained shape the **table is left standing**, which is the
// claim about order rather than about a restore: a store that wrote the key
// first and complained second would list the same table with a `0` in it.
func TestTheRefusalsReachHere(t *testing.T) {
	t.Run("or-else does not catch it", func(t *testing.T) {
		out, st := answersRun(t, "setopt ksharrays\ntypeset -A h=(one 1)\nh=string || print caught\nprint after")
		if strings.Contains(out, "caught") || strings.Contains(out, "after") || st != 1 {
			t.Errorf("= %q status %d, want the shell left at 1 with nothing catching it", out, st)
		}
	})
	t.Run("an always block still runs", func(t *testing.T) {
		out, st := answersRun(t, "setopt ksharrays\ntypeset -A h=(one 1)\n{ h=string } always { print always }\nprint after")
		if !strings.Contains(out, "always") {
			t.Errorf("= %q, want the always block run", out)
		}
		if strings.Contains(out, "after") || st != 1 {
			t.Errorf("= %q status %d, want the shell left at 1", out, st)
		}
	})
	t.Run("a function's name goes in front", func(t *testing.T) {
		out, st := answersRun(t, "setopt ksharrays\nf() { typeset -A h=(one 1); h=string }\nf\nprint after")
		if !strings.HasPrefix(out, "f: h: "+scalarOverTableRefusal) {
			t.Errorf("= %q, want the function's name in front of the refusal", out)
		}
		if strings.Contains(out, "after") || st != 1 {
			t.Errorf("= %q status %d, want the shell left at 1", out, st)
		}
	})
	for _, tc := range []struct{ name, wrap string }{
		{"eval contains it", "eval 'h=string'"},
		{"a subshell contains it", "(h=string)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t,
				"setopt ksharrays\ntypeset -A h=(one 1 two 2)\n"+tc.wrap+"\nprint after\ntypeset -p h")
			if !strings.Contains(out, "h: "+scalarOverTableRefusal) {
				t.Errorf("= %q, want the refusal", out)
			}
			if want := "after\ntypeset -A h=( [one]=1 [two]=2 )\n"; !strings.HasSuffix(out, want) || st != 0 {
				t.Errorf("= %q status %d, want it to end %q at 0", out, st, want)
			}
		})
	}
}

// Answered when the store runs, like every axis here: a function's
// `setopt localoptions ksharrays` reaches a store inside it and not one after
// it has returned.
func TestTheRefusalIsAnsweredWhenTheStoreRuns(t *testing.T) {
	out, st := answersRun(t, "typeset -A h=(one 1)\nf() { setopt localoptions ksharrays; h=string }\nf\nprint after")
	if !strings.HasPrefix(out, "f: h: "+scalarOverTableRefusal) || strings.Contains(out, "after") || st != 1 {
		t.Errorf("inside = %q status %d, want the refusal at 1", out, st)
	}
	out, st = answersRun(t, "typeset -A h=(one 1)\nf() { setopt localoptions ksharrays; }\nf\nh=string\ntypeset -p h")
	if want := "typeset h=string"; strings.TrimSpace(out) != want || st != 0 {
		t.Errorf("after it returns = %q status %d, want %q at 0", out, st, want)
	}
}

// And `emulate ksh` and `emulate sh` carry it, which is how a script reaches
// the option without naming it.
func TestTheEmulationsCarryTheRefusal(t *testing.T) {
	for _, em := range []string{"emulate ksh", "emulate sh"} {
		out, st := answersRun(t, em+"\ntypeset -A h=(one 1)\nh=string\nprint after")
		if !strings.Contains(out, "h: "+scalarOverTableRefusal) || strings.Contains(out, "after") || st != 1 {
			t.Errorf("%s: = %q status %d, want the refusal at 1", em, out, st)
		}
	}
}

// The answer and the wording, pinned so that no preset here drifts off the
// column they were measured from. The option-off answer is the dialect's
// default and setKshArrays is what moves it — see ksharrays_test.go.
func TestTheScalarOverATableAnswerIsThisDialects(t *testing.T) {
	if got := zsh.Semantics().ScalarStoredOverATableIsRefused; got != interp.No {
		t.Errorf("ScalarStoredOverATableIsRefused = %v, want No", got)
	}
	if got, want := zsh.Diagnostics().ScalarStoredOverATable,
		"%s: attempt to set associative array to scalar"; got != want {
		t.Errorf("ScalarStoredOverATable = %q, want %q", got, want)
	}
}
