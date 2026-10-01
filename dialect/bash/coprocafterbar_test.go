// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// **A `coproc` may begin any element of a pipeline here** — measured
// 2026-10-01 on bash 5.3.20, where zsh refuses the same lines. See
// syntax.Dialect.CoprocAfterABar.
func TestACoprocAfterABarParses(t *testing.T) {
	for _, src := range []string{
		"echo | coproc true",
		"echo hi | coproc { cat; }",
		"echo |& coproc cat",
		"echo |\ncoproc cat",
	} {
		if _, err := syntax.Parse(src, bash.Dialect()); err != nil {
			t.Errorf("%q: %v, want it to parse", src, err)
		}
	}
}
