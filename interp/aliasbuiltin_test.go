// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// aliasRun runs src with the panel's shared answers, so a test that cares
// about one axis says only that one.
func aliasRunArgs(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string, params ...string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.AliasParsesOptions = Yes
	sem.AliasHasPrintOption = Yes
	sem.AliasReportsNotFound = Yes
	sem.UnaliasReportsNotFound = Yes
	sem.AliasNotFoundStatusCounts = No
	sem.UnaliasAllRefusesOperands = No
	sem.AliasQuoting = ListingQuoteAlwaysEscaped
	sem.BadOptionToSpecialBuiltinFatal = No
	if tweak != nil {
		tweak(&sem)
	}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh", Params: params})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// aliasRun is aliasRunArgs with no positional parameters.
func aliasRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	return aliasRunArgs(t, tweak, dg, src)
}

// The table holds what was put in it, and listing gives it back.
func TestAliasKeepsWhatItWasGiven(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`alias a='echo x'; alias a`, "a='echo x'"},
		{`alias a=1 b=2; alias b`, "b='2'"},
		// A later definition replaces the earlier one rather than adding to it.
		{`alias a=1; alias a=2; alias a`, "a='2'"},
		// An empty value is a definition, not a lookup.
		{`alias a=; alias a; echo "st=$?"`, "a=''"},
	} {
		if out, _ := aliasRun(t, nil, Diagnostics{}, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, out, c.want)
		}
	}
}

// One dialect writes `alias ` in front of every line so the listing reads back
// as commands, and the other three write only the assignment.
func TestTheListingPrefixIsTheDialects(t *testing.T) {
	prefixed := Diagnostics{AliasListPrefix: "alias "}
	for _, src := range []string{`alias a=1; alias a`, `alias a=1; alias`} {
		out, _ := aliasRun(t, nil, prefixed, src)
		if !strings.HasPrefix(out, "alias a='1'") {
			t.Errorf("%s: said %q, want the dialect's prefix", src, out)
		}
	}
	// And a dialect with none gets none — the assignment alone.
	for _, src := range []string{`alias a=1; alias a`, `alias a=1; alias`} {
		out, _ := aliasRun(t, nil, Diagnostics{}, src)
		if !strings.HasPrefix(out, "a='1'") {
			t.Errorf("%s: said %q, want no prefix", src, out)
		}
	}
}

// Listing with no operand gives every entry, in name order rather than in the
// order they were defined — which is the only order a map can promise.
func TestListingIsSortedAndComplete(t *testing.T) {
	out, _ := aliasRun(t, nil, Diagnostics{}, `alias c=3 a=1 b=2; alias`)
	want := "a='1'\nb='2'\nc='3'\n"
	if out != want {
		t.Errorf("said %q, want %q", out, want)
	}
	// And an empty table lists nothing at all rather than a blank line.
	if out, _ := aliasRun(t, nil, Diagnostics{}, `alias`); out != "" {
		t.Errorf("said %q, want nothing from an empty table", out)
	}
}

// unalias removes one, and -a removes the lot.
func TestUnaliasRemoves(t *testing.T) {
	out, _ := aliasRun(t, nil, Diagnostics{}, `alias a=1 b=2; unalias a; alias`)
	if strings.Contains(out, "a=") || !strings.Contains(out, "b='2'") {
		t.Errorf("said %q, want a gone and b kept", out)
	}
	out, _ = aliasRun(t, nil, Diagnostics{}, `alias a=1 b=2; unalias -a; alias; echo "st=$?"`)
	if strings.Contains(out, "a=") || strings.Contains(out, "b=") {
		t.Errorf("said %q, want the table emptied", out)
	}
	if !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want -a to succeed", out)
	}
}

// A name the table does not hold is two questions — whether to speak, and in
// which words — and the panel does not pair the two builtins.
func TestANameTheTableDoesNotHold(t *testing.T) {
	dg := Diagnostics{
		AliasNotFound:   "%[1]s: %[2]s: not found",
		UnaliasNotFound: "%[1]s: %[2]s: gone",
	}
	out, _ := aliasRun(t, nil, dg, `alias nope; echo "st=$?"`)
	if !strings.Contains(out, "alias: nope: not found") || !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want the alias wording and status 1", out)
	}
	out, _ = aliasRun(t, nil, dg, `unalias nope; echo "st=$?"`)
	if !strings.Contains(out, "unalias: nope: gone") || !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want the unalias wording and status 1", out)
	}
	// Silence is per builtin: the split ksh93 has, and zsh's reverse of it.
	out, _ = aliasRun(t, func(s *Semantics) { s.AliasReportsNotFound = No }, dg,
		`alias nope; unalias nope; echo "st=$?"`)
	if strings.Contains(out, "not found") {
		t.Errorf("said %q, want `alias` silent", out)
	}
	if !strings.Contains(out, "gone") {
		t.Errorf("said %q, want `unalias` still speaking", out)
	}
	// And a silent refusal still reports 1.
	out, _ = aliasRun(t, func(s *Semantics) {
		s.AliasReportsNotFound, s.UnaliasReportsNotFound = No, No
	}, dg, `alias nope; echo "st=$?"`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want silence to still fail", out)
	}
}

// Two of the four write the not-found line with no shell and no line in front
// of it, which they do almost nowhere else.
//
// Asserting the wording alone could not see this: a Contains check matches the
// same text whether or not something precedes it, which is how a mutant that
// ignored the flag entirely survived.
func TestTheNotFoundLineMayCarryNoPrefix(t *testing.T) {
	prefixed := Diagnostics{
		AliasNotFound:   "alias: %[2]s: not found",
		UnaliasNotFound: "unalias: %[2]s: not found",
	}
	out, _ := aliasRun(t, nil, prefixed, `alias nope`)
	if !strings.HasPrefix(out, "testsh: alias: nope") {
		t.Errorf("said %q, want the shell named in front", out)
	}
	out, _ = aliasRun(t, nil, prefixed, `unalias nope`)
	if !strings.HasPrefix(out, "testsh: unalias: nope") {
		t.Errorf("said %q, want the shell named in front", out)
	}

	bare := prefixed
	bare.AliasNotFoundUnprefixed = true
	out, _ = aliasRun(t, nil, bare, `alias nope`)
	if !strings.HasPrefix(out, "alias: nope") {
		t.Errorf("said %q, want no shell in front", out)
	}
	// And the flag is per builtin: `unalias` still carries its prefix here.
	out, _ = aliasRun(t, nil, bare, `unalias nope`)
	if !strings.HasPrefix(out, "testsh: unalias: nope") {
		t.Errorf("said %q, want unalias unaffected by the alias flag", out)
	}
	bare.UnaliasNotFoundUnprefixed = true
	out, _ = aliasRun(t, nil, bare, `unalias nope`)
	if !strings.HasPrefix(out, "unalias: nope") {
		t.Errorf("said %q, want no shell in front", out)
	}
}

// One dialect answers with how many it could not find.
func TestTheStatusMayCountTheMissingNames(t *testing.T) {
	counts := func(s *Semantics) { s.AliasNotFoundStatusCounts = Yes }
	for _, c := range []struct {
		src  string
		want string
	}{
		{`alias n1; echo "st=$?"`, "st=1"},
		{`alias n1 n2; echo "st=$?"`, "st=2"},
		{`alias n1 n2 n3; echo "st=$?"`, "st=3"},
		// Only the ones it missed, not the ones it printed.
		{`alias a=1; alias a n1; echo "st=$?"`, "st=1"},
		// And nothing missing is still success.
		{`alias a=1; alias a; echo "st=$?"`, "st=0"},
	} {
		if out, _ := aliasRun(t, counts, Diagnostics{}, c.src); !strings.Contains(out, c.want) {
			t.Errorf("counting %s: said %q, want %q", c.src, out, c.want)
		}
	}
	// Without the axis it is 1 however many were missing.
	if out, _ := aliasRun(t, nil, Diagnostics{}, `alias n1 n2 n3; echo "st=$?"`); !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want a plain 1", out)
	}
}

// `unalias -a` with a name beside it: one dialect calls that too many
// arguments and clears nothing.
func TestUnaliasAllMayRefuseOperands(t *testing.T) {
	refuse := func(s *Semantics) { s.UnaliasAllRefusesOperands = Yes }
	out, _ := aliasRun(t, refuse, Diagnostics{UnaliasAllWithOperands: "too many arguments"},
		`alias a=1; unalias -a a; echo "st=$?"; alias`)
	if !strings.Contains(out, "too many arguments") {
		t.Errorf("said %q, want the refusal", out)
	}
	if !strings.Contains(out, "a='1'") {
		t.Errorf("said %q, want the table left alone", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want status 1", out)
	}
	// Where it is accepted the names are ignored and the table is emptied.
	out, _ = aliasRun(t, nil, Diagnostics{}, `alias a=1; unalias -a a; echo "st=$?"; alias`)
	if strings.Contains(out, "a='1'") || !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want the table emptied and success", out)
	}
	// A bare -a never reaches the question.
	out, _ = aliasRun(t, refuse, Diagnostics{UnaliasAllWithOperands: "too many arguments"},
		`alias a=1; unalias -a; alias; echo "st=$?"`)
	if strings.Contains(out, "too many") || strings.Contains(out, "a='1'") {
		t.Errorf("said %q, want a bare -a to empty the table", out)
	}
}

// Reading options at all is a separate question from having `-p`: one dialect
// reads none, so `-p` is a name it looks up and fails to find.
func TestReadingOptionsIsNotTheSameAsHavingOne(t *testing.T) {
	dg := Diagnostics{
		AliasNotFound:    "%[1]s: %[2]s: not found",
		BuiltinBadOption: "%[1]s: bad option: %[2]s",
		AliasListPrefix:  "",
	}
	// Reads none: `-p` is an operand, and an operand is a name.
	out, _ := aliasRun(t, func(s *Semantics) { s.AliasParsesOptions = No }, dg, `alias -p; echo "st=$?"`)
	if !strings.Contains(out, "alias: -p: not found") {
		t.Errorf("said %q, want -p looked up as a name", out)
	}
	// Reads options but has no `-p`: a refusal rather than a lookup.
	out, _ = aliasRun(t, func(s *Semantics) { s.AliasHasPrintOption = No }, dg, `alias -p; echo "st=$?"`)
	if !strings.Contains(out, "bad option: -p") {
		t.Errorf("said %q, want -p refused as an option", out)
	}
	if strings.Contains(out, "not found") {
		t.Errorf("said %q, want it not treated as a name", out)
	}
	// Has it: the listing gets the prefix even where the plain one has none.
	out, _ = aliasRun(t, nil, dg, `alias a=1; alias -p`)
	if !strings.Contains(out, "alias a='1'") {
		t.Errorf("said %q, want -p to prefix the listing", out)
	}
	// And the plain listing keeps the dialect's own prefix, which is none here.
	out, _ = aliasRun(t, nil, dg, `alias a=1; alias a`)
	if strings.Contains(out, "alias a=") {
		t.Errorf("said %q, want no prefix without -p", out)
	}
}

// `unalias` with no name and no -a: a usage line in two, silence in one, and
// a shortage of arguments in the fourth.
func TestUnaliasWithNothingToRemove(t *testing.T) {
	out, _ := aliasRun(t, nil, Diagnostics{
		UnaliasUsage: "unalias: usage: unalias [-a] name [name ...]",
	}, `unalias; echo "st=$?"`)
	if !strings.Contains(out, "usage: unalias") || !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want the usage line and status 2", out)
	}
	// A dialect that prints none reports success — nothing was wrong.
	out, _ = aliasRun(t, nil, Diagnostics{}, `unalias; echo "st=$?"`)
	if strings.Contains(out, "usage") || !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want silence and success", out)
	}
	// And the status is the dialect's where it has one.
	out, _ = aliasRun(t, nil, Diagnostics{
		UnaliasUsage: "not enough arguments", UnaliasNoOperandStatus: 1,
	}, `unalias; echo "st=$?"`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want the dialect's status", out)
	}
}

// aliasRunWithValue defines one alias holding value and returns the single
// line `alias` lists it as, with the trailing newline removed.
//
// The value goes in through a positional parameter rather than through the
// source, so a value holding quotes or a tab is stored exactly as written here
// and the test is about the *spelling* rather than about the parser.
func aliasRunWithValue(t *testing.T, style ListingQuotingStyle, value string) (string, int) {
	t.Helper()
	out, st := aliasRunArgs(t, func(s *Semantics) { s.AliasQuoting = style }, Diagnostics{},
		`alias a="$1"; alias a`, value)
	return strings.TrimRight(out, "\n"), st
}
