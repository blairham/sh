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
	return run(t, src, func(r *Runner) {
		withSem(s)(r)
		// `emacs` is a declared name since #3366 rather than one of the
		// substrate's own: BusyBox ash has no such option, so a shell that
		// wants the row says so.
		r.AddSetOptions("emacs")
	})
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
//
// Three readings in one row, because the option is one this shell keeps as a
// *negative* field and reaches through a name a dialect declares rather than
// through the common table: the body finds it off as the caller left it, can
// still move it, and the caller has it off again afterwards. The last of
// those is what says the walk is the whole roster and not the names the
// substrate happens to own.
func TestKeywordFunctionSeesACallersOptionTurnedOff(t *testing.T) {
	const src = `set +o braceexpand
function g { echo {a,b}; set -o braceexpand; echo {a,b}; }
g
echo {a,b}`
	out, _ := run(t, src, func(r *Runner) {
		s := keywordOptionsSem()
		// The preset has no braces at all, which is the standard's answer
		// and would make every line of this row read `{a,b}` — a pass for
		// the wrong reason. The option is the switch beside that axis, not
		// a substitute for it.
		s.BraceExpansion = Yes
		r.Semantics = &s
		r.AddSetOptions("braceexpand")
	})
	if out != "{a,b}\na b\n{a,b}\n" {
		t.Errorf("got %q, want the body to find it off, move it, and give it back", out)
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

// `pipefail` is the one name whose *existence* is an axis rather than a table
// entry, so its row carries a read and no write and the restore reaches the
// state directly. Without that, the option is saved and never put back — and
// nothing else here notices, because every other name this walk touches has
// a writer in the table.
func TestKeywordFunctionRestoresPipefail(t *testing.T) {
	const src = `set -o pipefail; function g { set +o pipefail; }; g; false | true; echo "st=$?"`
	s := keywordOptionsSem()
	s.PipefailOption = Yes
	out, _ := run(t, src, func(r *Runner) {
		r.Semantics = &s
		r.AddSetOptions("pipefail")
	})
	if out != "st=1\n" {
		t.Errorf("got %q, want the caller's pipefail back", out)
	}
}

// The two editing modes are two names over one state, which is the pair a
// restore is most likely to get wrong. It does not: turning a mode off moves
// nothing unless that mode is the selected one, so the walk's order does not
// decide the answer. Both directions, because a pair that works one way round
// and not the other is the failure this row is for.
func TestKeywordFunctionRestoresTheEditingMode(t *testing.T) {
	for _, tc := range []struct{ name, outer, inner, want string }{
		{"emacs displaced by vi", "emacs", "vi", "emacs          on\nvi             off\n"},
		{"vi displaced by emacs", "vi", "emacs", "emacs          off\nvi             on\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `set -o ` + tc.outer + `; function g { set -o ` + tc.inner + `; }; g
set -o | grep -E '^(emacs|vi) '`
			out, _ := keywordOptionsRun(t, src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// KeywordFunctionSuspendsErrexitAndXtrace: two options a keyword-defined body
// is not handed at all, which is the half of the same word's option scoping
// that a restore cannot say (#3860).
//
// Every row here runs with the axis on and again with it off, and the pairing
// is what makes each one a control on the other: the "off" column is what
// every other shell in the panel does, so a row that answered the same either
// way would be measuring the construct and not the axis.
func suspendingSem() Semantics {
	s := keywordOptionsSem()
	s.KeywordFunctionSuspendsErrexitAndXtrace = Yes
	return s
}

func suspendRun(t *testing.T, on bool, src string) (string, int) {
	t.Helper()
	s := keywordOptionsSem()
	if on {
		s = suspendingSem()
	}
	return run(t, src, func(r *Runner) {
		withSem(s)(r)
		r.AddSetOptions("emacs")
	})
}

// `-e` is the row that decides whether a script survives, so it is first.
func TestAKeywordFunctionBodyRunsWithoutErrexit(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		on, off     string
		onSt, offSt int
	}{
		{
			// The body's failure is not fatal there and the script runs on.
			name: "the keyword form",
			src:  "set -e\nfunction f { false; echo after; }\nf\necho tail\n",
			on:   "after\ntail\n", onSt: 0,
			off: "", offSt: 1,
		},
		{
			// **The control**: the same body written the other way is the
			// ordinary `-e`, with the axis on as well as off. It is what
			// keys the rule to the definition form rather than to the call.
			name: "the POSIX form",
			src:  "set -e\ng() { false; echo after; }\ng\necho tail\n",
			on:   "", onSt: 1,
			off: "", offSt: 1,
		},
		{
			// And it comes back: the caller is running under `-e` again the
			// moment the call returns, which is the restore beside this
			// doing its half.
			name: "the caller has it back",
			src:  "set -e\nfunction f { false; echo after; }\nf\nfalse\necho unreached\n",
			on:   "after\n", onSt: 1,
			off: "", offSt: 1,
		},
		{
			// A name the body declares is untouched by any of it — the row
			// that says what moved is two options and not the body's state.
			name: "the body still runs",
			src:  "set -e\nfunction f { v=set; echo \"[$v]\"; }\nf\necho tail\n",
			on:   "[set]\ntail\n", onSt: 0,
			off: "[set]\ntail\n", offSt: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := suspendRun(t, true, tc.src)
			if out != tc.on || st != tc.onSt {
				t.Errorf("with the axis on: %q status %d, want %q status %d", out, st, tc.on, tc.onSt)
			}
			out, st = suspendRun(t, false, tc.src)
			if out != tc.off || st != tc.offSt {
				t.Errorf("with it off: %q status %d, want %q status %d", out, st, tc.off, tc.offSt)
			}
		})
	}
}

// `-x` is the second of the two, and the one the shell with this rule gives
// back through a mark on the name instead.
func TestAKeywordFunctionBodyRunsWithoutXtrace(t *testing.T) {
	for _, tc := range []struct{ name, src, on, off string }{
		{
			name: "the keyword form",
			src:  "set -x\nfunction f { echo A; }\nf\n",
			on:   "+ f\nA\n",
			off:  "+ f\n+ echo A\nA\n",
		},
		{
			// **The control**, again the definition form.
			name: "the POSIX form",
			src:  "set -x\ng() { echo A; }\ng\n",
			on:   "+ g\n+ echo A\nA\n",
			off:  "+ g\n+ echo A\nA\n",
		},
		{
			// What the body itself asks for is its own, and it does not
			// leak out — the restore's half again, on the other option.
			name: "the body turns it on",
			src:  "function f { set -x; echo A; }\nf\necho tail\n",
			on:   "+ echo A\nA\ntail\n",
			off:  "+ echo A\nA\ntail\n",
		},
		{
			// A function the body calls is not traced either, however that
			// one was written: what was suspended is the option, for
			// everything the call reaches.
			name: "a POSIX function the body calls",
			src:  "set -x\ng() { echo P; }\nfunction f { g; }\nf\n",
			on:   "+ f\nP\n",
			off:  "+ f\n+ g\n+ echo P\nP\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := suspendRun(t, true, tc.src)
			if out != tc.on {
				t.Errorf("with the axis on: %q, want %q", out, tc.on)
			}
			out, _ = suspendRun(t, false, tc.src)
			if out != tc.off {
				t.Errorf("with it off: %q, want %q", out, tc.off)
			}
		})
	}
}

// Two options and not the table: the third control, and the widest one.
//
// A suspension that took the whole option table — or one name too many —
// passes both suites above and fails here.
func TestAKeywordFunctionBodyKeepsEveryOtherOption(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// `-u` is on inside, so reading an unset name is still fatal.
			name: "nounset",
			src:  "set -u\nfunction f { echo \"[${nope}]\"; }\nf\necho tail\n",
			want: "sh: nope: parameter not set\n",
		},
		{
			// And `-f` is on inside, so a pattern is still a word.
			name: "noglob",
			src:  "set -f\nfunction f { echo *; }\nf\n",
			want: "*\n",
		},
		{
			// The pairing for `-u`, so the row above cannot pass for having
			// no name to read: with the option off the same body prints.
			name: "nounset off",
			src:  "function f { echo \"[${nope}]\"; }\nf\necho tail\n",
			want: "[]\ntail\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := suspendRun(t, true, tc.src)
			if out != tc.want {
				t.Errorf("%q, want %q", out, tc.want)
			}
		})
	}
}
