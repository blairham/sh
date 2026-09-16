// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// OptionsGoBackAtTheReturnOfAKeywordFunction: the option table as a third
// thing a `function name { … }` call scopes, beside its locals and its trap
// table (#3308).
//
// Every row runs the same body twice — once as `function g { … }` and once as
// `g() { … }` — because the pairing is the whole of the answer. A row with
// only the keyword form in it passes in any shell that scopes both forms, and
// there is one.
//
// The rows that matter most are the ones where the body **reads** an option
// rather than writing one. A probe that only writes and then checks the
// caller cannot tell a table that was *restored* from one that was *reset*:
// both leave `function g { set -o noglob; }; g` reporting the option off
// afterwards. What parts them is what the body saw on the way in.

func keywordOptionsSem() Semantics {
	s := permissive()
	s.FunctionLocalOptions = OptionsGoBackAtTheReturnOfAKeywordFunction
	// The letter row below asks for `set -f`, which is an axis of its own
	// and unanswered in the preset. Every shell in the panel that has the
	// letter at all reads it this way; see Semantics.SetFTurnsOffGlobbing.
	s.SetFTurnsOffGlobbing = Yes
	return s
}

func keywordOptionsRun(t *testing.T, src string) (string, int) {
	t.Helper()
	s := keywordOptionsSem()
	return run(t, src, withSem(s))
}

// reportNoglob writes whether `set -f` is on, through the one reading every
// panel shell that has the letter agrees about.
const reportNoglob = `case $- in (*f*) echo on;; (*) echo off;; esac`

// The pairing, on the option the letter and the name both reach.
func TestKeywordFunctionOptionsAreScopedAndPosixOnesAreNot(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { set -o noglob; }`, "off\n"},
		{"the POSIX form", `g() { set -o noglob; }`, "on\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := keywordOptionsRun(t, tc.def+`; g; `+reportNoglob)
			if out != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q", out, st, tc.want)
			}
		})
	}
}

// The letter spelling is the same state and the same scope. Two spellings of
// one question are not allowed to answer differently.
func TestKeywordFunctionOptionsScopeTheLetterSpelling(t *testing.T) {
	out, _ := keywordOptionsRun(t, `function g { set -f; }; g; `+reportNoglob)
	if out != "off\n" {
		t.Errorf("got %q, want the letter scoped too", out)
	}
}

// **Restored, not reset**, in the direction a default that points the other
// way cannot explain away: the caller turned an option *on* that starts off,
// and the body reads it on.
//
// This is the row the trap answer beside it fails. A `function` call is
// handed an *empty* trap table; it is handed the caller's *live* options.
func TestKeywordFunctionSeesTheCallersOptions(t *testing.T) {
	out, _ := keywordOptionsRun(t, `set -o noglob; function g { `+reportNoglob+`; }; g`)
	if out != "on\n" {
		t.Errorf("got %q, want the body to see the caller's table; a reset would say off", out)
	}
}

// And the other direction, over an option that starts **on**, so that neither
// reading can be an accident of which way a default points. A table emptied
// or reset on the way in answers `on` here.
func TestKeywordFunctionSeesACallersOptionTurnedOff(t *testing.T) {
	const src = `set +o braceexpand; function g { echo {a,b}; }; g; echo {a,b}`
	out, _ := run(t, src, func(r *Runner) {
		s := keywordOptionsSem()
		r.Semantics = &s
		r.AddSetOptions("braceexpand")
	})
	if out != "{a,b}\n{a,b}\n" {
		t.Errorf("got %q, want the body to expand nothing either; a reset would expand", out)
	}
}

// The return is a restore and not a reset either, which is the row the panel
// was filed with: the caller had it on, the body turned it off, and it is on
// again afterwards.
func TestKeywordFunctionPutsBackWhatTheCallerHad(t *testing.T) {
	out, _ := keywordOptionsRun(t, `set -o noglob; function g { set +o noglob; }; g; `+reportNoglob)
	if out != "on\n" {
		t.Errorf("got %q, want the caller's state back; a reset to defaults would say off", out)
	}
}

// And an option the body raised that the caller never had goes back off, which
// is the same restore read from the other end.
func TestKeywordFunctionClearsWhatItRaised(t *testing.T) {
	out, _ := keywordOptionsRun(t, `function g { set -o noglob; }; g; `+reportNoglob)
	if out != "off\n" {
		t.Errorf("got %q, want the option cleared at the return", out)
	}
}

// It is the whole table rather than the one field the first row happens to
// move: three more options, each moved by the body and each back at the
// return, and one of them is a name this shell records rather than acts on.
func TestKeywordFunctionRestoresTheWholeTable(t *testing.T) {
	const src = `set -o noclobber; set -o allexport; set -o notify
function g { set +o noclobber; set +o allexport; set +o notify; }
g
set -o | grep -E '^(noclobber|allexport|notify)'`
	out, _ := keywordOptionsRun(t, src)
	const want = "allexport      on\nnoclobber      on\nnotify         on\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// `set -o errexit` is the same scope, and the row says so where a script can
// feel it: the caller carries on past a failure the body asked it to die on.
func TestKeywordFunctionScopesErrexit(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { set -o errexit; }`, "alive\n"},
		{"the POSIX form", `g() { set -o errexit; }`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := keywordOptionsRun(t, tc.def+`; g; false; echo alive`)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And `set -u`, which is the same scope reaching a *reader* rather than a
// status: the caller expands an unset name and is not refused.
func TestKeywordFunctionScopesNounset(t *testing.T) {
	out, st := keywordOptionsRun(t, `function g { set -u; }; g; echo "[$nope]"`)
	if out != "[]\n" || st != 0 {
		t.Errorf("got %q/%d, want the caller's nounset back off", out, st)
	}
}

// The boundary is the `function` word and not the call. A keyword function
// called from inside another has a table of its own, so what it moved is back
// while the outer one is still running.
func TestKeywordFunctionNestsAScopeOfItsOwn(t *testing.T) {
	const src = `function o { set -o noglob; function i { set +o noglob; }; i; ` + reportNoglob + `; }
o
` + reportNoglob
	out, _ := keywordOptionsRun(t, src)
	if out != "on\noff\n" {
		t.Errorf("got %q, want the inner call's change undone inside the outer one", out)
	}
}

// A POSIX-form function called from inside a keyword one is *not* a boundary,
// which is the other half of the same rule: what it moves stands until the
// enclosing `function` returns.
func TestPosixFunctionInsideAKeywordOneMovesTheCallersTable(t *testing.T) {
	const src = `function o { p() { set -o noglob; }; p; ` + reportNoglob + `; }
o
` + reportNoglob
	out, _ := keywordOptionsRun(t, src)
	if out != "on\noff\n" {
		t.Errorf("got %q, want the inner POSIX call to move the enclosing table", out)
	}
}

// An early `return` is still a return, so the table goes back there too.
func TestKeywordFunctionRestoresOptionsOnAnEarlyReturn(t *testing.T) {
	out, st := keywordOptionsRun(t, `function g { set -o noglob; return 3; }; g; echo "st=$?"; `+reportNoglob)
	if out != "st=3\noff\n" || st != 0 {
		t.Errorf("got %q/%d, want the table back after an early return", out, st)
	}
}

// And the other answer leaves both forms alone, which is what makes this a
// value of its own rather than a reading of the default.
func TestOptionsSurviveTheFunctionScopesNeitherForm(t *testing.T) {
	for _, def := range []string{`function g { set -o noglob; }`, `g() { set -o noglob; }`} {
		s := permissive()
		s.FunctionLocalOptions = OptionsSurviveTheFunction
		out, _ := run(t, def+`; g; `+reportNoglob, withSem(s))
		if out != "on\n" {
			t.Errorf("%s: got %q, want the option left standing", def, out)
		}
	}
}
