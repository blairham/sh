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
