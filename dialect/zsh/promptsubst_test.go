// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `setopt PROMPT_SUBST` is asked at every draw, not read once.
//
// The option was remembered and reported correctly all along and consulted
// by nothing, which is why this asserts on the *live* answer rather than on
// the listing: `setopt promptsubst` then `unsetopt` has to move
// PromptStyle.Expand both ways in one shell, because a value settled when
// the shell started would pass a test that only ever looks once.
func TestPromptSubstMovesTheAnswerWhileTheShellRuns(t *testing.T) {
	expand := zsh.PromptStyle().Expand
	if expand == nil {
		t.Fatal("Expand is nil; the option could never turn it on")
	}
	r := zshRunnerForTest(t)
	if expand(r) {
		t.Error("a fresh shell expands its prompt; it must not until asked")
	}
	runInZshRunner(t, r, "setopt promptsubst")
	if !expand(r) {
		t.Error("`setopt promptsubst` did not turn the expansion on")
	}
	runInZshRunner(t, r, "unsetopt promptsubst")
	if expand(r) {
		t.Error("`unsetopt promptsubst` did not turn it back off")
	}
}

// zshRunnerForTest is a runner with this dialect applied, for a test that
// has to move an option and then ask a style about it.
//
// Through the preset, because a runner built by hand has a nil Dialect —
// which is the core rather than this shell, so every nested parse would run
// as a shell these tests are not about. internal/dialecttest has a guard
// that fails for exactly that, and it caught this one.
func zshRunnerForTest(t *testing.T) *interp.Runner {
	t.Helper()
	return preset.Runner(dialecttest.Base{Dir: t.TempDir()})
}

func runInZshRunner(t *testing.T, r *interp.Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(t.Context(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}
