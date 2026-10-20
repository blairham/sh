// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/terminfofixture"
)

// `echoti cup` against a description whose `cup` carries padding, the shape
// TERM=vt100's has: measured on zsh 5.9.2, `echoti cup 3 4` is `\e[4;5H`,
// `echoti cup 3` is `\e[4;1H`, and a bare `echoti cup` writes the language
// without the `$<5>` (#5150).
func TestEchotiComputesParametersAndDropsPadding(t *testing.T) {
	const stringCursorAddress = 10 // cup
	dir := terminfofixture.Database(t, terminfofixture.Description{
		Name: fixtureTerm, StrCount: stringCursorAddress + 1,
		Strs: map[int]string{stringCursorAddress: "\x1b[%i%p1%d;%p2%dH$<5>"},
	})
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(),
		Vars: map[string]string{
			"PATH": t.TempDir(), "TERM": fixtureTerm, "TERMINFO": dir,
			"HOME": t.TempDir(), "TERMINFO_DIRS": "",
		},
	}, "echoti cup 3 4; print -r -- \"|$?\"\necho"+"ti cup 3; print -r -- \"|$?\"\nechoti cup; print -r -- \"|$?\"\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "\x1b[4;5H|0\n\x1b[4;1H|0\n\x1b[%i%p1%d;%p2%dH|0\n"
	if out != want || st != 0 {
		t.Errorf("echoti cup = %q (status %d), want %q", out, st, want)
	}
}
