// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A frozen name at a `{name}` redirection, which is one root and two
// sentences.
//
// `A04redirect.ztst` stops on both halves in consecutive chunks — "Error
// opening file descriptor using readonly variable" and "Error closing file
// descriptor using readonly variable" — and they looked like separate bugs:
// the open was refused here with the wrong words, and the close was not
// refused at all. They are the same question asked on the way in and on the
// way out, and the preposition is what carries which:
//
//	{m}>f     with m frozen   can't allocate file descriptor **to** readonly parameter m
//	{m}>&-    with m frozen   can't close file descriptor **from** readonly parameter m
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device. Both leave 1 and neither runs the command.
//
// **Neither is the generic refusal.** `typeset -r m; m=3` is `read-only
// variable: m` in the same shell, and that sentence appears at neither
// redirection — which is why the open wording replaces the store's rather
// than following it the way bash's second line does.
func TestAFrozenNameRefusesTheRedirectionInItsOwnWords(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, wantErr string }{
		{
			"opening on a command", "typeset -r m\nprint -r -- x {m}>f\n",
			"zsh:2: can't allocate file descriptor to readonly parameter m\n",
		},
		{
			"opening on exec", "typeset -r m\nexec {m}>f\n",
			"zsh:2: can't allocate file descriptor to readonly parameter m\n",
		},
		{
			"appending", "typeset -r m\nexec {m}>>f\n",
			"zsh:2: can't allocate file descriptor to readonly parameter m\n",
		},
		{
			"duplicating an open descriptor", "typeset -r m\nexec {m}>&1\n",
			"zsh:2: can't allocate file descriptor to readonly parameter m\n",
		},
		{
			"closing, written with >", "exec {m}>f\ntypeset -r m\nexec {m}>&-\n",
			"zsh:3: can't close file descriptor from readonly parameter m\n",
		},
		{
			"closing, written with <", "exec {m}>f\ntypeset -r m\nexec {m}<&-\n",
			"zsh:3: can't close file descriptor from readonly parameter m\n",
		},
		{
			"closing on a command rather than exec", "exec {m}>f\ntypeset -r m\nprint -r -- x {m}>&-\n",
			"zsh:3: can't close file descriptor from readonly parameter m\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src+"print -r -- \"st $?\"\n")
			if out != "st 1\n" || errs != tc.wantErr || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, "st 1\n", tc.wantErr)
			}
		})
	}
}

// The controls, which are what keep the two rules from being "refuse
// everything that mentions a frozen name".
//
// The close row here is the one that matters most: **the close does not write
// the name**, so nothing about the operation requires it to be writable. A
// successful `exec {m}>&-` leaves `m` holding the number it held — measured —
// and that is why refusing it is a rule about the attribute rather than a
// consequence of an assignment being refused, and why it is an axis
// (interp.Semantics.ReadonlyFdVariableRefusesAClose) rather than a wording.
// bash 5.3.20 closes it and says nothing.
func TestAWritableNameIsUntouchedAndTheGenericRefusalIsStillItself(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, wantOut, wantErr string }{
		{
			"a close through a writable name keeps the number",
			"exec {m}>f\nexec {m}>&-\nprint -r -- \"st $? m=[$m]\"\n",
			"st 0 m=[11]\n", "",
		},
		{
			"an open through a writable name",
			"exec {m}>f\nprint -r -- \"st $? set=${m:+yes}\"\n",
			"st 0 set=yes\n", "",
		},
		{
			"and a frozen name nothing redirects through is not refused",
			"typeset -r m\nexec 3>f\nprint -r -- \"st $?\"\n",
			"st 0\n", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src)
			if out != tc.wantOut || errs != tc.wantErr || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, tc.wantOut, tc.wantErr)
			}
		})
	}
	// The generic refusal, which is a row of its own because it **ends the
	// script** where neither redirection refusal does: `typeset -r m; m=3`
	// writes `read-only variable: m` and nothing after it runs, in this shell
	// and in the reference. That difference is the clearest evidence that the
	// two redirection sentences are not the store's own with a new face —
	// they leave the script running and they leave the command at 1.
	t.Run("a plain assignment to a frozen name is the generic sentence, and fatal", func(t *testing.T) {
		out, st, errs := runZshSplit(t, dir, "typeset -r m\nm=3\nprint -r -- \"never\"\n")
		if out != "" || errs != "zsh:2: read-only variable: m\n" || st != 1 {
			t.Errorf("out %q err %q status %d, want nothing, the generic sentence, and 1", out, errs, st)
		}
	})
}
