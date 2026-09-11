// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A function *defined in a file* keeps its location when it is called at a
// prompt. That is the fact TestASourcedFileInASessionKeepsItsLine records,
// reached one step later: the body was read from a file, and a file is a file
// however it was reached.
//
// It needs a question of its own because `borrowedFiles` has already gone back
// to zero by the time the *call* happens — the file is closed, and only where
// the body came from can say where it came from.
//
// Measured on zsh 5.9.2, 2026-09-11, with `lib.zsh` holding `myfunc(){ setopt
// monitor }`. `setopt monitor` is refused where the shell has no terminal, so
// the refusal is the probe:
//
//	source lib.zsh; myfunc     myfunc:setopt:1: can't change option: monitor
//
// Both the name and the line, exactly as the same script reports it
// non-interactively. #2032 took both away (#2052).
func TestAFunctionFromAFileKeepsItsLocationAtAPrompt(t *testing.T) {
	sh := shell()
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.BuiltinLocation = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	dg.PromptBuiltinLocation = interp.LocationBuiltinNameOnly
	sh.Diagnostics = dg

	path := filepath.Join(t.TempDir(), "lib.sh")
	if err := os.WriteFile(path, []byte("myfunc(){\n  nosuchcmd_zz\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs, _ := runPipedShell(t, sh, ". "+path+"\nmyfunc\n", "testsh", "-i")
	if !strings.Contains(errs, "line 2") {
		t.Errorf("a function read from a file said %q, want its own line named", errs)
	}
}

// And the control, which is what says the rule is about where the **body** was
// read from rather than about being inside a function at all: a function typed
// at the prompt is located the prompt's way, with no line.
//
// Measured in the same session on zsh 5.9.2: `myfunc(){ setopt monitor }`
// typed at the prompt and called reports `myfunc:setopt:` — named, no line —
// against `myfunc:setopt:1:` for the same function read from a file.
func TestAFunctionTypedAtThePromptIsStillLocatedThePromptsWay(t *testing.T) {
	sh := shell()
	dg := interp.CoreDiagnostics()
	dg.Location = interp.LocationLineWord
	dg.PromptLocation = interp.LocationNameOnly
	sh.Diagnostics = dg

	_, errs, _ := runPipedShell(t, sh, "myfunc(){ nosuchcmd_zz; }\nmyfunc\n", "testsh", "-i")
	if errs == "" {
		t.Fatal("nothing was reported")
	}
	if strings.Contains(errs, "line ") {
		t.Errorf("a function typed at the prompt said %q, want no line", errs)
	}
}
