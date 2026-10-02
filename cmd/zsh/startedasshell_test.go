// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runZshAs runs a command string through the front end under argv, with
// nothing inherited but PATH.
func runZshAs(t *testing.T, argv ...string) string {
	t.Helper()
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin"}
	driver.MainArgs(sh, argv)
	return out.String() + errs.String()
}

const startupProbe = `print -r -- "$- $ZSH_NAME [$PS4] ${(q)IFS}"`

// TestAShellStartedAsShSeedsTheStandardsValues pins what a startup as `sh` or
// `ksh` seeds, by `--emulate` and by the name alike: `$-` written in sh's
// letters, `PS4` as `+ `, `IFS` without its NUL, and notify off. Started as
// itself or as `csh` the shell keeps its own. Measured 2026-10-02 on zsh
// 5.9.2 under `env -i PATH=/usr/bin:/bin` (#5336).
func TestAShellStartedAsShSeedsTheStandardsValues(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"zsh", "--emulate", "sh", "-fc", startupProbe}, "f zsh [+ ] \\ $'\\t'$'\\n'\n"},
		{[]string{"zsh", "--emulate", "ksh", "-fc", startupProbe}, "f zsh [+ ] \\ $'\\t'$'\\n'\n"},
		{[]string{"zsh", "--emulate", "sh", "-c", startupProbe}, " zsh [+ ] \\ $'\\t'$'\\n'\n"},
		{[]string{"sh", "-fc", startupProbe}, "f sh [+ ] \\ $'\\t'$'\\n'\n"},
		{[]string{"/x/ksh", "-fc", startupProbe}, "f ksh [+ ] \\ $'\\t'$'\\n'\n"},
		{[]string{"zsh", "--emulate", "csh", "-fc", startupProbe}, "69Xfu zsh [+%N:%i> ] \\ $'\\t'$'\\n'$'\\0'\n"},
		{[]string{"zsh", "-fc", startupProbe}, "569Xf zsh [+%N:%i> ] \\ $'\\t'$'\\n'$'\\0'\n"},
	} {
		if got := runZshAs(t, tc.argv...); got != tc.want {
			t.Errorf("%v\n got %q\nwant %q", tc.argv[:len(tc.argv)-1], got, tc.want)
		}
	}
}

// TestShLettersWriteDollarDash pins `$-` under sh's letters at run time.
// Measured 2026-10-02 on zsh 5.9.2 (#5336).
func TestShLettersWriteDollarDash(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"emulate sh; print -r -- $-", "b\n"},
		{"emulate sh; set -f; print -r -- $-", "bf\n"},
		{"emulate sh; set +b; [[ -o notify ]] || print off; print -r -- $-", "off\n\n"},
		{"emulate sh; set -o markdirs; print -r -- $-", "Xb\n"},
		{"emulate sh; emulate zsh; print -r -- $-", "569Xf\n"},
	} {
		if got := runZshAs(t, "zsh", "-fc", tc.src); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestZshNameIsTheInvocationsName pins ZSH_NAME as argv[0]'s base name, a
// login shell's dash taken off. See interp.Semantics.InvocationNameParameter.
func TestZshNameIsTheInvocationsName(t *testing.T) {
	for argv0, want := range map[string]string{
		"zsh": "zsh\n", "/x/y/myzsh": "myzsh\n", "-zsh": "zsh\n", "./sh": "sh\n",
	} {
		if got := runZshAs(t, argv0, "-fc", "print -r -- $ZSH_NAME"); got != want {
			t.Errorf("argv[0] %q: got %q, want %q", argv0, got, want)
		}
	}
}
