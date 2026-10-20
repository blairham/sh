// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `read`'s two number letters, which are the only place in the tree the
// optstring's optional-number shape is used.
//
// `-t`'s argument is **optional**: `read -t v` reads into `v`, and spelling
// the letter `t:` made `v` its argument and answered `v: invalid number`.
// `-k`'s is optional too, and the pair differ only in what they accept once a
// word has been taken — `-k` an integer, `-t` a decimal.
//
// Graded here as well as on the helper in `interp`, because the helper is a
// predicate and cannot see which letter uses it: a mutation putting `t:` back
// killed nothing until this file existed.
//
// Measured 2026-09-28 against zsh 5.9.2 (aarch64-apple-darwin25.4.0),
// `go version -m` *not a Go executable*, from script files under
// `env -i PATH=/usr/bin:/bin` with standard input on `/dev/null`.
func TestReadsTwoNumberLetters(t *testing.T) {
	seed := `print -r -- "one two three" > in.txt` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		// `-t` with no number at all: the next word is the *name*.
		{"a bare -t reads into the name", `read -t v < in.txt`, "v=[one two three]"},
		{"and into two names", `read -t a b < in.txt`, "a=[one]"},
		// A number after it is the timeout, detached or attached.
		{"a detached number is the timeout", `read -t 5 v < in.txt`, "v=[one two three]"},
		{"an attached one too", `read -t5 v < in.txt`, "v=[one two three]"},
		// **A decimal is fine for this letter**, and it is the row the
		// whole-word test got wrong by reading `0.5` as a name.
		{"and a decimal is taken", `read -t 0.5 v < in.txt`, "v=[one two three]"},
		// A word that does not begin with a digit is a name however numeric
		// it looks, so this one is `not an identifier`.
		{"while a leading dot is a name", `read -t .5 v < in.txt`, "not an identifier: .5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := seed + tc.src + "\n" + `print -r -- "st=$? v=[$v] a=[$a]"` + "\n"
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s = %q, want it to contain %q", tc.src, out, tc.want)
			}
		})
	}
	// `-k` takes a word the same way and then refuses what `-t` accepts,
	// which is what says the taking and the validating are two questions.
	t.Run("-k takes the same word and refuses a decimal", func(t *testing.T) {
		src := seed + `read -k 0.5 v < in.txt` + "\n" + `print -r -- "st=$?"` + "\n"
		out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "number expected after -k: 0.5") {
			t.Errorf("out = %q, want the count refused rather than read as a name", out)
		}
	})
}
