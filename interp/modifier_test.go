// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runWithModifiers runs src with the substring range read as a modifier list,
// and with wordings that name the two shapes of the refusal apart.
func runWithModifiers(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.SubstringRangeReadsModifiers = Yes
		r.Semantics = &sem
		diag := CoreDiagnostics()
		if r.Diagnostics != nil {
			diag = *r.Diagnostics
		}
		diag.UnrecognizedModifier = "no such modifier <%[1]s>"
		diag.UnrecognizedModifierAlone = "no such modifier"
		r.Diagnostics = &diag
	})
}

// runWithoutModifiers is the same source with the axis answered the other way,
// where the range is the arithmetic it looks like.
func runWithoutModifiers(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.SubstringRangeReadsModifiers = No
		r.Semantics = &sem
	})
}

// The axis is the whole of it: one range, two readings, and the same bytes.
// Under one answer `${x:i:2}` is a substring at the offset `i` holds; under
// the other it is a modifier list, `i` is not a modifier, and the expansion is
// refused. Answering it the first way in every dialect was silent — a
// plausible substring where the script should have stopped.
func TestASubstringRangeIsAModifierListOrAnExpression(t *testing.T) {
	const src = `x=abcdef; i=2; echo "[${x:i:2}]"; echo after`
	out, _ := runWithoutModifiers(t, src)
	if got := strings.TrimSpace(out); got != "[cd]\nafter" {
		t.Errorf("as an expression: got %q, want %q", got, "[cd]\nafter")
	}
	out, st := runWithModifiers(t, src)
	if !strings.Contains(out, "no such modifier <i>") {
		t.Errorf("as a modifier list: got %q, want the modifier named", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the command abandoned", out)
	}
	if st == 0 {
		t.Error("status = 0, want a failure")
	}
}

// The reading is decided by the first byte of the segment and by nothing else,
// which is why a name is not what makes a modifier. Each of these holds a name
// and each is still a range.
func TestOnlyALeadingLetterMakesAModifier(t *testing.T) {
	for _, src := range []string{
		`x=abcdef; _q=1; echo "[${x:_q:2}]"`,
		`x=abcdef; i=1; echo "[${x: i:2}]"`,
		`x=abcdef; i=1; echo "[${x:(i):2}]"`,
		`x=abcdef; i=1; echo "[${x:$i:2}]"`,
		`x=abcdef; h=1; echo "[${x:"h"}]"`,
	} {
		out, st := runWithModifiers(t, src)
		if got := strings.TrimSpace(out); !strings.HasPrefix(got, "[bc") || st != 0 {
			t.Errorf("%s: got %q (status %d), want a substring", src, got, st)
		}
	}
}

// The modifiers this implementation performs, each a pure function of the
// string. Values chosen for the edges rather than for the happy path: a
// trailing slash, a name with no slash at all, a leading dot, and a dot in a
// directory rather than in the last part.
func TestTheModifiersThatAreAFunctionOfTheString(t *testing.T) {
	for _, c := range []struct{ value, mod, want string }{
		{"/tmp/Dir/File.Txt", "h", "/tmp/Dir"},
		{"/tmp/Dir/File.Txt", "t", "File.Txt"},
		{"/tmp/Dir/File.Txt", "r", "/tmp/Dir/File"},
		{"/tmp/Dir/File.Txt", "e", "Txt"},
		{"/tmp/Dir/File.Txt", "l", "/tmp/dir/file.txt"},
		{"/tmp/Dir/File.Txt", "u", "/TMP/DIR/FILE.TXT"},
		{"/a/b//", "h", "/a"},
		{"/a/b//", "t", "b"},
		{"a//b", "h", "a"},
		{"a/", "h", "."},
		{"a/", "t", "a"},
		{"/", "h", "/"},
		{"/", "t", ""},
		{"", "h", "."},
		{"", "t", ""},
		{"nodot", "r", "nodot"},
		{"nodot", "e", ""},
		{".hidden", "r", ""},
		{".hidden", "e", "hidden"},
		{"x/.hidden", "r", "x/"},
		{"a.b.c", "r", "a.b"},
		{"a.b.c", "e", "c"},
		{"a/b.c/d", "r", "a/b.c/d"},
		{"a/b.c/d", "e", ""},
		{"..", "r", "."},
	} {
		src := `x='` + c.value + `'; echo "[${x:` + c.mod + `}]"`
		out, st := runWithModifiers(t, src)
		if got, want := strings.TrimSpace(out), "["+c.want+"]"; got != want || st != 0 {
			t.Errorf("%q:%s = %q (status %d), want %q", c.value, c.mod, got, st, want)
		}
	}
}

// Modifiers chain, left to right, and a chain of three arrives as one segment
// and a word holding the rest — so the rest has to be split again rather than
// evaluated whole.
func TestModifiersChainLeftToRight(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=/tmp/Dir/File.Txt; echo "[${x:h:t}]"`, "[Dir]"},
		{`x=/tmp/Dir/File.Txt; echo "[${x:t:r}]"`, "[File]"},
		{`x=/tmp/Dir/File.Txt; echo "[${x:h:h:t}]"`, "[tmp]"},
	} {
		out, st := runWithModifiers(t, c.src)
		if got := strings.TrimSpace(out); got != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, got, st, c.want)
		}
	}
}

// An offset and then a modifier: the two readings are segments of one range
// rather than alternatives, so the modifier applies to what the offset left.
func TestAModifierAfterAnOffsetAppliesToTheSlice(t *testing.T) {
	out, st := runWithModifiers(t, `x=/tmp/Dir/File.Txt; echo "[${x:2:t}]"`)
	if got := strings.TrimSpace(out); got != "[File.Txt]" || st != 0 {
		t.Errorf("got %q (status %d), want %q", got, st, "[File.Txt]")
	}
}

// A segment is one modifier and the letter is the whole of it. An unknown
// letter is named; a known letter with something after it is the same
// complaint with nothing named, which is why the refusal is two wordings.
func TestARefusedModifierIsNamedOnlyWhenTheLetterIsTheProblem(t *testing.T) {
	out, _ := runWithModifiers(t, `x=/tmp/a.b; echo "[${x:i}]"`)
	if !strings.Contains(out, "no such modifier <i>") {
		t.Errorf("got %q, want the letter named", out)
	}
	out, _ = runWithModifiers(t, `x=/tmp/a.b; echo "[${x:ha}]"`)
	if !strings.Contains(out, "no such modifier") || strings.Contains(out, "<") {
		t.Errorf("got %q, want the complaint with nothing named", out)
	}
}

// All thirteen letters are performed now, and the six that need something a
// string does not carry are the reason applyModifier is a method.
//
// Names the axis and never a shell, which is the rule for a test here. The
// values chosen are the ones no machine can disagree about: an absolute path
// needs no working directory, a path under `/no/such` exists nowhere, and a
// name holding a slash is never searched for.
func TestTheModifiersThatNeedMoreThanTheStringArePerformed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// `:a` is lexical — `..` and `.` cancelled by name, no link followed.
		{"absolute", `x=/x/y/../z/.; echo "[${x:a}]"`, "[/x/z]\n"},
		{"absolute keeps empty empty", `x=; echo "[${x:a}]"`, "[]\n"},
		// `:A` and `:P` resolve what exists and leave the rest alone, so a
		// path that is nowhere comes back as itself.
		{"resolved", `x=/no/such/path; echo "[${x:A}]"`, "[/no/such/path]\n"},
		{"real path keeps a trailing slash it could not resolve", `x=/no/such/; echo "[${x:P}]"`, "[/no/such/]\n"},
		{"resolved drops it", `x=/no/such/; echo "[${x:A}]"`, "[/no/such]\n"},
		// `:c` leaves a name it cannot find, and never touches one with a
		// slash in it.
		{"command not found", `x=nosuchcommand12345; echo "[${x:c}]"`, "[nosuchcommand12345]\n"},
		{"command with a slash", `x=./nosuch; echo "[${x:c}]"`, "[./nosuch]\n"},
		// `:q` and `:Q` are this shell's own quoting, out and back.
		{"quoted", `x="a b*c"; echo "[${x:q}]"`, "[a\\ b\\*c]\n"},
		{"quoted empty stays empty", `x=; echo "[${x:q}]"`, "[]\n"},
		{"unquoted", `x="'a b'"; echo "[${x:Q}]"`, "[a b]\n"},
		{"round trip", `x="a b"; echo "[${x:q:Q}]"`, "[a b]\n"},
		// `:s` replaces a literal substring, first occurrence or every one.
		{"substituted", `x=aXbXc; echo "[${x:s/X/-/}]"`, "[a-bXc]\n"},
		{"substituted globally", `x=aXbXc; echo "[${x:gs/X/-/}]"`, "[a-b-c]\n"},
		{"the delimiter may be a colon", `x=aXbXc; echo "[${x:s:X:-:}]"`, "[a-bXc]\n"},
		{"the matched text", `x=aXbXc; echo "[${x:s/X/[&]/}]"`, "[a[X]bXc]\n"},
		// A pattern is not a pattern: these are literal strings.
		{"a question mark is literal", `x=abc; echo "[${x:s/?/Z/}]"`, "[abc]\n"},
		{"and is replaced where it is there", `x="a?c"; echo "[${x:s/?/Z/}]"`, "[aZc]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWithModifiers(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The substitution is remembered for the shell rather than for the parameter,
// which is what an empty pattern and `:&` both reach for.
func TestASubstitutionIsRememberedForTheShell(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an empty pattern reuses it", `x=aXbXc; echo "[${x:s/X/-/}][${x:s//+/}]"`, "[a-bXc][a+bXc]\n"},
		{"across parameters", `x=aXbXc; y=aXd; echo "[${x:s/X/-/}][${y:s//+/}]"`, "[a-bXc][a+d]\n"},
		{"and `&` repeats the whole of it", `x=aXbXc; echo "[${x:s/X/-/}][${x:&}]"`, "[a-bXc][a-bXc]\n"},
		{"`&` with none before it is silence", `x=aXbXc; echo "[${x:&}]"`, "[aXbXc]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWithModifiers(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
			}
		})
	}
	// An empty pattern with nothing to reuse is refused by name, which is the
	// one place the two spellings differ: `:&` says nothing and this says so.
	out, st := runWithModifiers(t, `x=aXbXc; echo "[${x:s//+/}]"`)
	if !strings.Contains(out, "no previous substitution") || st == 0 {
		t.Errorf("got %q (status %d), want a refusal naming what is missing", out, st)
	}
}

// A count after `h` or `t` counts *separators*, from the left and from the
// right respectively — not repetitions of the modifier, which is what it
// looks like and is a different answer at almost every count.
func TestACountAfterHeadOrTailCountsSeparators(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// `:h` three times over would be `/a/b`, which is `:h3`'s answer by
		// coincidence and nothing else's.
		{`x=/a/b/c/d/e; echo "[${x:h1}]"`, "[/]\n"},
		{`x=/a/b/c/d/e; echo "[${x:h2}]"`, "[/a]\n"},
		{`x=/a/b/c/d/e; echo "[${x:h3}]"`, "[/a/b]\n"},
		{`x=/a/b/c/d/e; echo "[${x:h9}]"`, "[/a/b/c/d/e]\n"},
		{`x=/a/b/c/d/e; echo "[${x:t2}]"`, "[d/e]\n"},
		{`x=/a/b/c/d/e; echo "[${x:t5}]"`, "[a/b/c/d/e]\n"},
		{`x=/a/b/c/d/e; echo "[${x:t9}]"`, "[/a/b/c/d/e]\n"},
		// No count and `0` are the same answer, and neither is `1`.
		{`x=/a/b/c/d/e; echo "[${x:h}][${x:h0}]"`, "[/a/b/c/d][/a/b/c/d]\n"},
		// A trailing run of slashes separates nothing, so this path has two
		// separators and not three.
		{`x=/a/b//; echo "[${x:h2}][${x:h3}]"`, "[/a][/a/b//]\n"},
	} {
		if out, st := runWithModifiers(t, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
		}
	}
}

// A length *and then* a modifier list, which the parser cannot split on its
// own: a range is split once, so `5:t` arrived whole and reached the
// evaluator as an expression.
func TestAModifierMayFollowBothAnOffsetAndALength(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=/tmp/Dir/File.Txt.gz; echo "[${x:1:5:t}]"`, "[D]\n"},
		{`x=abcdefgh; echo "[${x:1:5:u}]"`, "[BCDEF]\n"},
		// Still a length and still a modifier where there is only one of them.
		{`x=/tmp/Dir/File.Txt.gz; echo "[${x:2:t}]"`, "[File.Txt.gz]\n"},
		{`x=/tmp/Dir/File.Txt.gz; echo "[${x:2:2}]"`, "[mp]\n"},
	} {
		if out, st := runWithModifiers(t, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
		}
	}
	// And a digit that is neither is still refused, naming itself.
	if out, _ := runWithModifiers(t, `x=abcdefgh; echo "[${x:1:2:3}]"`); !strings.Contains(out, "no such modifier <3>") {
		t.Errorf("got %q, want the digit named", out)
	}
}

// The refusal names **one byte**, not the whole segment. `${x:zz}` is a
// complaint about `z`; the second `z` has not been looked at, and naming both
// would say the pair is the modifier that is missing.
func TestAnUnrecognizedModifierNamesOneByte(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=/tmp/a.b; echo "[${x:zz}]"`, "no such modifier <z>"},
		{`x=/tmp/a.b; echo "[${x:iq}]"`, "no such modifier <i>"},
		{`x=/tmp/a.b; echo "[${x:h:zz}]"`, "no such modifier <z>"},
		// `g` is a prefix rather than a letter, so alone it names itself.
		{`x=/tmp/a.b; echo "[${x:g}]"`, "no such modifier <g>"},
	} {
		out, st := runWithModifiers(t, tc.src)
		if !strings.Contains(out, tc.want) || st == 0 {
			t.Errorf("%s\ngot  %q (status %d)\nwant it to contain %q", tc.src, out, st, tc.want)
		}
	}
	// A letter that *is* a modifier with junk after it is the other shape,
	// and names nothing.
	out, _ := runWithModifiers(t, `x=/tmp/a.b; echo "[${x:hzz}]"`)
	if !strings.Contains(out, "no such modifier") || strings.Contains(out, "<") {
		t.Errorf("got %q, want the complaint with nothing named", out)
	}
}
