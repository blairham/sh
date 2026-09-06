// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `${=spec}` in this dialect, which is where the axis it overrides lives.
//
// The substrate's tests name the grammar flag; this one names the shell,
// because the flag is only ever *visible* against this dialect's answer: an
// unquoted expansion's result is not split here, so the construct is the only
// thing that can split one. Measured 2026-09-06 on zsh 5.9.2.
func TestTheSplitFlagIsThisDialects(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${v}`, `1:[a b c]`},
		{`f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${=v}`, `3:[a][b][c]`},
		{`f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f "${=v}"`, `3:[a][b][c]`},
		{`f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${==v}`, `1:[a b c]`},
		{`f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b"; a=(${=v}); printf "%d" ${#a}`, `2`},
		// The option this overrides, from the other side: with it on, a
		// doubled `=` still refuses to split.
		{`setopt shwordsplit
f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${v}`, `3:[a][b][c]`},
		{`setopt shwordsplit
f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; v="a b c"; f ${==v}`, `1:[a b c]`},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A parse error is not what this is: the `=` is grammar here, so a `${=x}` in
// a file this dialect reads goes through.
func TestTheSplitFlagParsesInThisDialect(t *testing.T) {
	if _, err := parseZsh("echo ${=x}\necho ${==x}\necho ${(U)=x}\necho ${=~x}"); err != nil {
		t.Errorf("parse: %v", err)
	}
	// And the shape the shell refuses stays refused, as a bad substitution
	// when the expansion is reached rather than at the read.
	out, st := runZsh(t, t.TempDir(), `echo ${=(U)x}`)
	if !strings.Contains(out, "bad substitution") || st == 0 {
		t.Errorf("${=(U)x} = %q (status %d), want a bad substitution", out, st)
	}
}

// The shape a version predicate in this shell's own function library is
// written in, and the reason the refusal was worse than a loud one.
//
// Written here rather than copied: the library's `is-at-least` splits both
// operands with `${=1}` and `${=2:-$ZSH_VERSION}` and walks the two arrays.
// With the flag refused, both arrays are empty, the walk compares nothing,
// and the answer is "at least" for *every* version — at status 0, with no
// diagnostic, so nothing downstream can tell. A row that asserted only the
// absence of an error would have passed against exactly that.
func TestTheSplitFlagCarriesAVersionPredicate(t *testing.T) {
	fp := fpathDir(t, map[string]string{
		"at-least": `local -a want have
want=(${=1})
have=(${=2})
local i
for ((i = 1; i <= ${#want}; i++)); do
  [[ ${have[i]:-0} -gt ${want[i]} ]] && return 0
  [[ ${have[i]:-0} -lt ${want[i]} ]] && return 1
done
return 0`,
	})
	for _, tc := range []struct{ want, have, answer string }{
		{"99", "5 9 2", "NO"},
		{"5 10", "5 9 2", "NO"},
		{"5 9", "5 9 2", "YES"},
		{"4 0", "5 9 2", "YES"},
		{"5 9 2", "5 9 2", "YES"},
	} {
		src := `fpath=(` + fp + `)
builtin autoload -Uz at-least
at-least "` + tc.want + `" "` + tc.have + `" && print -r -- YES || print -r -- NO`
		out, st := runZsh(t, t.TempDir(), src)
		if out != tc.answer+"\n" || st != 0 {
			t.Errorf("at-least %q %q = %q (status %d), want %q",
				tc.want, tc.have, out, st, tc.answer+"\n")
		}
	}
}
