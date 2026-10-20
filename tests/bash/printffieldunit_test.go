// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A string conversion's field is counted in bytes here, and in characters
// where the `l` modifier is written and the locale has them. Measured
// 2026-09-16 on bash 5.3.20 from a script file under `env -i
// PATH=/usr/bin:/bin`, with `LC_ALL` as each row sets it (#2298). See
// Semantics.PrintfFieldCountsCharacters and
// Semantics.PrintfLongModifierCountsCharacters.

func TestAPrintfFieldIsBytesAndTheLongModifierMakesItCharacters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a precision", `LC_ALL=en_US.UTF-8; printf '[%.2s]' αβγ`, "[α]"},
		{"a width", `LC_ALL=en_US.UTF-8; printf '[%7s]' αβγ`, "[ αβγ]"},
		{"%b", `LC_ALL=en_US.UTF-8; printf '[%.2b]' αβγ`, "[α]"},
		{"%q", `LC_ALL=en_US.UTF-8; printf '[%.2q]' αβγ`, "[α]"},
		{"the l precision", `LC_ALL=en_US.UTF-8; printf '[%.2ls]' αβγ`, "[αβ]"},
		{"the l width", `LC_ALL=en_US.UTF-8; printf '[%7ls]' αβγ`, "[    αβγ]"},
		{"%lc", `LC_ALL=en_US.UTF-8; printf '[%lc]' αβγ`, "[α]"},
		{"%lc in a width", `LC_ALL=en_US.UTF-8; printf '[%3lc]' αβγ`, "[  α]"},
		{"%lc's precision", `LC_ALL=en_US.UTF-8; printf '[%.0lc]' abc`, "[]"},
		{"%lb", `LC_ALL=en_US.UTF-8; printf '[%.2lb]' αβγ`, "[α]"},
		// An unset locale is a Unicode-aware one in this column.
		{"no locale named", `unset LC_ALL LC_CTYPE LANG; printf '[%.2ls]' αβγ`, "[αβ]"},
		{"the C locale", `LC_ALL=C; printf '[%.2ls|%lc]' αβγ αβγ`, "[α|\xce]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", tc.src})
			if out.String() != tc.want || errs.String() != "" || code != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out.String(), errs.String(), code, tc.want)
			}
		})
	}
}
