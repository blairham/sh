// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestABadSubstitutionNamesTheRunAsListed — bash names the run around a
// refused expansion the way it lists the word, and inside `$(( ))` names the
// expression. Measured 2026-10-04 on bash 5.3.20; see parameter-expansion.md.
func TestABadSubstitutionNamesTheRunAsListed(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "${v}${x@Z}"`, "${v}${x@Z}: bad substitution"},
		{`echo "$(echo  >>/dev/null)${x@Z}"`, "$(echo >> /dev/null)${x@Z}: bad substitution"},
		{`echo "${x//$'\t'/a}${x@Z}"`, "${x//'\t'/a}${x@Z}: bad substitution"},
		{`echo "[$(( ${x@Z} + 1 ))]"`, "line 1:  ${x@Z} + 1 : bad substitution"},
		{`echo "$(( 1 ))${x@Z}"`, "$(( 1 ))${x@Z}: bad substitution"},
	} {
		_, errs, _ := runAlias(t, "x=1; "+c.src)
		if !strings.Contains(errs, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, errs, c.want)
		}
	}
}
