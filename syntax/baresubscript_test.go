// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/syntax"
)

// bare is the core plus the no-brace subscript. The flag is named here and the
// shell that sets it is not: what belongs to a dialect lives under dialect/.
//
// ArraySubscript travels with it — the flag only decides where the word ends,
// and the parser still needs the subscript itself to be a construct — so a
// grammar with one and not the other is not a shell anybody has.
func bare() syntax.Dialect {
	d := syntax.Core()
	d.BareSubscript = true
	return d
}

// spansOf returns the spans of the word after the command name, which is where
// every case below writes its expansion.
//
// The word after the name rather than the last word, because half the point is
// that some of these are *two* words: `echo $a[1 ]` is an unfinished subscript
// followed by a bracket, and a helper reading the last word would report on the
// bracket.
func spansOf(t *testing.T, src string, d syntax.Dialect) []syntax.Span {
	t.Helper()
	cmd := onlyCommand(t, src, d)
	simple, ok := cmd.(*syntax.SimpleCmd)
	if !ok || len(simple.Args) < 2 {
		t.Fatalf("parse %q: not a simple command with an operand", src)
	}
	return simple.Args[1].Spans
}

// The whole of the flag: `$a[1]` is one expansion where it is on, and an
// expansion followed by two spans of text where it is off.
//
// The word boundary is the entire difference, which is why this is checked on
// the spans rather than on a result. Off, the `[1]` is a *pattern* by the time
// anything expands it, so the two readings do not even fail the same way.
func TestABareSubscriptIsPartOfTheExpansion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		value string // the expansion's inner text, as `${ … }` would hold it
		after string // the literal text left over, "" for none
	}{
		{"an element", "echo $a[1]", "a[1]", ""},
		{"a range", "echo $a[1,3]", "a[1,3]", ""},
		{"a negative subscript", "echo $a[-1]", "a[-1]", ""},
		{"a subscript holding an expansion", "echo $a[$i]", "a[$i]", ""},
		{"a nested subscript", "echo $a[$b[1]]", "a[$b[1]]", ""},
		// A substitution's own `(` is a word end everywhere else, so meeting
		// one here used to give the subscript up and leave the brackets as
		// text beside the *whole* array. Both spellings, because the
		// arithmetic one is the two characters `$(` as well (#2048).
		{"a subscript holding an arithmetic substitution", "echo $a[$((i))]", "a[$((i))]", ""},
		{"a subscript holding a command substitution", "echo $a[$(echo 1)]", "a[$(echo 1)]", ""},
		{"a subscript holding a backquoted substitution", "echo $a[`echo 1`]", "a[`echo 1`]", ""},
		// A substitution suspends the word-end test and not the bracket
		// count: the blank in `echo 3` no longer ends the word, and a `[`
		// written inside still has to be closed. Measured on zsh 5.9.2 —
		// `x=$a[$(echo 2; : [ ])]` is the second element.
		{"a balanced bracket inside the substitution", "echo $a[$(echo 2; : [ ])]", "a[$(echo 2; : [ ])]", ""},
		{"text after a subscript holding a substitution", "echo $a[$(echo 1)]x", "a[$(echo 1)]", "x"},
		{"text after the subscript", "echo $a[1]x", "a[1]", "x"},
		{"only the first subscript", "echo $a[1][2]", "a[1]", "[2]"},
		{"the whole positional list", "echo $@[1]", "@[1]", ""},
		{"the joined positional list", "echo $*[2]", "*[2]", ""},
		{"a length through a subscript", "echo $#a[2]", "#a[2]", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, bare())
			if len(spans) == 0 || spans[0].Kind != syntax.ParamExp {
				t.Fatalf("%q: first span is not an expansion: %+v", tc.src, spans)
			}
			if spans[0].Value != tc.value {
				t.Errorf("%q: expansion is %q, want %q", tc.src, spans[0].Value, tc.value)
			}
			var rest string
			for _, s := range spans[1:] {
				rest += s.Value
			}
			if rest != tc.after {
				t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
			}
		})
	}
}

// Without the flag the same words end at the name, and the brackets are text.
//
// The pair is the point: a grammar that read the subscript anyway would turn
// every `$dir[0-9]*` in a portable script into an element lookup, silently.
func TestWithoutTheFlagTheBracketsAreText(t *testing.T) {
	for _, tc := range []struct{ src, value, after string }{
		{"echo $a[1]", "a", "[1]"},
		{"echo $a[1,3]", "a", "[1,3]"},
		{"echo $#a", "#", "a"},
		{"echo $@[1]", "@", "[1]"},
	} {
		spans := spansOf(t, tc.src, syntax.Core())
		if len(spans) == 0 || spans[0].Value != tc.value {
			t.Fatalf("%q: expansion is %+v, want value %q", tc.src, spans, tc.value)
		}
		var rest string
		for _, s := range spans[1:] {
			rest += s.Value
		}
		if rest != tc.after {
			t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
		}
	}
}

// `$#name` is the length operator on the parameter that follows it, and the
// set of parameters it reaches has holes in it.
//
// Measured: `$#a` is a count, `$#0` and `$#1` are lengths, `$#@` and `$#*` are
// the number of positional parameters — and `$##` and `$#!` are the count and
// then a character of text, in the shell with the form as much as in the ones
// without it. A flag that read one more character than the shell does would
// differ only where nothing looks.
func TestABareLengthStopsWhereTheShellStops(t *testing.T) {
	for _, tc := range []struct{ src, value, after string }{
		{"echo $#a", "#a", ""},
		{"echo $#0", "#0", ""},
		{"echo $#1", "#1", ""},
		{"echo $#@", "#@", ""},
		{"echo $#*", "#*", ""},
		{"echo $#", "#", ""},
		{"echo $##", "#", "#"},
		{"echo $#!", "#", "!"},
		{"echo $#[1]", "#", "[1]"},
	} {
		spans := spansOf(t, tc.src, bare())
		if len(spans) == 0 || spans[0].Value != tc.value {
			t.Fatalf("%q: expansion is %+v, want value %q", tc.src, spans, tc.value)
		}
		var rest string
		for _, s := range spans[1:] {
			rest += s.Value
		}
		if rest != tc.after {
			t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
		}
	}
}

// A positional digit takes no subscript, in the grammar that has them.
//
// Measured on the shell: `set -- abcd; echo $1[2]` prints `abcd[2]` there as
// it does everywhere else. So the parameters that carry one are not simply all
// of them, and this is the row that keeps the flag from granting the form to
// every `$`.
func TestAPositionalDigitTakesNoBareSubscript(t *testing.T) {
	for _, src := range []string{"echo $1[2]", "echo $9[2]"} {
		spans := spansOf(t, src, bare())
		if len(spans) != 2 || spans[1].Value != "[2]" {
			t.Errorf("%q: spans are %+v, want the brackets left as text", src, spans)
		}
	}
}

// A subscript that never closes is not one, and its characters stay literal.
//
// The word decides how far the scan may look: unquoted it ends where the word
// does, and inside double quotes a blank is ordinary text, so the same
// characters are a subscript in one and not in the other. A closing quote is
// as far as the scan may reach either way — `"$a[" ]` has a `]` in it and no
// subscript, which is what the shell says too: it reports an invalid subscript
// rather than reading across the quote.
func TestAnUnclosedBareSubscriptIsText(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		value string
		after string
	}{
		{"the word ends first", "echo $a[1", "a", "[1"},
		{"a blank ends the word", "echo $a[1 ]", "a", "[1"},
		{"a blank inside quotes does not", `echo "$a[1 ]"`, "a[1 ]", ""},
		{"the quotes end first", `echo "$a["`, "a", "["},
		{"and a later bracket is not its own", `echo "$a[" ]`, "a", "["},
		{"a newline ends the word too", "echo $a[1\n", "a", "[1"},
		{"an operator ends the word", "echo $a[1<f", "a", "[1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, bare())
			if len(spans) == 0 || spans[0].Value != tc.value {
				t.Fatalf("%q: expansion is %+v, want value %q", tc.src, spans, tc.value)
			}
			var rest string
			for _, s := range spans[1:] {
				rest += s.Value
			}
			if rest != tc.after {
				t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
			}
		})
	}
}

// A newline is ordinary text inside double quotes, and so is part of a
// subscript there.
//
// The pair with the unquoted case above, and the reason it is a case at all:
// the scan had a rule stopping it at every newline, which is right for the
// unquoted half — the word has ended by then — and wrong for this one. The
// shell reads `"$a[1\n]"` as the first element, and nothing here noticed until
// the stop was mutated away and produced a *better* shell.
func TestAQuotedSubscriptMaySpanALine(t *testing.T) {
	spans := spansOf(t, "echo \"$a[1\n]\"", bare())
	if len(spans) != 1 || spans[0].Value != "a[1\n]" {
		t.Errorf("spans are %+v, want one expansion of `a[1\n]`", spans)
	}
}

// The span a bare subscript produces is the span the braced spelling produces,
// which is what keeps the parser, the interpreter and the printer out of it.
func TestABareSubscriptParsesAsTheBracedSpelling(t *testing.T) {
	for _, tc := range []struct{ bareSrc, braced string }{
		{"echo $a[1]", "echo ${a[1]}"},
		{"echo $#a", "echo ${#a}"},
		{"echo $#a[2]", "echo ${#a[2]}"},
		{`echo "$a[1]"`, `echo "${a[1]}"`},
	} {
		got, want := spansOf(t, tc.bareSrc, bare()), spansOf(t, tc.braced, bare())
		if len(got) != 1 || len(want) != 1 || got[0].Value != want[0].Value {
			t.Fatalf("%q gives %+v, %q gives %+v", tc.bareSrc, got, tc.braced, want)
		}
		p := got[0].Param
		if p == nil {
			t.Fatalf("%q: the expansion was not parsed", tc.bareSrc)
		}
		if q := want[0].Param; q == nil || p.Name != q.Name || p.Length != q.Length {
			t.Errorf("%q parsed to %+v, %q to %+v", tc.bareSrc, p, tc.braced, q)
		}
	}
}

// A tree holding a bare subscript prints back as source that parses to the
// same tree — the braced spelling, since the printer has no dialect and the
// braces are correct in every grammar that has subscripts at all.
func TestPrintingABareSubscript(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo $a[1]", "echo ${a[1]}"},
		{"echo $#a", "echo ${#a}"},
		{"echo $a[1]x", "echo ${a[1]}x"},
		{"echo $#a[2]", "echo ${#a[2]}"},
		{"echo $##", "echo $##"},
	} {
		f, err := syntax.Parse(tc.src, bare())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		got := syntax.Print(f)
		if got != tc.want {
			t.Errorf("printing %q gave %q, want %q", tc.src, got, tc.want)
		}
		again, err := syntax.Parse(got, bare())
		if err != nil {
			t.Fatalf("reparse %q: %v", got, err)
		}
		if second := syntax.Print(again); second != got {
			t.Errorf("printing %q again gave %q", got, second)
		}
	}
}

// The subscript an unbraced expansion carries is recorded twice: as a
// subscript, and as the text it would be if nothing read it as one. Which of
// the two a run takes is a semantics axis and not the grammar's, so the
// grammar keeps both.
//
// Only the unbraced spelling has the second reading. `${a[1]}` cannot mean
// anything but a subscript — the braces say where the expansion ends — which
// is why the field is nil there and why the field is what tells the two
// spellings apart once they are parsed.
func TestABareSubscriptIsKeptAsTextAsWell(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// The spans the bracket text was lexed into, as `kind:value`. nil
		// says the field itself must be absent.
		text []string
	}{
		{"an element", "echo $a[1]", []string{"lit:[1]"}},
		{"a range", "echo $a[1,3]", []string{"lit:[1,3]"}},
		{
			"a subscript holding an expansion", "echo $a[$i]",
			[]string{"lit:[", "param:i", "lit:]"},
		},
		{
			"a nested subscript", "echo $a[$b[1]]",
			[]string{"lit:[", "param:b[1]", "lit:]"},
		},
		{"only the first subscript", "echo $a[1][2]", []string{"lit:[1]"}},
		{"the braced spelling has no second reading", "echo ${a[1]}", nil},
		{"a name with no subscript at all", "echo $a", nil},
		{"a bracket that never closed", "echo $a[1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, bare())
			e := spans[0].Param
			if e == nil {
				t.Fatalf("%q: the first span is not an expansion", tc.src)
			}
			if tc.text == nil {
				if e.BareIndexText != nil {
					t.Errorf("%q: BareIndexText set, want nil", tc.src)
				}
				return
			}
			if e.BareIndexText == nil {
				t.Fatalf("%q: BareIndexText nil, want %v", tc.src, tc.text)
			}
			var got []string
			for _, s := range e.BareIndexText.Spans {
				kind := "lit"
				if s.Kind == syntax.ParamExp {
					kind = "param"
				}
				got = append(got, kind+":"+s.Value)
			}
			if !slices.Equal(got, tc.text) {
				t.Errorf("%q: BareIndexText spans %v, want %v", tc.src, got, tc.text)
			}
		})
	}
}

// The text reading is lexed in the expansion's own quoting, so what stands
// between the brackets is performed exactly as the word around it would
// perform it. Inside double quotes the spans are the quoted ones.
func TestABareSubscriptsTextKeepsTheExpansionsQuoting(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want syntax.Quoting
	}{
		{"unquoted", "echo $a[$i]", syntax.Unquoted},
		{"double quoted", `echo "$a[$i]"`, syntax.DoubleQuoted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := spansOf(t, tc.src, bare())[0].Param
			if e == nil || e.BareIndexText == nil {
				t.Fatalf("%q: no bracket text", tc.src)
			}
			for i, s := range e.BareIndexText.Spans {
				if s.Quoting != tc.want {
					t.Errorf("%q: span %d quoting %v, want %v", tc.src, i, s.Quoting, tc.want)
				}
			}
		})
	}
}

// A subscript a substitution leaves unbalanced is no subscript, and the
// characters go back to the word.
//
// The substitution suspends the word-end test and not the bracket count, so
// a `[` written inside it still has to be closed and a `]` written inside it
// still closes — and where either leaves the brackets unusable, the scan gives
// them up rather than committing. Measured on zsh 5.9.2 with
// `a=(one two three)`: both of these come back as the array joined with the
// brackets behind it as text, where `x=$a[$(echo 2; : [ ])]` — the same shape
// with the bracket closed — is the second element.
func TestAnUnbalancedSubstitutionGivesTheBracketsBack(t *testing.T) {
	for _, src := range []string{
		`echo $a[$(: ]; echo 2)]`, // the `]` inside closes the subscript
		`echo $a[$(echo 2; : [)]`, // the `[` inside is never closed
	} {
		spans := spansOf(t, src, bare())
		if len(spans) == 0 || spans[0].Kind != syntax.ParamExp {
			t.Fatalf("%q: first span is not an expansion: %+v", src, spans)
		}
		if spans[0].Value != "a" {
			t.Errorf("%q: expansion is %q, want %q — the brackets are not a subscript here", src, spans[0].Value, "a")
		}
		if len(spans) < 2 || spans[1].Kind != syntax.Literal || spans[1].Value != "[" {
			t.Errorf("%q: after the expansion comes %+v, want the literal `[`", src, spans[1:])
		}
	}
}
