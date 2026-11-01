// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A list that joins one descriptor both for reading and for writing is
// refused, and the shell ends where it ends over a duplication target that is
// not a descriptor. Measured 2026-10-16 on zsh 5.9.2 under `-fc` (#6647). See
// interp's joinModes.
func TestAJoinThatGoesBothWaysIsRefused(t *testing.T) {
	t.Parallel()
	const refused = "file mode mismatch on fd "
	for _, c := range []struct {
		src, out string
		status   int
		errs     string
	}{
		{`: 3</dev/null 3>/dev/null; echo after`, "", 1, refused + "3"},
		{`: 3<>/dev/null 3>/dev/null; echo after`, "", 1, refused + "3"},
		{`: 3</dev/null 3>&1; echo after`, "", 1, refused + "3"},
		{`: 3<<<x 3>/dev/null; echo after`, "", 1, refused + "3"},
		{`: >/dev/null 1</dev/null; echo after`, "", 1, refused + "1"},
		{`echo x | : >/dev/null 0>/dev/null; echo after`, "", 1, refused + "0"},
		{`f() { :; }; f 3</dev/null 3>/dev/null; echo after`, "", 1, refused + "3"},
		{`( : ) 3</dev/null 3>/dev/null; echo after $?`, "after 1\n", 0, refused + "3"},
		{`/bin/cat 3</dev/null 3>/dev/null </dev/null; echo after $?`, "after 1\n", 0, refused + "3"},
		// Joins that go one way, and a close, which goes neither.
		{`: 3</dev/null 3<>/dev/null; echo after`, "after\n", 0, ""},
		{`: 3>/dev/null 3>>/dev/null; echo after`, "after\n", 0, ""},
		{`: 3</dev/null 3>&-; echo after`, "after\n", 0, ""},
		{`: 1</dev/null; echo after`, "after\n", 0, ""},
		{`setopt nomultios; : 3</dev/null 3>/dev/null; echo after`, "after\n", 0, ""},
	} {
		out, st, errs := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
		if out != c.out || st != c.status || !strings.Contains(errs, c.errs) || (c.errs == "" && errs != "") {
			t.Errorf("%s: got %q, %d, %q; want %q, %d, %q", c.src, out, st, errs, c.out, c.status, c.errs)
		}
	}
}
