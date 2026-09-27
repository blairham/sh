// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell's `private`, and what only this dialect can say about it.
//
// The *rule* — that a deeper function frame reads past the declaration to
// whatever it displaced — is the substrate's and is pinned in
// interp/privatescope_test.go, where it names a word and no shell. What is
// here is the half that is this dialect's: the module that carries the word,
// the letters it takes, the words a type query writes, and the two refusals'
// exact sentences.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable*
// for it — run `-f` under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`,
// each row both under `-c` and from a script file.

// TestTheModuleThatCarriesPrivateLoads is #4738's first bar. The module names
// one feature, this shell has it, and so it loads — which is the ordinary
// rule at the top of zmodload.go rather than anything special to this module.
func TestTheModuleThatCarriesPrivateLoads(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private; print "st=$?"
zmodload -lF zsh/param/private
zmodload -e zsh/param/private; print "e=$?"`)
	want := "st=0\n+b:private\ne=0\n"
	if out != want || st != 0 {
		t.Errorf("the module = %q (status %d), want %q", out, st, want)
	}
}

// And the word is there without it, which is measured rather than a
// convenience: the reference **autoloads** the module off the first
// `private`, so a script that never writes the `zmodload` still gets the word
// and the whole visibility rule behind it. Measured — `zmodload` lists no
// such module before `private x=1` and lists it after.
//
// This shell autoloads nothing, so it registers the word from the start and
// arrives at the same place a script can see. See interp/privatebuiltin.go
// for the one word of divergence that leaves standing.
func TestPrivateIsThereWithoutTheZmodloadLine(t *testing.T) {
	out, st := answersRun(t, `v=9
g() { print "g=[$v]" }
f() { private v=1; g; print "f=[$v]" }
f
print "top=[$v]"`)
	want := "g=[9]\nf=[1]\ntop=[9]\n"
	if out != want || st != 0 {
		t.Errorf("private with no zmodload = %q (status %d), want %q", out, st, want)
	}
}

// The words a type query writes, which are this shell's own vocabulary and
// are where a private is visible at all: it carries `hide` and `special`
// without either having been asked for.
//
// The `local` row is the control and it is the row that makes the rest
// evidence: without it, five lines agreeing about a word this engine could be
// writing for every local would read exactly the same.
func TestAPrivateDescribesWithHideAndSpecial(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f1() { private v=1;    print "  ${(t)v}" }
f2() { private -i n=5; print "  ${(t)n}" }
f3() { private -a a;   print "  ${(t)a}" }
f4() { private -A m;   print "  ${(t)m}" }
f5() { private -x e=1; print "  ${(t)e}" }
f6() { local l=1;      print "  ${(t)l}" }
f1; f2; f3; f4; f5; f6`)
	want := "  scalar-local-hide-special\n" +
		"  integer-local-hide-special\n" +
		"  array-local-hide-special\n" +
		"  association-local-hide-special\n" +
		"  scalar-local-export-hide-special\n" +
		"  scalar-local\n"
	if out != want || st != 0 {
		t.Errorf("type words = %q (status %d), want %q", out, st, want)
	}
}

// A callee's view of the name is the ordinary one, and the two rows are the
// two shapes: the outer binding where there is one, and no such variable
// where there is not. The second is what says the callee is not looking at
// the private through a hole.
func TestACalleeSeesTheOuterNameAndNotThePrivate(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
v=9
g() { print "  ${(t)v} [$v]"; typeset -p v; print "  gp=$?" }
f() { private v=1; g }
f`)
	want := "  scalar [9]\ntypeset -g v=9\n  gp=0\n"
	if out != want || st != 0 {
		t.Errorf("with an outer name = %q (status %d), want %q", out, st, want)
	}
	out, _ = answersRun(t, `zmodload zsh/param/private
g() { print "  t=[${(t)v}] set=${+v}" }
f() { private v=1; g }
f`)
	if want := "  t=[] set=0\n"; out != want {
		t.Errorf("with no outer name = %q, want %q", out, want)
	}
}

// `typeset -p` writes no row for a private and the bare listing writes one,
// which is the pair rather than either alone: a listing that wrote neither
// would pass the first half for the wrong reason entirely.
func TestOnlyTheDashPListingPassesOverAPrivate(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private v=1; local w=2; typeset -p v; print "  st=$?"; typeset | grep -E '^local [vw]=' }
f`)
	want := "  st=0\nlocal v=1\nlocal w=2\n"
	if out != want || st != 0 {
		t.Errorf("the two listings = %q (status %d), want %q", out, st, want)
	}
}

// The two refusals, which are two different sentences for two different
// rules — and the one that is fatal is not the one that reads as the more
// serious of the two.
func TestPrivateRefusals(t *testing.T) {
	// A write to a name a caller's private displaced nothing under: fatal,
	// status 1, and the function's own name in the location.
	out, st := answersRun(t, `zmodload zsh/param/private
g() { v=7; print "in-g" }
f() { private v=1; g; print "back" }
f
print "after"`)
	if !strings.Contains(out, "g: v: can't change parameter attribute") {
		t.Errorf("the write refusal = %q, want the sentence in it", out)
	}
	if strings.Contains(out, "back") || strings.Contains(out, "after") {
		t.Errorf("the write refusal = %q, want the script to end there", out)
	}
	if st != 1 {
		t.Errorf("the write refusal's status = %d, want 1", st)
	}
	// `private` over a name this call has already declared: reported, status
	// 1, the binding untouched and the script carrying on.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private v=1; private v=6; print "st=$? v=[$v]" }
f
print "after"`)
	want := "f:private: can't change scope of existing param: v\nst=1 v=[1]\nafter\n"
	if out != want || st != 0 {
		t.Errorf("the redeclaration refusal = %q (status %d), want %q", out, st, want)
	}
}

// The letters, which are neither `local`'s nor `typeset`'s — see
// Semantics.PrivateOptions for the letter-at-a-time measurement.
//
// `-g` is the row worth having: the letter means *do not take a scope*, so
// the word and the letter are a contradiction, and a shell that took it would
// have written a global under a word whose whole purpose is a binding local
// to one call.
func TestPrivateRefusesTheLettersItsOwnMeaningRulesOut(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private -g v; print "  g=$?"; private -f v; print "  f=$?"; private -n v; print "  n=$?" }
f`)
	for _, letter := range []string{"-g", "-f", "-n"} {
		if !strings.Contains(out, "bad option: "+letter) {
			t.Errorf("private %s = %q, want a bad-option refusal", letter, out)
		}
	}
	if st != 0 {
		t.Errorf("status = %d, want 0: each refusal is the builtin's and not the script's", st)
	}
	// And the letters it takes, which is the control on the other side: a
	// word that refused everything would pass every row above.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private -i n=5; print "  ${(t)n}=$n"; private -a a; print "  ${(t)a}" }
f`)
	want := "  integer-local-hide-special=5\n  array-local-hide-special\n"
	if out != want || st != 0 {
		t.Errorf("the letters it takes = %q (status %d), want %q", out, st, want)
	}
}

// A `private` declaration's parenthesized operand is an **array literal**,
// which is a grammar question and not the builtin's: the word is one of this
// shell's declaration utilities, so `name=( … )` behind it is an element list
// exactly as it is behind `typeset` (#4855).
//
// Measured 2026-09-27 against zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`, every row behind the `zmodload`.
// Before this the parentheses were kept as characters — `${(t)q}` was
// `scalar-local-hide-special` holding the five characters `(1 2)` — and with
// `-a` or `-A` the text reached the re-read of
// Semantics.DeclarationRereadsAParenthesizedValue, which this shell leaves
// unanswered for this dialect, so the line also wrote a refusal to stderr.
//
// The `typeset -a` row is the control that says this is the second
// declaration word's and not the literal's: it was right throughout.
func TestAPrivateDeclarationTakesAnArrayLiteral(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f1() { private q=(1 2);       print "  1 ${(t)q} n=${#q} [$q]" }
f2() { private -a q=(1 2);    print "  2 ${(t)q} n=${#q} [$q]" }
f3() { private -A m=(k v);    print "  3 ${(t)m} n=${#m} [${m[k]}]" }
f4() { private -a q=();       print "  4 ${(t)q} n=${#q}" }
f5() { local -P q=(1 2);      print "  5 ${(t)q} n=${#q}" }
f6() { private -A m=([k]=v);  print "  6 ${(t)m} [${m[k]}]" }
f7() { typeset -a q=(1 2);    print "  7 ${(t)q} n=${#q}" }
f1; f2; f3; f4; f5; f6; f7
private topq=(1 2); print "  8 ${(t)topq} n=${#topq}"`)
	want := "  1 array-local-hide-special n=2 [1 2]\n" +
		"  2 array-local-hide-special n=2 [1 2]\n" +
		"  3 association-local-hide-special n=1 [v]\n" +
		"  4 array-local-hide-special n=0\n" +
		"  5 array-local-hide-special n=2\n" +
		"  6 association-local-hide-special [v]\n" +
		"  7 array-local n=2\n" +
		"  8 array n=2\n"
	if out != want || st != 0 {
		t.Errorf("a private's array literal = %q (status %d), want %q", out, st, want)
	}
}

// And the word has to be **written** for that reading, which is the row that
// says `private` belongs in Dialect.DeclarationUtilities rather than in a
// rule of its own: quote it and the parenthesis is a glob qualifier, exactly
// as `'typeset' a=(x y)` is in this shell.
//
// Measured 2026-09-27, `f() { 'private' q=(1 2); print no }` is
// `f: unknown file attribute: 1` with the `print` never reached.
func TestAQuotedPrivateIsNotADeclarationUtility(t *testing.T) {
	out, _ := answersRun(t, `zmodload zsh/param/private
f() { 'private' q=(1 2); print "reached" }
f`)
	if !strings.Contains(out, "unknown file attribute: 1") {
		t.Errorf("a quoted private = %q, want the glob qualifier's refusal", out)
	}
	if strings.Contains(out, "reached") {
		t.Errorf("a quoted private = %q, want the command never to run", out)
	}
}

// The kind a private declaration gave it still may not move, and that is what
// keeps the row above from being a loosening: the exemption is the
// declaration's **own** operand and nothing else.
//
// Measured 2026-09-27. Rows one and two are refusals the shell ends on; rows
// three and four are the controls that say the refusal is about the kind
// rather than about a literal over a private at all.
func TestAPrivateStillKeepsTheKindItsDeclarationGave(t *testing.T) {
	for _, row := range []struct {
		name string
		src  string
	}{
		{"a later line", `f() { private at; at=(p q); print "reached" }; f`},
		{"an append", `f() { private at=x; at+=(p q); print "reached" }; f`},
	} {
		out, st := answersRun(t, "zmodload zsh/param/private\n"+row.src+"\nprint after")
		if !strings.Contains(out, "at: attempt to assign array value to non-array") {
			t.Errorf("%s = %q, want the retype refusal", row.name, out)
		}
		if strings.Contains(out, "reached") || strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the script to end there", row.name, out)
		}
		if st != 1 {
			t.Errorf("%s status = %d, want 1", row.name, st)
		}
	}
	// The controls: an ordinary local is retyped by the same two lines, and a
	// private declared `-a` takes the literal because its kind is already the
	// one being written.
	out, st := answersRun(t, `zmodload zsh/param/private
f1() { local at; at=(p q);      print "  1 ${(t)at} n=${#at}" }
f2() { private -a at; at=(p q); print "  2 ${(t)at} n=${#at}" }
f1; f2`)
	want := "  1 array-local n=2\n  2 array-local-hide-special n=2\n"
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
}
