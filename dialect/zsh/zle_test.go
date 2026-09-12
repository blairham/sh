// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"slices"
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
//
// With an editor on the other end, because in a session there always is one:
// repl puts a repl.Actions on the context every widget call runs under, and
// `zle up-line-or-history` from inside a widget is a call back through it. A
// test that left it off would be testing the no-editor path and calling it the
// ordinary one.
func runWidget(t *testing.T, r *interp.Runner, out *bytes.Buffer, name string, in repl.Line) (repl.Line, bool, string) {
	t.Helper()
	line, ok, said, _ := runWidgetWatching(t, r, out, name, in, &stubEditor{})
	return line, ok, said
}

// runWidgetWatching is the same with the editor handed in, for a test that
// wants to see what the widget asked the editor to do.
func runWidgetWatching(
	t *testing.T, r *interp.Runner, out *bytes.Buffer, name string, in repl.Line, ed *stubEditor,
) (repl.Line, bool, string, *stubEditor) {
	t.Helper()
	out.Reset()
	line, ok := zsh.RunWidget(r, repl.WithActions(context.Background(), ed), name, in)
	return line, ok, out.String(), ed
}

// stubEditor is an editor for a widget to reach back into.
//
// It records rather than edits, and that is deliberate: what
// `up-line-or-history` does to a line is repl's to answer and repl's tests
// pin it. What this package has to prove is the *naming* — that `zle
// up-line-or-history` reaches the action repl calls WidgetPreviousHistory,
// that what comes back is written into the four parameters the next line of
// the widget reads, and that the two the editor declines are refused out loud.
// A stub that reimplemented the editor would let those pass while the real
// pairing was wrong.
type stubEditor struct {
	// performed is every action asked for, in order, and lines is what each
	// was handed.
	performed []repl.Widget
	lines     []repl.Line
	// refuse is the actions this editor declines, standing in for the two the
	// real one will not perform from inside a widget.
	refuse map[repl.Widget]bool
	// gives is what an action hands back, by action. An action with no entry
	// hands back the line it was given.
	gives map[repl.Widget]repl.Line
	// drawn is every Redisplay, and pushed is every PushKeys.
	drawn  []repl.Line
	pushed []string
}

func (e *stubEditor) Perform(w repl.Widget, in repl.Line) (repl.Line, bool) {
	e.performed = append(e.performed, w)
	e.lines = append(e.lines, in)
	if e.refuse[w] {
		return in, false
	}
	if out, ok := e.gives[w]; ok {
		return out, true
	}
	return in, true
}

func (e *stubEditor) Redisplay(in repl.Line) { e.drawn = append(e.drawn, in) }
func (e *stubEditor) PushKeys(s string)      { e.pushed = append(e.pushed, s) }

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
// a plugin would then believe its widget existed.
//
// `F`, `w` and now `C` used to be on this list and are not any more — the
// callback on a descriptor is built, in zlewatch.go, and the completion widget
// is built here — which is the shape of progress this test is meant to record:
// a letter leaves the list by being implemented and by nothing else. The three
// remaining spellings that need a seam repl has not got are named in zle.go's
// own comment rather than here.
func TestALetterThisShellHasNotGotSaysSo(t *testing.T) {
	for _, letter := range []string{"M", "I", "K", "T", "c", "f", "g", "m", "r", "G"} {
		out, st := runZsh(t, t.TempDir(), "zle -"+letter+" x y\n")
		want := "zsh:zle:1: -" + letter + " is not implemented yet\n"
		if out != want || st != 1 {
			t.Errorf("-%s = %q status %d, want %q at 1", letter, out, st, want)
		}
	}
}

// Two operation letters is a refusal, where this shell used to take whichever
// its `switch` reached first and do it in silence.
//
// Measured 2026-09-12 against zsh 5.9.2. The wording is one sentence for every
// pair and never names the letters, so the assertion is on the whole of what
// came back rather than on a substring.
//
// The last two rows are the ones that say *where* in the parse this happens.
// `zle -ND` with nothing after it answers this rather than `not enough
// arguments for -N`, so it is before the operands are counted; `zle -N -D w`
// answers it with the pair split over two words, so it is about the letters
// and not about one word of them. #1648.
func TestTwoOperationLettersAreRefused(t *testing.T) {
	const want = "zsh:zle:1: incompatible operation selection options\n"
	for _, src := range []string{
		"zle -ND w",
		"zle -NA a b",
		"zle -Dl",
		"zle -NC w complete-word f",
		"zle -NF 8 h",
		"zle -lD",
		"zle -AN a b",
		"zle -ND",
		"zle -N -D w",
	} {
		out, st := runZsh(t, t.TempDir(), src+"\n")
		if out != want || st != 1 {
			t.Errorf("%s = %q status %d, want %q at 1", src, out, st, want)
		}
	}
}

// And the three checks are in the order zsh answers them in.
//
// A letter this builtin does not have at all wins over the pair — measured,
// `zle -Nx w` and `zle -xN w` are both `bad option: -x` — and the pair wins
// over a letter this shell has not built, which is this shell's own stage and
// is why the order had to be chosen rather than fallen into. `zle -Nf w` is
// two operations to zsh and would be `-f is not implemented yet` here if the
// letters were not all read before either question is asked.
func TestTheThreeRefusalsComeInTheOrderZshAnswersThemIn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"zle -Nx w", "zsh:zle:1: bad option: -x\n"},
		{"zle -xN w", "zsh:zle:1: bad option: -x\n"},
		{"zle -Nf w", "zsh:zle:1: incompatible operation selection options\n"},
		{"zle -f w", "zsh:zle:1: -f is not implemented yet\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want || st != 1 {
			t.Errorf("%s = %q status %d, want %q at 1", c.src, out, st, c.want)
		}
	}
}

// A repeated letter is one operation and not two, and a modifier is not an
// operation at all.
//
// The counter-case to the test above, and what stops it being a rule that
// refuses everything with two letters in it. Every row here is status 0 in
// zsh 5.9.2, measured the same day: `-NN` and `-N -N` define a widget, and
// `-a`, `-w` and `-L` sit beside an operation without being one.
func TestARepeatedLetterAndAModifierAreNotASecondOperation(t *testing.T) {
	for _, src := range []string{
		"zle -NN w",
		"zle -N -N w",
		"zle -aC w complete-word f",
		"zle -C -w w complete-word f",
		"zle -NL w f",
		"zle -Naw w f",
		"zle -NwaL w f",
	} {
		out, st := runZsh(t, t.TempDir(), src+"\n")
		if out != "" || st != 0 {
			t.Errorf("%s = %q status %d, want silence at 0", src, out, st)
		}
	}
}

// A widget function is called with nothing at all.
//
// Measured 2026-09-12 through a pseudo-terminal against zsh 5.9.2, a key bound
// to each of the three kinds — a `-N` widget naming another function, a `-N`
// widget backed by a function of its own name, and a `-C` completion widget —
// with `$#` reported from inside. All three are 0. This shell passed the
// widget's name as `$1` until #1649, so a function that did `shift` or tested
// `$#` behaved differently here.
//
// `$#` and not `$WIDGET` is the discriminating probe: the name is available
// either way, and reading it back proves nothing about how it got there. The
// name is asserted alongside because it is what a wrapper shared between
// bindings is supposed to read instead.
func TestAWidgetFunctionIsCalledWithNoArguments(t *testing.T) {
	for _, c := range []struct{ name, src, widget string }{
		{
			"a widget naming another function",
			"nf() { print -r -- \"argc=$# args=[$*] widget=$WIDGET\"; }\nzle -N nnn nf\n",
			"nnn",
		},
		{
			"a widget backed by a function of its own name",
			"same() { print -r -- \"argc=$# args=[$*] widget=$WIDGET\"; }\nzle -N same\n",
			"same",
		},
		{
			"a completion widget",
			"cf() { print -r -- \"argc=$# args=[$*] widget=$WIDGET\"; }\nzle -C ccc complete-word cf\n",
			"ccc",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, out := zleRunner(t, c.src)
			_, ok, printed := runWidget(t, r, out, c.widget, repl.Line{Buffer: "abc", Cursor: 3})
			if !ok {
				t.Fatal("the widget did not run")
			}
			want := "argc=0 args=[] widget=" + c.widget + "\n"
			if printed != want {
				t.Errorf("the widget saw %q, want %q", printed, want)
			}
		})
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

// Invoking one of the *editor's* own actions reaches the editor, and the name
// it reaches by is this shell's half of the mapping.
//
// The assertion is on the action, not on the line: what
// WidgetPreviousHistory does to a buffer is repl's answer and repl's tests
// pin it. What can only be wrong here is the pairing — `up-line-or-history`
// is this shell's word for walking history back where the other shell with an
// editor says `previous-history`, and a mapping that reached the wrong action
// would still return 0 and still edit the line.
func TestInvokingABuiltInWidgetReachesTheEditor(t *testing.T) {
	r, out := zleRunner(t, "a() { zle up-line-or-history; print -r -- \"rc=$?\"; }\nzle -N a\n")
	_, ok, printed, ed := runWidgetWatching(t, r, out, "a", repl.Line{}, &stubEditor{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "rc=0\n"; printed != want {
		t.Errorf("output = %q, want %q — nothing is said and the status is 0", printed, want)
	}
	if want := []repl.Widget{repl.WidgetPreviousHistory}; !slices.Equal(ed.performed, want) {
		t.Errorf("performed %v, want %v", ed.performed, want)
	}
}

// What the editor hands back is written into the line the *widget* is holding,
// so the next line of the widget reads it.
//
// This is the fact the plugin the whole change was filed for depends on. Its
// `_history-substring-search-end` reads `$BUFFER` on the line after the walk
// and sets `CURSOR=${#BUFFER}` from it, so an effect deferred to the end of
// the widget — carried back the way an accept is — would arrive too late and
// the search would run against the line the keystroke started with. Measured
// 2026-09-12 against zsh 5.9.2: the buffer is the recalled entry immediately,
// with the cursor at its end.
func TestWhatTheEditorGivesBackIsVisibleToTheRestOfTheWidget(t *testing.T) {
	r, out := zleRunner(t,
		"a() { zle up-line-or-history; print -r -- \"[$BUFFER][$CURSOR][$LBUFFER]\"; }\nzle -N a\n")
	ed := &stubEditor{gives: map[repl.Widget]repl.Line{
		repl.WidgetPreviousHistory: {Buffer: "echo two", Cursor: 8},
	}}
	line, ok, printed, _ := runWidgetWatching(t, r, out, "a", repl.Line{}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "[echo two][8][echo two]\n"; printed != want {
		t.Errorf("the widget saw %q, want %q", printed, want)
	}
	if want := (repl.Line{Buffer: "echo two", Cursor: 8}); line != want {
		t.Errorf("line back = %+v, want %+v", line, want)
	}
}

// And the line the editor is *given* is the one the widget is holding now,
// not the one the keystroke arrived with.
//
// A widget that rewrites the buffer and then asks for the cursor to be moved
// means the end of what it just wrote. Passing the keystroke's line would make
// every such widget operate on a stale copy, and the two would drift further
// apart with every action in a run of them.
func TestTheEditorIsGivenTheLineTheWidgetIsHoldingNow(t *testing.T) {
	r, out := zleRunner(t, "a() { BUFFER=rewritten; CURSOR=2; zle end-of-line; }\nzle -N a\n")
	ed := &stubEditor{}
	if _, ok, _, _ := runWidgetWatching(t, r, out, "a", repl.Line{Buffer: "old", Cursor: 0}, ed); !ok {
		t.Fatal("the widget did not run")
	}
	if want := []repl.Line{{Buffer: "rewritten", Cursor: 2}}; !slices.Equal(ed.lines, want) {
		t.Errorf("the editor was given %+v, want %+v", ed.lines, want)
	}
}

// An action the editor will not perform from inside a widget is still refused
// out loud, in the wording a letter this shell has not got gets.
//
// There are two of them and they are the two that read a key of their own —
// which two is repl's answer, so this test names the refusal rather than the
// pair. A refusal a script can see beats a call that appears to work.
func TestAnActionTheEditorDeclinesIsRefusedByName(t *testing.T) {
	r, out := zleRunner(t,
		"a() { zle history-incremental-search-backward; print -r -- \"rc=$?\"; }\nzle -N a\n")
	ed := &stubEditor{refuse: map[repl.Widget]bool{repl.WidgetSearchHistoryBackward: true}}
	_, ok, printed, _ := runWidgetWatching(t, r, out, "a", repl.Line{}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := "a:zle: history-incremental-search-backward: " +
		"calling a built-in widget is not implemented yet\nrc=1\n"
	if printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
}

// A widget call with no editor on the other end is status 1 and says nothing.
//
// Reached where a dialect is driven without a session — an embedder with a
// Runner and a line and no editor of repl's — rather than by a script, which
// callWidget turns away earlier with its own wording. Silence, because there
// is no editor to have refused: the same answer a name nothing answers to
// gets.
func TestABuiltInWidgetWithNoEditorIsSilent(t *testing.T) {
	r, out := zleRunner(t, "a() { zle end-of-line; print -r -- \"rc=$?\"; }\nzle -N a\n")
	out.Reset()
	if _, ok := zsh.RunWidget(r, context.Background(), "a", repl.Line{}); !ok {
		t.Fatal("the widget did not run")
	}
	if want := "rc=1\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
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
	got := zsh.KeyBindings(r, repl.KeymapMain)
	if want := (repl.Binding{Function: "mine"}); got["\a"] != want {
		t.Errorf("table = %v, want %v for ^G", got, want)
	}
	// A widget nobody defined is still present and bound to nothing, which is
	// what stops the key doing what it used to — bindkey.go's own rule, and
	// unchanged by this.
	r, _ = zleRunner(t, "bindkey '^G' history-substring-search-up\n")
	if got := zsh.KeyBindings(r, repl.KeymapMain); got["\a"] != (repl.Binding{}) {
		t.Errorf("table = %v, want ^G present and doing nothing", got)
	}
	// And a definition wins over one of the editor's own names, which is what
	// redefining one means and the only order a plugin's wrapper can work in.
	r, _ = zleRunner(t, "f(){:}; zle -N end-of-line f\nbindkey '^G' end-of-line\n")
	if got, want := zsh.KeyBindings(r, repl.KeymapMain)["\a"], (repl.Binding{Function: "end-of-line"}); got != want {
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

// `zle -C` — the completion widget. Measured against zsh 5.9.2 on 2026-09-09
// under `-c` for the definition and the two listings, and through a
// pseudo-terminal with a key bound to one for what only a widget that ran can
// see. Every want below is what that binary wrote.

// TestACompletionWidgetIsDefinedAndSaidBack is the row the issue rests on: the
// line a completion loader writes runs to the end, and the widget is there
// afterwards in both listing spellings.
func TestACompletionWidgetIsDefinedAndSaidBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zle -C mywidget complete-word _main_complete; echo st=$?\nzle -l\nzle -l -L\n")
	want := "st=0\nmywidget -C complete-word _main_complete\n" +
		"zle -C mywidget complete-word _main_complete\n"
	if out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// A completion widget writes all three words in both listings even when the
// function is named identically to the widget — which is exactly the case the
// `-N` spelling abbreviates to a bare name. Measured: `zle -C w complete-word
// w` reads back in full both ways, because the completer in the middle is not
// recoverable from a default.
func TestACompletionWidgetListingNeverAbbreviates(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zle -C w complete-word w\nzle -l\nzle -l -L\n")
	if want := "w -C complete-word w\nzle -C w complete-word w\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// The two kinds sort together into one listing, each in its own spelling.
func TestTheTwoKindsOfWidgetShareOneListing(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"zle -N nw nf; zle -C cw complete-word cf\nzle -l\nzle -l -L\n")
	want := "cw -C complete-word cf\nnw (nf)\n" +
		"zle -C cw complete-word cf\nzle -N nw nf\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheCompleterIsAClosedSetOfCompletionWidgets is what makes the argument
// worth validating rather than storing. Measured by asking zsh for every one
// of the 386 names `zle -la` reports: exactly the eight completion widgets and
// their dotted spellings are accepted, and every other widget — `end-of-line`
// is one, and it is unarguably a widget — answers `invalid widget`, which is a
// different complaint from the `no such widget` that `-D` and `-A` make.
func TestTheCompleterIsAClosedSetOfCompletionWidgets(t *testing.T) {
	for _, name := range []string{
		"complete-word", "delete-char-or-list", "expand-or-complete",
		"expand-or-complete-prefix", "list-choices", "menu-complete",
		"menu-expand-or-complete", "reverse-menu-complete",
		".complete-word", ".list-choices", ".reverse-menu-complete",
	} {
		out, st := runZsh(t, t.TempDir(), "zle -C w "+name+" f; echo st=$?\n")
		if want := "st=0\n"; out != want || st != 0 {
			t.Errorf("%s: output = %q status %d, want %q at 0", name, out, st, want)
		}
	}
	for _, name := range []string{
		"end-of-line", "self-insert", "accept-line", ".self-insert",
		"_main_complete", "nosuchwidget",
	} {
		out, st := runZsh(t, t.TempDir(), "zle -C w "+name+" f\n")
		want := "zsh:zle:1: invalid widget `" + name + "'\n"
		if out != want || st != 1 {
			t.Errorf("%s: output = %q status %d, want %q at 1", name, out, st, want)
		}
	}
}

// `menu-select` is refused, and that is measured rather than left out: it is a
// completion widget only once `zsh/complist` is loaded, which this shell will
// not do, and zsh without that module refuses it here in the same words. The
// completion loader asks `zle -la menu-select` before it tries, so the two
// answers have to agree — a shell whose `-la` denies the name and whose `-C`
// accepts it would be inconsistent with itself.
func TestMenuSelectIsRefusedAsACompleterAndAbsentFromTheListing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -C w menu-select f\n")
	if want := "zsh:zle:1: invalid widget `menu-select'\n"; out != want || st != 1 {
		t.Errorf("output = %q status %d, want %q at 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "zle -la menu-select && echo HAS || echo NOPE\n")
	if want := "NOPE\n"; out != want || st != 0 {
		t.Errorf("listing: output = %q status %d, want %q at 0", out, st, want)
	}
}

// TestTheCompletionLoadersRebindingLoopRuns is the issue's own measurement:
// the loop that redefines each standard completion widget against the
// completion driver, which is the largest single class of complaint a real
// startup produced. All eight lines, no output, and eight widgets afterwards.
func TestTheCompletionLoadersRebindingLoopRuns(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"for w in complete-word delete-char-or-list expand-or-complete "+
			"expand-or-complete-prefix list-choices menu-complete "+
			"menu-expand-or-complete reverse-menu-complete; do\n"+
			"  zle -C $w .$w _main_complete || echo FAIL $w\ndone\n"+
			"echo st=$?\nprint -r -- \"n=${#${(f)\"$(zle -l)\"}}\"\n")
	if want := "st=0\nn=8\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// All three words are required, and unlike `-N` the function may not be left
// to default to the widget's name.
func TestACompletionWidgetTakesExactlyThreeWords(t *testing.T) {
	for _, args := range []string{"", " a", " a b"} {
		out, st := runZsh(t, t.TempDir(), "zle -C"+args+"\n")
		if want := "zsh:zle:1: not enough arguments for -C\n"; out != want || st != 1 {
			t.Errorf("`zle -C%s`: output = %q status %d, want %q at 1", args, out, st, want)
		}
	}
	out, st := runZsh(t, t.TempDir(), "zle -C a complete-word c d\n")
	if want := "zsh:zle:1: too many arguments for -C\n"; out != want || st != 1 {
		t.Errorf("output = %q status %d, want %q at 1", out, st, want)
	}
}

// The function does not have to exist yet, the same as `-N`: the completion
// loader defines every widget it installs against a driver it autoloads
// afterwards, so a shell that required the function here would fail all of
// them.
func TestACompletionWidgetDoesNotNeedItsFunctionYet(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -C w complete-word nosuchfn; echo st=$?\nzle -l\n")
	if want := "st=0\nw -C complete-word nosuchfn\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// A completion widget is a widget everywhere else too: `zle -l name` answers
// for it, `-D` removes it, and removing one from the middle of a table leaves
// the rest intact — which is the encoding's own test, since a widget occupies
// more than one slot of the array it is stored in.
func TestACompletionWidgetIsAWidgetEverywhereElse(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zle -C a complete-word af; zle -C b list-choices bf; zle -N c cf\n"+
			"zle -l b; echo q=$?\nzle -D b; echo d=$?\nzle -l\n")
	if want := "q=0\nd=0\na -C complete-word af\nc (cf)\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q at 0", out, st, want)
	}
}

// `zle -A` of a completion widget gives a copy that is itself one, completer
// and all — measured. A copy that kept only the function would be a widget
// that had quietly become an ordinary one.
func TestAliasingACompletionWidgetCopiesTheCompleter(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zle -C w complete-word f; zle -A w y\nzle -l\n")
	if want := "w -C complete-word f\ny -C complete-word f\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// Redefining across the two kinds replaces the whole definition rather than
// merging it: measured both ways, one widget of the later kind and no trace of
// the earlier one's completer.
func TestRedefiningAcrossTheTwoKindsReplacesTheWhole(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zle -N w f; zle -C w complete-word g\nzle -l\n")
	if want := "w -C complete-word g\n"; out != want {
		t.Errorf("-C over -N: output = %q, want %q", out, want)
	}
	out, _ = runZsh(t, t.TempDir(), "zle -C w complete-word g; zle -N w f\nzle -l\n")
	if want := "w (f)\n"; out != want {
		t.Errorf("-N over -C: output = %q, want %q", out, want)
	}
}

// TestACompletionWidgetRunsItsFunction is the anti-stub: a definition that
// registered a name and nothing else would pass every listing test above and
// fail this one. A key bound to a completion widget calls the function, which
// can see the line and the widget's own name.
func TestACompletionWidgetRunsItsFunction(t *testing.T) {
	r, out := zleRunner(t, "cf() { print -r -- \"ran W=$WIDGET B=[$BUFFER] C=$CURSOR L=[$LBUFFER]\"; }\n"+
		"zle -C cw complete-word cf\n")
	line, ok, printed := runWidget(t, r, out, "cw", repl.Line{Buffer: "abcdef", Cursor: 2})
	if !ok {
		t.Fatal("the completion widget did not run")
	}
	if want := "ran W=cw B=[abcdef] C=2 L=[ab]\n"; printed != want {
		t.Errorf("printed %q, want %q", printed, want)
	}
	if want := (repl.Line{Buffer: "abcdef", Cursor: 2}); line != want {
		t.Errorf("line = %+v, want %+v", line, want)
	}
}

// TestTheLineIsReadOnlyInsideACompletionWidget is the one thing about `zle -C`
// that is visible from inside the call rather than only in a listing, and the
// reason this is not the `-N` path under another letter. Measured through a
// pseudo-terminal: `${(t)BUFFER}` in a completion widget is
// `scalar-local-readonly-special` where the same probe in an ordinary widget
// says `scalar-local-special`, and each of the four assignments answers
// `read-only variable:` and stops the function where it stands.
func TestTheLineIsReadOnlyInsideACompletionWidget(t *testing.T) {
	for _, name := range zleLineParameterNames {
		src := "cf() { " + name + "=zz; print -r -- unreached; }\nzle -C cw complete-word cf\n"
		r, out := zleRunner(t, src)
		line, ok, printed := runWidget(t, r, out, "cw", repl.Line{Buffer: "abcd", Cursor: 2})
		if !ok {
			t.Fatalf("%s: the completion widget did not run", name)
		}
		if !strings.Contains(printed, "read-only variable: "+name) {
			t.Errorf("%s: printed %q, want the assignment refused by name", name, printed)
		}
		if strings.Contains(printed, "unreached") {
			t.Errorf("%s: printed %q, want the widget to have stopped at the refusal", name, printed)
		}
		if want := (repl.Line{Buffer: "abcd", Cursor: 2}); line != want {
			t.Errorf("%s: line = %+v, want %+v — a completion widget does not rewrite it", name, line, want)
		}
	}
}

// zleLineParameterNames is the four a widget reads and writes the line
// through. Spelled out here rather than reached for across the package
// boundary: these tests are an external package, the same way the interp ones
// are, so the names have to be written down on this side.
var zleLineParameterNames = []string{"BUFFER", "CURSOR", "LBUFFER", "RBUFFER"}

// And an ordinary widget is untouched by any of that: the same four are
// writable, which is what makes the read-only mark a property of the
// definition rather than of the shell.
func TestTheLineStaysWritableInsideAnOrdinaryWidget(t *testing.T) {
	r, out := zleRunner(t, "nf() { BUFFER=zz; CURSOR=1; print -r -- \"reached B=[$BUFFER] C=$CURSOR\"; }\n"+
		"zle -N nw nf\n")
	line, ok, printed := runWidget(t, r, out, "nw", repl.Line{Buffer: "abcd", Cursor: 2})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "reached B=[zz] C=1\n"; printed != want {
		t.Errorf("printed %q, want %q", printed, want)
	}
	if want := (repl.Line{Buffer: "zz", Cursor: 1}); line != want {
		t.Errorf("line = %+v, want %+v", line, want)
	}
}

// The read-only mark is lifted with the rest of the call, so an ordinary
// widget that runs after a completion one still edits the line — and a script
// at the next prompt finds four ordinary variables. Marking a name read-only
// with no way to lift it would strand the whole session after one keystroke.
func TestTheReadOnlyLineIsLiftedWhenTheCompletionWidgetReturns(t *testing.T) {
	r, out := zleRunner(t, "cf() { :; }\nnf() { BUFFER=edited; }\n"+
		"zle -C cw complete-word cf\nzle -N nw nf\n")
	if _, ok, _ := runWidget(t, r, out, "cw", repl.Line{Buffer: "abcd", Cursor: 2}); !ok {
		t.Fatal("the completion widget did not run")
	}
	line, ok, _ := runWidget(t, r, out, "nw", repl.Line{Buffer: "abcd", Cursor: 2})
	if !ok {
		t.Fatal("the ordinary widget did not run")
	}
	if want := (repl.Line{Buffer: "edited", Cursor: 2}); line != want {
		t.Errorf("line = %+v, want %+v", line, want)
	}
	res, _ := runZshVars(t, r, "BUFFER=fine; print -r -- \"after=[$BUFFER]\"")
	if want := "after=[fine]\n"; res != want {
		t.Errorf("after the call: %q, want %q", res, want)
	}
}

// `zle .accept-line` inside a widget asks the editor to commit the line, and
// the request is carried back on the line the widget leaves.
//
// The dotted spelling names the **built-in** widget explicitly, past whatever
// a plugin has rebound the bare name to. That is how a wrapper reaches the
// thing it wrapped, and it is not a corner: zsh-users' zsh-autosuggestions
// writes exactly
//
//	_zsh_autosuggest_orig_accept-line() { zle .accept-line }
//
// and rebinds every widget, `accept-line` included, to a wrapper that calls
// it. Without this the editor reads keys, draws them, and commits nothing —
// `exit` included, since that is committed by the same widget, so the session
// cannot even be left (#2082).
func TestAWidgetCanAcceptTheLine(t *testing.T) {
	r, out := zleRunner(t, "w(){ zle .accept-line }; zle -N w")
	line, ok, said := runWidget(t, r, out, "w", repl.Line{Buffer: "echo hi", Cursor: 7})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if said != "" {
		t.Errorf("it said %q, want nothing", said)
	}
	if !line.Accept {
		t.Error("the line came back without the accept, so the editor would not commit it")
	}
	if line.Buffer != "echo hi" {
		t.Errorf("buffer = %q, want it left alone", line.Buffer)
	}
}

// The undotted spelling asks the same thing where nothing has rebound the
// name — which is what a widget calling `zle accept-line` means in a shell
// with no plugins loaded.
func TestTheUndottedAcceptAsksTheSameThing(t *testing.T) {
	r, out := zleRunner(t, "w(){ zle accept-line }; zle -N w")
	line, ok, _ := runWidget(t, r, out, "w", repl.Line{Buffer: "x", Cursor: 1})
	if !ok || !line.Accept {
		t.Errorf("ok=%v accept=%v, want the line accepted", ok, line.Accept)
	}
}

// A widget may rewrite the line and accept it in one call, which is what a
// wrapper that adds something before committing does.
func TestAWidgetMayRewriteAndAccept(t *testing.T) {
	r, out := zleRunner(t, "w(){ BUFFER='rewritten'; zle .accept-line }; zle -N w")
	line, ok, _ := runWidget(t, r, out, "w", repl.Line{Buffer: "typed", Cursor: 5})
	if !ok || !line.Accept || line.Buffer != "rewritten" {
		t.Errorf("line = %+v ok=%v, want %q accepted", line, ok, "rewritten")
	}
}

// The request belongs to the keystroke that made it. A second widget that
// does not ask must not inherit the first one's accept.
func TestAnAcceptDoesNotSurviveIntoTheNextWidget(t *testing.T) {
	r, out := zleRunner(t, "a(){ zle .accept-line }; b(){ :; }; zle -N a; zle -N b")
	if line, _, _ := runWidget(t, r, out, "a", repl.Line{Buffer: "one", Cursor: 3}); !line.Accept {
		t.Fatal("the first widget did not accept")
	}
	if line, _, _ := runWidget(t, r, out, "b", repl.Line{Buffer: "two", Cursor: 3}); line.Accept {
		t.Error("the second widget inherited the accept, so every later keystroke would commit")
	}
}

// The dotted spelling reaches the editor too, and it does not accept.
//
// `zle .end-of-line` is how a wrapper reaches past whatever a plugin rebound
// the bare name to — the spelling zsh-autosuggestions is built on — so it has
// to arrive at the same action the bare name does. And it must not set the
// accept: only the editor's own "commit this line" does that, and a widget
// whose every action committed the line would run one command per keystroke.
func TestTheDottedSpellingReachesTheEditorAndDoesNotAccept(t *testing.T) {
	r, out := zleRunner(t, "w(){ zle .end-of-line }; zle -N w")
	line, ok, said, ed := runWidgetWatching(t, r, out, "w", repl.Line{Buffer: "x", Cursor: 1}, &stubEditor{})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if line.Accept {
		t.Error("an ordinary action asked for the line to be committed")
	}
	if said != "" {
		t.Errorf("it said %q, want nothing", said)
	}
	if want := []repl.Widget{repl.WidgetEndOfLine}; !slices.Equal(ed.performed, want) {
		t.Errorf("performed %v, want %v", ed.performed, want)
	}
}

// `zle -R` from inside a widget is a redraw, and it is given the line as the
// widget has it now.
//
// The screen catches up when the widget returns whether this is called or not,
// so what a widget is asking for is the line on the screen *before* it does
// something slow. The plugin this was filed for draws and then waits up to a
// second for a keystroke; without the draw the person spends that second
// looking at the line as it was.
func TestRedisplayDrawsTheLineTheWidgetHasNow(t *testing.T) {
	r, out := zleRunner(t, "a() { BUFFER=drawn; CURSOR=5; zle -R; print -r -- \"rc=$?\"; }\nzle -N a\n")
	ed := &stubEditor{}
	_, ok, printed, _ := runWidgetWatching(t, r, out, "a", repl.Line{Buffer: "old"}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "rc=0\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if want := []repl.Line{{Buffer: "drawn", Cursor: 5}}; !slices.Equal(ed.drawn, want) {
		t.Errorf("drew %+v, want %+v", ed.drawn, want)
	}
}

// A display string is refused by name: it goes on a status line under the
// prompt, and nothing under repl/ owns one for something other than a search
// or a listing to write on.
func TestRedisplayWithADisplayStringIsRefused(t *testing.T) {
	r, out := zleRunner(t, "a() { zle -R hi; print -r -- \"rc=$?\"; }\nzle -N a\n")
	ed := &stubEditor{}
	_, ok, printed, _ := runWidgetWatching(t, r, out, "a", repl.Line{}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := "a:zle: -R with a display string is not implemented yet\nrc=1\n"
	if printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if len(ed.drawn) != 0 {
		t.Errorf("it drew %+v, want the refusal to have drawn nothing", ed.drawn)
	}
}

// Outside a widget `zle -R` is status 1 and says nothing at all.
//
// Measured 2026-09-12 against zsh 5.9.2, and it is the odd one of three:
// naming an action outside a widget is `widgets can only be called when ZLE is
// active`, `zle -U` outside one is `can only be called from widget function`,
// and this one is silent. A shell that gave all three the same answer would be
// tidier and would not be this shell.
func TestRedisplayOutsideAWidgetIsSilent(t *testing.T) {
	for _, src := range []string{"zle -R\n", "zle -R hi\n"} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != "" || st != 1 {
			t.Errorf("%q = %q status %d, want silence at 1", src, out, st)
		}
	}
}

// `zle -U` puts characters where the editor reads them next.
func TestPushingKeysBackReachesTheEditor(t *testing.T) {
	// The string is built in the body: a widget function is called with no
	// arguments, which is measured and is what TestAWidgetFunctionIsCalledWithNoArguments pins.
	r, out := zleRunner(t, "a() { local k=q; zle -U -- \"${k}x\"; print -r -- \"rc=$?\"; }\nzle -N a\n")
	ed := &stubEditor{}
	_, ok, printed, _ := runWidgetWatching(t, r, out, "a", repl.Line{}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	if want := "rc=0\n"; printed != want {
		t.Errorf("output = %q, want %q", printed, want)
	}
	if want := []string{"qx"}; !slices.Equal(ed.pushed, want) {
		t.Errorf("pushed %v, want %v", ed.pushed, want)
	}
}

// It takes exactly one operand, and the arity is checked before the question
// of whether there is an editor at all.
//
// Measured 2026-09-12 against zsh 5.9.2, in a script as well as in a widget:
// `zle -U` alone is `not enough arguments for -U` and `zle -U x y` is `too
// many arguments for -U`, both at status 1 and both *outside* a widget too —
// where a shell that asked about the widget first would say `can only be
// called from widget function` instead.
//
// Worth pinning because the idiom is written `zle -U -- "$REPLY"`. A shell
// that took the operands as a list would silently push something else where a
// `$REPLY` split into two words.
func TestPushingKeysTakesExactlyOneOperand(t *testing.T) {
	for src, want := range map[string]string{
		"zle -U\n":       "zsh:zle:1: not enough arguments for -U\n",
		"zle -U x y\n":   "zsh:zle:1: too many arguments for -U\n",
		"zle -U x y z\n": "zsh:zle:1: too many arguments for -U\n",
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != want || st != 1 {
			t.Errorf("%q = %q status %d, want %q at 1", src, out, st, want)
		}
	}
}

// And with the right arity but no widget, it says which of the two it is.
func TestPushingKeysOutsideAWidgetSaysSo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "zle -U x\n")
	want := "zsh:zle:1: can only be called from widget function\n"
	if out != want || st != 1 {
		t.Errorf("= %q status %d, want %q at 1", out, st, want)
	}
}

// Neither `-R` nor `-U` may be paired with another operation letter, which is
// the answer every pair of them gets.
//
// The pairing rule was measured across the whole alphabet this builtin has
// before either letter was built — see zleOperationLetters — so this is
// checking that promoting them out of the unimplemented set left them on the
// side the measurement put them.
func TestRedisplayAndPushAreOperationLetters(t *testing.T) {
	const want = "zsh:zle:1: incompatible operation selection options\n"
	for _, src := range []string{"zle -NR w\n", "zle -NU w\n", "zle -RU\n"} {
		out, st := runZsh(t, t.TempDir(), src)
		if out != want || st != 1 {
			t.Errorf("%q = %q status %d, want %q at 1", src, out, st, want)
		}
	}
}
