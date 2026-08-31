// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

func TestEqualsExpansionIsZshAlone(t *testing.T) {
	// `echo =ls` prints a path in zsh and the literal text everywhere else,
	// with nothing reported either way — the `&>` failure mode in an
	// expansion.
	if got, _ := run(t, `echo =ls`, withSem(zsh.Semantics())); !strings.HasSuffix(strings.TrimSpace(got), "/ls") {
		t.Errorf("zsh should expand to a path, got %q", got)
	}
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"bash", bash.Semantics()}, {"dash", dash.Semantics()}} {
		if got, _ := run(t, `echo =ls`, withSem(tc.sem)); got != "=ls\n" {
			t.Errorf("%s should take it literally, got %q", tc.name, got)
		}
	}
	// The core refuses, because both answers are plausible output.
	if _, st := run(t, `echo =ls`, withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse, status %d", st)
	}
}

func TestEqualsExpansionOnlyAtTheHeadAndUnquoted(t *testing.T) {
	sem := zsh.Semantics()
	for _, tc := range []struct{ src, want string }{
		// An assignment does not begin with `=`.
		{`echo a=b`, "a=b\n"},
		// Quoting removes it, as it does tilde expansion.
		{`echo "=ls"`, "=ls\n"},
		// A lone `=` names nothing after it and is left alone.
		{`echo =`, "=\n"},
	} {
		if got, _ := run(t, tc.src, withSem(sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestEqualsExpansionFailureIsFatal(t *testing.T) {
	// zsh reports the name without a colon and abandons the script.
	out, st := run(t, `echo =nosuchcommand_xyz; echo after`, withSem(zsh.Semantics()))
	if strings.Contains(out, "after") {
		t.Errorf("the script continued: %q", out)
	}
	if !strings.Contains(out, "nosuchcommand_xyz not found") {
		t.Errorf("got %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}
