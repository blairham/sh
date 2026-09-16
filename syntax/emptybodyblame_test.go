// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// blamingTheTokenAfter is the pair one dialect needs: the style, and the set
// of terminators it applies to.
func blamingTheTokenAfter() Dialect {
	d := Core()
	d.EmptyBodyBlame = BlameTheTokenAfterIt
	d.EmptyBodyBlamed = map[Kind]bool{TokAmp: true}
	return d
}

// blamingTheKeywordAfter is the other style, over the other set — and with
// the flag that lets a body be empty, because the dialect that answers this
// way has it and the two must not be confused for one another.
func blamingTheKeywordAfter() Dialect {
	d := Core()
	d.EmptyCompoundBody = true
	d.EmptyBodyBlame = BlameTheKeywordAfterIt
	d.EmptyBodyBlamed = map[Kind]bool{
		TokPipe: true, TokPipeAmp: true, TokAndAnd: true, TokOrOr: true,
	}
	return d
}

// A terminator standing where a body or a condition must have something in
// it is named where it stands by default, and two styles name what is behind
// it instead. See [Dialect.EmptyBodyBlame] for the measured table (#2235).
func TestWhichTokenAnEmptyBodyIsBlamedOn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		dialect Dialect
		src     string
		want    string
	}{
		// The token after it, one token on and whatever kind it is.
		{"a keyword after the terminator", blamingTheTokenAfter(), "if & then :; fi\n", "then"},
		{"a loop's keyword", blamingTheTokenAfter(), "while & do :; done\n", "do"},
		{"the construct's own closer", blamingTheTokenAfter(), "if & fi\n", "fi"},
		{"a brace group's closer", blamingTheTokenAfter(), "{ & }\n", "}"},
		{"a subshell's closer", blamingTheTokenAfter(), "( & )\n", ")"},
		{"a separator after it", blamingTheTokenAfter(), "if & ; then :; fi\n", ";"},
		{"an operator after it", blamingTheTokenAfter(), "if & || :; then :; fi\n", "||"},
		// A command could begin there, so the shell would have taken the
		// line and there is no token behind the terminator to name.
		{"a command after it", blamingTheTokenAfter(), "{ & :; }\n", "&"},
		// And a newline, which is the one place the shell this was measured
		// from takes the line outright.
		{"a newline after it", blamingTheTokenAfter(), "if &\nthen :; fi\n", "&"},
		// A terminator outside the set is named where it stands.
		{"a terminator outside the set", blamingTheTokenAfter(), "if | :; then :; fi\n", "|"},

		// A condition is read forward to the keyword that ends its header,
		// however much stands in between.
		{"the keyword a condition ends at", blamingTheKeywordAfter(), "if | :; then :; fi\n", "then"},
		{"two statements in between", blamingTheKeywordAfter(), "if | :; :; then :; fi\n", "then"},
		{"a group in between", blamingTheKeywordAfter(), "if | { :; }; then :; fi\n", "then"},
		{"a loop's keyword", blamingTheKeywordAfter(), "while | :; do :; done\n", "do"},
		{"the construct's own closer", blamingTheKeywordAfter(), "if | :; fi\n", "fi"},
		{"a separator stepped over", blamingTheKeywordAfter(), "if | ; then :; fi\n", "then"},
		{"a newline stepped over", blamingTheKeywordAfter(), "if |\nthen :; fi\n", "then"},
		{"the and-or operators too", blamingTheKeywordAfter(), "if && then :; fi\n", "then"},
		// A body is named at a keyword standing next, and at the operator
		// otherwise — which is what parts it from a condition.
		{"a keyword next in a body", blamingTheKeywordAfter(), "if :; then | fi\n", "fi"},
		{"a command next in a body", blamingTheKeywordAfter(), "while :; do | :; done\n", "|"},
		// The closing brace is the reserved word this style never names.
		{"a brace group's closer", blamingTheKeywordAfter(), "{ | }\n", "|"},
		// And what is not a reserved word at all.
		{"a subshell's closer", blamingTheKeywordAfter(), "( | )\n", "|"},
		{"a command after it", blamingTheKeywordAfter(), "{ | :; }\n", "|"},
		{"a terminator outside the set", blamingTheKeywordAfter(), "if & then :; fi\n", "&"},

		// Saying nothing names the terminator, everywhere.
		{"nothing said, an ampersand", Core(), "if & then :; fi\n", "&"},
		{"nothing said, a bar", Core(), "if | :; then :; fi\n", "|"},
	} {
		t.Run(tc.name+"/"+tc.src, func(t *testing.T) {
			_, err := Parse(tc.src, tc.dialect)
			if err == nil {
				t.Fatalf("%q parsed, want a refusal naming %q", tc.src, tc.want)
			}
			se, ok := err.(*Error)
			if !ok {
				t.Fatalf("%q: %v is not a parse error", tc.src, err)
			}
			if se.Token != tc.want {
				t.Errorf("%q: named %q, want %q", tc.src, se.Token, tc.want)
			}
		})
	}
}

// A list that may legitimately be empty is not refused by any of this: the
// style only ever looks at a terminator in its own set, and a `case` arm's
// body, a command substitution's program and a `;;` in either position are
// none of them.
func TestAnEmptyListTheGrammarAllowsIsStillAllowed(t *testing.T) {
	t.Parallel()
	for _, d := range []Dialect{blamingTheTokenAfter(), blamingTheKeywordAfter()} {
		for _, src := range []string{
			"case x in x) ;; esac\n",
			"x=$( )\n",
			"case x in x) : ;; esac\n",
		} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("%q: %v", src, err)
			}
		}
	}
}
