// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A refused creation must not have happened.
//
// These are the O_CREAT half of what the O_TRUNC tests already assert, and
// they exist because the two were not the same for a while: truncation was
// held back until the check had passed and creation was not, so `> link`
// pointing out of a policy's reach reported a refusal and left an empty file
// where the refusal said nothing would be. The sandbox grader found it from
// outside as write/through-symlink; these pin it where the decision is.
//
// The fixture is the same shape every time and it is the only shape that
// tests anything: a directory the check permits, a symbolic link inside it
// whose target is outside, and a path *through* the link. A name the check
// would refuse on sight is refused before any of this runs, which is why the
// direct spelling never showed the bug.

// refusing is a check that permits only paths under ok, and counts.
func refusing(ok string, asked *[]string) (func(Reached) error, error) {
	refused := errors.New("refused")
	return func(r Reached) error {
		*asked = append(*asked, r.Name)
		if rest, under := beneath(r.Name, ok); !under || rest == "" {
			return refused
		}
		return nil
	}, refused
}

// linked builds the fixture and returns the workspace and the path through
// the link that lands outside it.
func linked(t *testing.T) (root, ws, through string) {
	t.Helper()
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	// Not t.TempDir(): a fixture under TMPDIR is how a containment test comes
	// to assert nothing, and while nothing in *this* package exempts it, the
	// test that would notice must not be the one that breaks quietly.
	root, err := os.MkdirTemp(".", "createcheck")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if root, err = filepath.Abs(root); err != nil {
		t.Fatal(err)
	}
	ws = filepath.Join(root, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	return root, ws, filepath.Join(ws, "out", "made")
}

func TestARefusedCreationDoesNotCreate(t *testing.T) {
	root, ws, through := linked(t)
	var asked []string
	check, refused := refusing(ws, &asked)

	f, err := Verified(through, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666, check)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, refused) {
		t.Fatalf("opening through the link gave %v, want the check's own refusal", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "made")); err == nil {
		t.Fatal("the refusal left the file it refused to make")
	}
	if len(asked) != 1 || asked[0] != filepath.Join(root, "made") {
		t.Errorf("the check was asked %v, want one question naming the path the open would have reached", asked)
	}
}

func TestAPermittedCreationThroughALinkStillHappens(t *testing.T) {
	_, ws, _ := linked(t)
	var asked []string
	// The whole tree is permitted, so the link is not an escape and the open
	// has to work: a boundary that cannot be opened where it was told to is
	// as broken as one that cannot be closed.
	check, _ := refusing(filepath.Dir(ws), &asked)

	f, err := Verified(filepath.Join(ws, "out", "made"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666, check)
	if err != nil {
		t.Fatalf("a permitted creation was refused: %v", err)
	}
	if _, err := f.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if got := len(asked); got != 1 {
		t.Errorf("the check was asked %d times, want 1 — a second consultation is a second audit record for one open", got)
	}
}

func TestARefusedOpenOfAnExistingFileDoesNotEmptyIt(t *testing.T) {
	root, ws, _ := linked(t)
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var asked []string
	check, refused := refusing(ws, &asked)

	f, err := Verified(filepath.Join(ws, "out", "victim"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666, check)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, refused) {
		t.Fatalf("got %v, want the check's refusal", err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "contents\n" {
		t.Errorf("the refused open left %q, want the file untouched", got)
	}
}

// An existing file is opened, not created, so the descriptor check is the one
// that decides it — and it is still only asked once.
func TestAnExistingFileIsCheckedOnTheDescriptor(t *testing.T) {
	root, ws, _ := linked(t)
	if err := os.WriteFile(filepath.Join(root, "there"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var asked []string
	check, refused := refusing(ws, &asked)

	f, err := Verified(filepath.Join(ws, "out", "there"), os.O_WRONLY|os.O_CREATE, 0o666, check)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, refused) {
		t.Fatalf("got %v, want the check's refusal", err)
	}
	if len(asked) != 1 || asked[0] != filepath.Join(root, "there") {
		t.Errorf("the check was asked %v, want one question naming the resolved path", asked)
	}
}

// O_EXCL keeps meaning "fail if it is there", which the probe that holds back
// O_CREAT could have quietly lost: it opens the existing file to find out it
// exists, and a probe that returned that descriptor would turn the caller's
// EEXIST into a successful open.
func TestExclusiveCreationStillFailsOnAFileThatExists(t *testing.T) {
	_, ws, _ := linked(t)
	there := filepath.Join(ws, "there")
	if err := os.WriteFile(there, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var asked []string
	check, _ := refusing(ws, &asked)

	f, err := Verified(there, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666, check)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("got %v, want EEXIST — O_EXCL on a file that is there", err)
	}
	if len(asked) != 0 {
		t.Errorf("the check was asked %v, want nothing: no access was made", asked)
	}
}
