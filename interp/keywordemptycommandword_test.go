// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A word after a command word that expanded to nothing is still an argument,
// so `set -k` still takes it (#3903).
//
// The command word is the first word the script *wrote*, not the first one
// that survived expansion. This asked the expanded list instead, so `$e c=7`
// with `$e` empty made `c=7` the command and ran it: `c=7: command not found`
// at 127, with the assignment lost.
//
// Measured 2026-09-20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, with `set -k` and `e=` above each
// probe. **bash 5.3.20, bash 3.2.57 and ksh93u+ 2012-08-01 answer all seven
// identically**, so this is a defect and not a dialect split — the two
// columns that have the option agree, and zsh's `-k` is `interactivecomments`
// rather than this option at all, so it is not a column here. dash and
// BusyBox ash refuse the letter.
//
//	$e c=7                  st=0, c is 7 afterwards
//	$e c=8 f                `in=[8]`, st=0, c unset afterwards — a prefix
//	$e c=9 d=10             st=0, both set afterwards
//	$e c=11 true            st=0, c unset afterwards
//	$e $e c=12              st=0, c is 12 afterwards
//	$e 'c=13'               command not found, 127, c unset
//	$e c=14 env             `c=14` is in the child's environment
func TestTheKeywordOptionTakesAWordAfterAnEmptyCommandWord(t *testing.T) {
	for _, c := range []struct{ name, src, want, why string }{
		{
			"nothing but the promotion, so it is a plain assignment",
			`$e c=7; echo "st=$? c=[${c-unset}]"`,
			"st=0 c=[7]",
			"with no command word left, the assignment is the command and outlives it",
		},
		{
			"two empty words and then the promotion",
			`$e $e c=12; echo "st=$? c=[${c-unset}]"`,
			"st=0 c=[12]",
			"the rule is about the written command word, not about how many expanded away",
		},
		{
			"several promotions",
			`$e c=9 d=10; echo "st=$? c=[${c-unset}] d=[${d-unset}]"`,
			"st=0 c=[9] d=[10]",
			"every assignment word of the command, as when a command word survives",
		},
		{
			"a command word after the promotion",
			"f(){ echo \"in=[${c-unset}]\"; }\n" +
				`$e c=8 f; echo "st=$? c=[${c-unset}]"`,
			"in=[8]\nst=0 c=[unset]",
			"the next word becomes the command and the promotion is its prefix, " +
				"so it reaches the call and does not outlive it — a function " +
				"rather than a builtin, because a prefix at a builtin asks a " +
				"further axis this file has no answer for and no opinion about",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimSpace(keywordScript(t, c.src)); got != c.want {
				t.Errorf("wrote %q, want %q — %s", got, c.want, c.why)
			}
		})
	}
}

// Two rows about the same line that must not move, and this comment is the
// honest label they need: **both are pins rather than controls.** Neither can
// be made to fail by mutating what this change touched, because the two rules
// they assert live one layer down and are pinned there already — a quoted
// word is not assignment-shaped at all, so keywordPromotable never sees it
// (`TestWhichWordsTheKeywordOptionTakes` is where dropping that check fails),
// and the option gate is `TestAnUnansweredKeywordOptionIsRefused`'s.
//
// They are worth the lines anyway, because they are the measured rows an
// empty command word must not buy an exemption from: with `set -k` off, every
// column in the panel — bash, ksh93, zsh and dash alike — reports 127 for
// this line, and a quoted name is a command in the two columns that have the
// option.
//
// The mutation that does discriminate for this change is putting the
// `len(argv) > 0` guard back, which takes every row of the test above.
func TestAWordAfterAnEmptyCommandWordIsStillJudgedAsWritten(t *testing.T) {
	t.Run("a quoted name is not a name", func(t *testing.T) {
		out := keywordScript(t, `$e 'c=13'; echo "st=$? c=[${c-unset}]"`)
		if got, want := strings.TrimSpace(out), "c=[unset]"; !strings.HasSuffix(got, want) {
			t.Errorf("wrote %q, want it to end %q — the written word decides", got, want)
		}
		if !strings.Contains(out, "c=13") {
			t.Errorf("wrote %q, want the word run as a command and not found", out)
		}
	})
	t.Run("with the option off it is a command", func(t *testing.T) {
		out := &strings.Builder{}
		r := keywordRunner(t, out)
		runCd(t, r, "e=\n"+`$e c=7; echo "st=$? c=[${c-unset}]"`)
		if got, want := strings.TrimSpace(out.String()), "c=[unset]"; !strings.HasSuffix(got, want) {
			t.Errorf("wrote %q, want it to end %q — nothing promotes it", got, want)
		}
	})
}

// keywordScript runs src with `set -k` on and an empty `e` above it. The
// status is read back through `$?` in the snippet rather than returned,
// because it is the *command's* status that these rows are about and the
// runner's is the script's.
func keywordScript(t *testing.T, src string) string {
	t.Helper()
	out := &strings.Builder{}
	r := keywordRunner(t, out)
	runCd(t, r, "e=\nset -k\n"+src)
	return out.String()
}
