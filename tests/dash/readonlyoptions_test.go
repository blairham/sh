// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `readonly` here takes POSIX's `-p` and nothing else, and the refusal is
// fatal because `readonly` is a special builtin. Measured 2026-09-12,
// dash 0.5.12, `env -i PATH=/usr/bin:/bin`, over `-c`:
//
//	readonly -p zz                 status 0
//	readonly -a zz; echo alive     dash: 1: readonly: Illegal option -a
//	                               and `alive` never prints
//
// `-A`, `-f` and `-n` are refused in the same words.
//
// This column accepted all four letters until #2277, because the letters were
// fixed in the interpreter rather than taken from Semantics.ReadonlyOptions
// the way `read`'s and `unset`'s already were. The letter mattering more than
// the wording is the point: a shell that accepts an option it does not have
// then has to answer questions the option raises, and dash has no arrays to
// answer them with.
func TestReadonlyTakesOnlyThePosixLetterAndTheRefusalIsFatal(t *testing.T) {
	for _, letter := range []string{"a", "A", "f"} {
		t.Run("-"+letter, func(t *testing.T) {
			src := "readonly -" + letter + " zz; echo alive"
			out, _, err := preset.Combined(t, dialecttest.Base{
				Name: "dash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, src)
			if err != nil {
				t.Fatalf("run %q: %v", src, err)
			}
			if want := "readonly: Illegal option -" + letter; !strings.Contains(out, want) {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
			}
			if strings.Contains(out, "alive") {
				t.Errorf("got %q — a special builtin's bad option ends the shell here", out)
			}
		})
	}
}
