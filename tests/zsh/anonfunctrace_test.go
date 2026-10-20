// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A nameless function's **call** is traced, the way a named one's is (#5075).
//
// A named function's call is a simple command, so the ordinary trace line
// covers it. A nameless one is a compound command with no word of its own and
// wrote nothing at all here: the body's lines appeared under a frame the
// script never saw entered.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), which `go version -m` reports as *not a Go
// executable*, so the reference is that shell and not another build of this
// one. Every `want` below is its own bytes, from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME and standard input on the
// null device, with the script's own name rewritten to the one this harness
// gives it.
func TestANamelessFunctionsCallIsTraced(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			"setopt xtrace\n() { print A }\n",
			"+zsh:2> '(anon)'\n+(anon):0> print A\nA\n",
		},
		{
			"setopt xtrace\n() { print A } a b\n",
			"+zsh:2> '(anon)' a b\n+(anon):0> print A\nA\n",
		},
		// The words are quoted the way a command's are, which is the row
		// that says they go through the trace's own quoting rather than
		// being joined.
		{
			"setopt xtrace\n() { print A } \"a b\"\n",
			"+zsh:2> '(anon)' 'a b'\n+(anon):0> print A\nA\n",
		},
		// A body with nothing in it still made a call, and the line is the
		// only thing that says so.
		{"setopt xtrace\n() { }\n", "+zsh:2> '(anon)'\n"},
		// The other spelling of the same construct.
		{
			"setopt xtrace\nfunction { print A } x y\n",
			"+zsh:2> '(anon)' x y\n+(anon):0> print A\nA\n",
		},
		// The control: with the option off there is no line at all, so what
		// is under test is a *trace* and not something written always.
		{"() { print A }\n", "A\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// The line is written in the **caller's** frame, before the call's own is
// pushed.
//
// This is what the prefix says and it is the half a grid of top-level calls
// cannot grade: every row above stands in the script, where the file's name
// and the call's line are what `%N:%i` would draw either way. These two hold
// the call fixed and move what encloses it, so the name in the prefix is the
// only thing that can answer.
func TestTheTracedCallIsDrawnInTheFrameThatMadeIt(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			"f() { () { print A } }\nsetopt xtrace\nf\n",
			"+zsh:3> f\n+f:0> '(anon)'\n+(anon):0> print A\nA\n",
		},
		// And one nameless function inside another: the inner call's line
		// carries the outer one's invented name.
		{
			"setopt xtrace\n() { () { print i } }\n",
			"+zsh:2> '(anon)'\n+(anon):0> '(anon)'\n+(anon):0> print i\ni\n",
		},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// A redirection after the **brace** body is the call's, not the body's.
//
// The parser read it as the body group's, because a `{ … }` standing as a
// command takes its own trailing redirections and the body here is parsed by
// that same reader. The difference is only visible from outside the body, and
// the trace line above is what made it visible: the call's line is written
// before the frame is pushed, so a redirection that belongs to the *call*
// catches it and one that belongs to the *body* does not.
//
// `PS4` is set to something without `%N` in it so that these rows grade where
// the bytes went and nothing else.
func TestTheRedirectionAfterABraceBodyBelongsToTheCall(t *testing.T) {
	const pre = "PS4='@ '\nsetopt xtrace\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"the call's own line lands in the file",
			pre + "() { print A } 2>e\nunsetopt xtrace\nprint -r -- \"[$(<e)]\"\n",
			"A\n@ unsetopt xtrace\n[@ '(anon)'\n@ print A]\n",
		},
		{
			"and standard output is redirected around the whole call",
			pre + "() { print A } >o\nunsetopt xtrace\nprint -r -- \"[$(<o)]\"\n",
			"@ '(anon)'\n@ print A\n@ unsetopt xtrace\n[A]\n",
		},
		// The restriction, and the row that grades it. A body that is a
		// *simple command* has no closing token, so a redirection written
		// after it is one of that command's own words — the file is left
		// empty and both lines go to the terminal.
		{
			"but a body with no braces keeps its own",
			pre + "() print A 2>e\nunsetopt xtrace\nprint -r -- \"[$(<e)]\"\n",
			"@ '(anon)'\n@ print A\nA\n@ unsetopt xtrace\n[]\n",
		},
		// The bare keyword is the third spelling and answers with the
		// simple command's rule for the same reason: what carries the
		// redirection there is the null command, and a simple command's
		// line is written before its files are opened.
		{
			"and the bare keyword writes its line before the file is opened",
			"NULLCMD=:\n" + pre + "function 2>f1\nunsetopt xtrace\nprint -r -- \"[$(<f1)]\"\n",
			"@ '(anon)'\n@ :\n@ unsetopt xtrace\n[]\n",
		},
		{
			"the bare keyword traces its call too",
			"NULLCMD=:\n" + pre + "function >f1\nprint -r -- --\n",
			"@ '(anon)'\n@ :\n@ print -r -- --\n--\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
				t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
			}
		})
	}
}

// A redirection that will not open is reported against the **line**, and the
// body never runs.
//
// The same fact from the other side: the file is opened before the frame is
// pushed, so there is no `(anon)` to name yet. This engine named one, which
// is what a redirection held by the body produces.
func TestAFailedOpenOnTheCallIsNamedAtItsLine(t *testing.T) {
	const src = "setopt xtrace\n() { print A } >/nosuchdir/f\nprint -r -- after\n"
	const want = "zsh:2: no such file or directory: /nosuchdir/f\n+zsh:3> print -r -- after\nafter\n"
	if out, st := runZsh(t, t.TempDir(), src); out != want || st != 0 {
		t.Errorf("%q = %q (status %d), want %q", src, out, st, want)
	}
}

// Words the shell could not expand end the call, and nothing is traced for
// it.
//
// The words after the body are a *heading*: they are expanded before the
// construct decides what to run, so one that failed means it does not run.
// This engine reported the failure and then ran the body anyway and carried
// on to the next line, which is the report-then-do-it-anyway shape.
//
// It is also what keeps the trace honest, and the second row is why the
// guard is written where it is rather than after the line: a call that never
// happened writes no line for itself.
func TestACallWhoseWordsFailedIsNeitherTracedNorMade(t *testing.T) {
	const pre = "PS4='@ '\nsetopt xtrace\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"an expression that would not compute",
			pre + "() { print A } $((1/0))\nprint -r -- after\n",
			"zsh:3: division by zero\n",
		},
		{
			"a pattern that matched nothing",
			pre + "() { print A } *nosuchthing*\nprint -r -- after\n",
			"zsh:3: no matches found: *nosuchthing*\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 1 {
				t.Errorf("%q = %q (status %d), want %q at 1", c.src, out, st, c.want)
			}
		})
	}
}
