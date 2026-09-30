// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A narrowed `zmodload -F` brings the module's gated parameters **that it
// named**, and not the rest of them.
//
// The installer is keyed on the module — `registerSystemParameters` brings
// `sysparams` and `errnos` together and there is no spelling that brings one
// without the other — and the selection is per feature. That mismatch is the
// whole of the row: a narrowed load installed the module's entire roster, and
// `zmodload -lF` reported the unnamed ones off in the same breath.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — run `-f` from a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, one
// shell per row (#5043).
func TestANarrowedLoadBringsOnlyTheParametersItNamed(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		// **The sharpest row: the selection names a builtin and no parameter
		// at all**, and all three of the module's parameters arrived.
		{
			"a builtin-only selection brings no parameter",
			"zmodload -F zsh/datetime b:strftime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"0 0 0\n",
		},
		{
			"one parameter of three",
			"zmodload -F zsh/datetime p:EPOCHSECONDS\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"1 0 0\n",
		},
		{
			"one parameter of two",
			"zmodload -F zsh/system p:sysparams\nprint \"${+sysparams} ${+errnos}\"",
			"1 0\n",
		},
		{
			"a deselection after a whole load",
			"zmodload zsh/system\nzmodload -F zsh/system -p:errnos\n" +
				"print \"${+sysparams} ${+errnos}\"",
			"1 0\n",
		},
		{
			"a deselection of one of three",
			"zmodload zsh/datetime\nzmodload -F zsh/datetime -p:epochtime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"1 0 1\n",
		},
		// **The controls.** A whole load still brings everything, the name it
		// did select really answers rather than merely counting, and a module
		// whose gated roster is one name cannot show the difference either
		// way — which is why the rows above are all multi-name modules.
		{
			"a whole load brings them all",
			"zmodload zsh/datetime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"1 1 1\n",
		},
		{
			"the selected name answers",
			"zmodload -F zsh/datetime p:EPOCHSECONDS\nprint $(( EPOCHSECONDS > 0 ))",
			"1\n",
		},
		{
			"a one-name module",
			"zmodload -F zsh/langinfo p:langinfo\nprint \"${+langinfo}\"",
			"1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestTheSelectionMovesBackAndForthOverAGatedParameter is what says the fix
// is an agreement with the selection rather than a one-way withdrawal.
//
// **The widening row is the one that was broken in a second way.** A
// withdrawal recorded *in front of* the registration captured nothing and the
// installer then wrote real producers over a name the record still called
// withdrawn — so the name read as **taken**, and `zmodloadRestorable` refused
// the widening with `Can't add module parameter` at status 2. The reference
// widens in silence at 0. One root, two sentences.
func TestTheSelectionMovesBackAndForthOverAGatedParameter(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"a plain load widens a narrowed module",
			"zmodload -F zsh/datetime b:strftime\nzmodload zsh/datetime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"1 1 1\n",
		},
		{
			"a later +p: selects the one left out",
			"zmodload -F zsh/system p:sysparams\nzmodload -F zsh/system +p:errnos\n" +
				"print \"${+sysparams} ${+errnos}\"",
			"1 1\n",
		},
		{
			"an unload still releases the lot",
			"zmodload zsh/datetime\nzmodload -u zsh/datetime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime}\"",
			"0 0\n",
		},
		{
			"and a load after it brings them back",
			"zmodload zsh/datetime\nzmodload -u zsh/datetime\nzmodload zsh/datetime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			"1 1 1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q (status %d), want %q (nothing said)", out, st, tc.want)
			}
		})
	}
}

// And the refusal the widening row must not have swallowed: a name the script
// has since taken still cannot be restored, which is `zmodloadRestorable`'s
// own measured row and the control for the one above it.
//
// Measured in the same run: `errnos=(a b)` after a deselection, then
// `+p:errnos`, is two sentences at status 2 in zsh 5.9.2 and here alike.
func TestATakenNameStillRefusesToComeBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/system\nzmodload -F zsh/system -p:errnos\nerrnos=(a b)\n"+
			"zmodload -F zsh/system +p:errnos\nprint st=$?")
	const want = "zsh:4: Can't add module parameter `errnos': parameter already exists\n" +
		"zsh:zsh/system:4: error when adding parameter `errnos'\nst=2\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// A module load does not take a name the script already owns.
//
// Measured 2026-09-30 on zsh 5.9.2. The **value** is what these rows assert,
// not the withdrawn record: an earlier fix in this file corrected the record
// and left the installer writing a real producer straight over the name, so a
// row reading `${+name}` or the feature listing would have passed while the
// script's variable was gone.
//
//	zmodload -F zsh/datetime p:EPOCHSECONDS
//	f() { local EPOCHSECONDS=mine; zmodload zsh/datetime; print $EPOCHSECONDS }
//
//	zsh 5.9.2   mine
//	before      1790755518        the load wrote the clock over the local
//
// **The control is the same script with the load taken out**, and both shells
// print `mine` there — which is what makes this a statement about the load
// rather than about locals.
//
// Note the load is **not refused** in that row and neither shell says
// anything about it: the producer displaces a binding it should sit under.
func TestAModuleLoadDoesNotTakeANameTheScriptOwns(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control: no load, so nothing can displace anything.
			"the control, with no load at all",
			"zmodload -F zsh/datetime p:EPOCHSECONDS\n" +
				"f() { local EPOCHSECONDS=mine; print -r -- $EPOCHSECONDS }\nf",
			"mine\n",
		},
		{
			"a local, over a module narrowed to that name",
			"zmodload -F zsh/datetime p:EPOCHSECONDS\n" +
				"f() { local EPOCHSECONDS=mine; zmodload zsh/datetime; print -r -- $EPOCHSECONDS }\nf",
			"mine\n",
		},
		{
			"a global, over a first whole load",
			"EPOCHSECONDS=mine\nzmodload zsh/datetime\nprint -r -- $EPOCHSECONDS",
			"mine\n",
		},
		{
			// The array of the same module, so the rule is not about scalars.
			"an array name the script owns",
			"epochtime=mine\nzmodload zsh/datetime\nprint -r -- $epochtime",
			"mine\n",
		},
		{
			// **The siblings still arrive**, which is what says the load is
			// declined per name rather than skipped. Taking the installer
			// out altogether passes every row above and fails this one.
			"the module's other names are produced as usual",
			"f() { local EPOCHSECONDS=mine; zmodload zsh/datetime\n" +
				"print -r -- ${${(M)$(( ${epochtime[1]} > 0 )):#1}:-NO} }\nf",
			"1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("= %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
