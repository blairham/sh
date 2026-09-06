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
	if !rec.seen(func(e interp.Event) bool {
		return e.Kind == interp.EventDenied && e.Action.Kind == interp.ActionOpen &&
			e.Action.Path == link && e.Action.Resolved == object
	}) {
		t.Errorf("no denial naming %q and resolving to %q reached the event stream", link, object)
	}

	// And the indistinguishability, stated exactly rather than by inspection:
	// a script refused *by name* produces the same sentence with the same
	// status, so the two differ in nothing but the path each was given.
	plain := filepath.Join(dir, "plain.sh")
	if err := os.WriteFile(plain, []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	byName := &recorder{deny: denyPath(plain)}
	shByName := shell()
	shByName.Gate, shByName.Events = byName, byName
	_, plainErrs, plainCode := runArgs(t, shByName, "testsh", plain)

	if want := strings.Replace(plainErrs, plain, link, 1); errs != want {
		t.Errorf("a refusal through a link reads\n\t%q\nand one by name reads\n\t%q\n"+
			"want them to differ only in the path", errs, want)
	}
	if plainCode != code {
		t.Errorf("a refusal by name exits %d and one through a link exits %d", plainCode, code)
	}

	// Both of those go through the gate, so a change that broke the wording of
	// every refusal alike would leave the comparison above satisfied. The
	// third run is the one with no policy at all: a script this process really
	// may not read, refused by the kernel. What the gate says has to be that
	// sentence — "a refusal comes back as a permission error" is the claim
	// driver.readFile is written for, and the operating system is the oracle
	// for what one of those reads like.
	// Stated outright as well, because the run below is only an oracle for a
	// process that is not root, and a root test run would skip it silently.
	// EACCES is "permission denied" on both platforms this test runs on; the
	// dialect here is Core, whose wording for an unreadable script is the
	// errno's own.
	if !strings.Contains(errs, "Permission denied") {
		t.Errorf("the refusal reads %q, want a permission error", errs)
	}

	unreadable := filepath.Join(dir, "unreadable.sh")
	if err := os.WriteFile(unreadable, []byte("echo ran\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(unreadable); err == nil {
		_ = f.Close()
		t.Skip("this process can read a mode-0 file, so the kernel is not an oracle here")
	}
	_, kernelErrs, kernelCode := runArgs(t, shell(), "testsh", unreadable)
	if want := strings.Replace(kernelErrs, unreadable, link, 1); errs != want {
		t.Errorf("a refused script reads\n\t%q\nand one the kernel would not open reads\n\t%q\n"+
			"want the policy's refusal to be a permission error, in those words", errs, want)
	}
	if kernelCode != code {
		t.Errorf("the kernel's refusal exits %d and the policy's exits %d", kernelCode, code)
	}
}
