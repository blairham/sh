// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A declaration with no value on it — #4753, and the fourth axis an emulation
// moves that has no option name over it.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` under `env -i PATH=/usr/bin:/bin`;
// `go version -m` says *not a Go executable* for it. Read two ways, which is
// what says the **mode** carries this and not the word: the reference copied
// to files called `zsh`, `sh` and `ksh` — the first letter of argv[0] is the
// whole of what differs — and `emulate MODE` under the reference's own name,
// plain and `-R`, which agree row for row.
//
//	typeset X; printf '[%s]' "${X+set}"
//
//	under the name zsh   [set]        emulate zsh   [set]   emulate -R zsh   [set]
//	under the name sh    []           emulate sh    []      emulate -R sh    []
//	under the name ksh   []           emulate ksh   []      emulate -R ksh   []
//	                                  emulate csh   [set]   emulate -R csh   [set]
//
// The two falsifiers are what keep this from being keyed on the wrong noun:
// **the name does not matter** — `declare X`, `integer N` and `local X` inside
// a function all answer the same way as `typeset X` — and **the mode does**.
func TestTheEmulationDecidesWhetherAValuelessDeclarationSetsTheName(t *testing.T) {
	if got := zsh.Semantics().DeclaredNameWithoutValueIsEmpty; got != interp.Yes {
		t.Errorf("the preset answers %v, want interp.Yes", got)
	}
	for _, tc := range []struct {
		mode string
		want interp.Answer
	}{
		{"zsh", interp.Yes},
		{"sh", interp.No},
		{"ksh", interp.No},
		// csh is on zsh's side here and on the sh side of CdWithoutHomeIsAnError,
		// which is why the emulation table carries four booleans and not one.
		{"csh", interp.Yes},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
			b, ok := r.Builtin("emulate")
			if !ok {
				t.Fatal("no `emulate` builtin, so nothing here is being asked")
			}
			if st := b(r, context.Background(), []string{"--", tc.mode}); st != 0 {
				t.Fatalf("emulate %s: status %d", tc.mode, st)
			}
			if got := r.Semantics.DeclaredNameWithoutValueIsEmpty; got != tc.want {
				t.Errorf("emulate %s left DeclaredNameWithoutValueIsEmpty %v, want %v",
					tc.mode, got, tc.want)
			}
		})
	}
}

// And what a script sees, per mode and per spelling.
//
// The last row of each mode is the control that stops the fix from being "a
// declaration never creates the name": `typeset X=` is an assignment of the
// empty string and reads `set` under every mode, in the reference and here.
func TestAValuelessDeclarationPerModeAndSpelling(t *testing.T) {
	for _, tc := range []struct {
		name, src       string
		zshWant, shWant string
	}{
		{"typeset", `typeset X; printf '[%s]' "${X+set}"`, "[set]", "[]"},
		{"declare", `declare X; printf '[%s]' "${X+set}"`, "[set]", "[]"},
		{"integer", `integer N; printf '[%s]' "${N+set}"`, "[set]", "[]"},
		{"local in a function", `f(){ local X; printf '[%s]' "${X+set}"; }; f`, "[set]", "[]"},
		{"the assigned control", `typeset X=; printf '[%s]' "${X+set}"`, "[set]", "[set]"},
	} {
		for _, mode := range []struct {
			mode, want string
		}{
			{"zsh", tc.zshWant},
			{"sh", tc.shWant},
			{"ksh", tc.shWant},
		} {
			t.Run(tc.name+"/"+mode.mode, func(t *testing.T) {
				dir := t.TempDir()
				out, _, err := preset.Combined(t, dialecttest.Base{Dir: dir},
					"emulate "+mode.mode+"\n"+tc.src+"\n")
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				if out != mode.want {
					t.Errorf("emulate %s; %s wrote %q, want %q", mode.mode, tc.src, out, mode.want)
				}
			})
		}
	}
}
