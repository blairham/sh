// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

// TestUnterminatedBracketHasThreeAnswers is the axis that does not fit Answer.
// `[` is the name of the test builtin, so this is load-bearing rather than
// exotic.
func TestUnterminatedBracketHasThreeAnswers(t *testing.T) {
	const src = `case "[" in [) echo hit;; *) echo miss;; esac`
	for _, tc := range []struct {
		name string
		sem  Semantics
		want string
	}{
		{"bash", bash.Semantics(), "hit\n"},
		{"ksh93", ksh.Semantics(), "hit\n"},
		{"dash", dash.Semantics(), "miss\n"},
	} {
		if got, _ := run(t, src, withSem(tc.sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	// zsh rejects the pattern and abandons the script — with status 0, where
	// the same pattern against the filesystem gives 1. Both are measured;
	// neither is guessable from the other.
	out, st := run(t, src+`; echo after`, withSem(zsh.Semantics()))
	if !strings.Contains(out, "bad pattern: [") {
		t.Errorf("zsh: got %q", out)
	}
	if strings.Contains(out, "after") || strings.Contains(out, "hit") || strings.Contains(out, "miss") {
		t.Errorf("zsh: the script should stop, got %q", out)
	}
	if st != 0 {
		t.Errorf("zsh: status = %d, want 0", st)
	}
	// The core has no answer and says so.
	if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse, status %d", st)
	}
}

// TestBracketPolicyIsAskedOnlyWhenItApplies keeps ordinary patterns usable in
// the core, the same way the caret axis does.
func TestBracketPolicyIsAskedOnlyWhenItApplies(t *testing.T) {
	if got, st := run(t, `case b in [abc]) echo in;; *) echo out;; esac`, withSem(CoreSemantics())); got != "in\n" || st != 0 {
		t.Errorf("a closed class needs no answer: %q status %d", got, st)
	}
	if got, _ := run(t, `case a in a) echo plain;; esac`, withSem(CoreSemantics())); got != "plain\n" {
		t.Errorf("a pattern with no bracket at all: %q", got)
	}
}

// TestTestBuiltinSurvivesTheBracketPolicy is why this axis matters: `[` is a
// command name, and a policy applied to the filesystem too eagerly stops it
// running.
func TestTestBuiltinSurvivesTheBracketPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"zsh", zsh.Semantics()}, {"dash", dash.Semantics()}, {"bash", bash.Semantics()}} {
		if got, _ := run(t, `[ a = a ] && echo yes`, withSem(tc.sem)); got != "yes\n" {
			t.Errorf("%s: got %q, want %q", tc.name, got, "yes\n")
		}
		if got, _ := run(t, `echo [`, withSem(tc.sem)); got != "[\n" {
			t.Errorf("%s: a lone [ is literal against the filesystem, got %q", tc.name, got)
		}
	}
	// zsh rejects one with anything else in the field, and that is the whole
	// field rather than where the bracket sits in it.
	for _, src := range []string{`echo [a`, `echo a[`} {
		out, st := run(t, src, withSem(zsh.Semantics()))
		if !strings.Contains(out, "bad pattern") {
			t.Errorf("zsh %s: got %q", src, out)
		}
		if st != 1 {
			t.Errorf("zsh %s: status = %d, want 1", src, st)
		}
	}
}

// TestEqualsExpansionRunsBeforeGlobbing is measured rather than assumed:
// `echo [[a == a]]` reports the `==` and never reaches the `[[a`.
func TestEqualsExpansionRunsBeforeGlobbing(t *testing.T) {
	out, _ := run(t, `echo [[a == a]]`, withSem(zsh.Semantics()))
	if strings.Contains(out, "bad pattern") {
		t.Errorf("globbing ran first: %q", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("got %q", out)
	}
}
