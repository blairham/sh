// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// This shell names the whole word, quotes and all, where the shell with the
// `@` family names the run of it that shares the expansion's quoting.
// Measured 2026-09-05 against ksh93 AJM 93u+ in the oracle environment.

func TestBadSubstitutionNamesTheWholeWord(t *testing.T) {
	if got, want := ksh.Diagnostics().BadSubstitutionNames, interp.NamesTheWholeWord; got != want {
		t.Errorf("BadSubstitutionNames = %v, want %v", got, want)
	}
	for _, c := range []struct{ src, want string }{
		{`x=a; echo "[${x@QQ}]"`, `"[${x@QQ}]": bad substitution`},
		{`x=a; echo pre${x@QQ}post`, `pre${x@QQ}post: bad substitution`},
		// The quotes of a word that changes quoting stay on, which is what
		// separates this answer from the other one.
		{`x=a; echo 'lit'"${x@QQ}"`, `'lit'"${x@QQ}": bad substitution`},
		// This shell has no family at all, so a subscript in front of the
		// operator does not rescue it.
		{`a=(one two); printf "[%s]" "${a[@]@Q}"`, `"${a[@]@Q}": bad substitution`},
	} {
		out, _ := answersRun(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q", c.src, out, c.want)
		}
	}
}

// "The whole word" means the word the command line holds, however deeply the
// failure was nested. Measured 2026-09-12 against ksh93 AJM 93u+: an operand
// of an expansion is not the word, so the sentence still carries the outer
// expansion and the literal text on either side of it (#1064).
func TestBadSubstitutionInAnOperandStillNamesTheWholeWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=a; v=abc; printf "[%s]" "${v#${x@QQ}}"`, `"${v#${x@QQ}}": bad substitution`},
		{`x=a; v=abc; printf "[%s]" "${v#pre${x@QQ}post}"`, `"${v#pre${x@QQ}post}": bad substitution`},
		{`x=a; printf "[%s]" "${u:-${x@QQ}}"`, `"${u:-${x@QQ}}": bad substitution`},
		{`x=a; printf "[%s]" "pre${u:-${x@QQ}}post"`, `"pre${u:-${x@QQ}}post": bad substitution`},
		{`x=a; printf "[%s]" "${u:=${x@QQ}}"`, `"${u:=${x@QQ}}": bad substitution`},
		// A `case` arm and a condition operand are whole words in their own
		// right, so there the two readings coincide -- which is why the rows
		// above are the ones that separate them.
		{`x=a; case abc in pre${x@QQ}post) ;; esac`, `pre${x@QQ}post: bad substitution`},
		{`x=a; [[ abc == pre${x@QQ}post ]]`, `pre${x@QQ}post: bad substitution`},
	} {
		out, _ := answersRun(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q", c.src, out, c.want)
		}
	}
}

// Without the family the letter is not a letter, so there is nothing to check
// against a value: an unset name is refused exactly as a set one is.
func TestABadSubstitutionIsRefusedWithoutAValueToo(t *testing.T) {
	out, st := answersRun(t, `echo "[${u@QQ}]"; echo unreached`)
	if !strings.Contains(out, "bad substitution") || strings.Contains(out, "unreached") {
		t.Errorf("output = %q, want the expansion refused and the rest abandoned", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want this shell's fatal status", st)
	}
}
