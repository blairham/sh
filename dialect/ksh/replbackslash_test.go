// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A backslash an unquoted expansion put in a replacement quotes a backslash
// behind it and stands as itself in front of anything else. Measured
// 2026-10-03 on ksh93u+.
func TestAnExpandedReplacementBackslashQuotesABackslash(t *testing.T) {
	src := `v=abc; p='[\&]'; q='[\\&]'; r='[\a]'; s='x\\\\y'; printf "[%s]" "${v/b/$p}" "${v/b/$q}" "${v/b/$r}" "${v/b/$s}" "${v/b/"$q"}"`
	const want = `[a[\&]c][a[\&]c][a[\a]c][ax\\yc][a[\\&]c]`
	var out, errs bytes.Buffer
	driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", src})
	if got := out.String() + errs.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
