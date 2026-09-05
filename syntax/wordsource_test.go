// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Naming a word, and naming the part of it that shares one expansion's
// quoting. Both exist for a diagnostic: a grammar that blames the word an
// unreadable expansion sits in has to be able to say the word, and by then it
// is a list of spans.

func firstArg(t *testing.T, src string, d Dialect) *Word {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	cmd, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%q: not a simple command", src)
	}
	if len(cmd.Args) < 2 {
		t.Fatalf("%q: no operand to name", src)
	}
	return cmd.Args[1]
}

func TestPrintWordGivesTheWordBack(t *testing.T) {
	d := transformDialect()
	for _, c := range []struct{ src, want string }{
		{`echo "[${x@QQ}]"`, `"[${x@QQ}]"`},
		{`echo pre${x@QQ}post`, `pre${x@QQ}post`},
		{`echo 'lit'"${x@QQ}"`, `'lit'"${x@QQ}"`},
		{`echo "${x@QQ}"$(f)`, `"${x@QQ}"$(f)`},
	} {
		if got := PrintWord(firstArg(t, c.src, d)); got != c.want {
			t.Errorf("%q gave %q, want %q", c.src, got, c.want)
		}
	}
	if got := PrintWord(nil); got != "" {
		t.Errorf("a nil word gave %q, want nothing", got)
	}
}

// The run is bounded by a change of quoting and by nothing else, so a word
// written in one pair of quotes answers whole and a word that changes quoting
// mid-way answers with the part around the index asked for.
func TestPrintWordQuotingRunStopsAtAChangeOfQuoting(t *testing.T) {
	d := transformDialect()
	for _, c := range []struct {
		src  string
		span int
		want string
	}{
		// One quoting throughout: the whole word, without its quotes.
		{`echo "[${x@QQ}]"`, 1, `[${x@QQ}]`},
		{`echo pre${x@QQ}post`, 1, `pre${x@QQ}post`},
		{`echo "$(f)${x@QQ}"`, 1, `$(f)${x@QQ}`},
		// A change of quoting ends it on either side.
		{`echo 'lit'"${x@QQ}"`, 1, `${x@QQ}`},
		{`echo "${x@QQ}"'lit'`, 0, `${x@QQ}`},
		{`echo $y"${x@QQ}"`, 1, `${x@QQ}`},
		// Literal text goes in as it stands: there are no quotes around the
		// result to protect anything from.
		{`echo "\"${x@QQ}\""`, 1, `"${x@QQ}"`},
		{`echo "*${x@QQ}"`, 1, `*${x@QQ}`},
	} {
		if got := PrintWordQuotingRun(firstArg(t, c.src, d), c.span); got != c.want {
			t.Errorf("%q span %d gave %q, want %q", c.src, c.span, got, c.want)
		}
	}
	if got := PrintWordQuotingRun(nil, 0); got != "" {
		t.Errorf("a nil word gave %q, want nothing", got)
	}
	w := firstArg(t, `echo "[${x@QQ}]"`, d)
	if got := PrintWordQuotingRun(w, len(w.Spans)); got != "" {
		t.Errorf("an index past the end gave %q, want nothing", got)
	}
}

// BadTransform narrows Bad to the `@` family, and only where the grammar has
// the family: without the flag the whole construct is unknown rather than the
// letter, which is a different failure and carries a different status.
func TestBadTransformMarksOnlyTheFamilyAndOnlyWhereItExists(t *testing.T) {
	for _, c := range []struct {
		src        string
		withFamily bool
		want       bool
	}{
		{`echo ${x@QQ}`, true, true},
		{`echo ${x@}`, true, true},
		{`echo ${x@ Q}`, true, true},
		{`echo ${a[0]@QQ}`, true, true},
		// The same spelling where the grammar has no family at all.
		{`echo ${x@Q}`, false, false},
		// Not the family: an operator nothing could read.
		{`echo ${x ~}`, true, false},
		{`echo ${x ~}`, false, false},
	} {
		d := Core()
		d.ArraySubscript = true
		d.ParamTransformations = c.withFamily
		f, err := Parse(c.src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", c.src, err)
		}
		var found *ParamExpr
		walkParams(f, func(e *ParamExpr) { found = e })
		if found == nil {
			t.Fatalf("%q: no ParamExpr in the tree", c.src)
		}
		if !found.Bad {
			t.Fatalf("%q with the family=%v: not marked bad", c.src, c.withFamily)
		}
		if found.BadTransform != c.want {
			t.Errorf("%q with the family=%v: BadTransform %v, want %v",
				c.src, c.withFamily, found.BadTransform, c.want)
		}
	}
}
