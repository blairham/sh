// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The four bytes a listing does not spell the same way in every dialect.
//
// Measured 2026-09-14, `env -i PATH=/usr/bin:/bin HOME=<scratch>`, from a
// script file, over `set`, a keyed `typeset -p` and an alias listing — which
// agree within each column, so one table answers for all three:
//
//	              ! bare   ^ bare   = bare   a byte above ASCII bare
//	bash 5.3.15   no       no       yes      yes
//	ksh93u+       yes      yes      *        no — `$'\xc3\xa9'`, byte by byte
//	zsh 5.9.2     yes      no       no       yes
//
// No two columns group the same way, and no two bytes group the same way
// either, which is why these are four questions.
// The `*` is ksh93's fourth answer: a leading `name=` is bare and the rest is
// quoted on its own.
//
// This engine had one shared set with `^` in it and neither `=` nor a
// non-ASCII byte, which is ksh93's answer to the first and nobody's to the
// other two (#2820).
//
// Keys and values in one table because the same predicate decides both, and
// because the issue this came from read the divergence as the keys' alone: it
// swept forty keys and never asked what the same three characters did to a
// value, where the same three columns diverge in the same directions.
func TestHowAListingSpellsTheThreeBytesThePanelSplitsOn(t *testing.T) {
	presets := map[string]dialecttest.Preset{
		"bash": {
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		},
		"ksh": {
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		},
		"zsh": {
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		},
	}
	for _, tc := range []struct {
		name, dialect, src, want string
	}{
		// The keys, which is the shape the listing is read back from. The
		// caret arrives through a parameter because one dialect keeps the
		// quotes of a literal subscript as part of the key, which would make
		// the three columns spell different keys.
		{
			"bash quotes a caret key and leaves an equals bare", "bash",
			`k='^'; b='!'; typeset -A w; w[a=b]=1; w[$k]=2; w[é]=3; w[$b]=4; typeset -p w`,
			"declare -A w=([\"!\"]=\"4\" [\"^\"]=\"2\" [a=b]=\"1\" [é]=\"3\" )\n",
		},
		{
			"ksh leaves the caret and the assignment head bare and spells the byte out", "ksh",
			`k='^'; b='!'; typeset -A w; w[a=b]=1; w[$k]=2; w[é]=3; w[$b]=4; typeset -p w`,
			"typeset -A w=([!]=4 [^]=2 [a=b]=1 [$'\\xc3\\xa9']=3)\n",
		},
		{
			"zsh quotes the caret and the equals and leaves the byte alone", "zsh",
			`k='^'; b='!'; typeset -A w; w[a=b]=1; w[$k]=2; w[é]=3; w[$b]=4; typeset -p w`,
			"typeset -A w=( [!]=4 ['^']=2 ['a=b']=1 [é]=3 )\n",
		},
		// And the values, in the two dialects whose declaration listing asks
		// at all — bash's double-quotes whatever it is given, so its answer
		// is the keys above and its `set`.
		{
			"ksh values follow the same three rules", "ksh",
			`v1='^'; v2='a=b'; v3='x=y=z'; v4=é; v5='!'; typeset -p v1 v2 v3 v4 v5`,
			"v1=^\nv2=a=b\nv3=x='y=z'\nv4=$'\\xc3\\xa9'\nv5=!\n",
		},
		{
			"zsh values follow the same three rules", "zsh",
			`v1='^'; v2='a=b'; v3='x=y=z'; v4=é; v5='!'; typeset -p v1 v2 v3 v4 v5`,
			"typeset v1='^'\ntypeset v2='a=b'\ntypeset v3='x=y=z'\ntypeset v4=é\ntypeset v5=!\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := presets[tc.dialect].Combined(t, dialecttest.Base{}, tc.src)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			if out != tc.want {
				t.Errorf("listed\n  %q\nwant\n  %q", out, tc.want)
			}
		})
	}
}

// ksh93's `=` is a rule about the front of the word rather than a character
// class, and the shapes that separate the two are the ones with no name in
// front of the first `=`.
//
// Measured the same day, `typeset -p` of each: `a=b` bare, `a=` bare,
// `a=b c` as `a='b c'`, `x=y=z` as `x='y=z'`, `a=b=c` as `a='b=c'` — so the
// head is taken once and at the front — and `=x`, `1=2` and `a.b=c` quoted
// whole, because what stands before the first `=` is not a name.
//
// A doubled `=` is measured and not reproduced: ksh93u+ writes `a==b` bare
// where this gives `a='=b'`. Both read back as the value; see
// Runner.listedAssignmentHead.
func TestTheBareAssignmentHeadIsTakenOnceAndOnlyAfterAName(t *testing.T) {
	preset := dialecttest.Preset{
		Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
		Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
	}
	for _, tc := range []struct{ value, want string }{
		{`a=b`, `v=a=b`},
		{`a=`, `v=a=`},
		{`a=b c`, `v=a='b c'`},
		{`x=y=z`, `v=x='y=z'`},
		{`a=b=c`, `v=a='b=c'`},
		{`=x`, `v='=x'`},
		{`1=2`, `v='1=2'`},
		{`a.b=c`, `v='a.b=c'`},
	} {
		t.Run(tc.value, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{},
				"v='"+tc.value+"'; typeset -p v")
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			if out != tc.want+"\n" {
				t.Errorf("listed %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// And the vectors say so, so that a dialect added later has to answer rather
// than inherit whichever set happened to be in the shared helper.
func TestEachDialectAnswersTheListedByteAxes(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		sem                                 interp.Semantics
		bang, caret, equals, nonASCII, head interp.Answer
	}{
		{"bash", bash.Semantics(), interp.No, interp.No, interp.Yes, interp.Yes, interp.No},
		{"ksh", ksh.Semantics(), interp.Yes, interp.Yes, interp.No, interp.No, interp.Yes},
		{"zsh", zsh.Semantics(), interp.Yes, interp.No, interp.No, interp.Yes, interp.No},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, got := range []struct {
				field string
				have  interp.Answer
				want  interp.Answer
			}{
				{"ListedBangIsOrdinary", tc.sem.ListedBangIsOrdinary, tc.bang},
				{"ListedCaretIsOrdinary", tc.sem.ListedCaretIsOrdinary, tc.caret},
				{"ListedEqualsIsOrdinary", tc.sem.ListedEqualsIsOrdinary, tc.equals},
				{"ListedNonAsciiIsOrdinary", tc.sem.ListedNonAsciiIsOrdinary, tc.nonASCII},
				{"ListedAssignmentPrefixIsBare", tc.sem.ListedAssignmentPrefixIsBare, tc.head},
			} {
				if got.have != got.want {
					t.Errorf("%s = %v, want %v", got.field, got.have, got.want)
				}
			}
		})
	}
}
