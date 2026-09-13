// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// `%m` counts and `%M` does not, through this dialect's own vector.
//
// The substrate's test says what the field does; this one says that zsh is
// the shell that asks for it — the table names FieldHost for `%m`, and the
// count only reaches that field because PromptStyle sets NumericArgument. A
// dialect that dropped the flag would answer `a` to every row below and pass
// every test in interp.
//
// Measured on zsh 5.9.2, 2026-09-13 with `HOST=a.b.c.d`, which is the shortest
// name that tells "the leading n" apart from "the whole name": this machine's
// own has two components, where the two readings agree.
func TestTheShortHostCodeCountsAndTheLongOneDoesNot(t *testing.T) {
	r := zshRunnerForTest(t)
	r.SetPromptHost("a.b.c.d")
	for _, tc := range []struct {
		field interp.PromptField
		code  string
		arg   string
		want  string
	}{
		{interp.FieldHost, "%m", "", "a"},
		{interp.FieldHost, "%1m", "1", "a"},
		{interp.FieldHost, "%2m", "2", "a.b"},
		{interp.FieldHost, "%3m", "3", "a.b.c"},
		{interp.FieldHost, "%9m", "9", "a.b.c.d"},
		{interp.FieldHost, "%-m", "-", "d"},
		{interp.FieldHost, "%-2m", "-2", "c.d"},
		{interp.FieldHostFull, "%M", "", "a.b.c.d"},
		{interp.FieldHostFull, "%2M", "2", "a.b.c.d"},
		{interp.FieldHostFull, "%-2M", "-2", "a.b.c.d"},
	} {
		got, ok := r.PromptField(tc.field, tc.arg, false)
		if !ok || got != tc.want {
			t.Errorf("%s = %q (ok=%v), want %q", tc.code, got, ok, tc.want)
		}
	}
}
