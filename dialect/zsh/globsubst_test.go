// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// GLOB_SUBST makes the *result* of an expansion a pattern rather than literal
// text, and this shell read it and stored it and asked it nowhere.
//
// The direction of the bug is the reason nothing caught it: with the option
// off this shell was right, so only a script that sets the option was
// affected. The discriminating row is `${~u}` — the per-expansion spelling of
// the same request was carried and honored, so the machinery existed and the
// option was what nothing consulted (#1734).
//
// Measured on zsh 5.9.2, 2026-09-10, and every row below is that shell's own
// answer.
func TestGlobSubstReachesEveryPatternOperand(t *testing.T) {
	const src = `setopt globsubst
u='a*b'
[[ 'axb' == $u ]] && echo "1 yes" || echo "1 no"
case axb in ($u) echo "2 yes";; (*) echo "2 no";; esac
w='axbtail'; print -r -- "3 [${w#$u}]"
print -r -- "4 [${(M)${:-axb}:#$u}]"
v='a*b'
[[ $v == ${(q)v} ]] && echo "5 yes" || echo "5 no"
[[ $v == ${(b)v} ]] && echo "6 yes" || echo "6 no"
[[ 'axb' == ${~u} ]] && echo "7 yes" || echo "7 no"
`
	want := "1 yes\n2 yes\n3 [tail]\n4 [axb]\n5 yes\n6 yes\n7 yes\n"
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", src})
	if out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q (stderr %q)", out.String(), code, want, errs.String())
	}
}

// TestWithTheOptionOffTheResultIsStillText: the other half, and the half this
// shell already had right — a fix that turned the reading on unconditionally
// would pass the test above and break every script that never asked.
func TestWithTheOptionOffTheResultIsStillText(t *testing.T) {
	const src = `u='a*b'
[[ 'axb' == $u ]] && echo "1 yes" || echo "1 no"
case axb in ($u) echo "2 yes";; (*) echo "2 no";; esac
w='axbtail'; print -r -- "3 [${w#$u}]"
`
	want := "1 no\n2 no\n3 [axbtail]\n"
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", src})
	if out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q (stderr %q)", out.String(), code, want, errs.String())
	}
}

// TestGlobSubstIsTheAxisAndSoStaysInASubshell: it moves the semantics vector
// rather than a bit of its own, which is what makes a subshell's change the
// subshell's — measured, and the same arrangement `multios` and `shwordsplit`
// use.
func TestGlobSubstIsTheAxisAndSoStaysInASubshell(t *testing.T) {
	const src = `u='a*b'
setopt globsubst
(unsetopt globsubst; [[ 'axb' == $u ]]; echo "sub=$?")
[[ 'axb' == $u ]]; echo "outer=$?"
unsetopt globsubst
(setopt globsubst; [[ 'axb' == $u ]]; echo "sub2=$?")
[[ 'axb' == $u ]]; echo "outer2=$?"
`
	want := "sub=1\nouter=0\nsub2=0\nouter2=1\n"
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", src})
	if out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q (stderr %q)", out.String(), code, want, errs.String())
	}
}

// `BARE_GLOB_QUAL` decides whether a trailing `(…)` on a pattern is a glob
// qualifier list or part of the pattern. It was ignored here and the
// qualifier applied either way (#1729).
//
// It is not a corner a script has to opt into: the preamble an agent harness
// puts in front of **every** command it runs turns it off, so it is the state
// every command under one is expanded in.
//
// Measured on zsh 5.9.2, 2026-09-10, in a directory holding `AGENTS.md`,
// `CLA.md`, `xN` and `xy`.
func TestBareGlobQualifiersCanBeTurnedOff(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"AGENTS.md", "CLA.md", "xN", "xy"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name, src, out, errs string
		status               int
	}{
		{
			"a qualifier is pattern text with the option off",
			"setopt NO_BARE_GLOB_QUAL; echo *.md(N)",
			"", "zsh:1: no matches found: *.md(N)\n", 1,
		},
		{
			"and a qualifier again with it on",
			"echo *.md(N)",
			"AGENTS.md CLA.md\n", "", 0,
		},
		{
			"the group matches the letters it holds",
			"setopt NO_BARE_GLOB_QUAL; echo x(N)",
			"xN\n", "", 0,
		},
		{
			"an alternation was never a qualifier list and is unaffected",
			"setopt NO_BARE_GLOB_QUAL; echo x(N|y)",
			"xN xy\n", "", 0,
		},
		{
			// The measurement that keeps this a check of its own rather
			// than a second reading of the extended-pattern option.
			"the `(#q…)` spelling still qualifies",
			"setopt extendedglob NO_BARE_GLOB_QUAL; echo *(#q.)",
			"AGENTS.md CLA.md xN xy\n", "", 0,
		},
		{
			"a condition has no qualifier list to lose",
			"setopt NO_BARE_GLOB_QUAL; [[ xN == x(N) ]]; echo cond=$?",
			"cond=0\n", "", 0,
		},
		{
			// The other name the harness preamble sets, measured beside it
			// because they are set together and only one of them was wrong.
			"turning the extended operators off does not turn this off",
			"setopt NO_EXTENDED_GLOB; echo *.md(N)",
			"AGENTS.md CLA.md\n", "", 0,
		},
		{
			"the harness preamble, written as it is written",
			"{ shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; } >/dev/null 2>&1 || true\necho *.md(N)",
			"", "zsh:2: no matches found: *.md(N)\n", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, c.src)
			if out != c.out || errs != c.errs || st != c.status {
				t.Errorf("out %q / %q status %d, want %q / %q at %d",
					out, errs, st, c.out, c.errs, c.status)
			}
		})
	}
}
