// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A gate that matches names and a script that writes a symbolic link are the
// oldest way past a path policy, and these are the tests that say it no longer
// works here. Each one is a bypass first and an assertion second: the fixture
// is the attack, and what is asserted is that the attack fails.
//
// The fixtures live under t.TempDir() and nowhere else. A symbolic link is a
// thing that points somewhere, and a test that pointed one at a person's home
// directory would be a test that could damage the machine it ran on.

// linkFarm builds the shape every test here needs: a directory the policy
// hides, a file in it, and a directory the policy allows holding a link that
// reaches into the first. It returns the root, the hidden file and the link.
func linkFarm(t *testing.T) (secret, link string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret = filepath.Join(dir, "secret", "data")
	if err := os.WriteFile(secret, []byte("classified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "work", "innocent")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	return secret, link
}

// hidesSecrets is the policy every test here runs under, and it is written
// the way the recorded limit says a name policy has to be written: it refuses
// a place, by name, and knows nothing about links.
//
// It matches on a component rather than on the whole path because the kernel
// answers with the physical spelling of a name — /private/var/… for a
// /var/folders/… temporary directory on a Mac — and a rule pinned to one
// spelling would pass by being unreachable rather than by being obeyed.
func hidesSecrets(denied *[]Action) func(*Runner) {
	return func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			if strings.Contains(a.Path, string(filepath.Separator)+"secret"+string(filepath.Separator)) ||
				strings.HasSuffix(a.Path, string(filepath.Separator)+"secret") {
				if denied != nil {
					*denied = append(*denied, a)
				}
				return Deny
			}
			return Allow
		})
	}
}

// TestALinkedReadIsRefused is the bypass in #703, as a test.
//
// `deny read /etc/**` did not stop `cat < link` where the link reached into
// /etc, because the decision was made about the name and the name was not the
// object. The open now says which object it got.
func TestALinkedReadIsRefused(t *testing.T) {
	_, link := linkFarm(t)
	out, st := run(t, "cat <"+link, hidesSecrets(nil))
	if strings.Contains(out, "classified") {
		t.Fatalf("a link reached a file the policy hides: %q", out)
	}
	if !strings.Contains(out, "refused") {
		t.Errorf("the refusal must be reported, got %q", out)
	}
	if st == 0 {
		t.Error("a refused redirect must fail its command")
	}
}

// TestALinkedWriteIsRefusedBeforeAnythingIsDestroyed.
//
// The truncation is the half that would have made a refusal worthless. A `>`
// empties its target as part of the open, so a gate that opened first and
// refused afterwards would report a refusal over a file it had already
// destroyed — the write is stopped and the damage is done. Both halves are
// asserted, and the second is the one worth having.
func TestALinkedWriteIsRefusedBeforeAnythingIsDestroyed(t *testing.T) {
	secret, link := linkFarm(t)
	out, st := run(t, "echo clobbered >"+link, hidesSecrets(nil))
	if !strings.Contains(out, "refused") || st == 0 {
		t.Errorf("the write must be refused, got %q status %d", out, st)
	}
	after, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "classified\n" {
		t.Errorf("the hidden file is now %q; a refused write truncated it anyway", after)
	}
}

// TestALinkedAppendIsRefused, because `>>` is a write the policy speaks about
// in the same breath and reaches the same open.
func TestALinkedAppendIsRefused(t *testing.T) {
	secret, link := linkFarm(t)
	out, _ := run(t, "echo more >>"+link, hidesSecrets(nil))
	if !strings.Contains(out, "refused") {
		t.Errorf("the append must be refused, got %q", out)
	}
	after, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "classified\n" {
		t.Errorf("the hidden file is now %q; a refused append wrote anyway", after)
	}
}

// TestASourcedLinkIsRefused. `.` pulls a file into the interpreter and runs
// it, which makes it the open with the most to lose and the one a redirect
// test would not cover: it reads through a different call.
func TestASourcedLinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "secret", "hidden.sh")
	if err := os.WriteFile(script, []byte("echo ran-the-hidden-script\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent.sh")
	if err := os.Symlink(script, link); err != nil {
		t.Fatal(err)
	}
	var kinds []EventKind
	out, st := run(t, ". "+link, func(r *Runner) {
		hidesSecrets(nil)(r)
		r.Events = SinkFunc(func(_ context.Context, e Event) {
			if e.Action.Kind == ActionOpen {
				kinds = append(kinds, e.Kind)
			}
		})
	})
	if strings.Contains(out, "ran-the-hidden-script") {
		t.Fatalf("a link sourced a file the policy hides: %q", out)
	}
	if st == 0 {
		t.Errorf("a refused `.` must fail, got %q status %d", out, st)
	}
	// The record has to say it was refused and not that the read failed. A
	// policy decision reported as an I/O error is a decision nobody auditing
	// the run can find, and the two are trivially confused here because both
	// reach the same diagnostic.
	if len(kinds) != 1 || kinds[0] != EventDenied {
		t.Errorf("open events = %v, want one denial and no error", kinds)
	}
}

// TestAGlobDoesNotEnumerateAHiddenDirectoryThroughALink.
//
// A listing is the loudest oracle a filesystem has — the names in a directory
// are often the secret — so the directory read is verified for the same
// reason the file read is, and refuses the same way a missing directory
// answers: with nothing, and quietly.
func TestAGlobDoesNotEnumerateAHiddenDirectoryThroughALink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first-secret", "second-secret"} {
		if err := os.WriteFile(filepath.Join(dir, "secret", name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "peek")
	if err := os.Symlink(filepath.Join(dir, "secret"), link); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, "echo "+link+"/*", hidesSecrets(nil))
	if strings.Contains(out, "first-secret") || strings.Contains(out, "second-secret") {
		t.Fatalf("a link listed a directory the policy hides: %q", out)
	}
}

// TestARefusalNamesTheLinkAndNotWhereItWent.
//
// The decision this pins down: a refusal that named the target would hand the
// script the one fact the rule exists to withhold, and would let it read a
// hidden directory one link at a time by asking to be refused. So the
// diagnostic names what the script wrote, and the audit record — the
// operator's side, which has to be actionable — names what was reached.
func TestARefusalNamesTheLinkAndNotWhereItWent(t *testing.T) {
	_, link := linkFarm(t)
	var denied []Action
	var events []Event
	out, _ := run(t, "cat <"+link, func(r *Runner) {
		hidesSecrets(&denied)(r)
		r.Events = SinkFunc(func(_ context.Context, e Event) {
			if e.Kind == EventDenied {
				events = append(events, e)
			}
		})
	})
	if !strings.Contains(out, filepath.Base(link)) {
		t.Errorf("the diagnostic must name the path as written, got %q", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("the diagnostic disclosed where the link went: %q", out)
	}
	// The record carries both names, which is #943: Path is the name the
	// script wrote and Resolved is where it went. Half of one fact in each of
	// two records is what this used to be — a refusal named the object and
	// not the name that reached it, an allowed open named the name and not the
	// object.
	if len(events) != 1 {
		t.Fatalf("the audit record must exist, got %v", events)
	}
	if got := events[0].Action.Path; got != link {
		t.Errorf("the record names %q, want the path as written", got)
	}
	if !strings.Contains(events[0].Action.Resolved, "secret") {
		t.Errorf("the audit record must say what was reached, got %+v", events[0].Action)
	}
	if len(denied) == 0 || events[0].Action.ID != denied[len(denied)-1].ID {
		t.Errorf("the record and the consultation must be the same action, got %v and %v", events, denied)
	}
}

// TestALinkToAnAllowedFileStillOpens. A check that refuses everything is not
// a check, and this is the half that would catch one.
func TestALinkToAnAllowedFileStillOpens(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ordinary")
	if err := os.WriteFile(target, []byte("visible\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "pointer")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	out, st := run(t, "cat <"+link, hidesSecrets(nil))
	if out != "visible\n" || st != 0 {
		t.Errorf("got %q status %d, want the file read through the link", out, st)
	}
}

// TestAnOrdinaryWriteUnderAGateStillTruncates.
//
// The truncation is held back until the object is verified, which means the
// ordinary path now truncates somewhere it did not before. `>` over a longer
// file is where a mistake there would show, and nowhere else.
func TestAnOrdinaryWriteUnderAGateStillTruncates(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "log")
	if err := os.WriteFile(target, []byte("a much longer previous line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, st := run(t, "echo short >"+target, hidesSecrets(nil)); st != 0 {
		t.Fatalf("the write failed, status %d", st)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "short\n" {
		t.Errorf("file = %q, want the old contents gone", got)
	}
}

// TestWritingToADeviceUnderAGate. O_TRUNC is a no-op in the kernel for
// anything that is not a regular file, and ftruncate is not — so the held-back
// half has to know the difference or `> /dev/null` stops working, which is
// about the most common thing a script does.
func TestWritingToADeviceUnderAGate(t *testing.T) {
	if _, st := run(t, "echo into-the-void >/dev/null", hidesSecrets(nil)); st != 0 {
		t.Errorf("`> /dev/null` failed under a gate, status %d", st)
	}
}

// TestAHardLinkIsNotReliablyCovered records the scope decision as something
// that runs — and records, since #1029, that on one platform the scope is not
// a decision anybody made.
//
// A hard link has no link to follow and no second name to notice: both names
// are real names for one inode. #922 recorded that as out of scope on the
// grounds that "the kernel answers with whichever name the descriptor was
// opened by", so the second consultation is handed the name the script wrote
// and the limit is simply open. On Linux that premise is exact: /proc answers
// from the dentry the descriptor was opened through, one dentry per name, and
// nothing afterwards moves it.
//
// On Darwin it is false. F_GETPATH answers from the vnode, which carries one
// name for an object however many the filesystem holds, and a lookup of any
// of them re-stamps it — so which of an inode's names comes back is not a
// property of the descriptor at all. The consequence here is that this test's
// subject is inherently a coin: the limit needs one allowed name and one
// denied name for the same object, which is exactly the configuration whose
// answer Darwin does not fix. Measured on 25.5.0: the open is refused 18 runs
// in 3000 with no load and no -race, which is what made #1029 a red required
// check on unrelated pull requests, and 0.12% through a bare open of the same
// fixture. No layout removes it — two names in one directory measures 0.06%,
// which is half of nothing rather than none.
//
// So the limit is asserted where it is determinate and nowhere else.
// internal/opened's TestTwoNamesForOneObjectAndWhetherTheAnswerHolds pins the
// mechanism at 500 out of 500 on Darwin and 500 out of 500 the other way on
// Linux, and it is the test a future change closing this gap will fail.
// docs/design/sandboxing.md carries what it costs — including the direction
// that is not a flake, where a second hard link in an allowed place reopens
// the #703 symbolic-link bypass this file exists to have closed.
//
// What is asserted on Darwin is the half that does not depend on the coin,
// and it is the half about the boundary: whichever name the gate judged, it
// was a real name of the object, the operator's record says which, and the
// script was told only the name it wrote. A refusal that named the target
// would hand a script the one fact the rule exists to withhold, and it would
// do so on the 0.6% of runs nobody is watching.
func TestAHardLinkIsNotReliablyCovered(t *testing.T) {
	secret, _ := linkFarm(t)
	hard := filepath.Join(filepath.Dir(filepath.Dir(secret)), "work", "hard")
	if err := os.Link(secret, hard); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	var denied []Action
	out, st := run(t, "cat <"+hard, hidesSecrets(&denied))

	if runtime.GOOS != "darwin" {
		// The premise #922 recorded, asserted as the single answer it is.
		if out != "classified\n" || st != 0 {
			t.Errorf("got %q status %d — a hard link is a recorded limit, and this now behaves differently", out, st)
		}
		if len(denied) != 0 {
			t.Errorf("the gate was consulted a second time about %+v; the name the script wrote reaches this object", denied)
		}
		return
	}

	if st == 0 {
		if out != "classified\n" {
			t.Errorf("got %q with status 0, want the file read through the link", out)
		}
		if len(denied) != 0 {
			t.Errorf("an allowed open recorded a denial: %+v", denied)
		}
		return
	}
	// The kernel named the object by its other name, so the gate judged the
	// name it was given and refused. Every part of that has to hold.
	if !strings.Contains(out, "refused") {
		t.Errorf("a non-zero status must be reported as a refusal, got %q", out)
	}
	if strings.Contains(out, "classified") {
		t.Errorf("a refused open returned the contents anyway: %q", out)
	}
	if strings.Contains(out, "/secret/") {
		t.Errorf("the refusal told the script where the name went: %q", out)
	}
	if !strings.Contains(out, hard) {
		t.Errorf("the refusal must name the path as written, got %q", out)
	}
	if len(denied) != 1 {
		t.Fatalf("want exactly one recorded denial, got %+v", denied)
	}
	if !strings.HasSuffix(denied[0].Resolved, string(filepath.Separator)+
		filepath.Join("secret", "data")) {
		t.Errorf("the operator's record resolved to %q, want the object's other real name",
			denied[0].Resolved)
	}
	if denied[0].Path != denied[0].Resolved {
		t.Errorf("the second consultation carried Path %q and Resolved %q, want the object in both",
			denied[0].Path, denied[0].Resolved)
	}
}

// TestAnUngatedRunIsUnaffected. Everything above is guarded on a gate being
// there, and a shell without one must open files by exactly the calls it did
// before — including through a link, which is an ordinary thing scripts do.
func TestAnUngatedRunIsUnaffected(t *testing.T) {
	secret, link := linkFarm(t)
	out, st := run(t, "cat <"+link, nil)
	if out != "classified\n" || st != 0 {
		t.Errorf("got %q status %d, want the link read", out, st)
	}
	if _, st := run(t, "echo replaced >"+link, nil); st != 0 {
		t.Fatalf("the write failed, status %d", st)
	}
	got, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replaced\n" {
		t.Errorf("target = %q, want the write to have gone through the link", got)
	}
}
