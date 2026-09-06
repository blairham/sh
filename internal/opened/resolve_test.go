// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The walk has to agree with the kernel, and "has to" is the whole of it: it
// replaces the kernel's own resolution for every open a gate sees, so a path
// the kernel resolves and it does not is a file the shell can no longer open,
// and a path they resolve differently is a rule applied to the wrong name.
//
// So these are differential rather than expected-value tests. The oracle is the
// platform: open the path the ordinary way and ask it what it got — F_GETPATH
// on Darwin, /proc on Linux — which on a filesystem nobody is changing is the
// right answer and is the answer the check used before the walk existed. Then
// walk the same path and require the same name, or the same errno.
//
// Expected values are written down only where there is no oracle: a limit, a
// loop, a permission. Everywhere else the kernel is the specification and the
// fixture is the interesting part.

// oracle is what the platform says an ordinary open of path reached.
func oracle(t *testing.T, path string, flags int) (name string, named bool, err error) {
	t.Helper()
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()
	name, named = Path(f)
	return name, named, nil
}

// errnoOf digs the errno out of whatever wrapping a call put on it.
func errnoOf(err error) syscall.Errno {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return 0
}

// TestTheWalkAgreesWithTheKernel is the differential body, over a fixture built
// to be awkward.
func TestTheWalkAgreesWithTheKernel(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, p), []byte("contents\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	mkdir("a/b/c")
	write("a/b/c/file")
	write("a/file")
	mkdir("other")
	write("other/file")

	link("a/b/c", "toc")                   // absolute-ish relative link to a directory
	link(filepath.Join(dir, "a/b"), "tob") // an absolute target
	link("../../other", "a/b/up")          // a target climbing out with ..
	link("file", "a/b/c/tofile")           // a link beside its target
	link("toc/file", "chain1")             // a link through a link
	link("chain1", "chain2")               // and again
	link("nowhere", "dangling")
	link("loopb", "loopa")
	link("loopa", "loopb")
	link("a/b/c/", "trailing") // a target with a trailing slash
	link("a/b/", "midslash")   // the same, on a link used as an intermediate
	link("a/file/", "fileslash")

	cases := []struct {
		name string
		path string
	}{
		{"a plain file", "a/b/c/file"},
		{"a directory", "a/b/c"},
		{"the temporary root", ""},
		{"a doubled separator", "a//b//c//file"},
		{"a dot component", "a/./b/./c/./file"},
		{"a dot-dot component", "a/b/c/../../b/c/file"},
		{"dot-dot above the fixture", "a/../../../../../../.."},
		{"a trailing slash on a directory", "a/b/c/"},
		{"a trailing slash on a file", "a/b/c/file/"},
		{"a file used as a directory", "a/file/nope"},
		{"a link to a directory", "toc/file"},
		{"a link with an absolute target", "tob/c/file"},
		{"a link climbing out with dot-dot", "a/b/up/file"},
		{"a link beside its target", "a/b/c/tofile"},
		{"a link through a link", "chain1"},
		{"a link through two links", "chain2"},
		{"a link as an intermediate component", "toc/../c/file"},
		{"a link whose target ends in a slash", "trailing"},
		{"a link with a slashed target, used as an intermediate", "midslash/c/file"},
		{"a link whose slashed target is a file", "fileslash"},
		{"a trailing slash through a link to a file", "a/b/c/tofile/"},
		{"a dangling link", "dangling"},
		{"a link loop", "loopa"},
		{"a name that is not there", "a/b/c/absent"},
		{"a name that is not there under a link", "toc/absent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := dir
			if tc.path != "" {
				path = filepath.Join(dir, tc.path)
				if strings.HasSuffix(tc.path, "/") {
					path += "/"
				}
				if strings.Contains(tc.path, "//") || strings.Contains(tc.path, "/./") {
					// filepath.Join would clean these away, and they are the
					// subject.
					path = dir + "/" + tc.path
				}
			}
			wantName, wantNamed, wantErr := oracle(t, path, os.O_RDONLY)
			r, gotErr := walkOpen(path, os.O_RDONLY, 0)
			if r.File != nil {
				defer func() { _ = r.File.Close() }()
			}

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("the kernel says %v and the walk says %v", wantErr, gotErr)
			}
			if wantErr != nil {
				if k, g := errnoOf(wantErr), errnoOf(gotErr); k != g {
					t.Errorf("the kernel refused with %v and the walk with %v", k, g)
				}
				var pathErr *fs.PathError
				if !errors.As(gotErr, &pathErr) {
					t.Fatalf("the walk's error is %T, want *fs.PathError as os.OpenFile gives", gotErr)
				}
				if pathErr.Op != "open" || pathErr.Path != path {
					t.Errorf("the walk's error is %q on %q, want an open error naming the path as written",
						pathErr.Op, pathErr.Path)
				}
				return
			}
			if !wantNamed {
				// The platform has no name for it. There is nothing to compare
				// and Open's fallback is what covers it; see its own test.
				return
			}
			if r.Name != wantName {
				t.Errorf("the walk reached %q, the kernel says %q", r.Name, wantName)
			}
			if got := r.File.Name(); got != path {
				t.Errorf("File.Name() = %q, want the path as written %q — a script is shown this", got, path)
			}
		})
	}
}

// TestTheWalkAgreesWithTheKernelOnPlacesThisMachineActuallyHas takes the
// fixture out of the test's hands, because the things a temporary directory
// cannot contain are exactly the ones worth checking: a mount boundary, a
// firmlink, the operating system's own links.
//
// Mount points are not created here. Creating one needs a privilege a test must
// not have, and the machine already has several — /dev on both platforms, /proc
// and /sys on Linux, the data volume on a Mac. Walking through them and back
// out with `..` is the case where a lexical parent and the kernel's parent could
// disagree, and the walk answers `..` by *popping a descriptor it already
// holds*, so what this asserts is that popping and the kernel agree on a real
// mount tree rather than on a reasoned-about one.
func TestTheWalkAgreesWithTheKernelOnPlacesThisMachineActuallyHas(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	paths := []string{
		"/", "/..", "/../..", "/dev", "/dev/..", "/dev/null", "/dev/./null",
		"/dev/../dev/null", "/tmp", "/tmp/", "/var", "/etc", "/etc/..",
		"/usr/bin", "/usr/bin/..", "/usr/../usr/bin",
		"/proc/self", "/proc/self/..", "/sys", "/sys/..", "/dev/shm",
		"/System/Volumes/Data", "/System/Volumes/Data/..",
	}
	crossed := 0
	for _, path := range paths {
		wantName, wantNamed, wantErr := oracle(t, path, os.O_RDONLY)
		r, gotErr := walkOpen(path, os.O_RDONLY, 0)
		if r.File != nil {
			_ = r.File.Close()
		}
		if wantErr != nil {
			// Not on this machine, or not readable by this user. Both are
			// fine; what is not fine is the walk succeeding where the kernel
			// refused.
			if gotErr == nil {
				t.Errorf("%s: the kernel refused with %v and the walk opened it", path, wantErr)
			}
			continue
		}
		if gotErr != nil {
			t.Errorf("%s: the kernel opened it and the walk refused with %v", path, gotErr)
			continue
		}
		crossed++
		if wantNamed && r.Name != wantName {
			t.Errorf("%s: the walk reached %q, the kernel says %q", path, r.Name, wantName)
		}
	}
	if crossed < 6 {
		t.Fatalf("only %d of these paths exist here; this test is not measuring anything", crossed)
	}
}

// TestTheWalkRefusesWhereTheKernelRefuses covers the two answers a fixture can
// arrange but an oracle cannot be asked for twice: a directory with no search
// permission, and a caller's own O_NOFOLLOW.
func TestTheWalkRefusesWhereTheKernelRefuses(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	if os.Geteuid() == 0 {
		t.Skip("root traverses anything, so there is no refusal to compare")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "shut", "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shut", "inner", "file"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "shut"), filepath.Join(dir, "toshut")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "shut"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "shut"), 0o755) })

	shut := filepath.Join(dir, "shut", "inner", "file")
	if _, _, err := oracle(t, shut, os.O_RDONLY); errnoOf(err) != syscall.EACCES {
		t.Fatalf("the kernel answered %v for an unsearchable directory, want EACCES", err)
	}
	if _, err := walkOpen(shut, os.O_RDONLY, 0); errnoOf(err) != syscall.EACCES {
		t.Errorf("the walk answered %v, want the EACCES the kernel gives", err)
	}

	// A caller's own O_NOFOLLOW means refuse a symbolic link, and the walk must
	// not quietly follow one on its way to being helpful.
	if _, err := walkOpen(filepath.Join(dir, "toshut"), os.O_RDONLY|syscall.O_NOFOLLOW, 0); errnoOf(err) != syscall.ELOOP {
		t.Errorf("the walk answered %v for a link opened O_NOFOLLOW, want ELOOP", err)
	}
}

// TestAWalkThroughADirectoryItMayNotReadStillWalks is the measurement that
// decided which flags the walk opens directories with, made to run.
//
// Passing through a directory needs execute permission and reading one needs
// read permission. A resolver built on the readable open would refuse every
// path through a 0111 directory — an ordinary way to publish one file out of a
// private tree — while the kernel walks straight through. That is a policy
// breaking accesses it was never asked about, which is worse than the gap the
// walk is here to close.
func TestAWalkThroughADirectoryItMayNotReadStillWalks(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads anything, so the distinction does not arise")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "thin", "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "thin", "inner", "file")
	if err := os.WriteFile(target, []byte("published\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "thin"), 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "thin"), 0o755) })

	// The premise: this is a directory the kernel walks through and will not
	// let anyone list. If a platform stops making that distinction the test
	// below stops meaning anything, so it is asserted rather than assumed.
	if _, err := os.ReadDir(filepath.Join(dir, "thin")); err == nil {
		t.Skip("this filesystem lets a 0111 directory be listed")
	}
	r, err := walkOpen(target, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("the walk refused a path the kernel resolves: %v", err)
	}
	defer func() { _ = r.File.Close() }()
	physical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != physical {
		t.Errorf("the walk reached %q, want %q", r.Name, physical)
	}
}

// TestTheWalkRefusesTheSameChainTheKernelDoes puts the symbolic-link budget
// where it cannot drift.
//
// maxSymlinks is a measured constant per platform, and a constant is exactly
// the kind of thing that is right when it is written and wrong two kernels
// later. So this does not compare the walk to the number: it lengthens a chain
// until the *kernel* refuses, and requires the walk to refuse at the same link
// and with the same errno.
//
// A walk that refused earlier would make a policy break paths that work without
// one; a walk that refused later would open chains a shell cannot otherwise
// open. Both are the gate changing what the shell can do rather than what it
// may, so both are failures here.
func TestTheWalkRefusesTheSameChainTheKernelDoes(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "end"), []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := "end"
	kernelStopped, walkStopped := 0, 0
	for i := 1; i <= maxSymlinks+8; i++ {
		name := fmt.Sprintf("s%03d", i)
		if err := os.Symlink(prev, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		prev = name
		path := filepath.Join(dir, name)

		_, _, kernelErr := oracle(t, path, os.O_RDONLY)
		r, walkErr := walkOpen(path, os.O_RDONLY, 0)
		if r.File != nil {
			_ = r.File.Close()
		}
		if kernelErr != nil && kernelStopped == 0 {
			kernelStopped = i
			if errnoOf(kernelErr) != syscall.ELOOP {
				t.Fatalf("the kernel refused a chain of %d with %v, want ELOOP", i, kernelErr)
			}
		}
		if walkErr != nil && walkStopped == 0 {
			walkStopped = i
			if errnoOf(walkErr) != syscall.ELOOP {
				t.Errorf("the walk refused a chain of %d with %v, want ELOOP", i, walkErr)
			}
		}
		if (kernelErr == nil) != (walkErr == nil) {
			t.Fatalf("at a chain of %d the kernel says %v and the walk says %v", i, kernelErr, walkErr)
		}
		if kernelStopped != 0 {
			break
		}
	}
	if kernelStopped == 0 {
		t.Fatalf("this kernel followed %d links without complaining; the budget is not where it was measured",
			maxSymlinks+8)
	}
	if walkStopped != kernelStopped {
		t.Errorf("the kernel stops at a chain of %d and the walk at %d", kernelStopped, walkStopped)
	}
}

// TestAMagicLinkOpensAndHasNoName is the one thing a userspace walk cannot do,
// and the fallback that keeps it working.
//
// /proc/<pid>/fd holds links whose targets are not paths — `pipe:[12345]` — and
// only the kernel can follow them. A walk that insisted on resolving every
// component itself would make `cat < /dev/fd/3` fail under a policy while
// working without one, which is a policy breaking a mechanism rather than
// refusing an access. On Darwin the same path is an ordinary entry in the
// fdesc filesystem and the walk resolves it; the fallback is Linux's, and this
// runs on both so that the answer is the same on both.
//
// What must hold either way is that it has no *name*: an object the filesystem
// does not name that way cannot be the subject of a rule about names, which is
// what Elsewhere has always done with it and is why the fallback is safe rather
// than convenient.
func TestAMagicLinkOpensAndHasNoName(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close(); _ = write.Close() }()
	if _, err := write.WriteString("through the pipe\n"); err != nil {
		t.Fatal(err)
	}

	path := fmt.Sprintf("/dev/fd/%d", read.Fd())
	if _, err := os.Lstat(path); err != nil {
		t.Skipf("no /dev/fd on this machine: %v", err)
	}
	r, err := Open(path, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Open(%s) failed: %v — a policy must not break a path that opens without one", path, err)
	}
	defer func() { _ = r.File.Close() }()

	if r.Name != "" {
		// Darwin names it, and that name is the path as written, so a gate is
		// not consulted a second time either way. What must never happen is a
		// name that is a *different* place.
		if r.Name != path {
			t.Errorf("Open reached %q for %q, want either no name or the name as written", r.Name, path)
		}
	}
	if where, elsewhere := Elsewhere(r, path); elsewhere {
		t.Errorf("Elsewhere reported %q; a descriptor on a pipe is not somewhere else", where)
	}

	buf := make([]byte, 32)
	n, err := r.File.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "through the pipe\n" {
		t.Errorf("read %q, want the pipe's contents", got)
	}
}

// TestTheFallbackDoesNotCoverAWalkThatIsMerelyWrong is the other half of the
// fallback, and the half that decides whether it is safe.
//
// The fallback exists for an object the platform has no name for. If the
// platform *does* have a name and the walk failed anyway, the two disagree
// about a path that does have a name — which is either a bug in the walk or a
// filesystem doing something neither understands, and both are things to refuse
// on rather than open a file on. A fallback that quietly opened would turn every
// future bug in the walk into an unchecked open, which is the failure mode a
// boundary can least afford.
func TestTheFallbackDoesNotCoverAWalkThatIsMerelyWrong(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	// The walk resolves this one, so the fallback is not reached at all and the
	// name is the target's. That is the premise: a fallback that fired here
	// would be handing out unnamed descriptors for ordinary symbolic links.
	r, err := Open(link, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.File.Close()
	physical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != physical {
		t.Fatalf("Open reached %q, want %q — an ordinary link must be walked, not fallen back on",
			r.Name, physical)
	}

	// And a link the walk cannot follow, to a name that is not there: the
	// kernel cannot open it either, so the fallback declines and the walk's own
	// error stands.
	dangling := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "absent"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dangling, os.O_RDONLY, 0); errnoOf(err) != syscall.ENOENT {
		t.Errorf("Open answered %v for a dangling link, want ENOENT", err)
	}
}

// TestTheFallbackFailsClosedOnADescriptorThePlatformCanName is the fallback's
// decision on its own, because it is the one place in this package that could
// fail open.
//
// It cannot be reached through Open without a bug in the walk, which is the
// point — the two answers it has to give are the difference between "the
// kernel resolved something userspace cannot follow" and "the walk is wrong
// about a path that has a perfectly good name", and only the second one is a
// hazard. So it is asked directly.
func TestTheFallbackFailsClosedOnADescriptorThePlatformCanName(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	sentinel := errors.New("the walk did not finish")

	// A pipe: the platform has no name for it, so this is the nameless case
	// and the descriptor is handed back for the caller to use.
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = write.Close() }()
	r, err := nameless(read, sentinel)
	if err != nil {
		t.Fatalf("nameless refused a pipe with %v; an object with no name is the case this exists for", err)
	}
	if r.File != read {
		t.Error("nameless did not hand back the descriptor it was given")
	}
	if r.Name != "" {
		t.Errorf("nameless named a pipe %q", r.Name)
	}
	_ = read.Close()

	// A regular file: the platform names it, so the walk and the platform
	// disagree about a path that has a name, and that is refused.
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := nameless(f, sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("nameless answered %v for a descriptor the platform names, want the walk's own error", err)
	}
	if got.File != nil {
		t.Error("nameless handed back a descriptor while refusing")
	}
	// And the descriptor is closed, not merely dropped: a refusal that leaked
	// an open descriptor would be a refusal in name only.
	if _, err := f.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Errorf("the refused descriptor reads with %v, want it closed", err)
	}
}

// TestTheWalkRefusesAPathnameTheKernelWouldNotTake is the limit the walk has to
// impose on itself, and the reason it has to.
//
// The kernel measures the pathname it is handed against PATH_MAX. The walk
// hands it one component at a time, and a component is never long — so without
// this, a path too long for the kernel would *open* under a policy and fail
// without one, which is a gate changing what a shell can do rather than what it
// may. The boundary is found by lengthening the path until the kernel refuses,
// rather than trusted from the constant.
//
// It is measured against the caller's own string and not the absolute form,
// because that is the string the kernel measures: a short relative name under
// a very deep working directory is accepted by both, and the second half of
// this asserts the walk accepts it too.
func TestTheWalkRefusesAPathnameTheKernelWouldNotTake(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	kernelStopped, walkStopped := 0, 0
	for pad := range pathMax {
		// `/.` pads the length without changing what is named.
		path := dir + strings.Repeat("/.", pad) + "/f"
		_, _, kernelErr := oracle(t, path, os.O_RDONLY)
		r, walkErr := walkOpen(path, os.O_RDONLY, 0)
		if r.File != nil {
			_ = r.File.Close()
		}
		if kernelErr != nil && kernelStopped == 0 {
			kernelStopped = len(path)
			if errnoOf(kernelErr) != syscall.ENAMETOOLONG {
				t.Fatalf("the kernel refused %d bytes with %v, want ENAMETOOLONG", len(path), kernelErr)
			}
		}
		if walkErr != nil && walkStopped == 0 {
			walkStopped = len(path)
			if errnoOf(walkErr) != syscall.ENAMETOOLONG {
				t.Errorf("the walk refused %d bytes with %v, want ENAMETOOLONG", len(path), walkErr)
			}
		}
		if kernelStopped != 0 {
			break
		}
	}
	if kernelStopped == 0 {
		t.Fatal("this kernel took a pathname of every length up to PATH_MAX; the limit moved")
	}
	if walkStopped != kernelStopped {
		t.Errorf("the kernel stops at %d bytes and the walk at %d", kernelStopped, walkStopped)
	}
}

// TestATrailingSlashWithOCreatRefusesWithoutCreating is the one corner where
// the two kernels do not agree with each other, written down rather than
// hidden in an exclusion.
//
// `> dir/new/` is a refusal on both — ENOENT on Darwin 25.5.0 and EISDIR on
// Linux 6.8 — so requiring one errno would require the walk to be wrong on one
// platform. What both agree on, and what actually matters, is that it refuses
// and that nothing is created; that is what this asserts, on both.
//
// This is not a test tolerant of two answers about a *boundary*: both answers
// are refusals, and the thing that could go wrong — a file appearing where the
// kernel makes none — is asserted exactly.
func TestATrailingSlashWithOCreatRefusesWithoutCreating(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "new") + "/"

	_, _, kernelErr := oracle(t, path, os.O_WRONLY|os.O_CREATE)
	if kernelErr == nil {
		t.Fatal("this kernel created a file for a name with a trailing slash")
	}
	if _, err := os.Lstat(filepath.Join(dir, "new")); !os.IsNotExist(err) {
		t.Fatalf("the kernel created something: %v", err)
	}

	r, walkErr := walkOpen(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if r.File != nil {
		_ = r.File.Close()
	}
	if walkErr == nil {
		t.Fatal("the walk opened a name with a trailing slash and O_CREAT")
	}
	if _, err := os.Lstat(filepath.Join(dir, "new")); !os.IsNotExist(err) {
		t.Errorf("the walk created something the kernel would not: %v", err)
	}
}

// TestTheWalkAgreesWithTheKernelOnARelativePath is the case the rest of this
// file cannot cover, because every path in it is absolute.
//
// A relative path is resolved by the walk against the process's directory, and
// that is the one place this package asks os.Getwd — interp's rule against it
// is about a *shell* having an opinion on where it is, and this is not an
// opinion but a reproduction of the resolution the open is about to perform.
// If it reproduced it wrongly, every relative redirect under a policy would be
// judged under the wrong name, which is the worst failure this file can have
// and the only one no absolute path would show.
func TestTheWalkAgreesWithTheKernelOnARelativePath(t *testing.T) {
	if !walkSupported {
		t.Skip("no walk on this platform")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file"), []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/file", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	for _, rel := range []string{"sub/file", "./sub/file", "sub/../sub/file", "link", "sub", "."} {
		wantName, wantNamed, wantErr := oracle(t, rel, os.O_RDONLY)
		r, gotErr := walkOpen(rel, os.O_RDONLY, 0)
		if r.File != nil {
			_ = r.File.Close()
		}
		if (wantErr == nil) != (gotErr == nil) {
			t.Errorf("%q: the kernel says %v and the walk says %v", rel, wantErr, gotErr)
			continue
		}
		if wantErr != nil {
			continue
		}
		if wantNamed && r.Name != wantName {
			t.Errorf("%q: the walk reached %q, the kernel says %q", rel, r.Name, wantName)
		}
		// And the name is absolute even though the caller's was not, which is
		// what makes a rule about places able to match it at all.
		if !filepath.IsAbs(r.Name) {
			t.Errorf("%q: the walk reached %q, want an absolute path", rel, r.Name)
		}
	}
}
