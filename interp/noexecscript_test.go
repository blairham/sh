// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/testenv"

	"github.com/blairham/sh/syntax"
)

// A file the kernel will not start is a shell script — see
// interp/noexecscript.go for the measurements these assert against. The rule
// this package follows holds here too: axes are named and shells are not.

// imageRun runs src in a directory of its own, with PATH pointing at that
// directory and nowhere else, so nothing on the developer's machine can
// answer a lookup one of these makes — `chmod` included, which is why every
// fixture here is written by writeImage rather than by the snippet.
func imageRun(t *testing.T, dir, src string, sem Semantics, dg Diagnostics) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// imageSemantics answers the two axes this file is about, plus the ones a
// snippet here happens to walk past, at the answer that does the most.
func imageSemantics() Semantics {
	s := permissive()
	s.BinaryContentIsNotRunAsAScript = Yes
	s.ScriptImageSeesTheResolvedPath = Yes
	return s
}

// write makes an executable fixture out of bytes, which is the only way to
// write the binary ones: a snippet cannot put a NUL in a file through `printf`
// in every dialect.
func writeImage(t *testing.T, dir, name string, content []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := testenv.WriteExecutable(path, content, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestAFileTheKernelWillNotStartIsRunAsAShellScript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "ne.scr", []byte("echo ran-as-script n=$# 1=$1 2=$2\n"), 0o755)
	out, st := imageRun(t, dir, `./ne.scr a b; echo "st=$?"`, imageSemantics(), Diagnostics{})
	want := "ran-as-script n=2 1=a 2=b\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("a shebang-less script:\n got %q at %d\nwant %q at 0", out, st, want)
	}
}

func TestAScriptRunThatWayCarriesItsOwnExitStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "e.scr", []byte("exit 7\n"), 0o755)
	out, _ := imageRun(t, dir, `./e.scr; echo "st=$?"`, imageSemantics(), Diagnostics{})
	if out != "st=7\n" {
		t.Errorf("the script's own status: got %q, want %q", out, "st=7\n")
	}
}

// The whole reason the shell has to run it rather than source it.
func TestAScriptRunThatWayIsAFreshShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "f.scr", []byte("echo \"[$U][$E]\"; g 2>/dev/null || echo no-function\n"), 0o755)
	out, st := imageRun(t, dir,
		`U=u; export E=e; g() { echo FROM-PARENT; }; ./f.scr; echo "st=$?"`,
		imageSemantics(), Diagnostics{})
	want := "[][e]\nno-function\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("the exported environment and nothing else:\n got %q at %d\nwant %q at 0", out, st, want)
	}
}

// The control, and the reason the fallback is keyed on one errno and then
// looks inside the file. A binary read as a script is a spray of
// `command not found` where a person needs one clear failure.
func TestAFileWithBinaryContentIsNotRunAsAScript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A Mach-O header: magic, then NULs, well inside the first line.
	writeImage(t, dir, "bin.img", append([]byte{0xcf, 0xfa, 0xed, 0xfe}, make([]byte, 60)...), 0o755)
	var buf bytes.Buffer
	sem := imageSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`./bin.img; echo "st=$?"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	out := buf.String()
	if !strings.Contains(out, "st=126") {
		t.Errorf("a binary must still fail at 126: got %q", out)
	}
	if strings.Contains(out, "command not found") || strings.Contains(out, "not found") {
		t.Errorf("a binary must not be read as script text: got %q", out)
	}
	// And the reason is the operating system's own, not Go's wrapper around
	// it — the string this bug was leaking.
	if strings.Contains(out, "fork/exec") {
		t.Errorf("a Go error must not reach a shell diagnostic: got %q", out)
	}
}

// Where the panel splits, and the only thing it splits on: one shell does not
// look inside the file at all.
func TestWhetherBinaryContentStopsTheFallbackIsAnAxis(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		axis  Answer
		wants string
	}{
		{"checked", Yes, "st=126"},
		{"unchecked", No, "ran-anyway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeImage(t, dir, "b.img", []byte("echo ran-anyway\x00\n"), 0o755)
			var buf bytes.Buffer
			sem := imageSemantics()
			sem.BinaryContentIsNotRunAsAScript = tc.axis
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf,
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: dir, Name: "testsh",
			})
			r.Vars = map[string]string{"PATH": dir}
			f, err := syntax.Parse(`./b.img; echo "st=$?"`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatalf("run: %v", rerr)
			}
			if !strings.Contains(buf.String(), tc.wants) {
				t.Errorf("axis %v: got %q, want it to contain %q", tc.axis, buf.String(), tc.wants)
			}
		})
	}
}

// The bound is the first line rather than the whole file, which is what lets
// a script with a NUL in its data run at all.
func TestOnlyTheFirstLineDecidesWhetherAFileIsBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "later.img", []byte("echo first-line-ran\necho t\x00wo\n"), 0o755)
	var buf bytes.Buffer
	sem := imageSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`./later.img`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if !strings.Contains(buf.String(), "first-line-ran") {
		t.Errorf("a NUL past the first line is not a binary: got %q", buf.String())
	}
}

// The other control: a shebang naming an interpreter that is not there is a
// different failure, and must stay one. It is ENOENT rather than ENOEXEC, so
// nothing here may catch it.
func TestAMissingInterpreterIsNotRunAsAShellScript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "bad.scr", []byte("#!/nonexistent/interp\necho SHOULD-NOT-RUN\n"), 0o755)
	out, _ := imageRun(t, dir, `./bad.scr; echo "st=$?"`, imageSemantics(), Diagnostics{})
	if strings.Contains(out, "SHOULD-NOT-RUN") {
		t.Errorf("a missing interpreter must not fall back to this shell: got %q", out)
	}
	if !strings.Contains(out, "st=126") && !strings.Contains(out, "st=127") {
		t.Errorf("a missing interpreter must still fail: got %q", out)
	}
}

// `$0` is where the panel parts, and it parts only on a name that came from
// PATH: a word with a slash in it is the same string under either reading.
func TestTheZeroAFileRunAsAScriptSeesIsAnAxis(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		axis Answer
		want string
	}{
		{"the-resolved-path", Yes, "resolved"},
		{"the-word-as-typed", No, "word"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sem := imageSemantics()
			sem.ScriptImageSeesTheResolvedPath = tc.axis
			dir := t.TempDir()
			writeImage(t, dir, "z.scr",
				[]byte(`case $0 in */z.scr) echo resolved;; z.scr) echo word;; *) echo "other=$0";; esac`+"\n"), 0o755)
			out, st := imageRun(t, dir, `z.scr`, sem, Diagnostics{})
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("axis %v: got %q at %d, want %q at 0", tc.axis, out, st, tc.want+"\n")
			}
		})
	}
}

func TestAWordWithASlashIsItsOwnZeroWhicheverWayTheAxisAnswers(t *testing.T) {
	t.Parallel()
	for _, axis := range []Answer{Yes, No} {
		sem := imageSemantics()
		sem.ScriptImageSeesTheResolvedPath = axis
		dir := t.TempDir()
		writeImage(t, dir, "s.scr", []byte("echo \"zero=$0\"\n"), 0o755)
		out, _ := imageRun(t, dir, `./s.scr`, sem, Diagnostics{})
		if out != "zero=./s.scr\n" {
			t.Errorf("axis %v: got %q, want %q", axis, out, "zero=./s.scr\n")
		}
	}
}

// The second door. `exec` reaches the file another way and must not answer
// differently — and it does not come back, which is what every column does.
func TestExecOnAFileTheKernelWillNotStartRunsItAndDoesNotReturn(t *testing.T) {
	t.Parallel()
	sem := imageSemantics()
	sem.ExecTakesOptions = No
	dir := t.TempDir()
	writeImage(t, dir, "x.scr", []byte(`echo ran-under-exec "$@"`+"\n"), 0o755)
	out, st := imageRun(t, dir, `exec ./x.scr a b; echo NOT-REACHED`, sem, Diagnostics{})
	want := "ran-under-exec a b\n"
	if out != want || st != 0 {
		t.Errorf("exec on a shebang-less script:\n got %q at %d\nwant %q at 0", out, st, want)
	}
}

func TestExecOnAFileWithBinaryContentStillFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "b.img", []byte("echo SHOULD-NOT-RUN\x00\n"), 0o755)
	var buf bytes.Buffer
	sem := imageSemantics()
	sem.ExecTakesOptions = No
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`exec ./b.img; echo NOT-REACHED`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	out := buf.String()
	if strings.Contains(out, "SHOULD-NOT-RUN") || strings.Contains(out, "NOT-REACHED") {
		t.Errorf("exec on a binary must fail and stop the shell: got %q", out)
	}
	if st != 126 {
		t.Errorf("exec on a binary: got status %d, want 126", st)
	}
}

// A descriptor the script parked reaches the file this shell runs as a
// script, in every column — including the one that keeps `exec`'s own
// descriptors from an external command.
func TestADescriptorParkedByExecReachesAFileRunAsAScript(t *testing.T) {
	t.Parallel()
	for _, axis := range []Answer{Yes, No} {
		sem := imageSemantics()
		sem.ExecOpenedFdReachesACommand = axis
		dir := t.TempDir()
		var buf bytes.Buffer
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &Diagnostics{},
			Dir: dir, Name: "testsh",
		})
		r.Vars = map[string]string{"PATH": dir}
		writeImage(t, dir, "t.scr", []byte("echo via3 >&3\n"), 0o755)
		f, err := syntax.Parse(`exec 3>out3; ./t.scr; echo "st=$?"`, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatalf("run: %v", rerr)
		}
		if buf.String() != "st=0\n" {
			t.Errorf("ExecOpenedFdReachesACommand=%v: got %q, want %q", axis, buf.String(), "st=0\n")
		}
		b, err := os.ReadFile(filepath.Join(dir, "out3"))
		if err != nil || string(b) != "via3\n" {
			t.Errorf("ExecOpenedFdReachesACommand=%v: descriptor 3 carried %q (%v), want %q", axis, b, err, "via3\n")
		}
	}
}

// The umask is the one process-wide hook the script is given, and what it
// sets is the script's own — which is what the fork a real shell does would
// have arranged for free. Without the hook a `umask 077` in such a script
// would do nothing and the file it then writes would be world-readable; with
// the mask left in the process, the caller would keep one it never set.
func TestAUmaskTheScriptSetsDoesNotOutliveIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "u.scr", []byte("umask 077\n"), 0o755)
	mask := 0o022
	var buf bytes.Buffer
	sem := imageSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
		SetUmask: func(m int) (int, error) {
			old := mask
			mask = m
			return old, nil
		},
	})
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`./u.scr; echo "st=$?"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if buf.String() != "st=0\n" {
		t.Errorf("the script: got %q, want %q", buf.String(), "st=0\n")
	}
	if held := umaskHeld(t, r); held != 0o022 {
		t.Errorf("the caller's umask after the script: got %#o, want %#o", held, 0o022)
	}
}

// And it really does reach the process while the script is running, which is
// the half the restore must not undo too early.
func TestAUmaskTheScriptSetsReachesTheProcessWhileItRuns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "u.scr", []byte("umask 077\numask\n"), 0o755)
	mask := 0o022
	var buf bytes.Buffer
	sem := imageSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
		SetUmask: func(m int) (int, error) {
			old := mask
			mask = m
			return old, nil
		},
	})
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`./u.scr`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if !strings.Contains(buf.String(), "077") {
		t.Errorf("the script's own reading of the mask it set: got %q", buf.String())
	}
}

// The fresh shell is a shell of the front end's kind, not a bare Runner.
//
// Built from the exported fields alone it was fresh in a sense no real shell
// is: an execve of the same binary runs the dialect's registrations and its
// prelude on the way up, and this ran neither — so a shebang-less script found
// `$RANDOM` empty and none of the builtins its dialect adds. Runner.SetUp is
// the seam that closes it, and it is carried on so a script that runs another
// such file gets the same shell again.
func TestTheFreshShellIsSetUpTheWayTheFrontEndSetsOneUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "outer.scr", []byte("only-in-a-composed-shell outer\n./inner.scr\n"), 0o755)
	writeImage(t, dir, "inner.scr", []byte("only-in-a-composed-shell inner\n"), 0o755)
	var buf bytes.Buffer
	sem := imageSemantics()
	setUp := func(r *Runner) {
		r.Register("only-in-a-composed-shell", func(r *Runner, _ context.Context, args []string) int {
			_, _ = fmt.Fprintf(r.Out(), "registered:%s\n", strings.Join(args, ","))
			return 0
		})
	}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh", SetUp: setUp,
	})
	setUp(r)
	r.Vars = map[string]string{"PATH": dir}
	f, err := syntax.Parse(`./outer.scr; echo "st=$?"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	want := "registered:outer\nregistered:inner\nst=0\n"
	if buf.String() != want {
		t.Errorf("the dialect's own builtins, one level down and two:\n got %q\nwant %q", buf.String(), want)
	}
}

// A `#!` line naming *nothing* is the other file the kernel refuses with
// ENOEXEC, and whether the fallback swallows it is an axis of its own — see
// Semantics.EmptyInterpreterLineIsNotAScript. Both answers are asserted,
// because a test holding only the refusing one would pass against a shell
// that refused every shebang-less file.
func TestWhetherAnEmptyInterpreterLineStopsTheFallbackIsAnAxis(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		axis  Answer
		line  string
		wants string
	}{
		{"refused", Yes, "#!\n", "st=126"},
		{"refused-blanks", Yes, "#!  \t \n", "st=126"},
		{"read-as-a-script", No, "#!\n", "ran-anyway"},
		{"read-as-a-script-blanks", No, "#!  \t \n", "ran-anyway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeImage(t, dir, "e.scr", []byte(tc.line+"echo ran-anyway\n"), 0o755)
			sem := imageSemantics()
			sem.EmptyInterpreterLineIsNotAScript = tc.axis
			out, _ := imageRun(t, dir, `./e.scr; echo "st=$?"`, sem, Diagnostics{})
			if !strings.Contains(out, tc.wants) {
				t.Errorf("axis %v on %q: got %q, want it to contain %q", tc.axis, tc.line, out, tc.wants)
			}
			if strings.Contains(out, "fork/exec") {
				t.Errorf("a Go error must not reach a shell diagnostic: got %q", out)
			}
		})
	}
}

// searchingSemantics is imageSemantics with the PATH search for a slashless
// `#!` word turned on — the answer one shell in the panel gives and the
// preset does not, so a row about the search has to say so.
func searchingSemantics() Semantics {
	s := imageSemantics()
	s.SlashlessInterpreterIsPathSearched = Yes
	return s
}

// interpreterRun is imageRun with the interpreter kept in a second directory
// — on PATH, and never the one the command runs in.
//
// That separation is the whole of the fixture and it is not tidiness. A
// relative `#!` word is resolved by the *kernel* against the child's working
// directory, so an interpreter sitting beside the script is found before this
// shell is ever asked, and a row arranged that way would report a PATH search
// that never happened.
func interpreterRun(t *testing.T, dir, bin, src string, sem Semantics, dg Diagnostics) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir + string(os.PathListSeparator) + bin}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// And a `#!` naming a word with no slash in it, which the kernel hands to
// execve as written and never searches for. Whether the shell searches is
// Semantics.SlashlessInterpreterIsPathSearched.
//
// The interpreter writes its own argv, which is the discriminating half: a
// shell that merely stopped complaining would print nothing at all here, and
// a shell that ran the file with *itself* would print `body` instead.
func TestWhetherASlashlessInterpreterIsSearchedForIsAnAxis(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		axis  Answer
		wants string
	}{
		{"searched", Yes, "INTERP ran"},
		{"not-searched", No, "st=127"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, bin := t.TempDir(), t.TempDir()
			writeImage(t, bin, "myint", []byte("#!/bin/sh\necho INTERP ran\n"), 0o755)
			writeImage(t, dir, "uses.scr", []byte("#!myint\necho body\n"), 0o755)
			sem := imageSemantics()
			sem.SlashlessInterpreterIsPathSearched = tc.axis
			out, _ := interpreterRun(t, dir, bin, `uses.scr a1; echo "st=$?"`, sem,
				Diagnostics{PathNotFound: "%[1]s: not found"})
			if !strings.Contains(out, tc.wants) {
				t.Errorf("axis %v: got %q, want it to contain %q", tc.axis, out, tc.wants)
			}
			if strings.Contains(out, "body") {
				t.Errorf("the file must not be read by this shell: got %q", out)
			}
			if strings.Contains(out, "fork/exec") {
				t.Errorf("a Go error must not reach a shell diagnostic: got %q", out)
			}
		})
	}
}

// The interpreter is handed what the kernel would have handed it: its own
// resolved path as argv[0], the one argument the `#!` line carried, the file,
// and then the command's own operands.
func TestASearchedInterpreterIsGivenTheKernelsArgv(t *testing.T) {
	t.Parallel()
	dir, bin := t.TempDir(), t.TempDir()
	writeImage(t, bin, "myint", []byte("#!/bin/sh\nprintf '[%s]' \"$0\" \"$@\"; echo\n"), 0o755)
	writeImage(t, dir, "uses.scr", []byte("#!myint extra\necho body\n"), 0o755)
	out, _ := interpreterRun(t, dir, bin, `uses.scr a1 a2`, searchingSemantics(), Diagnostics{})
	want := "[" + filepath.Join(bin, "myint") + "][extra][" +
		filepath.Join(dir, "uses.scr") + "][a1][a2]\n"
	if out != want {
		t.Errorf("the argv the kernel would have built:\n got %q\nwant %q", out, want)
	}
}

// One level, and no more. An interpreter that is itself a file with a `#!`
// this shell cannot resolve is not searched for a second time: the failure is
// reported against the *first* file, with the word that file's line held.
func TestASearchedInterpreterIsNotSearchedForAgain(t *testing.T) {
	t.Parallel()
	dir, bin := t.TempDir(), t.TempDir()
	writeImage(t, bin, "selfint", []byte("#!selfint\necho never\n"), 0o755)
	writeImage(t, dir, "loop.scr", []byte("#!selfint\necho body\n"), 0o755)
	dg := Diagnostics{BadInterpreter: "%[1]s: bad interpreter: %[2]s: %[3]s"}
	out, _ := interpreterRun(t, dir, bin, `loop.scr; echo "st=$?"`, searchingSemantics(), dg)
	want := filepath.Join(dir, "loop.scr") + ": bad interpreter: selfint: "
	if !strings.Contains(out, want) {
		t.Errorf("the first file and its own word:\n got %q\nwant it to contain %q", out, want)
	}
	if strings.Contains(out, "never") || strings.Contains(out, "body") {
		t.Errorf("nothing may have run: got %q", out)
	}
}

// A dialect with no `bad interpreter` sentence still numbers this 127 rather
// than the 126 an unstartable file otherwise reports — the answer a script
// testing `$? -eq 127` for "not found" is reading. It was 126, with Go's
// `fork/exec` wrapper in front of it, in every dialect (#4454).
func TestAMissingInterpreterIsNotFoundRatherThanUnstartable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeImage(t, dir, "bad.scr", []byte("#!/nonexistent/interp\necho SHOULD-NOT-RUN\n"), 0o755)
	out, _ := imageRun(t, dir, `./bad.scr; echo "st=$?"`, imageSemantics(),
		Diagnostics{PathNotFound: "%[1]s: not found"})
	if !strings.Contains(out, "./bad.scr: not found") {
		t.Errorf("the not-found wording: got %q", out)
	}
	if !strings.Contains(out, "st=127") {
		t.Errorf("status: got %q, want st=127", out)
	}
	if strings.Contains(out, "fork/exec") {
		t.Errorf("a Go error must not reach a shell diagnostic: got %q", out)
	}
}

// And the dialect that does have one numbers it as that dialect says —
// Diagnostics.BadInterpreterStatus, which is 126 in one of the two shells
// that word this and 127 in the other.
func TestTheBadInterpreterStatusIsTheDialectsOwn(t *testing.T) {
	t.Parallel()
	for _, want := range []int{126, 127} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeImage(t, dir, "bad.scr", []byte("#!/nonexistent/interp\n"), 0o755)
			out, _ := imageRun(t, dir, `./bad.scr; echo "st=$?"`, imageSemantics(), Diagnostics{
				BadInterpreter:       "%[1]s: bad interpreter: %[2]s: %[3]s",
				BadInterpreterStatus: want,
			})
			if !strings.Contains(out, fmt.Sprintf("st=%d", want)) {
				t.Errorf("got %q, want st=%d", out, want)
			}
		})
	}
}
