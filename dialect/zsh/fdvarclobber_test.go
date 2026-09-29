// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Under `noclobber`, a `{name}` redirection protects the **name**, not the
// file it is about to open.
//
// `A04redirect.ztst` stops here on "NO_CLOBBER with fd variable", and the
// chunk reads like the ordinary clobber rule wearing a new subject. It is not.
// The ordinary rule is about a file that already exists and `>|` defeats it;
// this one is about a parameter that already holds a descriptor this shell has
// open, `>|` does not defeat it, and no file is created when it fires.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i PATH=/usr/bin:/bin`
// with standard input on the null device.
//
// **Every row here holds the file fixed and varies what the name holds**,
// because that is the axis the rule is keyed on and a grid that varied the
// file would have agreed everywhere for the wrong reason. `f2` is a fresh
// path in every row; only `$m` moves.
func TestNoclobberProtectsANameHoldingAnOpenDescriptor(t *testing.T) {
	for _, tc := range []struct{ name, src, wantOut, wantErr string }{
		{
			"the chunk: the name holds one this shell opened",
			"exec {m}>f1\nexec {m}>f2\n",
			"st=1 m=11\n",
			"zsh:3: can't clobber parameter m containing file descriptor 11\n",
		},
		{
			"a named stream counts as open, though it is in no table",
			"m=0\nexec {m}>f2\n",
			"st=1 m=0\n",
			"zsh:3: can't clobber parameter m containing file descriptor 0\n",
		},
		{
			"standard error likewise",
			"m=2\nexec {m}>f2\n",
			"st=1 m=2\n",
			"zsh:3: can't clobber parameter m containing file descriptor 2\n",
		},
		{
			"an empty value is the number zero, not a word",
			"m=\nexec {m}>f2\n",
			"st=1 m=\n",
			"zsh:3: can't clobber parameter m containing file descriptor 0\n",
		},
		{
			"appending is refused in the same words",
			"exec {m}>f1\nexec {m}>>f2\n",
			"st=1 m=11\n",
			"zsh:3: can't clobber parameter m containing file descriptor 11\n",
		},
		{
			"and so is a redirection on an ordinary command",
			"exec {m}>f1\nprint -r -- x {m}>f2\n",
			"st=1 m=11\n",
			"zsh:3: can't clobber parameter m containing file descriptor 11\n",
		},
		{
			// The row that tells this rule from the clobber rule it
			// resembles. `>|` exists to defeat `set -C` on a file and it
			// does not defeat this, because the subject is the parameter.
			"the clobber override does not defeat it",
			"exec {m}>f1\nexec {m}>|f2\n",
			"st=1 m=11\n",
			"zsh:3: can't clobber parameter m containing file descriptor 11\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A directory per row. Several rows turn on whether a path
			// exists or a descriptor is open, so a shared one would have
			// let an earlier row answer a later one.
			out, st, errs := runZshSplit(t, t.TempDir(),
				"setopt noclobber\n"+tc.src+"print -r -- \"st=$? m=$m\"\n")
			if out != tc.wantOut || errs != tc.wantErr || st != 0 {
				t.Errorf("out %q err %q status %d, want %q / %q", out, errs, st, tc.wantOut, tc.wantErr)
			}
		})
	}
}

// The controls, which are what stop the rule from being "refuse every
// `{name}` redirection under noclobber".
//
// The first three are the same shape as the refusals above with one thing
// changed about the name, and each of them is allowed — so the rule is keyed
// on *the number the name holds being open here*, and not on noclobber, not
// on the name being set, and not on the operator.
func TestWhatNoclobberDoesNotProtect(t *testing.T) {
	for _, tc := range []struct{ name, src, wantOut string }{
		// Not a number, so not a descriptor. The name is overwritten.
		{"a word in the name", "m=hello\nexec {m}>f2\n", "st=0 m=11\n"},
		// Allowed for the same reason as the row above it: the sign
		// makes it not a number to this shell's reader, so it never
		// reaches the question of which descriptor it names.
		{"a negative number", "m=-1\nexec {m}>f2\n", "st=0 m=11\n"},
		// A number, but nothing is open there.
		{"a number this shell has not opened", "m=77\nexec {m}>f2\n", "st=0 m=11\n"},
		{"an unset name", "exec {m}>f2\n", "st=0 m=11\n"},
		// The descriptor, not the value, is what it is about: closing it
		// leaves `m` holding 11 and the redirection is then allowed.
		{"one that has been closed again", "exec {m}>f1\nexec {m}>&-\nexec {m}>f2\n", "st=0 m=11\n"},
		// And the file rule is still the file rule.
		{"a brand new file under noclobber", "print -r -- hi > brandnew\n", "st=0 m=\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(),
				"setopt noclobber\n"+tc.src+"print -r -- \"st=$? m=$m\"\n")
			if out != tc.wantOut || errs != "" || st != 0 {
				t.Errorf("out %q err %q status %d, want %q in silence", out, errs, st, tc.wantOut)
			}
		})
	}
	// Without noclobber the same script allocates a second descriptor, which
	// is the row that keeps the option in the rule.
	t.Run("and without noclobber it simply allocates another", func(t *testing.T) {
		out, st, errs := runZshSplit(t, t.TempDir(),
			"exec {m}>f1\nexec {m}>f2\nprint -r -- \"st=$? m=$m\"\n")
		if out != "st=0 m=12\n" || errs != "" || st != 0 {
			t.Errorf("out %q err %q status %d, want st=0 m=12 in silence", out, errs, st)
		}
	})
}

// Where the two refusals meet, the readonly one wins — and it has to be
// stated, because this check runs *before* the file is opened while the
// readonly check runs at the store, which is later. Putting this one in front
// reversed the order and moved a row that #5120 had pinned.
//
//	setopt noclobber; exec {m}>f1; typeset -r m; exec {m}>f2
//	    can't allocate file descriptor to readonly parameter m
//
// Not `can't clobber parameter m containing file descriptor 11`, though the
// name is frozen *and* holds an open descriptor and both rules would
// otherwise fire.
func TestTheReadonlyRefusalWinsOverThisOne(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(),
		"setopt noclobber\nexec {m}>f1\ntypeset -r m\nexec {m}>f2\nprint -r -- \"st=$? m=$m\"\n")
	if out != "st=1 m=11\n" ||
		errs != "zsh:4: can't allocate file descriptor to readonly parameter m\n" || st != 0 {
		t.Errorf("out %q err %q status %d, want the readonly sentence, not the clobber one", out, errs, st)
	}
}
