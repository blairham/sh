// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `-F` selection withdraws a feature on the way **off**, not for being off.
//
// A name that is present before its module is loaded is that shell's
// *autoloadable stub* — the distinction #4997 turns on, seen from the
// selection rather than from startup. A selection that never had the feature
// on has not replaced the stub, so there is nothing for it to take.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — run `-f` from a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, one
// shell per row (#5045).
func TestASelectionWithdrawsOnlyOnTheWayOff(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		// **The rows that were wrong.** The module is never loaded whole, so
		// nothing the selection leaves out was ever on.
		{
			"a parameter the selection did not name",
			"zmodload -F zsh/parameter p:funcstack\nf(){ :; }\n" +
				"print \"${+functions} ${#functions} ${+parameters}\"",
			"1 1 1\n",
		},
		{
			"another module's, so it is not one roster",
			"zmodload -F zsh/zleparameter p:keymaps\nprint \"${+keymaps} ${+widgets}\"",
			"1 1\n",
		},
		{
			"a builtin the selection did not name",
			"zmodload -F zsh/zutil b:zstyle\n" +
				"whence -w zparseopts; whence -w zstyle; whence -w zformat",
			"zparseopts: builtin\nzstyle: builtin\nzformat: builtin\n",
		},
		{
			"and another module's builtins",
			"zmodload -F zsh/zle b:zle\nwhence -w bindkey; whence -w vared",
			"bindkey: builtin\nvared: builtin\n",
		},
		// **The row that says it is a transition and not "the module is not
		// loaded".** The module *is* loaded here, narrowed to one feature,
		// and the second line names one that was already off.
		{
			"off and staying off, on a module that is loaded",
			"zmodload -F zsh/zutil b:zstyle\nzmodload -F zsh/zutil -b:zformat\n" +
				"whence -w zformat",
			"zformat: builtin\n",
		},
		{
			"the same for a parameter",
			"zmodload -F zsh/parameter p:funcstack\n" +
				"zmodload -F zsh/parameter -p:parameters\nprint \"${+parameters}\"",
			"1\n",
		},
		// **And the controls: a feature that really was on goes.** Same
		// spellings, one whole load in front of them.
		{
			"a parameter that was on",
			"zmodload zsh/parameter\nzmodload -F zsh/parameter -p:functions\n" +
				"f(){ :; }\nprint \"${+functions} ${#functions}\"",
			"0 0\n",
		},
		{
			"a builtin that was on",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\n" +
				"whence -w zparseopts; whence -w zstyle",
			"zparseopts: none\nzstyle: builtin\n",
		},
		{
			"on, off and on again",
			"zmodload -F zsh/parameter p:funcstack p:parameters\n" +
				"zmodload -F zsh/parameter -p:parameters\nprint \"${+parameters}\"",
			"0\n",
		},
		// The other control, which is what says the name was working before
		// any of this: no `zmodload` anywhere.
		{
			"no zmodload at all",
			"f(){ :; }\nprint \"${+functions} ${#functions} ${+funcstack} ${+parameters}\"",
			"1 1 1 1\n",
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

// TestAGatedNameNeedsNoExemptionFromTheTransition is the half that would have
// been broken by writing the rule as "a deselected feature is left alone".
//
// `strftime` and `zf_mkdir` are registered **withdrawn** at startup, so a
// selection that leaves them off leaves them exactly as they were — which is
// the same answer, arrived at rather than carved out. Without that, #4997's
// whole row would come back the moment any `-F` touched the module.
func TestAGatedNameNeedsNoExemptionFromTheTransition(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		// The status the last line leaves, which is half of `whence -w`'s
		// answer: 1 for a name it found nothing for. Asserted so a shell
		// that wrote the right word and the wrong number cannot pass.
		code int
		// Names to put on the scratch `PATH`, for a row whose answer is
		// `command`: runZsh's `PATH` is an empty temp directory, so without
		// this the row reads `none` for the reason every other row does and
		// stops separating anything.
		onPath []string
	}{
		{
			name: "a gated builtin the selection did not name",
			src:  "zmodload -F zsh/datetime p:EPOCHSECONDS\nwhence -w strftime",
			want: "strftime: none\n", code: 1,
		},
		{
			name: "a gated builtin and the plain spelling beside it",
			src: "zmodload -F zsh/files b:zf_rm\n" +
				"whence -w zf_rm; whence -w zf_mkdir; whence -w mkdir",
			want:   "zf_rm: builtin\nzf_mkdir: none\nmkdir: command\n",
			onPath: []string{"mkdir"},
		},
		{
			name: "a gated parameter the selection did not name",
			src:  "zmodload -F zsh/system p:sysparams\nprint \"${+sysparams} ${+errnos}\"",
			want: "1 0\n",
		},
		{
			name: "a builtin-only selection over a module of parameters",
			src: "zmodload -F zsh/datetime b:strftime\n" +
				"print \"${+EPOCHSECONDS} ${+epochtime} ${+EPOCHREALTIME}\"",
			want: "0 0 0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.onPath {
				pathStub(t, dir, name)
			}
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != tc.code {
				t.Errorf("= %q (status %d), want %q at %d", out, st, tc.want, tc.code)
			}
		})
	}
}

// A widening after a narrow that never withdrew anything says nothing, where
// it used to write two sentences per name.
//
// Same shape as the one #5043 fixed for the gated parameters and reached the
// same way: a withdrawal recorded over a name the module had not taken left
// it reading as **taken**, so `zmodloadRestorable` refused. The refusal
// itself still fires for a name the script really has taken, which is the
// control underneath.
func TestAWideningAfterSuchANarrowIsSilent(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload -F zsh/parameter p:funcstack\nzmodload zsh/parameter\n"+
			"f(){ :; }\nprint \"${+functions} ${#functions}\"")
	if want := "1 1\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q with nothing said", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(),
		"zmodload zsh/system\nzmodload -F zsh/system -p:errnos\nerrnos=(a b)\n"+
			"zmodload -F zsh/system +p:errnos\nprint st=$?")
	const want = "zsh:4: Can't add module parameter `errnos': parameter already exists\n" +
		"zsh:zsh/system:4: error when adding parameter `errnos'\nst=2\n"
	if out != want || st != 0 {
		t.Errorf("the refusal that must survive = %q (status %d), want %q", out, st, want)
	}
}
