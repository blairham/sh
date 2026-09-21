// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// `--mcp`, on the front end every binary is made of.
//
// This is the coverage `--acp` needed and did not have until #2585, written
// for the second protocol on the day it landed rather than a year later. The
// blind spot both times is the same shape: a protocol graded through
// `sh -dialect X` says nothing about whether the binary a shebang, `chsh`,
// `login` or an agent's configuration actually names has a flag to reach it
// with. Everything below drives MainArgs, which is the whole of a dialect
// binary's main.

// The flag reaches the hook, and the hook is handed the shell the invocation
// built rather than a fresh one.
func TestTheMCPFlagReachesTheHookOnEveryBinary(t *testing.T) {
	var out, errs strings.Builder
	reached := 0
	sh := gatedShell(&out, &errs)
	sh.ServeMCP = func(driver.Shell) int {
		reached++
		return 7
	}
	if code := driver.MainArgs(sh, []string{"testsh", "--mcp"}); code != 7 {
		t.Errorf("status %d, want the hook's own 7", code)
	}
	if reached != 1 {
		t.Errorf("the hook ran %d times, want once", reached)
	}
}

// A binary that never wired the hook refuses the word rather than accepting
// it and doing nothing. Silently accepting a flag and ignoring it is the exact
// failure `./bash -i` once had, and the mirror of it is worse because nothing
// says so.
func TestAMissingMCPHookRefusesTheFlag(t *testing.T) {
	var out, errs strings.Builder
	code := driver.MainArgs(gatedShell(&out, &errs), []string{"testsh", "--mcp"})
	if code == 0 {
		t.Error("a binary with no server accepted --mcp and exited 0")
	}
	if !strings.Contains(errs.String(), "--mcp") {
		t.Errorf("stderr = %q, want it to name the flag it refused", errs.String())
	}
}

// An attached value is refused rather than ignored: `--mcp=1` is somebody
// expecting the word to mean something, and a shell that read it as a bare
// flag would serve a protocol on a spelling nobody agreed. Operands are
// refused for the same reason — a tool call names its own command, so a word
// left on the line is somebody expecting something to run.
func TestTheMCPFlagRefusesAValueAndOperands(t *testing.T) {
	for _, args := range [][]string{
		{"testsh", "--mcp=1"},
		{"testsh", "--mcp", "script.sh"},
	} {
		var out, errs strings.Builder
		sh := gatedShell(&out, &errs)
		ran := false
		sh.ServeMCP = func(driver.Shell) int { ran = true; return 0 }
		if code := driver.MainArgs(sh, args); code == 0 {
			t.Errorf("%v exited 0, want a refusal", args)
		}
		if ran {
			t.Errorf("%v started the protocol anyway", args)
		}
	}
}

// The ordering rule, and it is the whole reason the option is recorded in the
// loop and acted on afterwards: a policy on the same line has to be installed
// *before* the protocol starts, because after that there is no invocation left
// to read — a tool call carries its own command and its own directory.
//
// This is the property #1334 asked for. An agent invoking `$SHELL -c` has
// nowhere to put a policy; an agent configured with `sh --mcp --policy p` has.
func TestAPolicyOnTheSameLineIsInstalledBeforeTheProtocolStarts(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside")
	policy := policyFile(t, dir, ws, false)

	var out, errs strings.Builder
	sh := gatedShell(&out, &errs)
	var served driver.Shell
	sh.ServeMCP = func(s driver.Shell) int { served = s; return 0 }
	if code := driver.MainArgs(sh, []string{"testsh", "--policy", policy, "--mcp"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs.String())
	}
	if served.Gate == nil {
		t.Fatal("the protocol was handed a shell with no gate: --policy did not reach it")
	}
	// And the gate is the policy rather than something that says yes: a nil
	// check alone would pass for a gate that permits everything, which is the
	// shape a broken installation has.
	if d := served.Gate.Allow(context.Background(), interp.Action{
		Kind: interp.ActionOpen, Path: outside, Write: true,
	}); d != interp.Deny {
		t.Errorf("the gate answered %v for a write the policy forbids, want Deny", d)
	}
	if d := served.Gate.Allow(context.Background(), interp.Action{
		Kind: interp.ActionOpen, Path: filepath.Join(ws, "inside"), Write: true,
	}); d != interp.Allow {
		t.Errorf("the gate answered %v for a write the policy permits, want Allow.\n"+
			"\tA gate that refuses everything scores as well as a working one on the\n"+
			"\tcheck above, which is why both are asked.", d)
	}
}
