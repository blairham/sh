// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// A command a dialect registers for its prelude answers inside a function the
// prelude defined, in a subshell of one too, and nowhere else: a script typing
// the word gets a command that was not found, and no listing names it.
func TestAPreludeCommandAnswersOnlyThePrelude(t *testing.T) {
	pre, err := syntax.Parse("helper() { secretcmd \"$@\"; }\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse(`helper a b
(helper c)
secretcmd d; echo "st=$?"
command -v secretcmd || echo none
`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sem, dg := permissive(), Diagnostics{}
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	r.RegisterPreludeCommand("secretcmd", func(r *Runner, _ context.Context, args []string) int {
		_, _ = r.Stdout.Write([]byte("ran"))
		for _, a := range args {
			_, _ = r.Stdout.Write([]byte(" " + a))
		}
		_, _ = r.Stdout.Write([]byte("\n"))
		return 0
	})
	r.SourcingPrelude(true)
	if _, err := r.Run(context.Background(), pre); err != nil {
		t.Fatal(err)
	}
	r.SourcingPrelude(false)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "ran a b\nran c\nst=127\nnone\n"; got != want {
		t.Errorf("stdout = %q, want %q (stderr %q)", got, want, errs.String())
	}
}
