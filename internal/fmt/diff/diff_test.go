// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package diff_test

import (
	"testing"

	"github.com/blairham/sh/internal/fmt/diff"
)

func TestEqualTextsDiffEmpty(t *testing.T) {
	if d := diff.Unified("a", "b", "x\ny\n", "x\ny\n"); d != "" {
		t.Errorf("expected empty diff, got:\n%s", d)
	}
}

func TestUnifiedShape(t *testing.T) {
	a := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n"
	b := "one\ntwo\nthree\nFOUR\nfive\nsix\nseven\neight\nNINE\n"
	got := diff.Unified("x.orig", "x", a, b)
	want := "--- x.orig\n+++ x\n" +
		"@@ -1,9 +1,9 @@\n" +
		" one\n two\n three\n-four\n+FOUR\n five\n six\n seven\n eight\n-nine\n+NINE\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPureAdditionAndRemoval(t *testing.T) {
	if d := diff.Unified("a", "b", "x\n", "x\ny\n"); d == "" {
		t.Error("addition should diff")
	}
	if d := diff.Unified("a", "b", "x\ny\n", "x\n"); d == "" {
		t.Error("removal should diff")
	}
}
