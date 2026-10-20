// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A colon closes a tilde prefix in an ordinary word **never** here. See
// interp.Semantics.TildeColonEndsAnOrdinaryWordsPrefix, which the panel splits
// three ways on — and which is not the assignment's value, where a colon closes
// one in every column.
//
// Measured 2026-09-22, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with `HOME=/usr/xyz`, each word given to `printf` (#4251).
func TestAColonAndAnOrdinaryWordsTildePrefix(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a colon closes nothing", `printf "[%s]" ~:x`, "[~:x]"},
		{"nor before a path", `printf "[%s]" ~:x/y`, "[~:x/y]"},
		{"nor at the end of the word", `printf "[%s]" ~:`, "[~:]"},
		{"nor before another tilde", `printf "[%s]" ~:~`, "[~:~]"},
		// A slash still closes it, which is what says the tilde road is there
		// and only the colon is out of it.
		{"a slash still closes it", `printf "[%s]" ~/m:x`, "[/h/m:x]"},
		// Unanimous in all seven and worth keeping under any reading: the name
		// in front of the colon is a user nobody has, a tilde that does not
		// open the word is ordinary text, and one behind a colon opens nothing.
		{"a user nobody has", `printf "[%s]" ~chet:x`, "[~chet:x]"},
		{"a tilde inside the word", `printf "[%s]" a~:x`, "[a~:x]"},
		{"a tilde behind a colon", `printf "[%s]" x:~/m`, "[x:~/m]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := tildeColonRun(t, tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// tildeColonRun runs a word with `HOME` in the **environment** rather than
// assigned by the script, which is not a convenience: one column reads a bare
// `~` from a copy of `HOME` taken at startup, so a script's own assignment
// would leave every row here reading the home this test process happens to
// have. See interp.Runner and dialect/bash/cachedhome_test.go.
func tildeColonRun(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Env: []string{"PATH=" + dir, "HOME=/h", "LC_ALL=C"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
