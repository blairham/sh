// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `[[ -o name ]]` reads this dialect's own option namespace rather than its
// `set -o` names, which is what the substrate's seam exists for.
//
// The three spellings are one question here and three unknown names anywhere
// else: case folded, underscores dropped, and a single `no` prefix negating
// what follows. Measured on zsh 5.9.2 — `[[ -o Err_Exit ]]` is the state of
// errexit, and `[[ -o noerrexit ]]` is its opposite.
//
// Both directions of the switch on the same run, because a namespace that
// answered the base name correctly and ignored the prefix would pass a test
// that only asked the true side.
func TestTheOptionTestReadsTheZshNamespace(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"the canonical spelling",
			`set -e; [[ -o errexit ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			"capitals and an underscore",
			`set -e; [[ -o Err_Exit ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			// Written through `if`, because the answer is a false and this
			// shell is holding errexit: a bare `[[ ]]` that answers false
			// under `set -e` ends the script, here and in zsh alike, so the
			// `echo` would never run to report it.
			"the `no` prefix, with the option on",
			`set -e; if [[ -o noerrexit ]]; then echo "st=on"; else echo "st=$?"; fi`,
			"st=1\n",
		},
		{
			"the `no` prefix, with the option off",
			`set +e; [[ -o no_err_exit ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			"a base name whose own spelling starts with no",
			`[[ -o nomatch ]]; echo "st=$?"`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The names three third-party files on this machine reach for, each in the
// spelling that file writes.
//
// These are the rows the change was made for, and each is a whole real line
// rather than a reduction of one. Every state below matches what real zsh
// answers for a shell in this state, which is what keeps the files taking the
// same branch here as they do there — a name resolved to the *wrong* state
// would parse fine and send the file down the other path in silence.
func TestTheOptionTestAnswersTheNamesRealStartupFilesAsk(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The prompt theme, at its seventeenth line. Quoted, negated,
			// and `aliases` is on in zsh and on here.
			"a prompt theme's quoted, negated name",
			`[[ ! -o 'aliases' ]]; echo "st=$?"`,
			"st=1\n",
		},
		{
			"and the two beside it, in the same file's spellings",
			`[[ ! -o 'sh_glob' ]]; echo "shglob=$?"; [[ ! -o 'no_brace_expand' ]]; echo "brace=$?"`,
			"shglob=0\nbrace=0\n",
		},
		{
			// The terminal integration, at its fifteenth line. This runner
			// is not interactive, so the answer is a false — and it is a
			// false rather than a complaint, which is the whole point of the
			// name being in the table.
			"a terminal integration's interactive test",
			`[[ -o interactive ]]; echo "st=$?"`,
			"st=1\n",
		},
		{
			// The plugin loader, at its forty-fifth line: the option test in
			// a compound condition, which neither of the other two writes.
			// `functionargzero` is on in zsh and on here, so the negation is
			// a false and the `||` walks on to the comparison.
			"a plugin loader's compound condition",
			`[[ ! -o functionargzero || x != */* ]]; echo "st=$?"`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The names the namespace claims are claimed honestly: `$0` inside a function
// really is the function's name here, so `functionargzero` reads on.
//
// The guard against filling the table with names answered by whatever value
// made a startup file take the branch we wanted. Every entry added for this
// change is checked against the behavior it names, from the other side.
func TestTheNamesTheNamespaceClaimsAreTrue(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `f() { echo "zero=$0"; }; f; [[ -o functionargzero ]]; echo "opt=$?"`)
	if want := "zero=f\nopt=0\n"; out != want {
		t.Errorf("got %q, want %q — the option and the behavior it names must agree", out, want)
	}
}

// A name outside the namespace: the complaint, the status that is neither of
// a condition's two, and the next command still reached.
//
// The same words `setopt` uses for the same mistake, said at the condition's
// own location rather than a builtin's — which is the half a shared wording
// constant cannot get right on its own.
func TestAnUnknownConditionOptionIsSaidAndIsNotFatal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `[[ -o nosuchoption ]]; echo "st=$?"; echo alive`)
	if want := "zsh:1: no such option: nosuchoption\nst=3\nalive\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0 — the script carries on to the last echo", st)
	}
}

// And it is a third value rather than a false, which negation is the sharpest
// way to show: `!` leaves it at 3 where it would turn a real false into a 0.
//
// Measured across the truth table on zsh 5.9.2. The `||` row is the other
// edge — it walks on past the status to a right-hand side that answers for the
// whole condition — and together they are what a "return false and paint the
// status on" implementation cannot produce.
func TestAnUnknownConditionOptionIsNotAFalseHere(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"negation leaves it alone",
			`[[ ! -o nosuchoption ]]; echo "st=$?"`,
			"zsh:1: no such option: nosuchoption\nst=3\n",
		},
		{
			"`||` walks on past it",
			`[[ -o nosuchoption || 1 == 1 ]]; echo "st=$?"`,
			"zsh:1: no such option: nosuchoption\nst=0\n",
		},
		{
			"`&&` stops on it",
			`[[ 1 == 1 && -o nosuchoption ]]; echo "st=$?"`,
			"zsh:1: no such option: nosuchoption\nst=3\n",
		},
		{
			"and an operand never reached is never complained about",
			`[[ 1 == 1 || -o nosuchoption ]]; echo "st=$?"`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `[[ -o ]]` and `setopt` reach one namespace, so a name is the same name
// through either — read through the condition and written through the builtin.
//
// The seam is what makes that true rather than a coincidence, and this is the
// row that would fail if the condition grew a table of its own.
func TestTheConditionAndSetoptShareOneNamespace(t *testing.T) {
	const src = `[[ -o glob ]]; echo "before=$?"; setopt no_glob; [[ -o glob ]]; echo "after=$?"`
	if out, _ := runZsh(t, t.TempDir(), src); out != "before=0\nafter=1\n" {
		t.Errorf("got %q, want %q", out, "before=0\nafter=1\n")
	}
}
