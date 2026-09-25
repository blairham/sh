// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/policy"
)

// TestAllowExecIsRefusedBecauseItRescindsTheFile is #4409, written as the
// policy that leaked rather than as a unit on the selector table.
//
// The shape is the one an agent sandbox is actually given: a workspace it may
// read and write, a secret carved out of it, and an interpreter on the
// allowlist because the job needs one. The first three lines are refusals the
// author believes in, and the fourth hands the whole filesystem to a process
// this package never hears about. Nothing in the file said so before this.
func TestAllowExecIsRefusedBecauseItRescindsTheFile(t *testing.T) {
	t.Parallel()
	_, err := policy.Parse(strings.NewReader(`version 1
default deny
allow read  /proj/**
allow write /proj/**
deny  read  /proj/.env
allow exec /usr/bin/python3
`))
	if err == nil {
		t.Fatal("`allow exec` parsed; a policy file that grants an ungated child has to say so")
	}
	// Each of these is load-bearing for someone reading the message, and each
	// is a separate way the diagnostic could be useless: a line to go to, the
	// program that is being handed the filesystem, the size of what is being
	// given back, and the words that fix it.
	for _, want := range []string{
		"line 6",
		"/usr/bin/python3",
		"the deny rule in this policy does not apply to it",
		"allow exec-unconfined /usr/bin/python3",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("diagnostic %q does not contain %q", err, want)
		}
	}
}

// The count is the part of the message that lands, so it is graded rather
// than assumed. A policy whose refusals are all defaults still has refusals,
// and saying "the 0 deny rules" there would read as though nothing was lost.
func TestTheDiagnosticNamesWhatIsBeingGivenBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"one rule", "version 1\ndefault allow\ndeny read /s\nallow exec /bin/sh\n",
			"the deny rule in this policy does not apply to it",
		},
		{
			"several rules",
			"version 1\ndefault allow\ndeny read /s\ndeny write /s\ndeny list /s\nallow exec /bin/sh\n",
			"the 3 deny rules in this policy do not apply to it",
		},
		{
			"refusals that are all defaults", "version 1\ndefault deny\nallow exec /bin/sh\n",
			"this policy's `default deny` does not apply to it",
		},
		{
			"a policy that refuses nothing at all", "version 1\ndefault allow\nallow exec /bin/sh\n",
			"nothing in this policy applies to it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := policy.Parse(strings.NewReader(tc.src))
			if err == nil {
				t.Fatal("parsed; want a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("diagnostic %q does not contain %q", err, tc.want)
			}
		})
	}
}

// TestExecUnconfinedIsTheRuleThatWorks, and means exactly what `allow exec`
// meant. The grammar change is a change to what the file says and not to what
// the gate decides, so the decision has to be graded as unchanged — a fix
// that also narrowed the rule would be a different change wearing this one's
// issue number.
func TestExecUnconfinedIsTheRuleThatWorks(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault deny\nallow exec-unconfined /usr/bin/git\n")
	want(t, p, interp.Allow, exec("/usr/bin/git"))
	want(t, p, interp.Deny, exec("/usr/bin/curl"))
	// Including the implication an exec grant earns: a program that may be run
	// may be found, or the policy does not mean what it says.
	want(t, p, interp.Allow, stat("/usr/bin/git"))
}

// A rule that *refuses* an exec is not a hole, so it keeps the plain word.
// Requiring `deny exec-unconfined` would be the grammar asking an author to
// say that a refusal gives something away, which is false.
func TestDenyExecKeepsThePlainWord(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny exec /bin/sh\n")
	want(t, p, interp.Deny, exec("/bin/sh"))
	want(t, p, interp.Allow, exec("/bin/echo"))
}

// TestADefaultIsTheOtherWayToGrantExec. A default names no pattern, so it
// never becomes a Rule and never reaches the check the rules get; refusing it
// is a second site, and a second site is a thing to grade rather than trust.
func TestADefaultIsTheOtherWayToGrantExec(t *testing.T) {
	t.Parallel()
	_, err := policy.Parse(strings.NewReader("version 1\ndefault deny\ndefault allow exec\n"))
	if err == nil {
		t.Fatal("`default allow exec` parsed; it grants an ungated child by another route")
	}
	if !strings.Contains(err.Error(), "default allow exec-unconfined") {
		t.Errorf("diagnostic %q does not name the replacement", err)
	}
	// And the replacement works, so the refusal is a redirection rather than a
	// removal.
	p := parse(t, "version 1\ndefault deny\ndefault allow exec-unconfined\n")
	want(t, p, interp.Allow, exec("/anything"))
	want(t, p, interp.Deny, open("/x", false))
	// `default deny exec` is a refusal and keeps the plain word, as a rule
	// does.
	q := parse(t, "version 1\ndefault allow\ndefault deny exec\n")
	want(t, q, interp.Deny, exec("/bin/sh"))
	want(t, q, interp.Allow, open("/x", false))
}

// TestPathDoesNotReachExec is the half of #4409 without which the rest is
// decoration: refusing `allow exec` moves the silent grant one word over
// unless `path` stops carrying it. `allow path /**` reads as a rule about
// files and granted execution of every program on the machine.
func TestPathDoesNotReachExec(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault deny\nallow path /**\n")
	want(t, p, interp.Deny, exec("/bin/sh"))
	// And the file actions it does carry are untouched, so this is an
	// exclusion rather than a `path` that stopped working.
	want(t, p, interp.Allow, open("/x", false), open("/x", true), stat("/x"), list("/x"))
}

// The policy file is the only surface this changes. `ParseRule` is how
// `cmd/sh -deny` reaches the gate and `New` is how an embedder builds one
// without a file; a Go caller is writing code rather than a security artifact
// a second person reads, and `-deny` has no file to annotate.
func TestTheGrammarChangeStopsAtTheFile(t *testing.T) {
	t.Parallel()
	r, err := policy.ParseRule(interp.Allow, "exec /usr/bin/git")
	if err != nil {
		t.Fatalf("ParseRule refused a rule the flag route builds: %v", err)
	}
	if r.Sel != policy.SelExec {
		t.Errorf("got selector %v, want exec", r.Sel)
	}
	built := policy.New(interp.Deny, r)
	if got := built.Allow(t.Context(), exec("/usr/bin/git")); got != interp.Allow {
		t.Errorf("New with an exec rule decided %v, want allow", got)
	}
}

// A rule remembers how it was written, because a rule renders itself back to
// an operator — `Rule.String` is what `-trace-events` prints — and text that
// round-trips to a rule the parser would now refuse is text that lies.
//
// The first draft of this test read `NormalizedText`, which returns only the
// rules that gained a platform alias and was therefore empty for every policy
// here: the loop asserting the spelling never ran once, and the test passed
// against a `String` that said anything at all. So the rendering is taken from
// `Rules` and the count is asserted before it is read.
func TestARuleRendersBackTheWordItWasWrittenWith(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault deny\nallow exec-unconfined /usr/bin/git\nallow read /proj/**\n")
	rules := p.Rules()
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 — the rest of this test reads them", len(rules))
	}
	got := rules[0].String()
	if got != "allow exec-unconfined /usr/bin/git" {
		t.Errorf("rendered %q, want `allow exec-unconfined /usr/bin/git`", got)
	}
	// And the rendering is a policy again. A file assembled from what the
	// shell says its rules are has to load, or the two spellings have drifted.
	var b strings.Builder
	b.WriteString("version 1\ndefault deny\n")
	for _, r := range rules {
		b.WriteString(r.String() + "\n")
	}
	if _, err := policy.Parse(strings.NewReader(b.String())); err != nil {
		t.Errorf("a policy's own rules do not parse as a policy: %v\n%s", err, b.String())
	}
}

// The same question for the surface that does the aliasing, with a path that
// actually gains a second name — the check above cannot ask it, because a
// rule with no alias never reaches `Normalized`.
func TestNormalizedTextKeepsTheWordToo(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here")
	}
	p := parse(t, "version 1\ndefault deny\nallow exec-unconfined /tmp/bin/**\n")
	lines := p.NormalizedText()
	if len(lines) != 1 {
		t.Fatalf("got %d normalized lines, want 1 — an empty list would pass every assertion below", len(lines))
	}
	if !strings.Contains(lines[0], "exec-unconfined") {
		t.Errorf("normalized line %q drops the word the rule was written with", lines[0])
	}
	if !strings.Contains(lines[0], "/private/tmp/bin/**") {
		t.Errorf("normalized line %q does not show the second name", lines[0])
	}
}
