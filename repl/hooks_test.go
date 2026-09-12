// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The hooks a session runs between commands, against what zsh 5.9.2 was
// measured doing — see hooks.go for the runs these assertions come from.

// hookShell is a session with hooks, whose Runner writes everything to one
// buffer so that the *order* of what ran is what the buffer holds.
func hookShell(t *testing.T, style HookStyle) (Shell, *strings.Builder) {
	t.Helper()
	var out strings.Builder
	r := newTestRunner(nil)
	r.Stdout, r.Stderr = &out, &out
	// The suffix a hook's list is spelled with is the *shell's*, not the
	// session's — a hook that fires inside `cd` reads the same list as one
	// that fires at the prompt — so it is set here on the runner rather than
	// in the table above. See interp.Semantics.HookListSuffix.
	r.Semantics.HookListSuffix = "_functions"
	return Shell{Runner: r, Out: &out, Err: &out, Name: "sh", Hooks: style}, &out
}

func define(t *testing.T, r *interp.Runner, name, body string) {
	t.Helper()
	if !r.DefineFunctionFromText(name, body) {
		t.Fatalf("defining %s", name)
	}
}

// hooksLikeZsh is the shape the one dialect with hooks asks for, spelled out
// here rather than imported: dialect/zsh imports this package, so a test in it
// cannot import the dialect back. What the dialect's own table holds is
// asserted in dialect/zsh, beside the measurement it came from.
//
// It has to keep matching that table, and the unfired list is the half that
// moves: `chpwd` left it in #1775 and `zshexit` in #2111, each on gaining a
// firing site of its own. A copy that still named one of those would have this
// package's tests asserting a notice the shipped shell no longer gives.
func hooksLikeZsh() HookStyle {
	return HookStyle{
		BeforePrompt:  "precmd",
		BeforeCommand: "preexec",
		CommandLayout: syntax.Layout{
			Indent: "\t", Nested: true, Lines: true,
			ThenOnItsOwnLine:           true,
			DoAfterCommandOnItsOwnLine: true,
			DoAfterWordsOnItsOwnLine:   true,
		},
		Unfired: []string{"periodic", "zshaddhistory"},
	}
}

// The named function first, then the list, in the order the list holds — and
// neither is deduplicated against the other or against itself.
func TestAHookRunsItsOwnFunctionAndThenItsList(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "precmd", `echo named`)
	define(t, s.Runner, "one", `echo one`)
	define(t, s.Runner, "two", `echo two`)
	s.Runner.SetArray("precmd_functions", []string{"one", "two", "one", "precmd"})

	s.fireHook(t.Context(), "precmd")

	const want = "named\none\ntwo\none\nnamed\n"
	if got := out.String(); got != want {
		t.Errorf("the chain printed %q, want %q", got, want)
	}
}

// A hook with no list is the named function and nothing else, which is what a
// shell with an empty HookListSuffix asks for.
func TestAHookWithoutAListIsTheNamedFunctionAlone(t *testing.T) {
	s, out := hookShell(t, HookStyle{BeforePrompt: "precmd"})
	s.Runner.Semantics.HookListSuffix = ""
	define(t, s.Runner, "precmd", `echo named`)
	define(t, s.Runner, "one", `echo one`)
	s.Runner.SetArray("precmd_functions", []string{"one"})

	s.fireHook(t.Context(), "precmd")

	if got := out.String(); got != "named\n" {
		t.Errorf("the chain printed %q, want %q", got, "named\n")
	}
}

// Every hook of a chain is told the status of the command before it, and none
// of them can change what the next command reads.
//
// Both halves measured together on 2026-09-07: after `(exit 7)`, a chain whose
// second member returned 3 still showed 7 to the third, and the next line
// typed read `$?` as 7.
func TestAHookIsToldTheStatusAndCannotChangeIt(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "precmd", `echo "named=$?"`)
	define(t, s.Runner, "one", `echo "one=$?"; return 3`)
	define(t, s.Runner, "two", `echo "two=$?"`)
	s.Runner.SetArray("precmd_functions", []string{"one", "two"})
	s.Runner.SetExitStatus(7)

	s.fireHook(t.Context(), "precmd")

	const want = "named=7\none=7\ntwo=7\n"
	if got := out.String(); got != want {
		t.Errorf("the chain saw %q, want %q", got, want)
	}
	if got := s.Runner.ExitStatus(); got != 7 {
		t.Errorf("$? after the chain is %d, want 7", got)
	}
}

// A name with nothing behind it is passed over in silence and stops nothing.
//
// Three shapes of "nothing", because they arrive by three routes: a name never
// defined, a name whose function was removed mid-session, and a name that
// resolves to a builtin. Measured, zsh ran none of them and said nothing about
// any of them, and the rest of the list still ran.
func TestANameThatIsNotAFunctionIsPassedOver(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "gone", `echo gone`)
	s.Runner.RemoveFunction("gone")
	define(t, s.Runner, "last", `echo last`)
	s.Runner.SetArray("precmd_functions", []string{"nosuchfn_zz", "gone", "echo", "last"})

	s.fireHook(t.Context(), "precmd")

	if got := out.String(); got != "last\n" {
		t.Errorf("the chain printed %q, want only %q", got, "last\n")
	}
}

// A hook that fails does not stop the ones after it.
func TestAFailingHookDoesNotStopTheChain(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "one", `echo one; return 3`)
	define(t, s.Runner, "two", `echo two`)
	s.Runner.SetArray("precmd_functions", []string{"one", "two"})

	s.fireHook(t.Context(), "precmd")

	if got := out.String(); got != "one\ntwo\n" {
		t.Errorf("the chain printed %q, want %q", got, "one\ntwo\n")
	}
}

// The three arguments the command hook is given.
//
// `$1` is the line as typed; `$2` is what will run written back on one line;
// `$3` is the same over as many lines as it takes. Measured, zsh 5.9.2:
//
//	typed          $1             $2            $3
//	echo  a   b    echo  a   b    echo a b      echo a b
//	true;false     true;false     true; false   true⏎false
func TestTheCommandHookIsGivenTheLineThreeWays(t *testing.T) {
	for _, c := range []struct{ name, typed, one, many string }{
		{"a simple command keeps its spacing only in the first", "echo  a   b", "echo a b", "echo a b"},
		{"two statements are joined in one and split in the other", "true;false", "true; false", "true\nfalse"},
		{
			"a loop is one line in the second and four in the third", "for i in 1 2; do echo $i; done",
			"for i in 1 2; do echo $i; done", "for i in 1 2\ndo\n\techo $i\ndone",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, out := hookShell(t, hooksLikeZsh())
			define(t, s.Runner, "preexec", `printf '1<%s>\n2<%s>\n3<%s>\nn=%s\n' "$1" "$2" "$3" "$#"`)

			s.fireBeforeCommand(t.Context(), c.typed+"\n", parsed(t, c.typed))

			want := "1<" + c.typed + ">\n2<" + c.one + ">\n3<" + c.many + ">\nn=3\n"
			if got := out.String(); got != want {
				t.Errorf("the hook was told\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// parsed is a typed line as the loops hand it to the hook: one file per line
// of input, in the dialect the hook's arguments were measured in.
func parsed(t *testing.T, text string) []*syntax.File {
	t.Helper()
	p := syntax.NewParser(text+"\n", syntax.Dialect{})
	files, err := collect(p)
	if err != nil {
		t.Fatalf("parsing %q: %v", text, err)
	}
	return files
}

// A hook this session will not fire is named, once, and only once it exists.
//
// The three claims are one rule: silence about a registered hook is the bug
// this file was written for, and a notice repeated at every prompt is a
// session nobody can read.
func TestAnUnfiredHookIsNamedOnce(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	s.hooks = &hookState{reported: map[string]bool{}}

	s.reportUnfiredHooks()
	if got := out.String(); got != "" {
		t.Errorf("a session with no such hook said %q, want nothing", got)
	}

	define(t, s.Runner, "periodic", `echo tick`)
	s.reportUnfiredHooks()
	s.reportUnfiredHooks()

	const want = "sh: periodic: hook not implemented yet\n"
	if got := out.String(); got != want {
		t.Errorf("the session said %q, want %q exactly once", got, want)
	}
}

// A hook registered only through its list counts as registered.
//
// This is the shape `add-zsh-hook` leaves behind — it appends to the array and
// defines no function of the hook's own name — so a check for the named
// function alone would find nothing and say nothing, which is #1281 again one
// level down.
func TestAHookRegisteredOnlyThroughItsListIsNamed(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	s.hooks = &hookState{reported: map[string]bool{}}
	define(t, s.Runner, "on_tick", `echo tick`)
	s.Runner.SetArray("periodic_functions", []string{"on_tick"})

	s.reportUnfiredHooks()

	if got := out.String(); !strings.Contains(got, "periodic: hook not implemented yet") {
		t.Errorf("the session said %q, want it to name periodic", got)
	}
}

// Nothing fires for a dialect that has no hooks, which is three of the four.
func TestADialectWithoutHooksFiresNothing(t *testing.T) {
	s, out := hookShell(t, HookStyle{})
	s.hooks = &hookState{reported: map[string]bool{}}
	define(t, s.Runner, "precmd", `echo named`)
	define(t, s.Runner, "preexec", `echo before`)

	s.fireBeforePrompt(t.Context(), false)
	s.fireBeforeCommand(t.Context(), "true\n", parsed(t, "true"))
	s.reportUnfiredHooks()

	if got := out.String(); got != "" {
		t.Errorf("a session without hooks printed %q, want nothing", got)
	}
}

// A continuation prompt is the middle of a line rather than the boundary
// between two, and fires nothing.
func TestThePromptHookDoesNotFireAtAContinuationPrompt(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "precmd", `echo named`)

	s.fireBeforePrompt(t.Context(), true)

	if got := out.String(); got != "" {
		t.Errorf("a continuation prompt ran %q, want nothing", got)
	}
}

// A hook that panics costs its own call and not the rest of the chain, which
// is the rule a prompt provider already follows.
//
// Reached through a Runner with no functions at all: fireHook asks the Runner
// for the list, and a nil Runner is the one shape that must not take the
// session down either.
func TestAHookOnASessionWithoutARunnerDoesNothing(t *testing.T) {
	s := Shell{Hooks: hooksLikeZsh()}
	s.fireHook(t.Context(), "precmd")
	s.reportUnfiredHooks()
}

// The whole thing, through a real terminal, one keystroke at a time.
//
// The unit tests above call the firing directly, which says what a chain does
// and nothing about whether the loop reaches it — and "registered and never
// fired" is precisely a bug that every direct test would have passed. This is
// the one that would have failed.
//
// The transcript is asserted whole rather than by containment, because the
// claims are about *order* and *count*: the prompt hook before the first
// prompt and once per line after it, the command hook once per line and only
// where something runs, and the status each was told.
func TestTheHooksFireThroughATerminal(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Hooks = hooksLikeZsh()
		define(t, sh.Runner, "precmd", `echo "precmd $?"`)
		define(t, sh.Runner, "preexec", `echo "preexec <$1>"`)
	})
	s.typeLine("echo one\n")
	waitFor(t, s.ran, "one\n", "the command's output")
	s.typeLine("(exit 7)\n")
	s.typeLine("echo st=$?\n")
	waitFor(t, s.ran, "st=7", "the status the line read")
	s.end()

	want := strings.Join([]string{
		"precmd 0",             // before the first prompt, with nothing run yet
		"preexec <echo one>",   // after the line was read and before it ran
		"one",                  //
		"precmd 0",             // before the second prompt
		"preexec <(exit 7)>",   //
		"precmd 7",             // told what the line before it left
		"preexec <echo st=$?>", // and told the same
		"st=7",                 // which no hook of either chain disturbed
		"precmd 0",             // before the prompt ^D was typed at
		"",
	}, "\n")
	if got := s.ran.String(); got != want {
		t.Errorf("the session ran\n%q\nwant\n%q", got, want)
	}
}

// An empty line draws a prompt and runs no command hook.
//
// Measured: zsh fires `precmd` for a bare newline and `preexec` for nothing at
// all, which is the same rule as a line the parser refused — the command hook
// announces a command, and there is not one.
func TestTheCommandHookDoesNotFireForALineWithNothingInIt(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "preexec", `echo before`)

	if s.runStmts(t.Context(), "\n", nil) {
		t.Error("an empty line ended the session")
	}

	if got := out.String(); got != "" {
		t.Errorf("an empty line ran %q, want nothing", got)
	}
}

// A whole session names the hooks it will not fire.
//
// The unit test above drives reportUnfiredHooks directly, which says what the
// notice is and nothing about whether a session ever asks for it — the same
// gap "registered and never fired" lived in.
func TestASessionNamesTheHooksItWillNotFire(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Hooks = hooksLikeZsh()
		sh.Runner.Semantics.HookListSuffix = "_functions"
		define(t, sh.Runner, "on_tick", `echo tick`)
		sh.Runner.SetArray("periodic_functions", []string{"on_tick"})
	})
	s.typeLine("true\n")
	s.end()

	const want = "sh: periodic: hook not implemented yet"
	switch got := strings.Count(s.errs.String(), want); got {
	case 1:
	case 0:
		t.Errorf("the session said %q, want it to contain %q", s.errs.String(), want)
	default:
		t.Errorf("the session said it %d times, want once", got)
	}
}

// A session with no terminal to hand back runs what it is given, and does not
// try to change a terminal it has not got.
//
// The second half is the one worth asserting: a shell reading a pipe has no
// descriptor to put in or out of raw mode, and reaching for one there would
// complain to the person once per prompt about an ioctl on nothing.
func TestWithoutATerminalTheWorkStillRuns(t *testing.T) {
	s, out := hookShell(t, HookStyle{})
	ran := false
	s.inLineDiscipline(nil, func() { ran = true })
	if !ran {
		t.Error("the work did not run")
	}
	if got := out.String(); got != "" {
		t.Errorf("a session with no terminal said %q, want nothing", got)
	}
}

// The job notices come first, then the hooks, then the prompt.
//
// Measured on 2026-09-07: with a background job finishing during the previous
// command, the screen held `[1] + done sleep 0.3`, then the hook's output,
// then the prompt. One buffer for both streams here, because the claim is the
// order rather than the contents.
func TestTheJobNoticesComeBeforeThePromptHook(t *testing.T) {
	var out strings.Builder
	r := runnerWithAFinishedJob(t)
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, Out: &out, Err: &out, Name: "sh",
		Hooks: hooksLikeZsh(), hooks: &hookState{reported: map[string]bool{}},
	}
	define(t, r, "precmd", `echo "[hook]"`)

	var pending strings.Builder
	drawn := s.beforeReading(t.Context(), nil, &pending)

	notice := strings.Index(out.String(), "Done")
	hook := strings.Index(out.String(), "[hook]")
	if notice < 0 || hook < 0 {
		t.Fatalf("the session drew %q, want a job notice and a hook", out.String())
	}
	if notice > hook {
		t.Errorf("the hook ran before the job notice: %q", out.String())
	}
	// And the prompt after both, which is what returning it here means: it is
	// drawn by the caller, so nothing it contains can be in the buffer yet.
	if strings.Contains(out.String(), drawn.text) && drawn.text != "" {
		t.Errorf("the prompt was drawn before the hook: %q", out.String())
	}
}

// A hook that ends the session ends it, and the line it announced does not run.
func TestACommandHookThatExitsEndsTheLine(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "preexec", `echo before; exit 3`)
	define(t, s.Runner, "after", `echo after`)

	if !s.runStmts(t.Context(), "after\n", parsed(t, "after")) {
		t.Error("the session did not end")
	}
	if got := out.String(); got != "before\n" {
		t.Errorf("the session ran %q, want the hook alone", got)
	}
}

// A Shell assembled by hand rather than by Run has neither of the two pointers
// the loops set, and must not take the session down over it.
func TestAHookOnAShellWithoutItsSessionStateDoesNothing(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "periodic", `echo tick`)

	s.reportUnfiredHooks()

	if got := out.String(); got != "" {
		t.Errorf("a shell with no session state said %q, want nothing", got)
	}
}

// What a hook prints reaches the terminal and is not recorded as the output of
// any command.
//
// Two things are being asserted at once and they are the same call. A hook
// prints through the Runner's streams, which under a block store are the far
// end of a conduit a goroutine forwards; the prompt is written to the terminal
// directly. So the wait is what keeps the two in the order they were written —
// measured, without it the prompt was drawn above the `precmd` output it was
// meant to follow. And the capture is emptied rather than read, because a
// `precmd` banner is not the output of the command typed after it.
func TestWhatAHookPrintsIsNotACommandsOutput(t *testing.T) {
	for _, c := range []struct {
		name string
		fire func(t *testing.T, s Shell)
	}{
		{"before the prompt", func(t *testing.T, s Shell) {
			define(t, s.Runner, "precmd", `echo "[hook]"`)
			var pending strings.Builder
			s.beforeReading(t.Context(), nil, &pending)
		}},
		{"before the command", func(t *testing.T, s Shell) {
			define(t, s.Runner, "preexec", `echo "[hook]"`)
			s.runStmts(t.Context(), "true\n", parsed(t, "true"))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, out := hookShell(t, hooksLikeZsh())
			s.hooks = &hookState{reported: map[string]bool{}}
			kept := blocks.NewCapture(4096)
			s.capture = &outputCapture{cap: kept}
			// The Runner's stream through the capture, as a session's is.
			s.Runner.Stdout, s.Runner.Stderr = kept.Stream(out), kept.Stream(out)

			c.fire(t, s)

			if text, _, _ := kept.Take(); text != "" {
				t.Errorf("the capture kept %q of what the hook printed", text)
			}
			if got := out.String(); !strings.Contains(got, "[hook]") {
				t.Errorf("the hook printed %q, want it to reach the terminal", got)
			}
		})
	}
}

// A prompt hook that ends the session ends it before the prompt is written,
// and nothing more is read.
//
// Driven through the loop that has no editor, because it is the one that had
// the prompt already written when this was found — the two loops must not
// disagree about it, which is the standing hazard beforeReading documents.
func TestAPromptHookThatExitsDrawsNoPrompt(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("echo unreachable\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeZsh(),
	}
	define(t, r, "precmd", `echo bye; exit 3`)

	status, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	if status != 3 {
		t.Errorf("the session ended with %d, want 3", status)
	}
	if errs.String() != "" {
		t.Errorf("a prompt was drawn: %q", errs.String())
	}
	if got := out.String(); got != "bye\n" {
		t.Errorf("the session ran %q, want the hook alone", got)
	}
}

// The neighboring mechanism: a variable whose *text* is evaluated before every
// prompt, against what bash 5.3.15 was measured doing — see hooks.go and
// dialect/bash's HookStyle for the runs these assertions come from.

// hooksLikeBash is the shape the one dialect with an evaluated hook asks for,
// spelled out here for the reason hooksLikeZsh is: dialect/bash imports this
// package, so a test in it cannot import the dialect back. What the dialect's
// own table holds is asserted in dialect/bash, beside the measurement.
func hooksLikeBash() HookStyle {
	return HookStyle{BeforePromptVariable: "PROMPT_COMMAND"}
}

// A scalar is one command text, and an array is one per element in order.
func TestThePromptVariableRunsItsTextAndEveryElementOfAList(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(r *interp.Runner)
		want string
	}{
		{"a scalar is one text", func(r *interp.Runner) {
			r.SetVar("PROMPT_COMMAND", "echo A; echo B")
		}, "A\nB\n"},
		{"an array is one text per element", func(r *interp.Runner) {
			r.SetArray("PROMPT_COMMAND", []string{"echo A", "echo B", "echo C"})
		}, "A\nB\nC\n"},
		{"an empty element runs nothing and stops nothing", func(r *interp.Runner) {
			r.SetArray("PROMPT_COMMAND", []string{"echo A", "", "   ", "echo B"})
		}, "A\nB\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, out := hookShell(t, hooksLikeBash())
			c.set(s.Runner)

			s.fireBeforePrompt(t.Context(), false)

			if got := out.String(); got != c.want {
				t.Errorf("the chain printed %q, want %q", got, c.want)
			}
		})
	}
}

// Every element is told the status of the line before the prompt, and none of
// them can change what the next command reads.
//
// Measured: after `(exit 5)`, an array whose second element ran `false` still
// showed the third element 5, and the next line typed read `$?` as 5.
func TestThePromptVariableIsToldTheStatusAndCannotChangeIt(t *testing.T) {
	s, out := hookShell(t, hooksLikeBash())
	s.Runner.SetArray("PROMPT_COMMAND", []string{
		`echo "one=$?"`, `echo "two=$?"; false`, `echo "three=$?"`,
	})
	s.Runner.SetExitStatus(5)

	s.fireBeforePrompt(t.Context(), false)

	const want = "one=5\ntwo=5\nthree=5\n"
	if got := out.String(); got != want {
		t.Errorf("the chain saw %q, want %q", got, want)
	}
	if got := s.Runner.ExitStatus(); got != 5 {
		t.Errorf("$? after the chain is %d, want 5", got)
	}
}

// The chain is the value as the prompt found it.
//
// Measured: an element that assigned a whole new array to the name did not
// change what the rest of *that* chain ran, and the new value took effect at
// the next prompt. It is the half of "read the name every time" that a
// snapshot could get wrong in either direction — re-reading mid-chain would
// run the new list here, and reading once per session would never run it.
func TestThePromptVariableChainIsTheValueThePromptFound(t *testing.T) {
	s, out := hookShell(t, hooksLikeBash())
	s.Runner.SetArray("PROMPT_COMMAND", []string{
		`echo old-one; PROMPT_COMMAND=(new-two)`, `echo old-two`,
	})
	define(t, s.Runner, "new-two", `echo new-two`)

	s.fireBeforePrompt(t.Context(), false)
	s.fireBeforePrompt(t.Context(), false)

	const want = "old-one\nold-two\nnew-two\n"
	if got := out.String(); got != want {
		t.Errorf("the two prompts ran %q, want %q", got, want)
	}
}

// Unset is silent and runs nothing, and so is a dialect that has no such
// variable at all with one set.
func TestAPromptVariableWithNothingBehindItRunsNothing(t *testing.T) {
	for _, c := range []struct {
		name  string
		style HookStyle
		set   func(r *interp.Runner)
	}{
		{"unset", hooksLikeBash(), func(*interp.Runner) {}},
		{"empty", hooksLikeBash(), func(r *interp.Runner) { r.SetVar("PROMPT_COMMAND", "") }},
		{"a dialect without one", HookStyle{}, func(r *interp.Runner) {
			r.SetVar("PROMPT_COMMAND", "echo ran")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, out := hookShell(t, c.style)
			c.set(s.Runner)

			s.fireBeforePrompt(t.Context(), false)

			if got := out.String(); got != "" {
				t.Errorf("the chain printed %q, want nothing", got)
			}
		})
	}
}

// Text that will not parse is reported against the variable's name, the
// variable is left set, and the session carries on.
//
// The name is the whole point of the assertion. Measured, bash says
// `PROMPT_COMMAND: line N: syntax error …`; a shell that said `eval` there
// would send the person looking for a builtin they never ran.
func TestTextThePromptVariableCannotParseIsReportedAgainstItsName(t *testing.T) {
	// A runner whose dialect does *not* make a parse failure inside a special
	// builtin fatal, because that axis decides whether the rest of the chain
	// runs and bash answers No to it. The default here is POSIX, which
	// answers Yes and would end the session — a true answer about a shell
	// this test is not about.
	s, out := hookShell(t, hooksLikeBash())
	sem := interp.PosixSemantics()
	sem.BuiltinSyntaxErrorFatal = interp.No
	s.Runner.Semantics = &sem
	s.Runner.SetArray("PROMPT_COMMAND", []string{"echo before", "if", "echo after"})

	s.fireBeforePrompt(t.Context(), false)

	got := out.String()
	if !strings.Contains(got, "PROMPT_COMMAND:") {
		t.Errorf("the failure was reported as %q, want the variable named in it", got)
	}
	if strings.Contains(got, "eval") {
		t.Errorf("the failure was reported as %q, want no mention of a builtin", got)
	}
	// And it stops nothing: the element after the bad one still ran.
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("the chain printed %q, want both of the elements that parse", got)
	}
	// Fired again, it complains again, because the variable is still set.
	if s.fireBeforePrompt(t.Context(), false); strings.Count(out.String(), "PROMPT_COMMAND:") < 2 {
		t.Errorf("the second prompt said %q, want the same complaint again", out.String())
	}
}

// A continuation prompt is the middle of a line, and evaluates nothing.
func TestThePromptVariableDoesNotFireAtAContinuationPrompt(t *testing.T) {
	s, out := hookShell(t, hooksLikeBash())
	s.Runner.SetVar("PROMPT_COMMAND", "echo ran")

	s.fireBeforePrompt(t.Context(), true)

	if got := out.String(); got != "" {
		t.Errorf("a continuation prompt ran %q, want nothing", got)
	}
}

// A shell assembled by hand with no Runner must not take the session down.
func TestThePromptVariableOnASessionWithoutARunnerDoesNothing(t *testing.T) {
	s := Shell{Hooks: hooksLikeBash()}
	s.fireBeforePrompt(t.Context(), false)
}

// The whole thing, through a real terminal, one keystroke at a time.
//
// The unit tests above call the firing directly, which says what a chain does
// and nothing about whether the loop reaches it — and #1458 was exactly that:
// the variable was accepted, kept, and never read, at status 0 and in silence.
// This is the test that would have failed.
//
// The transcript is asserted whole rather than by containment, because the
// claims are about order and count: before the first prompt with nothing run
// yet, once per line after, including the empty one, and told each line's
// status without reaching the next.
func TestThePromptVariableFiresThroughATerminal(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Hooks = hooksLikeBash()
		sh.Runner.SetArray("PROMPT_COMMAND", []string{`echo "pc $?"`, `echo "pc2"`})
	})
	s.typeLine("echo one\n")
	waitFor(t, s.ran, "one\n", "the command's output")
	s.typeLine("(exit 7)\n")
	s.typeLine("echo st=$?\n")
	waitFor(t, s.ran, "st=7", "the status the line read")
	s.end()

	want := strings.Join([]string{
		"pc 0", "pc2", // before the first prompt, with nothing run yet
		"one",         //
		"pc 0", "pc2", // before the second prompt
		"pc 7", "pc2", // told what the line before it left
		"st=7",        // which no element of the chain disturbed
		"pc 0", "pc2", // before the prompt ^D was typed at
		"",
	}, "\n")
	if got := s.ran.String(); got != want {
		t.Errorf("the session ran\n%q\nwant\n%q", got, want)
	}
}

// An element that ends the session ends it there: the rest of that element
// does not run, the rest of the chain does not run, and no prompt is drawn.
//
// Measured: `PROMPT_COMMAND='echo A; exit 3; echo NOTREACHED'` printed `A` and
// the shell was gone with status 3.
func TestAPromptVariableThatExitsDrawsNoPrompt(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("echo unreachable\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeBash(),
	}
	r.SetArray("PROMPT_COMMAND", []string{"echo bye; exit 3; echo NOTREACHED", "echo never"})

	status, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	if status != 3 {
		t.Errorf("the session ended with %d, want 3", status)
	}
	if errs.String() != "" {
		t.Errorf("a prompt was drawn: %q", errs.String())
	}
	if got := out.String(); got != "bye\n" {
		t.Errorf("the session ran %q, want the first element up to the exit", got)
	}
}

// A hook item that raises an interpreter bug costs its own item and not the
// rest of the chain, and never the session.
//
// The reason to guard at all is the reason a typed line is guarded: the
// process *is* the session, and a session open for hours must not end over a
// hook somebody wrote for decoration. Both chains are asserted because both
// run through the same interp.Runner.FireChain, and the guard is what this
// package wraps each item in on the way past — a guard that covered one of
// them would be exactly the shape of bug that helper exists to prevent.
func TestABugInOneHookItemCostsThatItemAlone(t *testing.T) {
	for _, c := range []struct {
		name  string
		style HookStyle
		set   func(t *testing.T, s Shell)
	}{
		{"the evaluated chain", hooksLikeBash(), func(_ *testing.T, s Shell) {
			s.Runner.SetArray("PROMPT_COMMAND", []string{"boom", "echo after"})
		}},
		{"the function chain", hooksLikeZsh(), func(t *testing.T, s Shell) {
			define(t, s.Runner, "one", `boom`)
			define(t, s.Runner, "two", `echo after`)
			s.Runner.SetArray("precmd_functions", []string{"one", "two"})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			r := newTestRunner(nil)
			r.Semantics.HookListSuffix = "_functions"
			r.Register("boom", func(*interp.Runner, context.Context, []string) int {
				panic("an invariant broke")
			})
			r.Stdout, r.Stderr = &out, &out
			s := Shell{Runner: r, Out: &out, Err: &errs, Name: "testsh", Hooks: c.style}
			c.set(t, s)

			s.fireBeforePrompt(t.Context(), false)

			if !strings.Contains(errs.String(), "testsh: internal error: an invariant broke") {
				t.Errorf("the bug was reported as %q, want it named as this shell's", errs.String())
			}
			if got := out.String(); !strings.Contains(got, "after") {
				t.Errorf("the chain printed %q, want the item after the bug to have run", got)
			}
		})
	}
}

// An error a hook raised costs the chain and not the session.
//
// The measurement is in interp's giveUpTheHook: zsh 5.9.2 through a
// pseudo-terminal, a `precmd` that raises each fatal expansion in turn, and in
// every case the diagnostic, the prompt, and an answer to the next line typed.
// Asserted on both chains because both run through interp.Runner.FireChain —
// bash 5.3.15 answers a failing `PROMPT_COMMAND` the same way — and asserted
// through Run rather than through fireBeforePrompt, because what was broken
// was the session ending and only the loop can say whether it did.
func TestAHookThatFailsCostsTheChainAndNotTheSession(t *testing.T) {
	for _, c := range []struct {
		name  string
		style HookStyle
		set   func(t *testing.T, s Shell)
	}{
		{"the evaluated chain", hooksLikeBash(), func(_ *testing.T, s Shell) {
			s.Runner.SetArray("PROMPT_COMMAND",
				[]string{"echo one", `echo two; echo ${NOPE?gone}; echo unreachable`, "echo three"})
		}},
		{"the function chain", hooksLikeZsh(), func(t *testing.T, s Shell) {
			define(t, s.Runner, "one", `echo one`)
			define(t, s.Runner, "two", `echo two; echo ${NOPE?gone}; echo unreachable`)
			define(t, s.Runner, "three", `echo three`)
			s.Runner.SetArray("precmd_functions", []string{"one", "two", "three"})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			r := newTestRunner(map[string]string{"PS1": "RDY> "})
			r.Semantics.HookListSuffix = "_functions"
			r.Interactive = true
			r.Stdout, r.Stderr = &out, &out
			s := Shell{
				Runner: r, In: strings.NewReader("echo typed\n"),
				Out: &out, Err: &errs, Name: "sh", Hooks: c.style,
			}
			c.set(t, s)

			status, err := s.Run(t.Context())
			if err != nil {
				t.Fatalf("running: %v", err)
			}

			if status != 0 {
				t.Errorf("the session ended with %d, want the typed line's 0", status)
			}
			if got := out.String(); !strings.Contains(got, "typed\n") {
				t.Errorf("the session ran %q, want the line after the failing hook to have run", got)
			}
			if got := out.String(); strings.Contains(got, "unreachable") {
				t.Errorf("the session ran %q, want the failing item to have stopped there", got)
			}
			if got := out.String(); strings.Contains(got, "three") {
				t.Errorf("the session ran %q, want nothing after the failing item", got)
			}
			if !strings.Contains(errs.String(), "RDY> ") {
				t.Errorf("the prompts drawn were %q, want one after the failing hook", errs.String())
			}
		})
	}
}

// And the status the chain saved is put back, so a failing hook does not reach
// the next command. Measured: `(exit 7)`, a `precmd` that fails, then `echo
// $?` says 7 at a zsh 5.9.2 prompt.
func TestAHookThatFailsLeavesTheStatusTheChainSaved(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Semantics.HookListSuffix = "_functions"
	r.Interactive = true
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("(exit 7)\necho status=$?\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeZsh(),
	}
	define(t, s.Runner, "precmd", `echo ${NOPE?gone}`)

	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("running: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "status=7\n") {
		t.Errorf("the session printed %q, want status=7", got)
	}
}

// A hook that *exits* still ends the session, which is the half that keeps
// this from making a session nobody can leave: measured, `exit 7` in a
// `precmd` ends a zsh 5.9.2 session with 7 and draws no prompt.
func TestAPromptHookThatExitsStillEndsAnInteractiveSession(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Semantics.HookListSuffix = "_functions"
	r.Interactive = true
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("echo unreachable\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeZsh(),
	}
	define(t, s.Runner, "precmd", `echo bye; exit 3`)

	status, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("running: %v", err)
	}

	if status != 3 {
		t.Errorf("the session ended with %d, want 3", status)
	}
	if got := out.String(); got != "bye\n" {
		t.Errorf("the session ran %q, want the hook alone", got)
	}
}

// The command hook is the same boundary: the error costs the chain, and the
// line the hook fired for still runs. Measured, a `preexec` that fails prints
// its diagnostic and zsh 5.9.2 runs the line anyway.
func TestACommandHookThatFailsStillRunsTheLine(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	s.Runner.Interactive = true
	define(t, s.Runner, "preexec", `echo before; echo ${NOPE?gone}; echo unreachable`)
	define(t, s.Runner, "after", `echo after`)

	if s.runStmts(t.Context(), "after\n", parsed(t, "after")) {
		t.Error("the session ended over a failing command hook")
	}
	if got := out.String(); !strings.Contains(got, "after\n") {
		t.Errorf("the session ran %q, want the line to have run", got)
	}
	if got := out.String(); strings.Contains(got, "unreachable") {
		t.Errorf("the session ran %q, want the failing hook to have stopped there", got)
	}
}
