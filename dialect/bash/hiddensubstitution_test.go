// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// TestASubstitutionHiddenFromTheLineIsNamedAndPlacedWhereTheShellWas — a
// `${ … }` operand inside double quotes is read a second time, and a `'` that
// quoted in the first read quotes nothing in the second, so the second read
// can open a substitution the line's own read never saw. The shell reaches
// that read when the word expands, with the line already behind it: the
// refusal is named `command substitution` and placed below the command.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. `a=4` is line one of every source, so the
// operand is reached — an operand a branch does not take is never read at all,
// which is the first control below.
//
// Two lines rather than one where the **substitution itself** is what ran out:
// a text handed to a reader that runs out is named on the line after it, and a
// body is one more such text. `'$('` runs out on the quote inside the body and
// `'$('\'` closes that quote again and runs out on the body, one line further
// down (#4201).
func TestASubstitutionHiddenFromTheLineIsNamedAndPlacedWhereTheShellWas(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the quote inside the body runs out",
			"a=4\necho one\necho \"${a+'$('}\"\necho two\n",
			"command substitution: line 4: unexpected EOF while looking for matching `''",
		},
		{
			"and the lines below it move with it",
			"a=4\necho one\necho two\necho three\necho \"${a+'$('}\"\n",
			"command substitution: line 6: unexpected EOF while looking for matching `''",
		},
		{
			"the body itself runs out, one line further",
			"a=4\necho \"${a+'$('\\'}\"\necho two\n",
			"command substitution: line 4: unexpected EOF while looking for matching `)'",
		},
		{
			"and that one moves the same way",
			"a=4\necho one\necho two\necho three\necho \"${a+'$('\\'}\"\n",
			"command substitution: line 7: unexpected EOF while looking for matching `)'",
		},
		{
			"inside a compound it is the command's line, not the compound's",
			"a=4\nif true; then\necho \"${a+'$('}\"\nfi\n",
			"command substitution: line 4: unexpected EOF while looking for matching `''",
		},
		{
			"a first word that spans lines takes the operand's line with it",
			"a=4\necho \\\n  \"${a+'$('}\"\n",
			"command substitution: line 3: unexpected EOF while looking for matching `''",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := runBashSplitFatal(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", errs, tc.want)
			}
		})
	}
	// The controls. An operand a branch does not take is never read, so the
	// same text says nothing at all — which is what makes this the second
	// read's failure and not the line's.
	for _, src := range []string{
		"a=4\necho \"${a-'$('}\"\n",
		"a=4\necho \"${a='$('}\"\n",
	} {
		if _, errs := runBashSplitFatal(t, src); errs != "" {
			t.Errorf("%q: stderr = %q, want nothing", src, errs)
		}
	}
	// And the discriminating control, which is why the route is asked of the
	// construct that was open and not of the token the read ran out on:
	// `"${a+'bar}"` runs out on a `'` exactly as the first row above does,
	// and it holds no substitution at all. The `'` that quotes nothing in the
	// second read quoted the closing brace in the **first**, so the line's
	// own read never finds one and refuses the file where it stands — it
	// never reaches the deferred read this test is about, in this shell or in
	// the reference.
	if _, err := syntax.Parse("a=4\necho \"${a+'bar}\"\necho two\n", bash.Dialect()); err == nil {
		t.Error("`\"${a+'bar}\"` parsed, want the line's own read to refuse it")
	}
}
