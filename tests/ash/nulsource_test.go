// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestANulInTheProgramIsNotThere: the byte is dropped from the text before it
// is read, in a script and in a file run because the kernel would not start
// it. Measured 2026-10-03 in the pinned image. See
// syntax.Dialect.SourceDropsNulBytes.
func TestANulInTheProgramIsNotThere(t *testing.T) {
	dir := t.TempDir()
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Env: []string{"PATH=/usr/bin:/bin"}, Dir: dir,
	}, "echo t\x00wo\nprintf 'echo ran\\0more\\n' > b.img; chmod +x b.img; ./b.img; echo \"st=$?\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "two\nranmore\nst=0\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
