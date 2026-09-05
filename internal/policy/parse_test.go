// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// Every parse failure here is a rule somebody believed was in force. That is
// the reason there is no recovery and no "unknown directive ignored": a typo
// in a security file that is read past is a hole with a comment above it
// explaining what it was supposed to do.

func TestAPolicyMustSayWhichFormatItIs(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"allow read /srv/**\n",
		"default deny\nversion 1\n",
		"version 2\n",
		"version\n",
		"",
		"# only a comment\n",
		// The *word* is checked and not only the number after it. A parser
		// that read the first directive's argument without looking at its
		// name would take any of these for a version line, which is how a
		// file in some other format gets half-read.
		"v 1\n",
		"policy 1\n",
		"allow 1\n",
	} {
		if _, err := policy.Parse(strings.NewReader(src)); err == nil {
			t.Errorf("parsing %q succeeded; a file this parser cannot vouch for must be refused", src)
		}
	}
	if _, err := policy.Parse(strings.NewReader("\n# a comment\n\nversion 1\n")); err != nil {
		t.Errorf("blank lines and comments before the version are fine, but: %v", err)
	}
}

func TestABadLineIsRefusedAndNamed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"unknown directive":         "version 1\npermit read /x\n",
		"unknown selector":          "version 1\nallow network /x\n",
		"inherit is never gated":    "version 1\nallow inherit /x\n",
		"relative pattern":          "version 1\nallow read srv/**\n",
		"no pattern":                "version 1\nallow read\n",
		"nothing at all":            "version 1\nallow\n",
		"a pattern on a signal":     "version 1\nallow signal /x\n",
		"a malformed glob":          "version 1\nallow read /srv/[abc\n",
		"the version stated twice":  "version 1\nversion 1\n",
		"the base default twice":    "version 1\ndefault deny\ndefault allow\n",
		"a selector default twice":  "version 1\ndefault deny stat\ndefault allow stat\n",
		"an overlapping default":    "version 1\ndefault deny read\ndefault allow stat\n",
		"a decision that is not":    "version 1\ndefault maybe\n",
		"a selector that is not":    "version 1\ndefault deny network\n",
		"a rule decision that is n": "version 1\nmaybe read /x\n",
	}
	for what, src := range cases {
		_, err := policy.Parse(strings.NewReader(src))
		if err == nil {
			t.Errorf("%s: parsing %q succeeded", what, src)
			continue
		}
		if !strings.Contains(err.Error(), "line ") {
			t.Errorf("%s: %q does not name the line", what, err)
		}
	}
}

// TestTheInheritErrorExplainsItself, because someone writing that rule has a
// reasonable model and a wrong one — the right response is the reason, not
// "unknown selector".
func TestTheInheritErrorExplainsItself(t *testing.T) {
	t.Parallel()
	_, err := policy.Parse(strings.NewReader("version 1\nallow inherit /x\n"))
	if err == nil || !strings.Contains(err.Error(), "never gated") {
		t.Errorf("got %v, want an error saying inherit is never gated", err)
	}
}

// TestAPatternIsTheRestOfTheLine is the reason the format has no quoting: a
// path with a space in it is written as it stands, and a third whitespace
// field would have silently truncated it to the first word.
func TestAPatternIsTheRestOfTheLine(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/my documents/**\n")
	want(t, p, interp.Allow, open("/srv/my documents/a", false))
	want(t, p, interp.Deny, open("/srv/my", false))
}

// TestCommentsAreWholeLines, because `#` is a legal character in a path and
// stripping a trailing comment would silently narrow a rule about a file whose
// name contains one.
func TestCommentsAreWholeLines(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\n  # a rule follows\nallow read /srv/note#1\n")
	want(t, p, interp.Allow, open("/srv/note#1", false))
	want(t, p, interp.Deny, open("/srv/note", false))
}

// TestWhitespaceIsForgiven — leading indentation, runs of spaces between
// fields, tabs, a trailing blank, and a CRLF file from another machine.
func TestWhitespaceIsForgiven(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\r\n\tallow\tread\t/srv/**   \r\n\n")
	want(t, p, interp.Allow, open("/srv/x", false))
}

// TestParseFileNamesTheFile, because a policy that will not load has to say
// which file and which line, in the shape a person greps for.
func TestParseFileNamesTheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := filepath.Join(dir, "bad.policy")
	if err := os.WriteFile(name, []byte("version 1\nallow read srv/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := policy.ParseFile(name)
	if err == nil {
		t.Fatal("a relative pattern loaded")
	}
	if !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("got %q, want it to name %s and line 2", err, name)
	}
	good := filepath.Join(dir, "good.policy")
	if err := os.WriteFile(good, []byte("version 1\nallow read /srv/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := policy.ParseFile(good)
	if err != nil {
		t.Fatalf("a good policy did not load: %v", err)
	}
	want(t, p, interp.Allow, open("/srv/x", false))
	if _, err := policy.ParseFile(filepath.Join(dir, "absent.policy")); err == nil {
		t.Error("a policy that is not there loaded")
	}
}

// ParseRule reads what a file line holds after its decision word, so a caller
// with a rule and no file gets the same rule.
//
// It exists because `cmd/sh -deny` had a matcher of its own, and two matchers
// over one gate is two answers to the same question. These are the cases that
// differed: the flag's list matched a path lexically where this cleans one
// first, and every rule it could hold named a path, so a signal could be
// watched and never refused.
func TestParseRuleReadsAFileLinesBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		sel        policy.Selector
		pattern    string
	}{
		{"a selector and a pattern", "path /etc/**", policy.SelPath, "/etc/**"},
		{"the kind with no pattern", "signal", policy.SelSignal, ""},
		{"one kind", "exec /usr/bin/**", policy.SelExec, "/usr/bin/**"},
		{"a pattern with a space in it", "read /srv/a b/**", policy.SelRead, "/srv/a b/**"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := policy.ParseRule(interp.Deny, tc.body)
			if err != nil {
				t.Fatalf("%q: %v", tc.body, err)
			}
			if r.Decision != interp.Deny || r.Sel != tc.sel || r.Pattern != tc.pattern {
				t.Errorf("got %+v, want deny %v %q", r, tc.sel, tc.pattern)
			}
		})
	}
}

// A body that names nothing is refused, and the diagnostic says which decision
// it was about — the caller has one and no word to put in the message itself.
func TestParseRuleRefusesWhatAFileWouldRefuse(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"empty", ""},
		{"a selector nobody has", "network /x"},
		{"a selector with no pattern", "exec"},
		{"a relative pattern", "path etc/**"},
		{"a pattern after signal", "signal /proc/1"},
		{"inherit", "inherit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := policy.ParseRule(interp.Deny, tc.body); err == nil {
				t.Errorf("%q was accepted, want a rule that can never fire refused", tc.body)
			}
		})
	}
	// And the decision reaches the wording, which is the only thing ParseRule
	// has to reconstruct that a line already carries.
	_, err := policy.ParseRule(interp.Allow, "")
	if err == nil || !strings.Contains(err.Error(), "allow") {
		t.Errorf("err = %v, want the decision named in it", err)
	}
}

// A rule built here and the same rule read from a file decide alike.
//
// The point of exporting this rather than writing a second matcher, asserted
// against the thing that would fail if the two ever came apart.
func TestARuleParsedAloneMatchesTheSameRuleInAFile(t *testing.T) {
	fromFile, err := policy.Parse(strings.NewReader("version 1\ndefault allow\ndeny path /etc/**\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := policy.ParseRule(interp.Deny, "path /etc/**")
	if err != nil {
		t.Fatal(err)
	}
	alone := policy.New(interp.Allow, r)
	for _, path := range []string{
		"/etc", "/etc/passwd", "/etcetera", "/srv/../etc/passwd", "/usr/bin/cat", "",
	} {
		for _, k := range []interp.ActionKind{
			interp.ActionExec, interp.ActionOpen, interp.ActionStat, interp.ActionReadDir,
		} {
			a := interp.Action{Kind: k, Path: path}
			if fromFile.Allow(t.Context(), a) != alone.Allow(t.Context(), a) {
				t.Errorf("the file and the parsed rule disagree about %v %q", k, path)
			}
		}
	}
}
