// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runPart runs a chunk and leaves the shell open, which is what a front end
// reading a line at a time does — and what these rows need, because the
// descriptor a shell keeps on its own directory is let go of when the shell
// ends. A test that ran each line as a whole shell would be putting the
// directory down between the `cd` and the rename, which is not a shape
// anything real has.
func runPart(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

// A shell's directory can be renamed out from under it, and what happens next
// splits into two questions that are easy to run together and are not the same
// one.
//
// The first has one answer everywhere. Measured 2026-09-26 with `d` renamed to
// `e` while the shell sat in it, zsh 5.9.2, bash 5.3, bash 3.2, ksh93, dash and
// BusyBox ash 1.37 all carry on: an external command started afterwards runs in
// the renamed directory, a relative redirection writes into it, and `pwd -P`
// names it. Six of six, so there is nothing for a dialect to answer, and the
// rows below name no shell.
//
// The second is an axis and lives in cdRenameAxis below.
//
// This is the whole of what #4653 was: none of it worked, because a Runner may
// not have a process working directory and so had nothing but a name.

// renamedTree makes `d` with a subdirectory and a file in it, and answers the
// resolved root — resolved because t.TempDir sits under a link on macOS and an
// unresolved base makes every row disagree with itself.
func renamedTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "d", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "there.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// renameAway renames the shell's directory and hands back where it went.
func renameAway(t *testing.T, root string) string {
	t.Helper()
	now := filepath.Join(root, "e")
	if err := os.Rename(filepath.Join(root, "d"), now); err != nil {
		t.Fatal(err)
	}
	// The control every row below leans on: the old name really is gone, so a
	// path built from it really would fail.
	if _, err := os.Stat(filepath.Join(root, "d")); err == nil {
		t.Fatal("the old name still reaches the directory; the case is not set up")
	}
	return now
}

func TestARelativePathStillReachesADirectoryThatHasBeenRenamed(t *testing.T) {
	for _, c := range []struct{ name, src, wrote string }{
		{
			// A redirection: the one that broke with `no such file or
			// directory: rel.txt` for a directory the shell was sitting in.
			"a redirection the shell opens itself",
			"echo hi > rel.txt", "rel.txt",
		},
		{
			// And the same question asked of a file that is already there,
			// so the row is not only about creating.
			"a file operand that is already there",
			"[ -f there.txt ] && echo hi > seen.txt", "seen.txt",
		},
		{
			// A subdirectory below it, which is the walk rather than the
			// directory itself.
			"a path with a component below",
			"echo hi > s/under.txt", "s/under.txt",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := renamedTree(t)
			sem := PosixSemantics()
			errs := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: filepath.Join(root, "d"), Stderr: errs,
			})
			// The shell has to have taken hold of the directory before the
			// rename, which is what a real shell's kernel does for it at the
			// chdir. A `cd` to where it already is, which is what a script
			// does when it starts.
			runPart(t, r, "cd .\n")
			now := renameAway(t, root)
			runPart(t, r, c.src+"\n")
			if errs.Len() != 0 {
				t.Fatalf("stderr = %q", errs.String())
			}
			if _, err := os.Stat(filepath.Join(now, c.wrote)); err != nil {
				t.Errorf("%s did not reach the renamed directory: %v", c.src, err)
			}
		})
	}
}

// The control for the rows above: with nothing renamed, the same sources write
// into the same place, so a pass up there is about the rename and not about
// the shell writing files anywhere at all.
func TestARelativePathReachesTheDirectoryWhenNothingHasMoved(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"),
	})
	runPart(t, r, "cd .\necho hi > rel.txt\n")
	if _, err := os.Stat(filepath.Join(root, "d", "rel.txt")); err != nil {
		t.Errorf("an ordinary redirection did not write where the shell is: %v", err)
	}
}

// An external command is the sharpest row, because its working directory is
// the kernel's own reading of a path rather than anything this package can
// paper over: a child handed a name that leads nowhere fails to start at all,
// which is how `/bin/pwd` came back 126 where all six real shells print the
// renamed directory and exit 0.
func TestAnExternalCommandRunsInADirectoryThatHasBeenRenamed(t *testing.T) {
	if _, err := os.Stat("/bin/pwd"); err != nil {
		t.Skip("no /bin/pwd on this machine")
	}
	root := renamedTree(t)
	sem := PosixSemantics()
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stdout: out, Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	now := renameAway(t, root)
	runPart(t, r, "/bin/pwd\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	got := strings.TrimSpace(out.String())
	if resolved, err := filepath.EvalSymlinks(got); err == nil {
		got = resolved
	}
	if got != now {
		t.Errorf("the child ran in %q, want the renamed directory %q", got, now)
	}
}

// The axis, and the pair that fixes its noun.
//
// **The path `cd` built** is what decides, not the shell's directory and not
// its inode. Both rows below rename `d` to `e` and then put a *different* `d`
// back, so in both of them the shell's own name resolves and in both of them
// the shell is in `e`. The only thing that moves is whether the path `cd`
// builds — `…/d/s` — is there.
func TestWhatDecidesIsThePathCdBuiltAndNotWhereTheShellIs(t *testing.T) {
	for _, c := range []struct {
		name           string
		impostorHoldsS bool
		want           string
	}{
		{
			// The built path is not there, so the operand is tried again from
			// the directory: bash and zsh arrive in `e/s`.
			"the name is back and the path below it is not",
			false, "e/s",
		},
		{
			// The built path *is* there, so it is taken — into the impostor,
			// which is measured and is the row that says the inode is not the
			// noun.
			"the name is back and so is the path below it",
			true, "d/s",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := renamedTree(t)
			sem := PosixSemantics()
			sem.CdDestinationIsNotThere = CdDestinationNotThereEntersAndTakesTheKernelsName
			out := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: filepath.Join(root, "d"), Stdout: out,
			})
			runPart(t, r, "cd .\n")
			renameAway(t, root)
			back := filepath.Join(root, "d")
			if c.impostorHoldsS {
				if err := os.MkdirAll(filepath.Join(back, "s"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(back, 0o755); err != nil {
				t.Fatal(err)
			}
			runPart(t, r, "cd s && pwd\n")
			want := filepath.Join(root, c.want)
			if got := strings.TrimSpace(out.String()); got != want {
				t.Errorf("cd s left %q, want %q", got, want)
			}
		})
	}
}

func TestCdFromARenamedDirectoryAnswersTheAxis(t *testing.T) {
	for _, c := range []struct {
		name   string
		policy CdDestinationNotTherePolicy
		want   string
		fails  bool
	}{
		{
			"refuses",
			CdDestinationNotThereRefuses, "d", true,
		},
		{
			"enters and takes the kernel's name",
			CdDestinationNotThereEntersAndTakesTheKernelsName, "e", false,
		},
		{
			"enters and keeps the built name",
			CdDestinationNotThereEntersAndKeepsTheBuiltName, "d", false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := renamedTree(t)
			sem := PosixSemantics()
			sem.CdDestinationIsNotThere = c.policy
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: filepath.Join(root, "d"), Stdout: out, Stderr: errs,
			})
			runPart(t, r, "cd .\n")
			renameAway(t, root)
			runPart(t, r, "cd .\necho $?\necho $PWD\n")
			lines := strings.Fields(out.String())
			if len(lines) != 2 {
				t.Fatalf("output = %q, want a status and a PWD", out.String())
			}
			if failed := lines[0] != "0"; failed != c.fails {
				t.Errorf("`cd .` status %q, want failure=%v (stderr %q)", lines[0], c.fails, errs.String())
			}
			if want := filepath.Join(root, c.want); lines[1] != want {
				t.Errorf("$PWD = %q, want %q", lines[1], want)
			}
			// And a shell that keeps the built name keeps it *knowingly*: the
			// move happened, so a relative path after it reaches the real
			// directory even though $PWD does not name it. That is ksh93's
			// row, measured, and it is what makes this a third answer rather
			// than a second way of failing.
			if c.policy == CdDestinationNotThereEntersAndKeepsTheBuiltName {
				runPart(t, r, "echo hi > moved.txt\n")
				if _, err := os.Stat(filepath.Join(root, "e", "moved.txt")); err != nil {
					t.Errorf("the move did not happen: %v", err)
				}
			}
		})
	}
}

// An unanswered axis refuses by name, and — the half that matters — it refuses
// only where the shells part. A `cd` that arrives, and a `cd` to a path that
// is not there from a directory that has not moved, are the same in all six
// columns and must put no question at all: otherwise a Runner with no answer
// here would refuse `cd nosuchdir`.
func TestOnlyACdFromARenamedDirectoryPutsTheQuestion(t *testing.T) {
	for _, c := range []struct {
		name, src string
		rename    bool
		refuses   bool
	}{
		{"a cd that arrives", "cd s", false, false},
		{"a cd to a path that is not there", "cd nosuch", false, false},
		{"a cd to an absolute path that is not there", "cd /nosuch/at/all", false, false},
		{"a cd from a renamed directory", "cd .", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := renamedTree(t)
			sem := PosixSemantics()
			sem.CdDestinationIsNotThere = CdDestinationNotThereUnspecified
			errs := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: filepath.Join(root, "d"), Stderr: errs,
			})
			runPart(t, r, "cd .\n")
			errs.Reset()
			if c.rename {
				renameAway(t, root)
			}
			runPart(t, r, c.src+"\n")
			said := strings.Contains(errs.String(), "renamed")
			if said != c.refuses {
				t.Errorf("%s said %q, want an unanswered-axis refusal = %v", c.src, errs.String(), c.refuses)
			}
		})
	}
}

// A directory whose last link has gone is the harsher case, and it is where
// the two answers that move meet rather than part: the move happens, and the
// name stays as it was because there is none to take. Measured — zsh, bash and
// ksh93 all answer `cd .` in a removed directory with 0 and an unchanged $PWD.
func TestCdInADirectoryThatHasBeenRemovedKeepsItsName(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	sem.CdDestinationIsNotThere = CdDestinationNotThereEntersAndTakesTheKernelsName
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d", "s"), Stdout: out, Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	if err := os.Remove(filepath.Join(root, "d", "s")); err != nil {
		t.Fatal(err)
	}
	runPart(t, r, "cd .\necho $?\necho $PWD\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	want := "0\n" + filepath.Join(root, "d", "s") + "\n"
	if out.String() != want {
		t.Errorf("`cd .` in a removed directory gave %q, want %q", out.String(), want)
	}
}

// A subshell is a copy in this process where a real shell's is a fork, and a
// fork inherits the *directory* rather than a name for it. So a copy made
// before a rename still finds the directory afterwards, which is what
// `mv ../d ../e; (cd .)` asks of every shell in the panel.
func TestACopyMadeBeforeTheRenameStillFindsTheDirectory(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	sem.CdDestinationIsNotThere = CdDestinationNotThereEntersAndTakesTheKernelsName
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stdout: out, Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	now := renameAway(t, root)
	runPart(t, r, "(cd . && pwd)\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	if got := strings.TrimSpace(out.String()); got != now {
		t.Errorf("a subshell's `cd .` left %q, want %q", got, now)
	}
}

// The same noun, asked of the two things that are not `cd`: a relative path
// the shell opens itself, and a child's working directory. Here the answer is
// the *directory* rather than the path, and that is not a contradiction — it
// is the split the panel has. With `d` renamed to `e` and a different `d` put
// back, the reference shells' children run in `e` and their relative
// redirections land in `e`, while a `cd .` walks into the new `d`. The name is
// consulted where the script asked to move and the directory is held where the
// operating system is about to be handed a path.
func TestARelativePathFollowsTheDirectoryAndNotTheNameThatCameBack(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	errs := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	now := renameAway(t, root)
	back := filepath.Join(root, "d")
	if err := os.Mkdir(back, 0o755); err != nil {
		t.Fatal(err)
	}
	runPart(t, r, "echo hi > rel.txt\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	if _, err := os.Stat(filepath.Join(now, "rel.txt")); err != nil {
		t.Errorf("the redirection did not reach the directory the shell is in: %v", err)
	}
	if _, err := os.Stat(filepath.Join(back, "rel.txt")); err == nil {
		t.Error("the redirection landed in the directory that took the old name back")
	}
}

// And a `cd` that takes the name back is a move even though the name did not
// change, which is the row a hold kept because `r.Dir` still reads the same
// gets wrong: the shell is in the new `d` afterwards, so what it writes goes
// there.
func TestCdIntoADirectoryThatTookTheNameBackMovesTheShell(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	sem.CdDestinationIsNotThere = CdDestinationNotThereEntersAndTakesTheKernelsName
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stdout: out, Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	now := renameAway(t, root)
	back := filepath.Join(root, "d")
	if err := os.Mkdir(back, 0o755); err != nil {
		t.Fatal(err)
	}
	runPart(t, r, "cd .\necho $PWD\necho hi > rel.txt\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	if got := strings.TrimSpace(out.String()); got != back {
		t.Errorf("$PWD = %q, want the name that came back, %q", got, back)
	}
	if _, err := os.Stat(filepath.Join(back, "rel.txt")); err != nil {
		t.Errorf("the shell did not move into the directory it named: %v", err)
	}
	if _, err := os.Stat(filepath.Join(now, "rel.txt")); err == nil {
		t.Error("the shell is still writing into the directory it left")
	}
}

// A copy does not open a hold of its own, and this is what says so: a shell
// that moves inside parentheses, over and over, must not be putting a
// descriptor down each time.
//
// The leak it guards against is a quiet one. Five of the seven places that
// clone a Runner reach endSubshell and two do not, so there is no one line
// where a copy ends — which is why a copy inherits the descriptor its parent
// has and opens none. A copy that has moved is back to resolving by name,
// which is what this package did before the hold existed.
func TestASubshellThatMovesLeavesNoDescriptorBehind(t *testing.T) {
	const rounds = 40
	before, err := openDescriptors()
	if err != nil {
		t.Skipf("this platform does not list its open descriptors: %v", err)
	}
	root := renamedTree(t)
	sem := PosixSemantics()
	errs := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stderr: errs,
	})
	for range rounds {
		runPart(t, r, "(cd s; cd ..)\n")
	}
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	// Counted straight rather than waited for: a descriptor a collector would
	// eventually take back is still a descriptor this shell is holding, and a
	// wait loop allocates enough to hide exactly that.
	after, err := openDescriptors()
	if err != nil {
		t.Fatal(err)
	}
	if after > before+descriptorSlack {
		t.Errorf("%d descriptors open after %d subshells that moved, %d before — "+
			"a hold nothing let go of", after, rounds, before)
	}
}

// A directory that has been renamed and *then* removed is where the kernel
// remembers a name for something that no longer has one: Darwin answers
// F_GETPATH for a removed directory with the name it had, which here is the
// name it was renamed to and not the name `cd` built. Taking that answer
// unverified would leave `$PWD` holding `…/e` for a directory called nothing
// at all.
//
// Measured 2026-09-26 on zsh 5.9.2 (`-f`): `cd d; mv ../d ../e; rmdir ../e;
// cd .` is status 0 with `$PWD` still `…/d`. This is the row that separates
// the verification from the removed-directory row above it, where the stale
// name and the built name happen to be the same string.
func TestADirectoryRenamedAndThenRemovedKeepsTheNameCdBuilt(t *testing.T) {
	root := renamedTree(t)
	sem := PosixSemantics()
	sem.CdDestinationIsNotThere = CdDestinationNotThereEntersAndTakesTheKernelsName
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: filepath.Join(root, "d"), Stdout: out, Stderr: errs,
	})
	runPart(t, r, "cd .\n")
	now := renameAway(t, root)
	if err := os.RemoveAll(now); err != nil {
		t.Fatal(err)
	}
	runPart(t, r, "cd .\necho $?\necho $PWD\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	want := "0\n" + filepath.Join(root, "d") + "\n"
	if out.String() != want {
		t.Errorf("`cd .` after a rename and a removal gave %q, want %q", out.String(), want)
	}
}
