// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedHolderTakesANumberOfItsOwn is the nesting half of
// interp.Semantics.ACommandHoldsAJobSlot: inside a command holding a number,
// `if`, the loops, `case`, `repeat`, `time`, `eval` and `.` each take another,
// where a brace group and a function call do not, and a nameless function
// holds nothing. Every row measured 2026-10-02 on zsh 5.9.2 under `-f -c`.
func TestANestedHolderTakesANumberOfItsOwn(t *testing.T) {
	const j = `/bin/sleep 0.3 & jobs`
	for _, c := range []struct{ src, want string }{
		{"f() { { " + j + " } }; f", "[2]"},
		{"g() { " + j + " }; f() { g }; f", "[2]"},
		{"f() { if true; then " + j + "; fi }; f", "[3]"},
		{"f() { eval '" + j + "' }; f", "[3]"},
		{"{ eval '" + j + "' }", "[3]"},
		{"eval 'eval \"" + j + "\"'", "[3]"},
		{"{ if true; then " + j + "; fi }", "[3]"},
		{"if true; then { " + j + " }; fi", "[2]"},
		{"for i in 1; do for j in 1; do " + j + "; done; done", "[3]"},
		{"f() { while true; do " + j + "; break; done }; f", "[3]"},
		{"f() { case a in a) " + j + ";; esac }; f", "[3]"},
		{"f() { repeat 1 { " + j + " } }; f", "[3]"},
		{"f() { ! { " + j + " } }; f", "[2]"},
		{"f() { if true; then if true; then eval '" + j + "'; fi; fi }; f", "[5]"},
		{"() { " + j + " }", "[1]"},
		{"() { if true; then " + j + "; fi }", "[2]"},
		{"() { { " + j + " } }", "[2]"},
		{"f() { () { " + j + " } }; f", "[2]"},
		// A subshell never moves the marker, so its job is listed unmarked.
		{"( eval '" + j + "' )", "[3]  "},
		{"( if true; then " + j + "; fi )", "[3]  "},
		{"( { " + j + " } )", "[2]  "},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"; wait\n")
		want := c.want + "  + running    /bin/sleep 0.3\n"
		if len(c.want) > 3 {
			want = c.want + "  running    /bin/sleep 0.3\n"
		}
		if out != want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, want)
		}
	}
}

// TestAPlusWithNowhereToGoStaysOnTheNumber is the release half: when a
// command ends with the `+` on its number and no job or held number to hand
// it to, it stays there, and only reading the table outside a command takes
// it off. Same shell, same day; `f` runs `sleep 0 & wait`, and `g` is
// `wait %%; wait %-`.
func TestAPlusWithNowhereToGoStaysOnTheNumber(t *testing.T) {
	const f = `f() { /bin/sleep 0 & wait }; g() { wait %%; wait %- }; f; `
	stays := "g:wait: %%: no such job\ng:wait: no previous job\n"
	gone := "g:wait: no current job\ng:wait: no previous job\n"
	for _, c := range []struct{ src, want string }{
		{f + "g", stays},
		{f + "true; g", stays},
		{f + ": ; g", stays},
		{f + "x=1; g", stays},
		{f + "{ true }; g", stays},
		{f + "h() { : }; h; g", stays},
		{f + "jobs; g", gone},
		{f + "wait; g", gone},
		{f + "kill -0 $$; g", gone},
		{f + "wait %%", "zsh:wait:1: no current job\n"},
		{f + "{ wait %% }", "zsh:wait:1: %%: no such job\n"},
		{f + "eval 'wait %%'", "(eval):wait:1: %%: no such job\n"},
		{f + "wait %% | /bin/cat", "zsh:wait:1: no current job\n"},
		{f + "(wait %%); echo s=$?", "s=0\n"},
		// The `-` is on the eval's number, and f's is passed over.
		{
			`f() { eval '/bin/sleep 0 & wait' }; f; h() { eval 'wait %%; wait %-' }; h`,
			"(eval):wait:1: %%: no such job\n(eval):wait:1: %-: no such job\n",
		},
		// Control: a `-` that is a job takes the `+`.
		{
			`/bin/sleep 0.3 & f() { /bin/sleep 0 & wait $! }; f; jobs %%; jobs %-; wait`,
			"[1]  + running    /bin/sleep 0.3\nzsh:jobs:1: no previous job\n",
		},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestAWaitThatReachedTheSubshellTakesItAway: in a `( … )`, which is a job of
// its own, a wait that reached it leaves no job for a spec to name. Same
// shell, same day.
func TestAWaitThatReachedTheSubshellTakesItAway(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"(wait %1; echo a=$?; wait %1; echo b=$?); :", "a=0\nzsh:wait:1: %1: no such job\nb=127\n"},
		{"(wait %1; echo a=$?; jobs %1; echo b=$?); :", "a=0\nzsh:jobs:1: %1: no such job\nb=127\n"},
		{"(wait %1; echo a=$?); (wait %1; echo b=$?); :", "a=0\nb=0\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestAJobSpecMissIsSilentUnderPosixBuiltins is
// interp.Semantics.JobSpecMissIsSilent, which zsh's POSIX_BUILTINS moves.
// Same shell, same day.
func TestAJobSpecMissIsSilentUnderPosixBuiltins(t *testing.T) {
	const body = `wait 1; echo $?; wait %1; echo $?; wait %%; echo $?; wait %-; echo $?; ` +
		`wait %foo; echo $?; jobs %1; echo $?; kill %1; echo $?; disown %1; echo $?`
	out, _ := runZsh(t, t.TempDir(), "setopt posixbuiltins; "+body+"\n")
	if want := "127\n127\n127\n127\n127\n127\n1\n127\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	out, _ = runZsh(t, t.TempDir(), "emulate sh; wait %1; echo $?\n")
	if want := "127\n"; out != want {
		t.Errorf("emulate sh: got %q, want %q", out, want)
	}
	// Control: without the option each one says so.
	out, _ = runZsh(t, t.TempDir(), "wait %1; echo $?; jobs %1; echo $?\n")
	if want := "zsh:wait:1: %1: no such job\n127\nzsh:jobs:1: %1: no such job\n127\n"; out != want {
		t.Errorf("control: got %q, want %q", out, want)
	}
}

// TestArithmeticReadsTheSpecialParameters is
// syntax.Dialect.ArithSpecialParameterOperands, and `repeat` reading the
// last status in its count — zsh's own A05execution asks the second (#5140).
// Same shell, same day.
func TestArithmeticReadsTheSpecialParameters(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"?", "3"},
		{"? + 1", "4"},
		{"1 ? ? : 2", "3"},
		{"- ?", "-3"},
		{"0 + ?", "3"},
		{"#", "2"},
		{"# + 1", "3"},
		{"#x", "0"},
		{"##x", "120"},
		{"$ > 0", "1"},
		{"?++", "zsh:1: bad math expression: lvalue required"},
		{"? = 5", "zsh:1: bad math expression: lvalue required"},
	} {
		out, _ := runZsh(t, t.TempDir(), "set -- a b; (exit 3); print -r -- $(( "+c.expr+" ))\n")
		if out != c.want+"\n" {
			t.Errorf("$(( %s )): got %q, want %q", c.expr, out, c.want+"\n")
		}
	}
	out, _ := runZsh(t, t.TempDir(), `(exit 3); repeat "$?" echo x; (exit 2); repeat '?' echo y; (exit 5); repeat 0 echo q; echo s=$?`+"\n")
	if want := "x\nx\nx\ny\ny\ns=0\n"; out != want {
		t.Errorf("repeat: got %q, want %q", out, want)
	}
}
