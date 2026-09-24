// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An unquoted `${*@X}` yields one field per parameter, exactly as `$*` and
// every other operator on `*` already did.
//
// The join is only a *different* reading where the split then runs on what it
// produced, which is the rule interp/expand.go's elementFields already holds
// and asks conditionally. This branch carried its own copy that joined
// unconditionally and split after, so with `IFS` set and empty — where there
// is no split to undo the join — it gave one field where the panel gives one
// per parameter.
//
// Measured 2026-09-23 with `set -- ' a ' ' b '` and `IFS=`,
// `printf '<%s>' …`, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	                the panel          here, before
//	${*@Q}          <' a '><' b '>       <' a '' b '>
//	${*@K}          <' a '><' b '>       <' a '' b '>
//	${*@E}          < a >< b >         < a  b >
//	${*@U}          < A >< B >         < A  B >
//	$*              < a >< b >         < a >< b >        already right
//	${*,,}          < a >< b >         < a >< b >        already right
//	${@@Q}          <' a '><' b '>       <' a '><' b '>  already right
//
// It is the same defect the scalar path was fixed for and this branch was
// left holding — the shape that keeps being found here, where a second helper
// omits the fix the first one carries.
func TestAnUnquotedStarTransformKeepsOneFieldPerParameter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"@Q with an empty IFS", `IFS=; printf '<%s>' ${*@Q}`, `<' a '><' b '>`},
		{"@E with an empty IFS", `IFS=; printf '<%s>' ${*@E}`, `< a >< b >`},
		{"@U with an empty IFS", `IFS=; printf '<%s>' ${*@U}`, `< A >< B >`},
		// The join is the reading a split can undo, and where a split can it
		// still stands: these rows say the fix did not simply stop joining.
		{"@Q with a one-character IFS", `IFS=:; printf '<%s>' ${*@Q}`, `<' a '><' b '>`},
		{"quoted, empty IFS", `IFS=; printf '<%s>' "${*@Q}"`, `<' a '' b '>`},
		{"quoted, one-character IFS", `IFS=:; printf '<%s>' "${*@Q}"`, `<' a ':' b '>`},
		// `@` is the other name, which never joins.
		{"at, empty IFS", `IFS=; printf '<%s>' ${@@Q}`, `<' a '><' b '>`},
		// And with no parameters at all, both names give no fields.
		{"no parameters", `set --; IFS=; printf '<%s>' ${*@Q}`, `<>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			sh.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
			code := driver.MainArgs(sh, []string{"bash", "-c", `set -- ' a ' ' b '; ` + c.src})
			if got := strings.TrimSpace(out.String()); got != c.want || code != 0 {
				t.Errorf("= %q status %d (stderr %q), want %q at 0",
					got, code, errs.String(), c.want)
			}
		})
	}
}
