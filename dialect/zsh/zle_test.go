// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// The `zle` builtin, measured against zsh 5.9.2 on 2026-09-07 with a scratch
// HOME and no startup files — under `-c` for what a script can see, and
// through a pseudo-terminal one keystroke at a time for what only a widget
// that ran can. Every want below is what that binary wrote.

// zleRunner runs some source and hands back the Runner it ran in, so a test
// can then run a widget against it the way the front end does. runZsh cannot
// be used for those: it keeps the Runner to itself, and the widget table and
// the line are both state.
func zleRunner(t *testing.T, src string) (*interp.Runner, *bytes.Buffer) {
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
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return r, &out
}

// runWidget is what repl does with a keystroke, spelled out so a test can see
// both halves: what the widget did to the line, and what it printed.
func runWidget(t *testing.T, r *interp.Runner, out *bytes.Buffer, name string, in repl.Line) (repl.Line, bool, string) {
	t.Helper()
	out.Reset()
	line, ok := zsh.RunWidget(r, context.Background(), name, in)
	return line, ok, out.String()
}

// TestAWidgetIsDefinedAndSaidBack is the honest minimum the issue asked for:
// an rc file that defines a widget runs to the end, and the widget is there
// afterwards in both spellings of the listing.
func TestAWidgetIsDefinedAndSaidBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"f(){:}; zle -N b f; zle -N a; zle -N c f; echo st=$?\nzle -l\n")
	if want := "st=0\na\nb (f)\nc (f)\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// The two listings are exact, and both are sorted by widget name whatever
// order the definitions arrived in.
func TestTheListingHasTwoSpellings(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "f(){:}; zle -N b f; zle -N a; zle -N c f\nzle -l -L\n")
	if want := "zle -N a\nzle -N b f\nzle -N c f\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestAWidgetNamingAFunctionThatIsNotThereIsAccepted is the fact that makes a
// real startup file work: a plugin defines its widgets and its functions in
// whichever order suits it, so `zle -N` cannot require the function yet.
func TestAWidgetNamingAFunctionThatIsNotThereIsAccepted(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -N foo; echo st=$?\nzle -l -L\n")
	if want := "st=0\nzle -N foo\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// `zle -l name` is a question and not a listing — no output, and the status
// says whether every name given is a widget. This is what a plugin's
// `zle -l foo || zle -N foo` asks, so it had to be got right rather than
// approximated.
func TestListingByNameIsAQuestion(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"f(){:}; zle -N a f; zle -N b f\n"+
			"zle -l a b; echo two=$?\nzle -l nosuch; echo one=$?\nzle -l a nosuch; echo mixed=$?\n")
	if want := "two=0\none=1\nmixed=1\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// With -L the ones that exist are written out, which is the only way the
	// name form prints anything.
	out, _ = runZsh(t, t.TempDir(), "f(){:}; zle -N a f\nzle -l -L a; echo st=$?\n")
	if want := "zle -N a f\nst=0\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// `-a` widens the question from the widgets somebody defined to every widget
// this shell has, and `zle -l` alone must not answer for the editor's own.
//
// The list is this shell's own and not zsh's 386. A name in the answer is a
// claim that a key bound to it does something, which is the rule bindkey.go
// applies to the keymaps.
func TestEveryWidgetIsADifferentQuestionFromTheDefinedOnes(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zle -l end-of-line; echo plain=$?\nzle -l -a end-of-line; echo all=$?\n")
	if want := "plain=1\nall=0\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// And the full listing has the editor's actions in it, under this
	// shell's names for them, with nothing defined — where the plain listing
	// has nothing at all.
	if bare, _ := runZsh(t, t.TempDir(), "zle -l\n"); bare != "" {
		t.Errorf("with nothing defined `zle -l` wrote %q, want nothing", bare)
	}
	out, _ = runZsh(t, t.TempDir(), "zle -la\n")
	for _, want := range []string{"end-of-line", "up-line-or-history", "undefined-key"} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("`zle -la` = %q, want %q among the names", out, want)
		}
	}
}

// TestZleRefusesEachMistakeItsOwnWay pins the wordings, which are facts about
// this builtin and not shared with the others: `bad option` is bindkey's and
// zmodload's rather than zstyle's `invalid option`, and the usage complaints
// name the letter that was short, which zstyle's do not.
func TestZleRefusesEachMistakeItsOwnWay(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a letter zsh has not got", "zle -Q", "zsh:zle:1: bad option: -Q\n", 1},
		{"nothing at all", "zle", "", 1},
		{"-N short", "zle -N", "zsh:zle:1: not enough arguments for -N\n", 1},
		{"-N long", "zle -N a b c", "zsh:zle:1: too many arguments for -N\n", 1},
		{"-D short", "zle -D", "zsh:zle:1: not enough arguments for -D\n", 1},
		{"-D unknown", "zle -D nosuch", "zsh:zle:1: no such widget `nosuch'\n", 1},
		{"-A short", "zle -A a", "zsh:zle:1: not enough arguments for -A\n", 1},
		{"-A long", "zle -A a b c", "zsh:zle:1: too many arguments for -A\n", 1},
		{"-A unknown", "zle -A nosuch x", "zsh:zle:1: no such widget `nosuch'\n", 1},
		{
			"a widget from a script",
			"f(){:}; zle -N w f; zle w",
			"zsh:zle:1: widgets can only be called when ZLE is active\n", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+"\n")
			if out != c.want || st != c.status {
				t.Errorf("output = %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// A letter this shell has not got says so, rather than being accepted and
// doing nothing.
//
// This is the whole reason the builtin is worth having: a `zle` that took
// everything would be worse than the `command not found` it replaces, because
// a plugin would then believe its widget existed. `zle -F` is the one that
// matters — the callback on a descriptor, which is how a plugin in this shell
// does asynchrony — and it is refused by name rather than half-built.
func TestALetterThisShellHasNotGotSaysSo(t *testing.T) {
	for _, letter := range []string{"F", "R", "M", "U", "C", "I", "K", "T", "w", "c", "f", "g", "m", "r", "G"} {
		out, st := runZsh(t, t.TempDir(), "zle -"+letter+" x y\n")
		want := "zsh:zle:1: -" + letter + " is not implemented yet\n"
		if out != want || st != 1 {
			t.Errorf("-%s = %q status %d, want %q at 1", letter, out, st, want)
		}
	}
}

// `-D` takes several, and a name nothing answers to costs the status rather
// than the rest of the list.
func TestDeletingWidgets(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"f(){:}; zle -N a f; zle -N b f\nzle -D a b; echo st=$?\nzle -l\n")
	if want := "st=0\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
	out, _ = runZsh(t, t.TempDir(),
		"f(){:}; zle -N a f\nzle -D nosuch a; echo st=$?\nzle -l\n")
	want := "zsh:zle:2: no such widget `nosuch'\nst=1\n"
	if out != want {
		t.Errorf("output = %q, want %q — the rest of the list still went", out, want)
	}
}

// `-A` is a copy and not a notion of an alias, which is measured: the second
// name lists exactly as a definition of its own would.
func TestAliasingAWidgetCopiesIt(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "f(){:}; zle -N w f; zle -A w x; echo st=$?\nzle -l\nzle -l -L\n")
	want := "st=0\nw (f)\nx (f)\nzle -N w f\nzle -N x f\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	// One of the editor's own actions is not copyable here, and says so: it
	// would need a widget name standing for a Widget rather than for a
	// function.
	out, st := runZsh(t, t.TempDir(), "zle -A end-of-line x\n")
	if w := "zsh:zle:1: -A of a built-in widget is not implemented yet\n"; out != w || st != 1 {
		t.Errorf("output = %q status %d, want %q at 1", out, st, w)
	}
}

// The parameters a widget reads are `local` to its call — `${(t)BUFFER}` says
// `scalar-local-special` — so a shell that is not running one must find them
// unset, and assigning to one there is an ordinary variable assignment.
func TestTheWidgetParametersAreNotThereOutsideAWidget(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		`print -r -- "B=[${BUFFER-UNSET}] C=[${CURSOR-UNSET}] L=[${LBUFFER-UNSET}] R=[${RBUFFER-UNSET}] W=[${WIDGET-UNSET}]"`+"\n"+
			"BUFFER=hi; print -r -- \"set=[$BUFFER]\"\n")
	want := "B=[UNSET] C=[UNSET] L=[UNSET] R=[UNSET] W=[UNSET]\nset=[hi]\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRunningAWidgetHandsOverTheLineAndTakesItBack is the round trip, and the
// parameters are the whole of what a widget can do with a line.
func TestRunningAWidgetHandsOverTheLineAndTakesItBack(t *testing.T) {
	r, out := zleRunner(t, `w() { print -r -- "W=$WIDGET B=[$BUFFER] C=$CURSOR L=[$LBUFFER] R=[$RBUFFER]"; BUFFER="rewritten"; CURSOR=3; }
zle -N w
`)
	line, ok, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "hello world", Cursor: 5})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "W=w B=[hello world] C=5 L=[hello] R=[ world]\n"; printed != want {
		t.Errorf("the widget saw %q, want %q", printed, want)
	}
	if want := (repl.Line{Buffer: "rewritten", Cursor: 3}); line != want {
		t.Errorf("line back = %+v, want %+v", line, want)
	}
}

// And each assignment is live in the arithmetic the other three answer with,
// read back inside the same widget — which is why these cannot be four plain
// variables reconciled when the call returns.
func TestTheFourParametersAreLiveInEachOthersArithmetic(t *testing.T) {
	r, out := zleRunner(t, `w() {
  BUFFER="abcdef"; CURSOR=2
  print -r -- "1 L=[$LBUFFER] R=[$RBUFFER] C=$CURSOR"
  LBUFFER="XY"
  print -r -- "2 B=[$BUFFER] C=$CURSOR"
  RBUFFER="ZZ"
  print -r -- "3 B=[$BUFFER] C=$CURSOR"
  LBUFFER+="!"
  print -r -- "4 B=[$BUFFER] C=$CURSOR"
}
zle -N w
`)
	line, ok, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "", Cursor: 0})
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := "1 L=[ab] R=[cdef] C=2\n" +
		"2 B=[XYcdef] C=2\n" +
		"3 B=[XYZZ] C=2\n" +
		"4 B=[XY!ZZ] C=3\n"
	if printed != want {
		t.Errorf("the widget saw:\n%q\nwant:\n%q", printed, want)
	}
	if got, w := line, (repl.Line{Buffer: "XY!ZZ", Cursor: 3}); got != w {
		t.Errorf("line back = %+v, want %+v", got, w)
	}
}

// A line that just got shorter cannot leave the cursor past its end, which is
// the clamp on the *other* parameter's assignment and the one a widget that
// replaces the whole buffer reaches.
func TestShorteningTheBufferBringsTheCursorWithIt(t *testing.T) {
	r, out := zleRunner(t, `w() { print -r -- "before=$CURSOR"; BUFFER="ab"; print -r -- "after=$CURSOR L=[$LBUFFER] R=[$RBUFFER]"; }
zle -N w
`)
	line, ok, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "abcdefgh", Cursor: 8})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "before=8\nafter=2 L=[ab] R=[]\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if want := (repl.Line{Buffer: "ab", Cursor: 2}); line != want {
		t.Errorf("line back = %+v, want %+v", line, want)
	}
}

// The cursor is clamped rather than refused: 999 on a four-character line
// reads back 4 and -5 reads back 0.
func TestTheCursorIsClampedRatherThanRefused(t *testing.T) {
	r, out := zleRunner(t, `w() { CURSOR=999; print -r -- "hi=$CURSOR"; CURSOR=-5; print -r -- "lo=$CURSOR"; }
zle -N w
`)
	_, ok, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "abcd", Cursor: 0})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "hi=4\nlo=0\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
}

// `$?` goes in and does not come out. With a failure before the keystroke the
// widget function sees it; what the widget leaves is not what the next command
// reads. The same discipline hooks run under, measured the same way.
func TestTheStatusGoesInAndDoesNotComeOut(t *testing.T) {
	r, out := zleRunner(t, "w() { print -r -- \"sees=$?\"; return 7; }\nzle -N w\nfalse\n")
	_, ok, printed := runWidget(t, r, out, "w", repl.Line{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "sees=1\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if got := r.ExitStatus(); got != 1 {
		t.Errorf("status after = %d, want 1 — the widget's own must not reach the next command", got)
	}
}

// A widget invoking another runs it against the same line, and `$WIDGET` stays
// the outer one's name — measured, `zle b` from `a` reports `a` inside `b`.
func TestOneWidgetInvokesAnotherAgainstTheSameLine(t *testing.T) {
	r, out := zleRunner(t, `b() { BUFFER="$BUFFER-B"; print -r -- "in-b W=$WIDGET"; }
a() { zle b; print -r -- "back rc=$? B=[$BUFFER]"; }
zle -N a; zle -N b
`)
	line, ok, printed := runWidget(t, r, out, "a", repl.Line{Buffer: "x", Cursor: 1})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "in-b W=a\nback rc=0 B=[x-B]\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if got, w := line.Buffer, "x-B"; got != w {
		t.Errorf("line back = %q, want %q", got, w)
	}
}

// A name nothing answers to is status 1 in *silence*, with no diagnostic at
// all. Measured with the widget's own stderr redirected to a file, so a redraw
// could not have hidden one.
func TestInvokingANameNothingAnswersToIsSilent(t *testing.T) {
	r, out := zleRunner(t, "a() { zle nosuch_zz; print -r -- \"rc=$?\"; }\nzle -N a\n")
	_, ok, printed := runWidget(t, r, out, "a", repl.Line{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "rc=1\n"; printed != want {
		t.Errorf("output = %q, want %q — nothing is said about the name", printed, want)
	}
}

// And invoking one of the *editor's* own actions refuses by name. The name
// resolves perfectly well; what it would take is for a shell function to reach
// back into the editor mid-keystroke.
func TestInvokingABuiltInWidgetRefusesByName(t *testing.T) {
	r, out := zleRunner(t, "a() { zle end-of-line; print -r -- \"rc=$?\"; }\nzle -N a\n")
	_, ok, printed := runWidget(t, r, out, "a", repl.Line{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := "a:zle: end-of-line: calling a built-in widget is not implemented yet\nrc=1\n"
	if printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
}

// A widget whose function never arrived does not run, which is what makes the
// key that names it do nothing — measured through a pseudo-terminal, silence
// and a live shell.
func TestAWidgetWithNoFunctionDoesNotRun(t *testing.T) {
	r, out := zleRunner(t, "zle -N w never_defined_zz\n")
	line, ok, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "abc", Cursor: 1})
	if ok {
		t.Error("it ran, want a refusal so the key does nothing")
	}
	if printed != "" {
		t.Errorf("output = %q, want nothing said", printed)
	}
	if want := (repl.Line{Buffer: "abc", Cursor: 1}); line != want {
		t.Errorf("line = %+v, want it untouched at %+v", line, want)
	}
	// And a name that is not a widget at all is the same answer.
	if _, ok := zsh.RunWidget(r, context.Background(), "not-a-widget", repl.Line{}); ok {
		t.Error("a name that is not a widget ran")
	}
}

// The parameters are taken away again when the call ends, so nothing about one
// keystroke's widget is visible to the next one's or to a script.
func TestTheParametersAreGoneWhenTheWidgetEnds(t *testing.T) {
	r, out := zleRunner(t, "w() { BUFFER=inside; }\nzle -N w\n")
	if _, ok, _ := runWidget(t, r, out, "w", repl.Line{Buffer: "abc", Cursor: 3}); !ok {
		t.Fatal("the widget did not run")
	}
	for _, name := range []string{"BUFFER", "CURSOR", "LBUFFER", "RBUFFER", "WIDGET"} {
		if v, set := r.GetVar(name); set {
			t.Errorf("%s = %q after the call, want unset", name, v)
		}
	}
}

// And they are taken away however the call ended, which is why the close is a
// defer: the panic guard a widget runs behind is repl's and is outside this
// call, so a straight-line close would be skipped and a script at the next
// prompt would find `$BUFFER` set — after a crash nobody would connect it to.
// A widget that called `exit` is the reachable version of the same path.
func TestTheParametersAreGoneHoweverTheCallEnded(t *testing.T) {
	r, out := zleRunner(t, "w() { BUFFER=x; exit 3; }\nzle -N w\n")
	if _, _, printed := runWidget(t, r, out, "w", repl.Line{Buffer: "abc", Cursor: 3}); printed != "" {
		t.Errorf("output = %q, want nothing said", printed)
	}
	if !r.Exited() {
		t.Fatal("the widget did not exit, so this is not the path under test")
	}
	for _, name := range []string{"BUFFER", "CURSOR", "LBUFFER", "RBUFFER", "WIDGET"} {
		if v, set := r.GetVar(name); set {
			t.Errorf("%s = %q after a widget that exited, want unset", name, v)
		}
	}
}

// TestAScriptCannotCallAWidgetAfterOneHasRun is the bug mutation testing
// found, and the reason the guard reads a *value* rather than asking whether
// the name is set.
//
// The state a call leaves behind is cleared by storing the empty string, which
// GetVar reports as set — so the first widget to run left every later script
// able to invoke widgets, at status 0 and with the function actually running,
// where the shell being modeled refuses every time. Nothing in the suite
// noticed, because every other test asked a *fresh* runner.
func TestAScriptCannotCallAWidgetAfterOneHasRun(t *testing.T) {
	r, out := zleRunner(t, "w() { print -r -- ran; }\nzle -N w\n")
	if _, ok, _ := runWidget(t, r, out, "w", repl.Line{Buffer: "x", Cursor: 1}); !ok {
		t.Fatal("the widget did not run, so this is not the path under test")
	}
	out.Reset()
	zle, ok := r.Builtin("zle")
	if !ok {
		t.Fatal("no zle builtin")
	}
	if st := zle(r, context.Background(), []string{"w"}); st != 1 {
		t.Errorf("`zle w` from a script = status %d, want 1", st)
	}
	want := "zsh:1: widgets can only be called when ZLE is active\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q — and above all not the widget having run", out.String(), want)
	}
}

// `$WIDGET` is read-only while a widget runs, measured: assigning to it answers
// `read-only variable: WIDGET` and the widget stops there. A writer that
// quietly stored the new name would be the silent no-op this builtin exists to
// avoid.
func TestTheWidgetNameIsReadOnly(t *testing.T) {
	r, out := zleRunner(t, "w() { WIDGET=changed; print -r -- \"unreached=[$WIDGET]\"; }\nzle -N w\n")
	if _, ok, _ := runWidget(t, r, out, "w", repl.Line{}); !ok {
		t.Fatal("the widget did not run")
	}
	if !strings.Contains(out.String(), "read-only variable: WIDGET") {
		t.Errorf("output = %q, want the assignment refused by name", out.String())
	}
	if strings.Contains(out.String(), "unreached") {
		t.Errorf("output = %q, want the widget to have stopped at the refusal", out.String())
	}
}

// And the read-only mark is lifted with the rest of the call, so a script
// outside a widget finds an ordinary variable rather than a name that refuses
// every assignment for the rest of the session. There is no other way to lift
// it, which is why UnsetDynamic does — see the note there.
func TestTheWidgetNameIsAnOrdinaryVariableAfterTheCall(t *testing.T) {
	r, out := zleRunner(t, "w() { :; }\nzle -N w\n")
	if _, ok, _ := runWidget(t, r, out, "w", repl.Line{}); !ok {
		t.Fatal("the widget did not run")
	}
	res, _ := runZshVars(t, r, "WIDGET=fine; print -r -- \"after=[$WIDGET]\"")
	if want := "after=[fine]\n"; res != want {
		t.Errorf("after the call: %q, want %q", res, want)
	}
}

// `zle -N` on a name that is already a widget replaces the function rather than
// adding a second entry — measured, `zle -N w f; zle -N w g` leaves one widget
// backed by `g`.
func TestRedefiningAWidgetReplacesIt(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"f() { :; }; g() { :; }; zle -N w f; zle -N w g\nzle -l\nzle -l -L\n")
	if want := "w (g)\nzle -N w g\n"; out != want {
		t.Errorf("output = %q, want %q — one widget, the later function", out, want)
	}
}

// Under `-L`, one of the editor's own actions is still its own name: there is
// no function behind it and no `zle -N` that would define it. Measured —
// `zle -la -L` in the real shell writes `accept-line`, not `zle -N accept-line`.
func TestTheSourceListingLeavesTheEditorsOwnActionsAlone(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zle -la -L\n")
	if !strings.Contains(out, "end-of-line\n") {
		t.Errorf("output = %q, want the action's bare name in it", out)
	}
	if strings.Contains(out, "zle -N end-of-line") {
		t.Errorf("output = %q, want no definition written for an action nothing defined", out)
	}
}

// TestAKeyBoundToADefinedWidgetReachesTheFrontEndAsOne is the contract between
// this builtin and the front end, and the half no output can show: what
// reaches repl for a key bound to a widget somebody defined is the *name*, not
// one of the editor's actions.
func TestAKeyBoundToADefinedWidgetReachesTheFrontEndAsOne(t *testing.T) {
	r, _ := zleRunner(t, "f(){:}; zle -N mine f\nbindkey '^G' mine\n")
	got := zsh.KeyBindings(r)
	if want := (repl.Binding{Function: "mine"}); got["\a"] != want {
		t.Errorf("table = %v, want %v for ^G", got, want)
	}
	// A widget nobody defined is still present and bound to nothing, which is
	// what stops the key doing what it used to — bindkey.go's own rule, and
	// unchanged by this.
	r, _ = zleRunner(t, "bindkey '^G' history-substring-search-up\n")
	if got := zsh.KeyBindings(r); got["\a"] != (repl.Binding{}) {
		t.Errorf("table = %v, want ^G present and doing nothing", got)
	}
	// And a definition wins over one of the editor's own names, which is what
	// redefining one means and the only order a plugin's wrapper can work in.
	r, _ = zleRunner(t, "f(){:}; zle -N end-of-line f\nbindkey '^G' end-of-line\n")
	if got, want := zsh.KeyBindings(r)["\a"], (repl.Binding{Function: "end-of-line"}); got != want {
		t.Errorf("table = %v, want %v", got, want)
	}
}

// The module the builtin belongs to still answers about itself rather than
// pretending, and `whence` finds the name because registering is enough.
func TestZleIsABuiltinTheShellWillOwnUpTo(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "whence -v zle; whence -w zle\n")
	if want := "zle is a shell builtin\nzle: builtin\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// runZshVars runs more source on a Runner a widget has already been through,
// which is what the tests about what a *later* script sees need.
func runZshVars(t *testing.T, r *interp.Runner, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	saved := r.Stdout
	savedErr := r.Stderr
	r.Stdout, r.Stderr = &out, &out
	st, rerr := r.Run(context.Background(), f)
	r.Stdout, r.Stderr = saved, savedErr
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}
