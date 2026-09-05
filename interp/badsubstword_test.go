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

// What a bad-substitution diagnostic names, how many of them one command
// writes, and what an unreadable `@` letter does about a name with no value.
// Three questions with one seam behind them: where the expander gives up on a
// word and what it hands back.

// badWordRun runs src through a grammar that has the `@` family, with the
// Diagnostics the caller wants, and returns stdout, stderr and the status.
func badWordRun(t *testing.T, src string, dg Diagnostics, sem Semantics, commandString bool) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamTransformations = true
	d.ArraySubscript = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs,
		Semantics: &sem, Diagnostics: &dg,
		Name: "testsh", Route: routeFor(commandString),
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// wordNamingSemantics answers the two axes these tests reach and nothing else.
func wordNamingSemantics() Semantics {
	s := permissive()
	s.FatalErrorStatusIsOne = Yes
	s.TransformLetterCheckedOnlyWhenValued = No
	return s
}

func TestBadSubstitutionNamesWhatTheDialectAsksFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		names BadSubstitutionSubject
		src   string
		want  string
	}{
		// The default: the `${…}` and nothing around it.
		{"the expansion", NamesTheExpansion, `x=a; echo "[${x@QQ}]"`, "${x@QQ}: bad substitution"},

		// The run of the word sharing the expansion's quoting, quotes off.
		{"the run, quoted", NamesTheQuotingRun, `x=a; echo "[${x@QQ}]"`, "[${x@QQ}]: bad substitution"},
		{"the run, bare", NamesTheQuotingRun, `x=a; echo pre${x@QQ}post`, "pre${x@QQ}post: bad substitution"},
		// A change of quoting inside the word ends the run, so the literal
		// in different quotes is not named.
		{"the run stops at a quoting change", NamesTheQuotingRun, `x=a; echo 'lit'"${x@QQ}"`, "${x@QQ}: bad substitution"},
		{"the run holds what follows it", NamesTheQuotingRun, `x=a; echo "${x@QQ}"'lit'`, "${x@QQ}: bad substitution"},

		// The whole word as written, quotes and all.
		{"the word, quoted", NamesTheWholeWord, `x=a; echo "[${x@QQ}]"`, `"[${x@QQ}]": bad substitution`},
		{"the word, bare", NamesTheWholeWord, `x=a; echo pre${x@QQ}post`, "pre${x@QQ}post: bad substitution"},
		{"the word, mixed quoting", NamesTheWholeWord, `x=a; echo 'lit'"${x@QQ}"`, `'lit'"${x@QQ}": bad substitution`},
	} {
		t.Run(c.name, func(t *testing.T) {
			dg := Diagnostics{BadSubstitutionNames: c.names}
			if c.names != NamesTheExpansion {
				// The wording receives the text whole rather than wrapping
				// it in braces, which is what the two word-naming answers
				// were measured writing.
				dg.BadSubstitution = "%[1]s: bad substitution"
			}
			_, errs, _ := badWordRun(t, c.src, dg, wordNamingSemantics(), false)
			if !strings.Contains(errs, c.want) {
				t.Errorf("said %q, want %q", errs, c.want)
			}
		})
	}
}

func TestACommandIsAbandonedAtItsFirstBadWord(t *testing.T) {
	for _, c := range []struct{ name, src, first string }{
		// A letter the family does not have, which is a failed expansion
		// and stops the script where it stands.
		{"the family's letter", `x=a; printf "[%s]" "${x@QQ}" "${x@ZZ}" "${x@YY}"`, "${x@QQ}"},
		// An operator nothing could read, which is not fatal on its own —
		// the command decides that after expanding, and used to expand
		// every remaining word before deciding.
		{"an unreadable operator", `printf "[%s]" "${x ~}" "${y ~}" "${z ~}"`, "${x ~}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs, _ := badWordRun(t, c.src, Diagnostics{}, wordNamingSemantics(), false)
			if n := strings.Count(errs, "bad substitution"); n != 1 {
				t.Errorf("wrote %d diagnostics in %q, want one", n, errs)
			}
			if !strings.Contains(errs, c.first) {
				t.Errorf("said %q, want the first bad word named", errs)
			}
		})
	}
}

func TestAWordIsAbandonedAtItsFirstBadExpansion(t *testing.T) {
	dg := Diagnostics{}
	_, errs, _ := badWordRun(t, `x=a; echo "${x@QQ}${x@ZZ}"`, dg, wordNamingSemantics(), false)
	if n := strings.Count(errs, "bad substitution"); n != 1 {
		t.Errorf("wrote %d diagnostics in %q, want one", n, errs)
	}
}

func TestUnsetNamesAreAbandonedAtTheFirstToo(t *testing.T) {
	d := syntax.Core()
	f, err := syntax.Parse(`set -u; printf "[%s]" "$a" "$b"`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := permissive()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{UnboundVariable: "%s: parameter not set"}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if n := strings.Count(buf.String(), "parameter not set"); n != 1 {
		t.Errorf("wrote %d diagnostics in %q, want one", n, buf.String())
	}
	if !strings.Contains(buf.String(), "a: parameter not set") {
		t.Errorf("said %q, want the first name", buf.String())
	}
}

func TestTheTransformLetterIsCheckedOnlyWhenTheAxisSaysThereIsAValue(t *testing.T) {
	for _, c := range []struct {
		name    string
		axis    Answer
		src     string
		quiet   bool
		wantOut string
	}{
		{"no value, checked anyway", No, `echo "[${u@QQ}]"`, false, ""},
		{"no value, left alone", Yes, `echo "[${u@QQ}]"`, true, "[]"},
		{"a value, always refused", Yes, `u=v; echo "[${u@QQ}]"`, false, ""},
		{"an empty value counts as one", Yes, `u=; echo "[${u@QQ}]"`, false, ""},
		{"an empty array is no value", Yes, `a=(); echo "[${a[@]@QQ}]"`, true, "[]"},
		{"a filled array is one", Yes, `a=(x); echo "[${a[@]@QQ}]"`, false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := wordNamingSemantics()
			sem.TransformLetterCheckedOnlyWhenValued = c.axis
			out, errs, st := badWordRun(t, c.src, Diagnostics{}, sem, false)
			if quiet := errs == ""; quiet != c.quiet {
				t.Errorf("stderr = %q, want quiet=%v", errs, c.quiet)
			}
			if strings.TrimSpace(out) != c.wantOut {
				t.Errorf("stdout = %q, want %q", out, c.wantOut)
			}
			if c.quiet && st != 0 {
				t.Errorf("status = %d, want 0 when nothing was refused", st)
			}
		})
	}
}

// A bad letter in a grammar that *has* the family is a failed expansion and
// carries the status one, which one dialect answers by how the shell started.
// Every other unreadable operator is a word that could not be read and keeps
// the ordinary fatal status.
func TestABadTransformLetterCarriesTheExpansionFailureStatus(t *testing.T) {
	sem := wordNamingSemantics()
	dg := Diagnostics{ExpansionFailureStatusFromCommandString: 127}

	if _, _, st := badWordRun(t, `x=a; echo "${x@QQ}"`, dg, sem, true); st != 127 {
		t.Errorf("status = %d, want the command-string expansion status", st)
	}
	if _, _, st := badWordRun(t, `x=a; echo "${x@QQ}"`, dg, sem, false); st != 1 {
		t.Errorf("status = %d, want the ordinary fatal status off a command string", st)
	}
	// The same grammar, an operator that is not the family at all.
	if _, _, st := badWordRun(t, `x=a; echo "${x ~}"`, dg, sem, true); st != 1 {
		t.Errorf("status = %d, want the fatal status for a word that could not be read", st)
	}
}

// A subscript is read before the operator is, so a node the grammar refused
// still carries one. Answering it as an array skipped the refusal entirely.
func TestASubscriptDoesNotRescueAnUnreadableOperator(t *testing.T) {
	for _, src := range []string{
		`a=(one two); printf "[%s]" "${a[@]@Q}"`,
		`a=(one two); printf "[%s]" "${a[0]@Q}"`,
	} {
		d := syntax.Core()
		d.ArraySubscript = true
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		sem := permissive()
		sem.FatalErrorStatusIsOne = Yes
		var buf bytes.Buffer
		r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Name: "testsh"})
		st, rerr := r.Run(context.Background(), f)
		if rerr != nil {
			t.Fatalf("run %q: %v", src, rerr)
		}
		if !strings.Contains(buf.String(), "bad substitution") {
			t.Errorf("%q gave %q at status %d, want a bad substitution", src, buf.String(), st)
		}
		if strings.Contains(buf.String(), "one") {
			t.Errorf("%q answered with the elements: %q", src, buf.String())
		}
	}
}
