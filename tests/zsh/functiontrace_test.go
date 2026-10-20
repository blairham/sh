// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `functions -t` and `functions -T`, the marks that make one function trace.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// from script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME and
// standard input on the null device. Every `want` below is that shell's own
// bytes (#5067).
//
// The rows that matter are the ones a **call** makes. A grid of declarations
// alone cannot tell "the mark was recorded" from "the mark is read when the
// function runs", and recording it and reading it nowhere is exactly the
// shape #5061 was filed as.

// The mark turns the trace on for the body it is on.
func TestATracedFunctionTracesItsOwnBody(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"f() { print A }\nfunctions -t f\nf\n", "+f:0> print A\nA\n"},
		{"f() { print A }\nfunctions -T f\nf\n", "+f:0> print A\nA\n"},
		{"f() { print A }\ntypeset -ft f\nf\n", "+f:0> print A\nA\n"},
		{"f() { print A }\ndeclare -ft f\nf\n", "+f:0> print A\nA\n"},
		{"f() { print A }\ntypeset -fT f\nf\n", "+f:0> print A\nA\n"},
		// The word's own pattern form marks rather than lists: the
		// reference writes nothing here and leaves `f` traced.
		{"f() { print A }\nfunctions -tm 'f*'\nf\n", "+f:0> print A\nA\n"},
		// An unmarked function is untouched, which is the row that says the
		// mark is read rather than the trace being on for everyone.
		{"f() { print A }\nf\n", "A\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// `-t` reaches what the body calls and `-T` stops at the body.
//
// This is the pair the whole feature turns on, and it is measured rather than
// read off the letters: with `g` unmarked and called from `f`, `-t` traces
// g's body and `-T` traces only the line in `f` that calls it.
func TestTheBoundedLetterStopsAtItsOwnBody(t *testing.T) {
	const pre = "g() { print G }\nf() { print A; g }\n"
	for _, c := range []struct{ word, want string }{
		{"-t", "+f:0> print A\nA\n+f:0> g\n+g:0> print G\nG\n"},
		{"-T", "+f:0> print A\nA\n+f:0> g\nG\n"},
		// Both letters at once is the bounded one, whichever order they
		// were written in.
		{"-tT", "+f:0> print A\nA\n+f:0> g\nG\n"},
	} {
		src := pre + "functions " + c.word + " f\nf\n"
		out, st := runZsh(t, t.TempDir(), src)
		if out != c.want || st != 0 {
			t.Errorf("functions %s f = %q (status %d), want %q", c.word, out, st, c.want)
		}
	}
	for _, src := range []string{
		pre + "functions -t f\nfunctions -T f\nf\n",
		pre + "functions -T f\nfunctions -t f\nf\n",
	} {
		out, _ := runZsh(t, t.TempDir(), src)
		if want := "+f:0> print A\nA\n+f:0> g\nG\n"; out != want {
			t.Errorf("%q = %q, want the bounded answer %q", src, out, want)
		}
	}
}

// A function holding its own mark traces wherever it is called from, so the
// bound is on the inheritance and not on the mark.
func TestAMarkedFunctionTracesThroughAnUntracedCaller(t *testing.T) {
	const pre = "h() { print H }\ng() { h }\nf() { g }\n"
	for _, c := range []struct{ src, want string }{
		// The bound reaches all the way down when nothing below holds a
		// mark of its own.
		{pre + "functions -T f\nf\n", "+f:0> g\nH\n"},
		// And stops where one does.
		{pre + "functions -T f\nfunctions -t h\nf\n", "+f:0> g\n+h:0> print H\nH\n"},
		// A mark on the callee alone traces the callee alone.
		{"g() { print G }\nf() { g }\nfunctions -t g\nf\n", "+g:0> print G\nG\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// The trace option is saved across **every** call in this column and put back
// at the return — which is what makes the mark need no restore of its own.
//
// Measured: `f() { set -x }; f; print after` leaves `after` untraced here and
// traced in bash 5.3, dash and /bin/ksh, while `f() { setopt extendedglob }`
// leaves that option on afterwards. So it is the trace alone, and it is not
// `localoptions`. See Semantics.FunctionCallRestoresTheTrace.
func TestACallPutsTheTraceBack(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"f() { set -x }\nf\nprint after\n", "after\n"},
		{"f() { setopt xtrace }\nf\nprint after\n", "after\n"},
		{"f() { set -x }\nf\n[[ -o xtrace ]] && print ON || print OFF\n", "OFF\n"},
		// The other direction: an option turned **off** inside a call is
		// put back on too.
		{"setopt xtrace\nf() { set +x }\nf\nprint after\n", "+f:0> set +x\n+zsh:4> print after\nafter\n"},
		// And it is the trace alone that behaves this way.
		{"f() { setopt extendedglob }\nf\n[[ -o extendedglob ]] && print ON || print OFF\n", "ON\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if !strings.HasSuffix(out, c.want) || st != 0 {
			t.Errorf("%q = %q (status %d), want it to end %q", c.src, out, st, c.want)
		}
	}
}

// `set +x` inside a traced body does not outlive the call, which falls out of
// the rule above rather than being the mark's own arrangement.
func TestTheTraceComesBackAfterAMarkedCall(t *testing.T) {
	src := "f() { set +x; print A }\nfunctions -t f\nsetopt xtrace\nf\nprint after\nunsetopt xtrace\n"
	out, _ := runZsh(t, t.TempDir(), src)
	if !strings.Contains(out, "+zsh:5> print after") {
		t.Errorf("%q = %q, want the trace back for `print after`", src, out)
	}
}

// The listing writes the mark **inside** the body, and the same line for both
// letters.
func TestTheListingWritesTheMarkInsideTheBody(t *testing.T) {
	const traced = "f () {\n\t# traced\n\tprint A\n}\n"
	for _, c := range []struct{ src, want string }{
		{"f() { print A }\nfunctions -t f\nfunctions\n", traced},
		{"f() { print A }\nfunctions -T f\nfunctions\n", traced},
		{"f() { print A }\nfunctions -t f\ntypeset -f\n", traced},
		{"f() { print A }\nfunctions -t f\nfunctions -m 'f*'\n", traced},
		// The indent moves with the listing's own.
		{"f() { print A }\nfunctions -t f\nfunctions -x2\n", "f () {\n  # traced\n  print A\n}\n"},
		// And it is not in the parameter: `${functions[f]}` is the body
		// without it.
		{"f() { print A }\nfunctions -t f\nprint ${functions[f]}\n", "\tprint A\n"},
		// An unmarked function has no such line, which is the row that
		// keeps this from being written for everyone.
		{"f() { print A }\nfunctions\n", "f () {\n\tprint A\n}\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// With no operands the letters are a **filter** over the table, separately
// for each letter and a union between them; a plus names instead of writing.
func TestTheLettersNarrowAListingWithNoOperands(t *testing.T) {
	const traced = "f () {\n\t# traced\n\tprint A\n}\n"
	for _, c := range []struct{ src, want string }{
		{"f() { print A }\nfunctions -t f\nfunctions -t\n", traced},
		{"f() { print A }\nfunctions -t f\ntypeset -ft\n", traced},
		// `-T` and `-t` are separate records: a name marked with one is not
		// listed by the other.
		{"f() { print A }\nfunctions -T f\nfunctions -t\n", ""},
		{"f() { print A }\nfunctions -t f\nfunctions -T\n", ""},
		{"f() { print A }\nfunctions -T f\nfunctions -T\n", traced},
		// And a union between them.
		{"f() { print A }\nfunctions -T f\nfunctions -tT\n", traced},
		// Nothing marked writes nothing, where a bare `functions` writes
		// the table — the row that says the narrowing happened at all.
		{"f() { print A }\nfunctions -t\n", ""},
		// A plus names them.
		{"f() { print A }\ng() { print G }\nfunctions -t f\nfunctions +t\n", "f\n"},
		{"f() { print A }\nfunctions -t f\ntypeset +ft\n", "f\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// A plus with operands takes that one letter off and leaves the other.
func TestThePlusTakesOneLetterOff(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"f() { print A }\nfunctions -t f\nfunctions +t f\nf\n", "A\n"},
		{"f() { print A }\nfunctions -T f\nfunctions +T f\nf\n", "A\n"},
		// `+T` leaves the `t` record alone.
		{
			"f() { print A }\nfunctions -t f\nfunctions +T f\nfunctions -t\n",
			"f () {\n\t# traced\n\tprint A\n}\n",
		},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// A name that is not a function is a silent 1, and the names beside it are
// still marked. A name `autoload` left waiting **is** a function for this.
func TestANameThatIsNotAFunctionIsASilentOne(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
		st   int
	}{
		{"f() { print A }\nfunctions -t nosuch\nprint st=$?\n", "st=1\n", 0},
		{"f() { print A }\nfunctions +t nosuch\nprint st=$?\n", "st=1\n", 0},
		{"f() { print A }\nfunctions +t f\nprint st=$?\n", "st=0\n", 0},
		// The status is 1 and `f` is still marked.
		{
			"f() { print A }\nfunctions -t f nosuch\nprint st=$?\nf\n",
			"st=1\n+f:0> print A\nA\n", 0,
		},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != c.st {
			t.Errorf("%q = %q (status %d), want %q (status %d)", c.src, out, st, c.want, c.st)
		}
	}
}

// A definition replaces the body and the mark goes with it.
func TestARedefinitionForgetsTheMark(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"f() { print A }\nfunctions -t f\nf() { print B }\nf\n", "B\n"},
		{"f() { print A }\nfunctions -t f\nunfunction f\nf() { print A }\nf\n", "A\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%q = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// The letters are spelled by this dialect and are not called missing — the
// paired-table invariant, asked of the word that gained them.
func TestTheTraceLettersAreSpelledAndNotCalledMissing(t *testing.T) {
	sem, dg := zsh.Semantics(), zsh.Diagnostics()
	if sem.FunctionTraceLetters != "tT" {
		t.Errorf("FunctionTraceLetters = %q, want %q", sem.FunctionTraceLetters, "tT")
	}
	if sem.FunctionTraceLettersBoundToTheBody != "T" {
		t.Errorf("FunctionTraceLettersBoundToTheBody = %q, want %q",
			sem.FunctionTraceLettersBoundToTheBody, "T")
	}
	for _, c := range sem.FunctionTraceLetters {
		if !strings.ContainsRune(sem.FunctionsOptions, c) {
			t.Errorf("-%c marks a function for tracing and is not in FunctionsOptions %q",
				c, sem.FunctionsOptions)
		}
		if !strings.ContainsRune(sem.DeclareOptions, c) {
			t.Errorf("-%c marks a function for tracing and is not in DeclareOptions %q",
				c, sem.DeclareOptions)
		}
		if strings.ContainsRune(dg.UnimplementedOptionLetters["functions"], c) {
			t.Errorf("functions: -%c is implemented and still called missing in %q",
				c, dg.UnimplementedOptionLetters["functions"])
		}
		for _, word := range []string{"typeset", "declare", "local"} {
			if strings.ContainsRune(dg.UnimplementedOptionLettersOnAFunctionLine[word], c) {
				t.Errorf("%s: -%c is implemented on a function line and still called missing",
					word, c)
			}
		}
	}
}
