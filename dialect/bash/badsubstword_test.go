// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// What this shell's bad-substitution sentence names, and what it does about a
// `@` letter on a name with no value. Measured 2026-09-05 against bash 5.3.15
// in the oracle environment.

func TestBadSubstitutionNamesTheQuotingRun(t *testing.T) {
	if got, want := bash.Diagnostics().BadSubstitutionNames, interp.NamesTheQuotingRun; got != want {
		t.Errorf("BadSubstitutionNames = %v, want %v", got, want)
	}
	for _, c := range []struct{ src, want string }{
		// `bash -c 'x=a; echo "[${x@QQ}]"'` blames the word, not the `${…}`.
		{`x=a; echo "[${x@QQ}]"`, "[${x@QQ}]: bad substitution"},
		{`x=a; echo pre${x@QQ}post`, "pre${x@QQ}post: bad substitution"},
		// A change of quoting inside the word ends what is named.
		{`x=a; echo 'lit'"${x@QQ}"`, "${x@QQ}: bad substitution"},
	} {
		out, _ := runBash(t, t.TempDir(), c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q", c.src, out, c.want)
		}
	}
}

// An operand is a word of its own, and the run named is the one inside it.
// Measured 2026-09-12 against bash 5.3.15: the literal text on either side of
// the *outer* expansion is never in the sentence, and neither is the outer
// expansion itself (#1064).
func TestBadSubstitutionInAnOperandNamesTheOperand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A pattern operand: `#`, `%` and `/` all build one the same way.
		{`x=a; v=abc; echo "${v#${x@QQ}}"`, "${x@QQ}: bad substitution"},
		{`x=a; v=abc; echo "${v%${x@QQ}}"`, "${x@QQ}: bad substitution"},
		{`x=a; v=abc; echo "${v/${x@QQ}/z}"`, "${x@QQ}: bad substitution"},
		// The literal text around the outer expansion stays out of it.
		{`x=a; v=abc; echo "pre${v#${x@QQ}}post"`, "${x@QQ}: bad substitution"},
		// A run *within* the operand is still a run: the operand's own
		// literal text is named because it shares the failure's quoting.
		{`x=a; v=abc; echo "${v#pre${x@QQ}post}"`, "pre${x@QQ}post: bad substitution"},
		// A value operand, which reaches the same subject by another route.
		{`x=a; echo "${u:-${x@QQ}}"`, "${x@QQ}: bad substitution"},
		// And one nested in the other.
		{`x=a; v=abc; echo "${v#${u:-${x@QQ}}}"`, "${x@QQ}: bad substitution"},
		// The pattern of a `case` arm and of a condition are the same word,
		// and there the operand *is* the whole word.
		{`x=a; case abc in pre${x@QQ}post) ;; esac`, "pre${x@QQ}post: bad substitution"},
		{`x=a; [[ abc == pre${x@QQ}post ]]`, "pre${x@QQ}post: bad substitution"},
	} {
		out, _ := runBash(t, t.TempDir(), c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q said %q, want %q", c.src, out, c.want)
		}
	}
}

// One diagnostic per command, not one per bad word.
func TestOnlyTheFirstBadWordIsDiagnosed(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `x=a; printf "[%s]" "${x@QQ}" "${x@ZZ}" "${x@YY}"`)
	if n := strings.Count(out, "bad substitution"); n != 1 {
		t.Errorf("wrote %d diagnostics in %q, want one", n, out)
	}
}

// The letter is checked against the value: `${u@QQ}` on an unset name is
// empty at status 0 and the same word on a set one is refused.
func TestATransformLetterIsCheckedOnlyOnAValue(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `echo "[${u@QQ}]"; echo "u=$?"`)
	if !strings.Contains(out, "[]") || !strings.Contains(out, "u=0") {
		t.Errorf("output = %q, want an empty expansion at status 0", out)
	}
	if strings.Contains(out, "bad substitution") {
		t.Errorf("output = %q, want no diagnostic for a name with no value", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}

	out, st = runBash(t, t.TempDir(), `x=a; echo "[${x@QQ}]"; echo unreached`)
	if !strings.Contains(out, "bad substitution") || strings.Contains(out, "unreached") {
		t.Errorf("output = %q, want the same word refused once it has a value", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want the fatal status off a command string", st)
	}
}
