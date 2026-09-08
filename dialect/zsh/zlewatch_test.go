// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// `zle -F`: arming a callback on a descriptor, and what actually calls it.
//
// The wordings and the arities are the half a script can see and are tested
// with `-c`; the calling is tested by handing the dialect a real pipe and
// asking it the question repl asks. See zlewatch.go for the measurements each
// of these pins.

// TestArmingAWatcherIsSilentAndSaidBack is the honest minimum, and it is the
// row the whole feature rests on: an rc file that arms a callback runs to the
// end at status 0 and not a word, and `zle -F` says it back as the command
// that would arm it again.
//
// Before this, that line was `-F is not implemented yet` — which is the right
// answer for something unimplemented and still ended the session, because the
// caller is the scheduler that runs at prompt time.
func TestArmingAWatcherIsSilentAndSaidBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "h(){ :; }\nzle -F 3 h; echo st=$?\nzle -F\n")
	if want := "st=0\nzle -F 3 h\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
	// And an empty table prints nothing at all, at 0.
	out, st = runZsh(t, t.TempDir(), "zle -F; echo st=$?\n")
	if want := "st=0\n"; out != want || st != 0 {
		t.Errorf("empty = %q status %d, want %q at 0", out, st, want)
	}
}

// TestAWatcherNeedsNeitherAFunctionNorALiveDescriptor is what makes the order
// in a real startup file work, and it is measured rather than lenient: every
// one of these is status 0 and silence in the real shell, and the entry is
// stored.
//
// A shell that refused any of them would fail a plugin that arms its callback
// before defining the function, or on a descriptor it opens later.
func TestAWatcherNeedsNeitherAFunctionNorALiveDescriptor(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"handler is nowhere", "zle -F 3 nosuchfn", "zle -F 3 nosuchfn\n"},
		{"descriptor is not open", "h(){ :; }; zle -F 55 h", "zle -F 55 h\n"},
		{"descriptor is absurd", "h(){ :; }; zle -F 99999 h", "zle -F 99999 h\n"},
		{"handler is empty", "zle -F 3 ''", "zle -F 3 \n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+"; echo st=$?\nzle -F\n")
			if want := "st=0\n" + c.want; out != want || st != 0 {
				t.Errorf("output = %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// TestTheListingIsInArmingOrder is the one thing about this listing that could
// not be guessed: it is in the order the watchers were armed and not sorted,
// which is the opposite of what `zle -l` does with widgets. Re-arming a
// descriptor replaces it where it stands.
func TestTheListingIsInArmingOrder(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"h(){ :; }; g(){ :; }\nzle -F 5 h; zle -F 2 g; zle -F 9 h\nzle -F\n")
	if want := "zle -F 5 h\nzle -F 2 g\nzle -F 9 h\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(),
		"h(){ :; }; g(){ :; }\nzle -F 5 h; zle -F 2 h; zle -F 5 g\nzle -F\n")
	if want := "zle -F 5 g\nzle -F 2 h\n"; out != want || st != 0 {
		t.Errorf("replaced = %q status %d, want %q at 0", out, st, want)
	}
}

// TestOmittingTheHandlerRemoves is the middle arity, and the complaint when
// there was nothing there is the only one in this operation that is about
// state rather than about the words used.
func TestOmittingTheHandlerRemoves(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"h(){ :; }\nzle -F 3 h\nzle -F 3; echo gone=$?\nzle -F\nzle -F 3; echo again=$?\n")
	want := "gone=0\nzsh:zle:5: No handler installed for fd 3\nagain=1\n"
	if out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// TestTheDescriptorIsANumberAndTheWholeWordMustBeOne pins the parse, and the
// third row is the one worth having a test for: `zle -F -3` reads like a
// removal and is not one, so a plugin's `zle -F -$fd` would appear to work
// against a shell that took it.
func TestTheDescriptorIsANumberAndTheWholeWordMustBeOne(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"leading zeros", "h(){ :; }; zle -F 007 h; zle -F", "zle -F 7 h\n", 0},
		{"minus nought is nought", "h(){ :; }; zle -F -0 h; zle -F", "zle -F 0 h\n", 0},
		{
			"a negative one is not a removal", "zle -F -3",
			"zsh:zle:1: Bad file descriptor number for -F: -3\n", 1,
		},
		{
			"trailing rubbish", "zle -F 3x h",
			"zsh:zle:1: Bad file descriptor number for -F: 3x\n", 1,
		},
		{
			"padded with spaces", `zle -F " 3 " h`,
			"zsh:zle:1: Bad file descriptor number for -F:  3 \n", 1,
		},
		{"too many operands", "h(){ :; }; zle -F 3 h x", "zsh:zle:1: too many arguments for -F\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+"\n")
			if out != c.want || st != c.status {
				t.Errorf("output = %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// TestAWordThatIsMinusThenADigitIsAnOperand is the parsing rule underneath the
// row above, and it is visible without `-F` at all: `zle -0` is an attempt to
// call a widget of that name rather than a bad option.
func TestAWordThatIsMinusThenADigitIsAnOperand(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -0\n")
	if want := "zsh:zle:1: widgets can only be called when ZLE is active\n"; out != want || st != 1 {
		t.Errorf("output = %q status %d, want %q at 1", out, st, want)
	}
}

// TestMinusWIsAModifierAndNotAnOperation: `-w` alone is the bare `zle`, and it
// makes no difference to `-N`. It changes only what `-F` arms, and the listing
// says so.
func TestMinusWIsAModifierAndNotAnOperation(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -w\n")
	if out != "" || st != 1 {
		t.Errorf("bare -w = %q status %d, want nothing at 1", out, st)
	}
	out, st = runZsh(t, t.TempDir(), "f(){ :; }\nzle -N -w a f; echo st=$?\nzle -l -L\n")
	if want := "st=0\nzle -N a f\n"; out != want || st != 0 {
		t.Errorf("-N -w = %q status %d, want %q at 0", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "h(){ :; }\nzle -F -w 3 h; echo st=$?\nzle -F\n")
	if want := "st=0\nzle -F -w 3 h\n"; out != want || st != 0 {
		t.Errorf("-F -w = %q status %d, want %q at 0", out, st, want)
	}
	// And a plain re-arm of the same descriptor drops the `-w`.
	out, st = runZsh(t, t.TempDir(), "h(){ :; }\nzle -F -w 3 h; zle -F 3 h\nzle -F\n")
	if want := "zle -F 3 h\n"; out != want || st != 0 {
		t.Errorf("re-armed = %q status %d, want %q at 0", out, st, want)
	}
}

// TestASubshellGetsItsOwnWatchers, the way it gets its own bindings and its
// own widgets: the table is the Runner's, so a watcher armed inside `( )` is
// not armed outside it.
func TestASubshellGetsItsOwnWatchers(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "h(){ :; }\n(zle -F 3 h; zle -F)\nzle -F; echo outer=$?\n")
	if want := "zle -F 3 h\nouter=0\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// watchRunner is a Runner with src already run in it, for the tests about what
// happens when a descriptor wakes rather than about what a script sees.
func watchRunner(t *testing.T, src string) (*interp.Runner, *bytes.Buffer) {
	t.Helper()
	return watchRunnerWith(t, src, nil)
}

// watchRunnerWith is the same with one more thing said about the Runner, for
// the test whose subject is a descriptor the shell was started with.
func watchRunnerWith(
	t *testing.T, src string, configure func(*interp.Runner),
) (*interp.Runner, *bytes.Buffer) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Dialect: presetDialect(),
	}
	if configure != nil {
		configure(r)
	}
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return r, &out
}

// pipeAt hands a pipe to a shell as a descriptor of its own, choosing a shell
// number that is deliberately **not** the one the kernel gave it.
//
// That is the whole point of the helper. The two numbers are different things
// — see zlewatch.go — and a test in which they coincide cannot tell a shell
// that translates from one that passes the number straight through. Choosing
// the shell's number as the kernel's plus one makes them differ every time
// rather than usually, which is what a skip would have made of it.
func pipeAt(t *testing.T) (shellFd, systemFd int, write *os.File, inherited []*os.File) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })
	systemFd = int(read.Fd())
	shellFd = systemFd + 1
	// InheritedFiles is laid out with entry i at descriptor 3+i.
	inherited = make([]*os.File, shellFd-2)
	inherited[shellFd-3] = read
	return shellFd, systemFd, write, inherited
}

// TestWhatIsArmedIsWhatIsWaitedOn is the first half of the seam, and what it
// asserts is the *translation*: the shell's numbers go into the table and the
// kernel's come out, in arming order, and a removal takes one out.
func TestWhatIsArmedIsWhatIsWaitedOn(t *testing.T) {
	shellFd, systemFd, _, inherited := pipeAt(t)
	arm := "h(){ :; }; zle -F " + strconv.Itoa(shellFd) + " h"
	r, _ := watchRunnerWith(t, arm, func(rr *interp.Runner) { rr.InheritedFiles = inherited })
	if got := zsh.WatchedDescriptors(r); !equalInts(got, []int{systemFd}) {
		t.Errorf("waited on %v, want the kernel's %v and not the shell's %d", got, []int{systemFd}, shellFd)
	}
	// And a removal takes it out again.
	r, _ = watchRunnerWith(t, arm+"; zle -F "+strconv.Itoa(shellFd),
		func(rr *interp.Runner) { rr.InheritedFiles = inherited })
	if got := zsh.WatchedDescriptors(r); len(got) != 0 {
		t.Errorf("after removal = %v, want none", got)
	}
	// A shell that has never armed one asks for nothing, which is what makes
	// repl skip the wait entirely.
	r, _ = watchRunner(t, ":")
	if got := zsh.WatchedDescriptors(r); len(got) != 0 {
		t.Errorf("never armed = %v, want none", got)
	}
}

// TestSeveralWatchersAreWaitedOnInArmingOrder, because the order is the
// listing's order and the translation must not quietly sort or dedupe them.
func TestSeveralWatchersAreWaitedOnInArmingOrder(t *testing.T) {
	firstShell, firstSystem, _, inherited := pipeAt(t)
	second, secondWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(); _ = secondWrite.Close() })
	secondSystem := int(second.Fd())
	secondShell := firstShell + 1
	for len(inherited) < secondShell-2 {
		inherited = append(inherited, nil)
	}
	inherited[secondShell-3] = second

	r, _ := watchRunnerWith(t,
		"h(){ :; }; zle -F "+strconv.Itoa(secondShell)+" h; zle -F "+strconv.Itoa(firstShell)+" h",
		func(rr *interp.Runner) { rr.InheritedFiles = inherited })
	if got := zsh.WatchedDescriptors(r); !equalInts(got, []int{secondSystem, firstSystem}) {
		t.Errorf("waited on %v, want %v in arming order", got, []int{secondSystem, firstSystem})
	}
}

// TestAPlainHandlerIsCalledWithTheDescriptorAndLeavesTheLineAlone is the other
// half, and every clause of it is measured: one argument, which is the
// descriptor; no line parameters; nothing to redraw.
func TestAPlainHandlerIsCalledWithTheDescriptorAndLeavesTheLineAlone(t *testing.T) {
	r, out := watchRunner(t,
		`h(){ print -r -- "called argc=$# one=$1 buffer=[${BUFFER-UNSET}] widget=[${WIDGET-UNSET}]"; }`+
			"\nzle -F 7 h")
	line := repl.Line{Buffer: "half typed", Cursor: 4}
	got, changed := zsh.DescriptorReady(r, context.Background(), 7, line)
	if changed {
		t.Error("a plain handler asked for a redraw; measured, nothing is drawn")
	}
	if got != line {
		t.Errorf("line = %#v, want it untouched as %#v", got, line)
	}
	want := "called argc=1 one=7 buffer=[UNSET] widget=[UNSET]\n"
	if out.String() != want {
		t.Errorf("handler output = %q, want %q", out.String(), want)
	}
}

// TestTheStatusGoesInAndDoesNotComeOut, the discipline widgets, hooks and
// `sched` already run under: a callback fires at a moment nobody chose, so
// what it leaves behind must not be what the next `&&` reads.
func TestACallbacksStatusGoesInAndDoesNotComeOut(t *testing.T) {
	r, out := watchRunner(t, "h(){ print -r -- \"saw=$?\"; return 7; }\nzle -F 7 h\n(exit 4)")
	if r.ExitStatus() != 4 {
		t.Fatalf("status before = %d, want 4", r.ExitStatus())
	}
	zsh.DescriptorReady(r, context.Background(), 7, repl.Line{Buffer: "", Cursor: 0})
	if want := "saw=4\n"; out.String() != want {
		t.Errorf("handler saw %q, want %q", out.String(), want)
	}
	if r.ExitStatus() != 4 {
		t.Errorf("status after = %d, want 4 — the handler's 7 must not come out", r.ExitStatus())
	}
}

// TestADescriptorNobodyArmedAndAHandlerThatIsNowhereDoNothing: both are the
// silent cases, and both have to be, because the table is allowed to hold a
// handler whose function has not arrived and repl may ask about a descriptor
// the handler removed a moment ago.
func TestADescriptorNobodyArmedAndAHandlerThatIsNowhereDoNothing(t *testing.T) {
	r, out := watchRunner(t, "h(){ print no; }\nzle -F 7 h")
	line := repl.Line{Buffer: "x", Cursor: 1}
	if got, changed := zsh.DescriptorReady(r, context.Background(), 8, line); changed || got != line {
		t.Errorf("unarmed descriptor = %#v %v, want the line untouched and false", got, changed)
	}
	if out.String() != "" {
		t.Errorf("unarmed descriptor printed %q", out.String())
	}
	r, out = watchRunner(t, "zle -F 7 nosuchfn")
	if got, changed := zsh.DescriptorReady(r, context.Background(), 7, line); changed || got != line {
		t.Errorf("missing handler = %#v %v, want the line untouched and false", got, changed)
	}
	if out.String() != "" {
		t.Errorf("missing handler printed %q", out.String())
	}
}

// TestMinusWHandsTheLineOverAndTakesItBack is the `-w` spelling, which is the
// widget round trip keyed on a descriptor: the five parameters are there, the
// argument is still the descriptor, `$WIDGET` names the handler, and an
// assignment to `BUFFER` comes back to be drawn.
func TestMinusWHandsTheLineOverAndTakesItBack(t *testing.T) {
	r, out := watchRunner(t,
		`w(){ print -r -- "argc=$# one=$1 widget=$WIDGET buffer=$BUFFER cursor=$CURSOR"; BUFFER=rewritten; }`+
			"\nzle -N w\nzle -F -w 7 w")
	got, changed := zsh.DescriptorReady(r, context.Background(), 7, repl.Line{Buffer: "abc", Cursor: 3})
	if !changed {
		t.Fatal("-w did not hand the line back; measured, its assignment to BUFFER is drawn")
	}
	if want := (repl.Line{Buffer: "rewritten", Cursor: 3}); got != want {
		t.Errorf("line = %#v, want %#v", got, want)
	}
	if want := "argc=1 one=7 widget=w buffer=abc cursor=3\n"; out.String() != want {
		t.Errorf("handler output = %q, want %q", out.String(), want)
	}
}

// TestMinusWOnSomethingThatIsNotAWidgetNeverFires. Measured: with `-w` and a
// handler that `zle -N` never defined, the real shell calls nothing at all, in
// silence — so a plugin that armed one and waited waits forever, and this
// shell has to wait with it rather than being helpfully different.
func TestMinusWOnSomethingThatIsNotAWidgetNeverFires(t *testing.T) {
	r, out := watchRunner(t, "h(){ print fired; }\nzle -F -w 7 h")
	line := repl.Line{Buffer: "abc", Cursor: 3}
	if got, changed := zsh.DescriptorReady(r, context.Background(), 7, line); changed || got != line {
		t.Errorf("result = %#v %v, want the line untouched and false", got, changed)
	}
	if out.String() != "" {
		t.Errorf("it fired, printing %q", out.String())
	}
}

// TestTheEditorIsRunningInsideAHandlerButTheLineIsNot is a pair of facts that
// look contradictory and were both measured: `zle some-widget` from inside a
// plain handler is status 0, and yet `$BUFFER` is unset there.
//
// The second half is the one that would rot: the marker this uses to answer
// the first is a variable set to the empty string, and a name set to the empty
// string still *exists*, so asking existence rather than the value made every
// prompt after the first widget of a session look like a widget's own call.
func TestTheEditorIsRunningInsideAHandlerButTheLineIsNot(t *testing.T) {
	r, out := watchRunner(t,
		"other(){ print -r -- ran-other; }\nzle -N other\n"+
			"h(){ zle other; print -r -- \"nested=$?\"; }\nzle -F 7 h")
	zsh.DescriptorReady(r, context.Background(), 7, repl.Line{Buffer: "", Cursor: 0})
	if want := "ran-other\nnested=0\n"; out.String() != want {
		t.Errorf("handler output = %q, want %q", out.String(), want)
	}
	// And afterwards the editor is not running again, so a script's `zle` is
	// refused the way it is refused before any callback has ever fired.
	out.Reset()
	f, err := syntax.Parse("zle other; echo after=$?\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	want := "zsh:zle:1: widgets can only be called when ZLE is active\nafter=1\n"
	if out.String() != want {
		t.Errorf("after the callback = %q, want %q", out.String(), want)
	}
}

// TestARealPipeIsWokenByOneNumberAndReadThroughTheOther is the test the whole
// feature turns on, and it is the one the tests above cannot be: it watches
// **both** numbers at once.
//
// The pipe reaches the shell as a descriptor of the shell's own, at a number
// chosen to differ from the kernel's. What goes to repl to be waited on must
// be the kernel's — waiting on the shell's would be waiting on something else
// entirely — and what the handler is told must be the shell's, because
// `read -u $1` inside it is a question about the shell's table.
//
// Get either backwards and every other test in this file still passes, the
// shell still says everything right to a script, and a real session never
// fires a callback once.
func TestARealPipeIsWokenByOneNumberAndReadThroughTheOther(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("payload\n"); err != nil {
		t.Fatal(err)
	}
	r, out := watchRunnerWith(t,
		"h(){ IFS= read -u $1 -r line; print -r -- \"got=[$line] arg=$1\"; }\n"+
			"zle -F "+strconv.Itoa(shellFd)+" h",
		func(rr *interp.Runner) { rr.InheritedFiles = inherited })

	if got := zsh.WatchedDescriptors(r); !equalInts(got, []int{systemFd}) {
		t.Fatalf("waited on %v, want the kernel's %v and not the shell's %d", got, []int{systemFd}, shellFd)
	}
	// Woken by the kernel's number, the handler reads through the shell's.
	zsh.DescriptorReady(r, context.Background(), systemFd, repl.Line{Buffer: "", Cursor: 0})
	want := "got=[payload] arg=" + strconv.Itoa(shellFd) + "\n"
	if out.String() != want {
		t.Errorf("handler output = %q, want %q", out.String(), want)
	}
}

// TestAWatcherOnANumberNothingIsOpenAtIsNeverWaitedOn is the other side of the
// translation: the entry is kept and said back — measured, arming one is
// silent whatever is at the number — and it is not something the editor can
// wait on, so it never fires. Which is what the real shell does with one.
func TestAWatcherOnANumberNothingIsOpenAtIsNeverWaitedOn(t *testing.T) {
	r, _ := watchRunner(t, "h(){ :; }; zle -F 55 h")
	if got := zsh.WatchedDescriptors(r); len(got) != 0 {
		t.Errorf("waited on %v, want none — nothing is open at 55", got)
	}
	out, st := runZsh(t, t.TempDir(), "h(){ :; }\nzle -F 55 h\nzle -F\n")
	if want := "zle -F 55 h\n"; out != want || st != 0 {
		t.Errorf("listing = %q status %d, want %q at 0 — it is still armed", out, st, want)
	}
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
