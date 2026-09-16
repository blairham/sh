// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell has no `$_` at all, and the difference from the three that do is
// a status rather than a wording.
//
// Measured 2026-09-16 over `-c` under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// on Apple's dash-16 (/bin/dash) and on upstream dash 0.5.12 built from
// source on the same machine — identical on both:
//
//	${_+x}                        empty
//	set -u; printf '[%s]' "$_"    dash: 1: _: parameter not set     status 2
//	_=inherited in the env        `inherited` — an ordinary name
//	_=mine; echo "$_"             mine
//
// bash 5.3.20, zsh 5.9.2 and ksh93 93u+ all answer `${_+x}` non-empty and
// read it under `set -u` at 0. BusyBox 1.37.0 agrees with this column, which
// is why the two POSIX-family dialects take the preset's No.
//
// interp.Runner.ensureSpecials registered the producer for every dialect, so
// this column answered `[]` at 0 — the script that its reference stops (#3380).
func TestThereIsNoUnderscoreParameter(t *testing.T) {
	base := func() dialecttest.Base {
		return dialecttest.Base{Name: "dash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}
	}
	out, _, err := preset.Combined(t, base(), `printf '[%s]\n' "${_+set}"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, want `[]` — this shell keeps no such parameter", out)
	}

	out, status, err := preset.Combined(t, base(), `set -u; printf '[%s]\n' "$_"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if status != 2 || !strings.Contains(out, "_: parameter not set") {
		t.Errorf("got %q at %d, want the reference's refusal at 2", out, status)
	}

	// The other half, and the reason this is not "dash has no `_`": an `_`
	// the environment brought is read, because the name is an ordinary one
	// here rather than a parameter the shell provides.
	env := base()
	env.Env = append(env.Env, "_=inherited")
	out, _, err = preset.Combined(t, env, `printf '[%s][%s]\n' "${_+set}" "$_"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[set][inherited]" {
		t.Errorf("got %q, want the environment's `_` to show through", out)
	}

	out, _, err = preset.Combined(t, base(), `_=mine; printf '[%s]\n' "$_"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[mine]" {
		t.Errorf("got %q, want `_` to be assignable like any other name", out)
	}
}
