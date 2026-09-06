// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A colon written before a trim is ignored here: `${v:#hel*}` is `${v#hel*}`.
//
// The third of three readings of the same six characters — an element
// exclusion in zsh, an arithmetic offset beginning `#hel*` in bash, which is
// a refusal, and the plain trim here. The first two were implemented and this
// one was not, so every snippet below was `#hel*: arithmetic syntax error`.
//
// Measured 2026-09-06 against ksh93u+ 2012-08-01.

// kshOut runs src through the ksh preset and returns everything it wrote.
func kshOut(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		return "parse: " + err.Error(), -1
	}
	var out bytes.Buffer
	s, d := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &s, Diagnostics: &d, Name: "ksh", Dialect: presetDialect()}
	ksh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return out.String(), st
}

// All four trims, and the shortest-against-longest distinction has to survive
// the colon — which is what says the colon is dropped and the operator scanned
// again, rather than the four being named over somewhere else.
func TestAColonBeforeATrimIsIgnored(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`v=hello; printf "[%s]" "${v:#hel*}"`, `[lo]`},
		{`v=hello; printf "[%s]" "${v:##hel*}"`, `[]`},
		{`v=hello; printf "[%s]" "${v:%lo}"`, `[hel]`},
		{`v=hello; printf "[%s]" "${v:%%l*}"`, `[he]`},
		// A pattern that does not match leaves the value, which is the trim's
		// own rule and not a swallowed error.
		{`v=hello; printf "[%s]" "${v:#l}"`, `[hello]`},
		// An empty pattern trims nothing. It is the plain `${v#}` rather than
		// a special case, and a reading that treated an absent pattern as `*`
		// would empty the value here.
		{`v=hello; printf "[%s]" "${v:#}"`, `[hello]`},
		// The pattern may come out of a word, so it is built from the operand
		// rather than from its text.
		{`v=hello; p=hel; printf "[%s]" "${v:#$p*}"`, `[lo]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The array forms, which come for free because the node *is* the trim: the
// per-element mapping under `[@]` and the single element under an index are
// the ones the plain spelling already had.
//
// `${a:#2}` answering `1` is this shell's other divergence showing through as
// well — a bare `$a` is `${a[0]}` here, so the subject is `1` and `#2` trims
// nothing from it.
func TestAColonBeforeATrimReachesTheArrayForms(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(1 2 3); printf "[%s]" ${a:#2}`, `[1]`},
		{`a=(foo bar baz); printf "[%s]" ${a:#ba*}`, `[foo]`},
		{`a=(foo bar baz); printf "[%s]" "${a[@]:#ba*}"`, `[foo][r][z]`},
		{`a=(foo bar baz); printf "[%s]" "${a[1]:#ba*}"`, `[r]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// Only the four trims. Everything else after a colon keeps the reading it had,
// which is the boundary that stops this from becoming "a colon means nothing".
func TestOnlyATrimSwallowsTheColon(t *testing.T) {
	// Still an offset, and still a length after a second colon.
	for _, c := range []struct{ src, want string }{
		{`v=hello; printf "[%s]" "${v:2}"`, `[llo]`},
		{`v=hello; printf "[%s]" "${v:2:2}"`, `[ll]`},
		{`v=hello; printf "[%s]" "${v:-d}"`, `[hello]`},
		{`v=; printf "[%s]" "${v:-d}"`, `[d]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
	// And still an arithmetic refusal for the operators the colon does not
	// cover. Asserted as a nonzero status with the offending token named,
	// because "it did not produce the trimmed value" would also pass against
	// a silent empty answer.
	for _, c := range []struct{ src, token string }{
		{`v=hello; printf "[%s]" "${v:/l/L}"`, "/l/L"},
		{`v=hello; printf "[%s]" "${v:^^}"`, "^^"},
		// A space between the colon and the `#` is not the form: the
		// disambiguation is the character immediately after the colon.
		{`v=hello; printf "[%s]" "${v: #hel*}"`, "#hel*"},
	} {
		out, st := kshOut(t, c.src)
		if st == 0 {
			t.Errorf("%s = %q at status 0, want a refusal", c.src, out)
		}
		if !bytes.Contains([]byte(out), []byte(c.token)) {
			t.Errorf("%s = %q, want the refusal to name %q", c.src, out, c.token)
		}
	}
}

// The flag is the whole of it, and it is the grammar's rather than a
// semantics answer: without it the same source is the arithmetic reading.
func TestTheColonTrimIsAGrammarFlag(t *testing.T) {
	if !ksh.Dialect().ParamColonBeforeTrimIsIgnored {
		t.Error("ParamColonBeforeTrimIsIgnored = false, want true")
	}
	// And this shell does not have the other reading of the same characters,
	// so the pair is never both on.
	if ksh.Dialect().ParamElementSelection {
		t.Error("ParamElementSelection = true, want false")
	}
}
