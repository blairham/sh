// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// What a prefix does to a reserved word behind it: an assignment takes the
// reading away in most columns and one keeps it, and a redirection takes it
// away nowhere — two of the panel read a compound command behind one. These
// name the flags and never a shell.

// prefixDialect is the core with every construct the rows below need, so a
// reserved word is reserved for the reason the flag says and not because the
// grammar happens to lack it.
func prefixDialect() syntax.Dialect {
	d := syntax.Core()
	d.DoubleBracket = true
	d.Select = true
	d.Repeat = true
	d.Foreach = true
	d.Coproc = true
	d.FunctionKeyword = true
	d.TimeKeyword = true
	return d
}

// A written-out reserved word behind an assignment prefix: kept, so the
// complaint lands on the word, or dropped, so it lands on whatever closes the
// construct it opened.
func TestWhetherAReservedWordStandsBehindAnAssignmentPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, named string }{
		{"v=x { :; }", "{"},
		{"v=x while :; do :; done", "while"},
		{"v=x if :; then :; fi", "if"},
		{"v=x for i in a; do :; done", "for"},
		{"v=x case a in a) :;; esac", "case"},
		{"v=x function f { :; }", "function"},
		{"v=x select i in a; do :; done", "select"},
		{"v=x until false; do :; done", "until"},
		{"v=x repeat 2 do :; done", "repeat"},
		{"v=x foreach i (a); :; end", "foreach"},
		{"v=x [[ -n a ]]", "[["},
		{"v=x time :", "time"},
		{"v=x !", "!"},
		{"v=x then", "then"},
		{"v=x end", "end"},
		{"v=x coproc cat", "coproc"},
	} {
		d := prefixDialect()
		d.ReservedWordStandsBehindAnAssignmentPrefix = true
		_, err := syntax.Parse(c.src, d)
		if err == nil {
			t.Errorf("kept: %q parsed, want it refused", c.src)
			continue
		}
		var se *syntax.Error
		if !asSyntaxError(err, &se) {
			t.Fatalf("%q: %v", c.src, err)
		}
		if se.Token != c.named {
			t.Errorf("kept: %q names %q, want %q", c.src, se.Token, c.named)
		}
		// Dropped, the word is an ordinary command name: either the line
		// parses as a command with arguments, or the complaint lands
		// somewhere other than the word itself.
		d.ReservedWordStandsBehindAnAssignmentPrefix = false
		_, err = syntax.Parse(c.src, d)
		if err == nil {
			continue
		}
		if !asSyntaxError(err, &se) {
			t.Fatalf("%q: %v", c.src, err)
		}
		if se.Token == c.named {
			t.Errorf("dropped: %q still names %q", c.src, c.named)
		}
	}
}

// Two words are **not** in the set, and they are the controls. `in` is special
// inside `for` and `case` and an ordinary command name where a command begins;
// `(` is an operator rather than a word, so no reading was ever taken from it
// and every column refuses it alike.
func TestWhatAnAssignmentPrefixNeverTakesTheReadingFrom(t *testing.T) {
	t.Parallel()
	d := prefixDialect()
	d.ReservedWordStandsBehindAnAssignmentPrefix = true
	if _, err := syntax.Parse("v=x in", d); err != nil {
		t.Errorf("`v=x in`: %v, want a command called in", err)
	}
	if _, err := syntax.Parse(`v=x "if"`, d); err != nil {
		t.Errorf("a quoted word: %v, want the reservation removed by the quotes", err)
	}
	for _, src := range []string{"v=x ( : )", "v=x echo hi"} {
		before := parseFails(t, src, d)
		d.ReservedWordStandsBehindAnAssignmentPrefix = false
		after := parseFails(t, src, d)
		d.ReservedWordStandsBehindAnAssignmentPrefix = true
		if before != after {
			t.Errorf("%q moved with the flag: %v against %v", src, before, after)
		}
	}
}

// A `}` behind an assignment prefix *ends* the command rather than standing
// behind it, where the dialect reserves the word everywhere: `{ a+=( $p ) }`
// is a brace body and the assignment is its last statement.
func TestAClosingBraceBehindAnAssignmentPrefixStillEndsTheCommand(t *testing.T) {
	t.Parallel()
	d := prefixDialect()
	d.ReservedWordStandsBehindAnAssignmentPrefix = true
	d.CloseBraceAlwaysReserved = true
	if _, err := syntax.Parse("{ v=x }", d); err != nil {
		t.Errorf("`{ v=x }`: %v, want the brace to close the body", err)
	}
	if _, err := syntax.Parse("v=x }", d); err == nil {
		t.Error("`v=x }` parsed, want the stray brace refused")
	}
}

// A redirection in front of a compound command: refused, taken before a
// parenthesized one, or taken before every compound the dialect has.
func TestWhichCompoundsMayStandBehindARedirection(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src           string
		parenthesized bool
		anyCompound   bool
	}{
		{">/dev/null ( echo hi )", true, true},
		{">/dev/null (( 1 ))", true, true},
		{">/dev/null { echo hi; }", false, true},
		{">/dev/null while false; do :; done", false, true},
		{">/dev/null if true; then echo hi; fi", false, true},
		{">/dev/null for i in a; do echo hi; done", false, true},
		{">/dev/null case a in a) echo hi;; esac", false, true},
		{">/dev/null until true; do :; done", false, true},
		{">/dev/null select i in a; do break; done", false, true},
		{">/dev/null repeat 2 do echo hi; done", false, true},
		{">/dev/null foreach i (a); echo hi; end", false, true},
	} {
		for _, p := range []struct {
			policy syntax.RedirectionBeforeACompoundPolicy
			want   bool
		}{
			{syntax.RedirectionBeforeACompoundIsRefused, false},
			{syntax.RedirectionMayPrecedeAParenthesizedCommand, c.parenthesized},
			{syntax.RedirectionMayPrecedeAnyCompoundCommand, c.anyCompound},
		} {
			d := prefixDialect()
			d.RedirectionBeforeACompound = p.policy
			_, err := syntax.Parse(c.src, d)
			if (err == nil) != p.want {
				t.Errorf("%v: %q err=%v, want accepted=%v", p.policy, c.src, err, p.want)
			}
		}
	}
}

// And the redirections are the command's, in front of whatever it carries of
// its own — the order they were written in, which is the order they are
// applied in. The canonical printer writes them all after the command, as it
// already does for a simple command's leading ones; the formatter writes them
// back where they stood.
func TestARedirectionInFrontOfACompoundBelongsToIt(t *testing.T) {
	t.Parallel()
	d := prefixDialect()
	d.RedirectionBeforeACompound = syntax.RedirectionMayPrecedeAnyCompoundCommand
	f, err := syntax.Parse(">a 2>b { echo hi; } >c", d)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := syntax.Print(f), "{ echo hi; } > a 2> b > c"; got != want {
		t.Errorf("printed %q, want %q", got, want)
	}
}

// An assignment prefix takes the reading away from the compound too, in both
// columns that take a redirection there — which is why the question is asked
// only where nothing but redirections has been read.
func TestAnAssignmentPrefixTakesTheCompoundBackFromARedirection(t *testing.T) {
	t.Parallel()
	for _, policy := range []syntax.RedirectionBeforeACompoundPolicy{
		syntax.RedirectionMayPrecedeAParenthesizedCommand,
		syntax.RedirectionMayPrecedeAnyCompoundCommand,
	} {
		d := prefixDialect()
		d.RedirectionBeforeACompound = policy
		for _, src := range []string{"v=x >/dev/null ( echo hi )", ">/dev/null v=x ( echo hi )"} {
			if _, err := syntax.Parse(src, d); err == nil {
				t.Errorf("%v: %q parsed, want it refused", policy, src)
			}
		}
	}
}

// The words that are reserved and are *not* compound commands have nowhere to
// stand behind a redirection, in the column that keeps the reading there.
func TestAReservedWordThatIsNotACompoundIsRefusedBehindARedirection(t *testing.T) {
	t.Parallel()
	d := prefixDialect()
	d.RedirectionBeforeACompound = syntax.RedirectionMayPrecedeAnyCompoundCommand
	for _, c := range []struct{ src, named string }{
		{">/dev/null then", "then"},
		{">/dev/null do", "do"},
		{">/dev/null fi", "fi"},
		{">/dev/null end", "end"},
		{">/dev/null ! false", "!"},
		{">/dev/null coproc cat", "coproc"},
	} {
		_, err := syntax.Parse(c.src, d)
		var se *syntax.Error
		if err == nil || !asSyntaxError(err, &se) {
			t.Errorf("%q: err=%v, want a refusal", c.src, err)
			continue
		}
		if se.Token != c.named {
			t.Errorf("%q names %q, want %q", c.src, se.Token, c.named)
		}
	}
	// And `time` is not one of them: it runs there, measured.
	if _, err := syntax.Parse(">/dev/null echo hi", d); err != nil {
		t.Errorf("an ordinary command behind a redirection: %v", err)
	}
}

func parseFails(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	if _, err := syntax.Parse(src, d); err != nil {
		return err.Error()
	}
	return ""
}
