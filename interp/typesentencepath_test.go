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

// The path written *inside* a `type`-family sentence, where the bare-path
// forms write the same path as it is — see
// Diagnostics.TypeSentencePathQuoting.
//
// These name no shell. The field exists because the two routes have to be
// able to **disagree**: one column writes the path the same way with or
// without a sentence around it, which Diagnostics.NameReportQuoting answers
// in Runner.reportedPath, and another quotes only in the sentence. A single
// field cannot say both.

// sentencePathRun runs src with the two quoting fields set, on a PATH holding
// a directory with an executable called `a b` in it.
//
// The blank is the whole point: a path that needs no quoting cannot tell the
// two fields apart, and neither can a *name* that needs none.
func sentencePathRun(t *testing.T, sentence, report TraceQuoting, src string) string {
	t.Helper()
	dir := t.TempDir()
	script(t, dir+"/bb", "a b", "hi", true)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.TypeOptions = "taPp"
	sem.TypeEndsOptionsWithDashDash = Yes
	sem.TypePSearchesPathPastTheShell = No
	sem.TypePathAnswerIsASentence = No
	dg := Diagnostics{
		TypeSentencePathQuoting: sentence,
		NameReportQuoting:       report,
		// The alphabet both fields read, and the one a blank is in.
		TraceMetacharacters: TraceMetacharacters{Anywhere: "*?[]{}~#=^"},
	}
	path := dir + "/bb:/usr/bin:/bin"
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": path},
		Env:  []string{"PATH=" + path},
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error()
	}
	out := strings.TrimRight(buf.String(), "\n")
	return strings.ReplaceAll(out, dir, "<dir>")
}

// The sentence quotes and the bare path does not, which is the whole field.
func TestTheSentenceQuotesThePathWhereTheBarePathFormDoesNot(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the plain sentence", `type "a b"`, "a b is '<dir>/bb/a b'"},
		{"the same sentence under command -V", `command -V "a b"`, "a b is '<dir>/bb/a b'"},
		{"and the one -a writes", `type -a "a b"`, "a b is '<dir>/bb/a b'"},
		{"a pathname operand, which is a sentence too", `type "./bb/a b"`, "./bb/a b is '<dir>/./bb/a b'"},
		// The bare-path routes, which are the other half of the claim: the
		// same resolved path, written plain.
		{"command -v", `command -v "a b"`, "<dir>/bb/a b"},
		{"type -p", `type -p "a b"`, "<dir>/bb/a b"},
		// The name is never quoted by this field, in either route. `a b is`
		// and not `'a b' is`, which is what keeps it apart from the field
		// that spells the name.
		{"a name needing quotes is still bare in the sentence", `type "a b"`, "a b is '<dir>/bb/a b'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentencePathRun(t, QuoteShellLazy, QuoteNever, tc.src); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The mutation: with the field at its zero, every sentence above writes the
// path exactly as it was resolved and nothing else moves.
func TestTheSentenceWritesThePathAsItIsWhereNothingSaysOtherwise(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the plain sentence", `type "a b"`, "a b is <dir>/bb/a b"},
		{"command -V", `command -V "a b"`, "a b is <dir>/bb/a b"},
		{"type -a", `type -a "a b"`, "a b is <dir>/bb/a b"},
		{"a pathname operand", `type "./bb/a b"`, "./bb/a b is <dir>/./bb/a b"},
		{"command -v, unchanged either way", `command -v "a b"`, "<dir>/bb/a b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentencePathRun(t, QuoteNever, QuoteNever, tc.src); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// And this is not the field that spells a *name*, which is the probe that can
// tell the two apart.
//
// Without it a reader could fold one into the other: both quote the path in
// the sentence. They part on the two places only one of them reaches — the
// name at the front of the sentence, and the bare path with no sentence
// around it at all.
func TestQuotingTheReportedNameAndQuotingTheSentencesPathAreNotOneField(t *testing.T) {
	// The name field alone: the name is quoted and so is the path, in the
	// sentence *and* in the bare-path form.
	if got := sentencePathRun(t, QuoteNever, QuoteShell, `type "a b"`); got != "'a b' is '<dir>/bb/a b'" {
		t.Errorf("the name field, sentence: %q, want both words quoted", got)
	}
	if got := sentencePathRun(t, QuoteNever, QuoteShell, `command -v "a b"`); got != "'<dir>/bb/a b'" {
		t.Errorf("the name field, bare path: %q, want the path quoted", got)
	}
	// The sentence field alone: the name is bare, the path is quoted in the
	// sentence, and the bare-path form is untouched.
	if got := sentencePathRun(t, QuoteShellLazy, QuoteNever, `type "a b"`); got != "a b is '<dir>/bb/a b'" {
		t.Errorf("the sentence field, sentence: %q, want the name bare", got)
	}
	if got := sentencePathRun(t, QuoteShellLazy, QuoteNever, `command -v "a b"`); got != "<dir>/bb/a b" {
		t.Errorf("the sentence field, bare path: %q, want it plain", got)
	}
}
