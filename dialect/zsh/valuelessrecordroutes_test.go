// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The two routes into a valueless record, which this column answers
// differently — #4787, and the pair that split one axis into two.
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` from a script file under `env -i
// PATH=/usr/bin:/bin`; `go version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/zsh` for ours.
//
//	emulate MODE; typeset X; typeset -p X                     the declaration
//	emulate MODE; v=g; g(){ local v; unset v; typeset -p v; }  the unset
//
//	mode   the declaration   the unset
//	zsh    typeset X=''      (nothing)
//	sh     typeset X         (nothing)
//	ksh    typeset X         (nothing)
//	csh    typeset X=''      (nothing)
//
// The `unset` column is the control and it is the whole of the finding: it
// does not move with the mode, and the other one does, so no single answer
// covers both routes here. The `zsh` and `csh` rows of the first column are
// DeclaredNameWithoutValueIsEmpty saying yes — the name has a value to list,
// so neither record is made — which is why one preset value for
// ValuelessDeclarationRecordsTheName is right under every mode and this is
// **not** a fifth field on the emulation table (#4753 put the field that does
// move there).
func TestTheTwoValuelessRecordRoutesPartUnderAnEmulation(t *testing.T) {
	if got := zsh.Semantics().ValuelessDeclarationRecordsTheName; got != interp.Yes {
		t.Errorf("the declaration's route answers %v, want interp.Yes", got)
	}
	if got := zsh.Semantics().UnsetOfALocalRecordsTheName; got != interp.No {
		t.Errorf("the unset's route answers %v, want interp.No", got)
	}
	for _, tc := range []struct {
		mode, declared string
	}{
		{"zsh", "typeset X=''\n"},
		{"sh", "typeset X\n"},
		{"ksh", "typeset X\n"},
		{"csh", "typeset X=''\n"},
	} {
		for _, route := range []struct {
			name, src, want string
		}{
			{"a bare declaration", "typeset X\ntypeset -p X\n", tc.declared},
			{
				"an unset of a local",
				"v=g\ng() { local v; unset v; typeset -p v; }\ng\n",
				"",
			},
		} {
			t.Run(tc.mode+"/"+route.name, func(t *testing.T) {
				// Combined, so a refusal or an unanswered axis would show up
				// in `out` rather than going unread.
				out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
					"emulate "+tc.mode+"\n"+route.src)
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				if out != route.want || st != 0 {
					t.Errorf("emulate %s; %s wrote %q (status %d), want %q",
						tc.mode, route.src, out, st, route.want)
				}
			})
		}
	}
}

// And the three controls the issue names, each of which would make the row
// above read as this fix when it is not.
//
// A name the shell has never heard of is the missing-name route and is
// refused; an *attributed* valueless operand already lists, by its letter,
// without reaching either record; and an assignment after the declaration
// gives the row its value back. All three measured in the same run, and all
// three unmoved by the mode.
func TestTheValuelessRecordControlsUnderTheShEmulation(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a name that is not there",
			"typeset -p NOSUCHNAME\nprint \"st=$?\"\n",
			"no such variable: NOSUCHNAME\nst=1\n",
		},
		{
			"an attributed operand",
			"typeset -x A\ntypeset -p A\n",
			"export A\n",
		},
		{
			"an assignment gives the row its value back",
			"typeset X\nX=v\ntypeset -p X\n",
			"typeset X=v\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
				"emulate sh\n"+tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("%s wrote %q, want it to end %q", tc.src, out, tc.want)
			}
		})
	}
}
