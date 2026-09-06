// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// The script operand, through a link.
//
// `sh -policy p link` where the link points into a directory the policy hides
// used to read the script and run it: the gate was asked about the name the
// invocation gave, and the front end then handed that name to os.ReadFile.
// The refusal has to happen on the object, and it has to be the ordinary
// refusal — a diagnostic that named the target would tell whoever wrote the
// link where it went, which is the one fact the rule exists to withhold.
func TestAScriptOperandThroughALinkIsCheckedOnWhatItReached(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := t.TempDir()
	hidden := filepath.Join(dir, "hidden")
	if err := os.Mkdir(hidden, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(hidden, "real.sh")
	if err := os.WriteFile(target, []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent.sh")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	object, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}

	// Parsed rather than hand-written: a rule that does not mean what its
	// author thinks matches nothing, and matching nothing reads exactly like
	// a rule being obeyed.
	p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
		"version 1\ndefault allow\ndeny read %s/**\n", hidden)))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	rec := &recorder{deny: func(a interp.Action) bool {
		return p.Allow(t.Context(), a) == interp.Deny
	}}
	sh := shell()
	sh.Gate, sh.Events = rec, rec

	out, errs, code := runArgs(t, sh, "testsh", link)

	if strings.Contains(out, "ran") {
		t.Errorf("out = %q, want a script reached through a link into a denied place not to have run", out)
	}
	if code != 126 {
		t.Errorf("status = %d, want 126 for a script that would not open", code)
	}
	// The diagnostic names the link and not the target, exactly as it does for
	// a path denied by name. A script that could tell the two apart could map
	// a hidden directory one link at a time by asking to be refused.
	if !strings.Contains(errs, link) {
		t.Errorf("err = %q, want the name as written", errs)
	}
	if strings.Contains(errs, object) || strings.Contains(errs, target) {
		t.Errorf("err = %q, want it not to say where the link went", errs)
	}
	// The record does say, because that half belongs to whoever wrote the
	// policy and cannot be acted on otherwise.
	if !opened(rec, interp.EventDenied, object) {
		t.Errorf("no denial naming %q reached the event stream", object)
	}
}
