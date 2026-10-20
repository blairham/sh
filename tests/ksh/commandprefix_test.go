// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `command` in front of a special builtin does not take its prefix away here,
// where dash and BusyBox ash both do. Measured 2026-09-18 on ksh93u+
// 2012-08-01 from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME (#3448).
func TestCommandDoesNotTakeASpecialBuiltinsPrefixAway(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		{`s=base; s=C command :; echo "[$s]"`, "[C]\n"},
		{`s=base; s=E command eval :; echo "[$s]"`, "[E]\n"},
		// The bare special builtin, which persists in dash too — the control
		// that says the difference is `command`'s.
		{`s=base; s=P :; echo "[$s]"`, "[P]\n"},
		// And a regular builtin behind the word, whose prefix never
		// persisted anywhere.
		{`s=base; s=Q command true; echo "[$s]"`, "[base]\n"},
		// The subscripted spelling rides along.
		{`arr=(x y z); arr[1]=A command :; echo "[${arr[1]}]"`, "[A]\n"},
	} {
		out, st := runKsh(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}

// And a whole-table listing walks the environment the command was handed, so
// the prefix's entry is in it and counts as exported however the attribute
// table answers — on the very line whose named listing writes the unexported
// form (#3446).
func TestAWholeTableListingWalksTheCommandsEnvironment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		{`export k=1; k=9 export -p`, "export k=9\n"},
		{`m=1; m=9 export -p`, "export m=9\n"},
		{`z=9 export -p`, "export z=9\n"},
		// The named listing on the same line writes the unexported form,
		// which is what says the export-ness belongs to the listing's
		// population and not to the name.
		{`m=1; m=9 typeset -p m`, "m=9\n"},
		// And the admission belongs to the *export* filter alone: a listing
		// narrowed some other way writes nothing for the name.
		{`m=1; m=9 readonly -p`, ""},
	} {
		out, st := runKsh(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s answered %d: %q", row.src, st, out)
			continue
		}
		if row.want == "" {
			if strings.Contains(out, "m=") {
				t.Errorf("%s = %q, want no row for the name", row.src, out)
			}
			continue
		}
		if !strings.Contains(out, row.want) {
			t.Errorf("%s = %q, want it to contain %q", row.src, out, row.want)
		}
	}
}
