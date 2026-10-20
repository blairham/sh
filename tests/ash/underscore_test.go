// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// BusyBox has no `$_` either, and this column is the one that could not be
// asked when #3380 was filed — the reference exists nowhere on a macOS
// machine, so the repair would have moved this dialect on a guess.
//
// Measured 2026-09-16 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, each probe over `-c` under `env -i PATH=/usr/bin:/bin`:
//
//	${_+x}                        empty
//	set -u; printf '[%s]' "$_"    /bin/ash: _: parameter not set    status 2
//	_=inherited in the env        `inherited`
//	true one two; ${_+x}          still empty — nothing tracks it
//	_=mine; echo "$_"             mine
//
// Row for row what dash answers, which is why both take the POSIX preset's
// No and the three that grew the parameter override it.
func TestThereIsNoUnderscoreParameter(t *testing.T) {
	base := func() dialecttest.Base {
		return dialecttest.Base{Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}}
	}
	out, _, err := preset.Combined(t, base(), `printf '[%s]\n' "${_+set}"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, want `[]` — BusyBox keeps no such parameter", out)
	}

	out, status, err := preset.Combined(t, base(), `set -u; printf '[%s]\n' "$_"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if status != 2 || !strings.Contains(out, "_: parameter not set") {
		t.Errorf("got %q at %d, want the reference's refusal at 2", out, status)
	}

	// The last argument does not move it, which is what says the absence is
	// the parameter rather than the tracking: a shell that tracked and had
	// no name would still write one on the first command.
	out, _, err = preset.Combined(t, base(), `true one two; printf '[%s]\n' "${_+set}"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, want a command to leave the name absent", out)
	}

	env := base()
	env.Env = append(env.Env, "_=inherited")
	out, _, err = preset.Combined(t, env, `printf '[%s][%s]\n' "${_+set}" "$_"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != "[set][inherited]" {
		t.Errorf("got %q, want the environment's `_` to show through", out)
	}
}
