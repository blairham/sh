// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How a `type`-family sentence spells the name it is reporting about, and
// which names that family speaks for at all. Named for the fields rather than
// for the shells that answer them; the presets' picks are asserted in
// dialect/.

// TestANameReportQuotesTheOperandWhereTheDialectSaysSo is
// Diagnostics.NameReportQuoting: three columns write the operand exactly as
// it was given and one writes it back shell-quoted, using the same spelling
// its trace uses — which is why the alphabet is TraceMetacharacters read
// again rather than a second list.
func TestANameReportQuotesTheOperandWhereTheDialectSaysSo(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "*?[]{}~#", Leading: "="}
	for _, c := range []struct {
		operand, bare, quoted string
	}{
		{"nosuchcmd", "nosuchcmd", "nosuchcmd"},
		{"a b", "a b", "'a b'"},
		{"x]", "x]", "'x]'"},
		// The position rule carries over from the same alphabet: a leading
		// `=` counts and a trailing one does not.
		{"=ab", "=ab", "'=ab'"},
		{"ab=", "ab=", "ab="},
		// And so does the `$'…'` spelling for a control character.
		{"a\tb", "a\tb", `$'a\tb'`},
	} {
		for _, q := range []struct {
			quoting TraceQuoting
			want    string
		}{
			{QuoteNever, c.bare},
			{QuoteDollar, c.quoted},
		} {
			out, _ := run(t, `type "`+c.operand+`"`, func(r *Runner) {
				sem := permissive()
				dg := Diagnostics{
					TypeNotFound:        "type: %[1]s: not found",
					NameReportQuoting:   q.quoting,
					TraceMetacharacters: meta,
				}
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			want := "type: " + q.want + ": not found"
			if !strings.Contains(out, want) {
				t.Errorf("%q at %v: out = %q, want %q", c.operand, q.quoting, strings.TrimSpace(out), want)
			}
		}
	}
}

// TestAnAliasTheDialectDoesNotReportIsStillAnAlias is
// Runner.SetAliasNotReported: the `type` family declines to speak for the
// name while everything else about the alias stays exactly as it was.
func TestAnAliasTheDialectDoesNotReportIsStillAnAlias(t *testing.T) {
	hide := func(r *Runner) {
		sem := permissive()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		sem.TypeNamesAnAliasOnlyWhenExpanded = No
		sem.AliasParsesOptions = No
		sem.AliasQuoting = ListingQuoteAlwaysEscaped
		sem.AliasRemembersTheNamesItNames = No
		r.Semantics, r.Diagnostics = &sem, &dg
		r.SetAlias("hidden", "echo one")
		r.SetAlias("shown", "echo two")
		r.SetAliasNotReported("hidden")
	}
	for _, c := range []struct{ name, src, want string }{
		{"the marked name is not accounted for", `type hidden`, "type: hidden: not found"},
		{"its neighbor is", `type shown`, "shown is an alias for echo two"},
		{"`command -V` agrees", `command -V hidden`, "command: hidden: not found"},
		// The alias itself is untouched: the listing has it, and a removal
		// removes it.
		{"the listing still holds it", `alias hidden`, "hidden='echo one'"},
		// A new value keeps the mark; taking the name away and defining it
		// again does not.
		{"a redefinition keeps the mark", `alias hidden='echo three'; type hidden`, "type: hidden: not found"},
		{"a removal takes it off", `unalias hidden; alias hidden=ls; type hidden`, "hidden is an alias for ls"},
	} {
		out, _ := run(t, c.src, hide)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: out = %q, want %q", c.name, strings.TrimSpace(out), c.want)
		}
	}
}

// And the sentence a *found* name gets is written back the same way, in both
// of its words (#3678).
//
// The not-found sentence above was one word of one sentence. This is the
// other half of the family: the name and the resolved **path** are each
// written back through the same function, independently, so a plain name
// whose directory holds a blank quotes the path and leaves the name alone.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, script files under `env -i
// PATH=<dir>:/usr/bin:/bin LC_ALL=C` with standard input on /dev/null, with a
// directory on PATH holding an executable file literally called `a b`:
//
//	whence -v 'a b'    'a b' is a tracked alias for '<dir>/a b'
//	whence 'a b'       '<dir>/a b'
//	whence -v zz       zz is a tracked alias for '<dir with a blank>/zz'
//	whence zz          '<dir with a blank>/zz'
//	whence -v ls       ls is a tracked alias for /bin/ls
//
// The third and fourth rows are what say the two words are written back one
// at a time rather than the sentence being quoted whole, and the fifth is the
// control: a name and a path that need nothing are unchanged.
//
// The path is written back in Runner.reportedPath, which is the one place
// every builtin asked *where* a command is passes through — so the sentence
// forms and the bare-path forms cannot part company, which is what the rows
// pairing `whence -v` with `whence` are for.
func TestATypeSentenceWritesBackTheNameAndThePath(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "*?[]{}~#", Leading: "="}
	for _, c := range []struct {
		name string
		// dir is the directory the image goes in, relative to the temporary
		// root, and image is what the file is called.
		dir, image string
		// what the sentence and the bare path say when the dialect quotes.
		sentence, bare string
	}{
		{
			// A name that cannot be written bare, in a directory that can.
			"a blank in the name", "bin", "a b",
			"'a b' is %[1]s", "%[1]s",
		},
		{
			// And the other way about: the name is plain and the path is
			// not, so only one of the two words moves.
			"a blank in the directory", "b c", "zz",
			"zz is %[1]s", "%[1]s",
		},
		{
			// The control, where neither word needs anything.
			"neither word needs quoting", "bin", "zz",
			"zz is %[1]s", "%[1]s",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, c.dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			path := writeImage(t, dir, c.image, []byte("#!/bin/sh\n"), 0o755)
			for _, q := range []struct {
				quoting TraceQuoting
				quotes  bool
			}{{QuoteNever, false}, {QuoteDollar, true}} {
				want := path
				if q.quotes && strings.ContainsRune(path, ' ') {
					// Written out rather than asked of the quoter, so the
					// expectation is an independent statement of the rule
					// rather than the code under test repeated.
					want = "'" + path + "'"
				}
				setup := func(r *Runner) {
					sem := permissive()
					dg := Diagnostics{
						TypeExternal:        "%[1]s is %[2]s",
						NameReportQuoting:   q.quoting,
						TraceMetacharacters: meta,
					}
					r.Semantics, r.Diagnostics = &sem, &dg
					r.Dir = root
					r.Env = []string{"PATH=" + dir}
				}
				// The sentence: both words, each written back on its own.
				out, _ := run(t, `type "`+c.image+`"`, setup)
				wantLine := c.sentence
				if !q.quotes {
					wantLine = strings.ReplaceAll(wantLine, "'", "")
				}
				wantLine = strings.ReplaceAll(wantLine, "%[1]s", want)
				if !strings.Contains(out, wantLine) {
					t.Errorf("sentence at %v: out = %q, want %q", q.quoting, strings.TrimSpace(out), wantLine)
				}
				// And the bare-path form, which has to agree about the path.
				out, _ = run(t, `command -v "`+c.image+`"`, setup)
				if strings.TrimSpace(out) != want {
					t.Errorf("bare path at %v: out = %q, want %q", q.quoting, strings.TrimSpace(out), want)
				}
			}
		})
	}
}
