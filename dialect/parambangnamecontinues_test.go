// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// Which characters carry a name on after `${!`, per dialect.
//
// One column indirects through the specials and through a positional; one has
// the construct and begins it at a name and nowhere else; three do not have it
// at all, so the `!` is always the parameter. See
// syntax.Dialect.ParamBangNameContinues for the seven columns measured a
// character at a time, and #3966 for why `$!` has to be *set* before the
// measurement says anything.
func TestWhichCharactersCarryABangNameOn(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"bash", "0123456789#?@*["},
		{"ksh", ""},
		{"zsh", ""},
		{"dash", ""},
		{"ash", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			switch tc.name {
			case "bash":
				got = bash.Dialect().ParamBangNameContinues
			case "ksh":
				got = ksh.Dialect().ParamBangNameContinues
			case "zsh":
				got = zsh.Dialect().ParamBangNameContinues
			case "dash":
				got = dash.Dialect().ParamBangNameContinues
			case "ash":
				got = ash.Dialect().ParamBangNameContinues
			}
			if got != tc.want {
				t.Errorf("ParamBangNameContinues is %q, want %q", got, tc.want)
			}
		})
	}
}

// And what that costs at the run, on the one row that parts every column.
//
// Measured 2026-09-20 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, stdin on /dev/null, with `$!` set by a completed background job
// so that an empty answer cannot be mistaken for a pid:
//
//	set -- a b c; sleep 0 & wait; printf '[%s]' "${!#}"
//
//	bash 5.3   [c] — the indirection through `$#`, which is `$3`
//	bash 3.2   [c]
//	ksh93      the pid: `$!` with a `#` trim of an empty pattern
//	dash       the pid
//	ash        the pid
//	zsh        the pid
//
// The test runs it without the background job, so the pid columns answer what
// an unset `$!` is there — empty in four of them and `0` in zsh, which is
// measured and is the same split syntax.Dialect.ParamLengthRefusesTheBangName
// records — and bash answers `c`. That is the whole thing in one row: an empty
// `$!` is what made the first reading of this construct the wrong one, and it
// is safe here only because the *indirecting* column's answer is not empty.
func TestTheBangNameOverALengthParameter(t *testing.T) {
	const src = `set -- a b c; printf "[%s]" "${!#}"`
	for _, p := range []dialecttest.Preset{
		{Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
		{Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics, Diagnostics: ksh.Diagnostics, Apply: ksh.Apply},
		{Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
		{Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics, Diagnostics: dash.Diagnostics, Apply: dash.Apply},
		{Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics, Diagnostics: ash.Diagnostics, Apply: ash.Apply},
	} {
		t.Run(p.Name, func(t *testing.T) {
			want := "[]"
			switch p.Name {
			case "bash":
				want = "[c]"
			case "zsh":
				want = "[0]"
			}
			out, st, err := p.Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != want || st != 0 {
				t.Errorf("${!#} answered %q at %d, want %q at 0", out, st, want)
			}
		})
	}
}
