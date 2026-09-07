// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A plus form asking for the readonly attribute back off a name —
// `typeset +r x`, Semantics.ReadonlyAttributeCanBeRemoved — and the other
// half of the same surface: how long the attribute a declaration *adds*
// lasts.
//
// Measured 2026-09-07 over a script file, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, ZDOTDIR and HISTFILE, against bash 5.3.15 (and bash 3.2.57,
// and the same binary under argv[0] of `sh`), zsh 5.9.2, ksh93u+ 2012-08-01
// and dash.
//
//	typeset -r s=1; typeset +r s; s=9; echo "st=$? s=[$s]"
//
//	zsh    silent, status 0, then s=[9] — the attribute is gone
//	bash   typeset: s: readonly variable, status 1, and carries on
//	ksh93  typeset: s: is read only, and the script ends
//	dash   no `typeset` or `declare`, so nothing there can ask
//
// zsh alone allows it, which is a different split from
// DeclarationMayShadowAReadonly — where ksh93 stands with zsh. Two questions,
// and the ksh93 row is what proves it.

// removingReadonly is declRun's setter for the answer that grants the
// removal, and unfreezingSem for the one that refuses it with the refusal
// non-fatal, so the line after it is there to be asked what happened.
func removingReadonly(s *Semantics) {
	s.ReadonlyAttributeCanBeRemoved = Yes
	s.DeclareOptions = "aAilprux"
	s.LocalOptions = "aAilprux"
	s.TypesetLocalNeedsKeywordFunction = No
}

func refusingRemoval(s *Semantics) {
	s.ReadonlyAttributeCanBeRemoved = No
	s.DeclareOptions = "aAilprux"
	s.LocalOptions = "aAilprux"
	s.ReadonlyReassignmentFatal = No
	s.ReadonlyReassignmentByDeclarationFatal = No
	s.TypesetLocalNeedsKeywordFunction = No
}

// readonlyWording is the sentence pair the refusal needs, in bash's words,
// with `typeset` among the builtins that name themselves.
func readonlyWording() Diagnostics {
	return Diagnostics{
		ReadonlyVariable:              "%s: readonly variable",
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
		// All three declaration words, which is bash's table — `local`
		// among them, and it is the word this file's shadow tests write.
		ReadonlyRefusalNamesBuiltin: map[string]bool{"typeset": true, "declare": true, "local": true},
	}
}

// Where the removal is granted the name is writable again, which is the whole
// of what the plus form is for. `biDeclare` only ever froze, so `+r` was a
// no-op that then met the refusal the *next* line was going to meet anyway
// (#1168).
func TestTheReadonlyAttributeComesOff(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r s=1
typeset +r s
echo "d=$?"
s=9
echo "st=$? s=[$s]"`, removingReadonly, readonlyWording())
	if out != "d=0\nst=0 s=[9]\n" || errs != "" || st != 0 {
		t.Errorf("`typeset +r` where the dialect grants it:\n"+
			"got  out %q errs %q status %d\nwant out %q, nothing on stderr, status 0\n"+
			"zsh takes the attribute off silently and the write after it lands",
			out, errs, st, "d=0\nst=0 s=[9]\n")
	}
}

// `declare` is the same word under its other spelling wherever both exist,
// which is a fact about registration rather than about this engine: only the
// dialect that has the second name binds it, so the corpus row
// `axis/readonly-attribute-removed-declare-spelling` is where that is
// asserted and there is nothing for a runner without a dialect to answer.

// The freeze comes off *before* the same command's own assignment, which is
// what the shell that grants it does: `typeset -r x=1; typeset +r x=5` leaves
// 5 there rather than refusing its own value.
func TestTheRemovalPrecedesTheDeclarationsOwnValue(t *testing.T) {
	out, errs, _ := declRun(t, `typeset -r s=1
typeset +r s=5
echo "d=$? s=[$s]"`, removingReadonly, readonlyWording())
	if out != "d=0 s=[5]\n" || errs != "" {
		t.Errorf("`typeset +r s=5` where the removal is granted:\n"+
			"got  out %q errs %q\nwant out %q and nothing on stderr", out, errs, "d=0 s=[5]\n")
	}
}

// The other answer: the refusal is the ordinary readonly refusal through a
// declaration, so it names the builtin, reports 1 and the next line runs.
func TestTheRemovalIsRefusedAndTheScriptRunsOn(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r s=1
typeset +r s
echo "d=$?"
echo end`, refusingRemoval, readonlyWording())
	if out != "d=1\nend\n" || errs != "testsh: typeset: s: readonly variable\n" || st != 0 {
		t.Errorf("`typeset +r` where the dialect refuses it:\n"+
			"got  out %q errs %q status %d\nwant out %q errs %q status 0\n"+
			"bash names the builtin, leaves 1 behind and runs the next line",
			out, errs, st, "d=1\nend\n", "testsh: typeset: s: readonly variable\n")
	}
}

// And where a declaration's refusal is fatal it is fatal here, which is
// measured rather than assumed: ksh93 ends the script over `typeset +r` on a
// frozen name exactly as it ends one over `export x=2`.
func TestTheRemovalRefusalTakesTheDeclarationFatality(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r s=1
typeset +r s
echo end`, func(s *Semantics) {
		refusingRemoval(s)
		s.ReadonlyReassignmentByDeclarationFatal = Yes
		s.FatalErrorStatusIsOne = Yes
	}, readonlyWording())
	if out != "" || st != 1 || !strings.Contains(errs, "s: readonly variable") {
		t.Errorf("`typeset +r` refused where a declaration's refusal is fatal:\n"+
			"got  out %q errs %q status %d\nwant no output, the refusal on stderr, status 1",
			out, errs, st)
	}
}

// A plus form carrying a value is refused as an *assignment*, not as a
// removal, and says nothing of its own. That is how ksh93 tells the two
// apart: `typeset +r x` is its builtin location with the builtin named and
// `typeset +r x=5` is the plain line form, through the identical word.
func TestARemovalWithAValueIsRefusedAsAnAssignment(t *testing.T) {
	d := Diagnostics{
		ReadonlyVariable:              "%s: is read only",
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		// The assignment table leaves `typeset` out and the removal table
		// puts it in, which is ksh93's own arrangement.
		ReadonlyRefusalNamesBuiltin: map[string]bool{"set": true},
		ReadonlyRemovalNamesBuiltin: map[string]bool{"typeset": true},
	}
	out, errs, _ := declRun(t, `typeset -r s=1
typeset +r s=5
echo end`, refusingRemoval, d)
	if out != "end\n" || errs != "testsh: s: is read only\n" {
		t.Errorf("`typeset +r s=5` refused:\ngot  out %q errs %q\nwant out %q errs %q\n"+
			"one sentence, and the builtin *not* named — the assignment is refused "+
			"before the attribute is reached", out, errs, "end\n", "testsh: s: is read only\n")
	}
}

// The valueless form through the same word, where the removal table does name
// the builtin. One word, two shapes, two sentences.
func TestTheRemovalRefusalNamesTheBuiltinFromItsOwnTable(t *testing.T) {
	d := Diagnostics{
		ReadonlyVariable:              "%s: is read only",
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"set": true},
		ReadonlyRemovalNamesBuiltin:   map[string]bool{"typeset": true},
	}
	_, errs, _ := declRun(t, `typeset -r s=1
typeset +r s`, refusingRemoval, d)
	if errs != "testsh: typeset: s: is read only\n" {
		t.Errorf("`typeset +r` refused in ksh93's arrangement:\ngot  %q\nwant %q",
			errs, "testsh: typeset: s: is read only\n")
	}
}

// A nil removal table falls back to the assignment one, which is what every
// dialect but ksh93 wants: bash names the builtin for both shapes.
func TestTheRemovalRefusalFallsBackToTheAssignmentTable(t *testing.T) {
	_, errs, _ := declRun(t, `typeset -r s=1
typeset +r s`, refusingRemoval, readonlyWording())
	if errs != "testsh: typeset: s: readonly variable\n" {
		t.Errorf("`typeset +r` refused with no removal table of its own:\ngot  %q\nwant %q",
			errs, "testsh: typeset: s: readonly variable\n")
	}
}

// A plus form on a name that is *not* frozen asks nothing and says nothing —
// which is every `typeset +r` a script writes to make sure a name is
// writable. If it reached the axis, the strict core would refuse a line all
// three shells that spell it report 0 for.
func TestARemovalOnAFreeNameAsksNothing(t *testing.T) {
	out, errs, st := declRun(t, `a=1
typeset +r a
echo "d=$?"
a=2
echo "st=$? a=[$a]"`, func(s *Semantics) {
		refusingRemoval(s)
		s.ReadonlyAttributeCanBeRemoved = Unspecified
	}, readonlyWording())
	if out != "d=0\nst=0 a=[2]\n" || errs != "" || st != 0 {
		t.Errorf("`typeset +r` on a name nothing froze, with the axis unanswered:\n"+
			"got  out %q errs %q status %d\nwant out %q, nothing on stderr, status 0",
			out, errs, st, "d=0\nst=0 a=[2]\n")
	}
}

// And where the dialect has not answered, the plus form on a frozen name is
// refused by name rather than taken one way in silence.
func TestTheRemovalAxisIsRefusedByName(t *testing.T) {
	_, errs, _ := declRun(t, `typeset -r s=1
typeset +r s`, func(s *Semantics) {
		refusingRemoval(s)
		s.ReadonlyAttributeCanBeRemoved = Unspecified
	}, readonlyWording())
	const phrase = "the readonly attribute being taken off a name"
	if !strings.Contains(errs, phrase) {
		t.Errorf("unanswered axis:\ngot  %q\nwant a report naming %q", errs, phrase)
	}
}

// The sign that decides the readonly attribute is the *letter's*, not the
// last option word's. `typeset -r +x n` freezes n in all three shells that
// spell both letters; reading the word's sign made it a request to unfreeze,
// and so a refusal where the shells say nothing.
func TestTheReadonlyLetterCarriesItsOwnSign(t *testing.T) {
	out, _, _ := declRun(t, `n=1
typeset -r +x n
n=2
echo "st=$? n=[$n]"`, func(s *Semantics) {
		removingReadonly(s)
		s.ReadonlyReassignmentFatal = No
	}, readonlyWording())
	if out != "st=1 n=[1]\n" {
		t.Errorf("`typeset -r +x n`:\ngot  %q\nwant %q\n"+
			"the `-r` freezes n; the plus belongs to the `x` beside it and takes "+
			"nothing off", out, "st=1 n=[1]\n")
	}
}

// `local +r` reaching a freeze this same call made, which is the one shape
// where the shadow cannot answer: it clears an attribute it *displaced*, and
// a second declaration of a name the scope already shadowed finds the copy
// made and nothing left to displace. zsh answers `in=[2]`; bash refuses the
// declaration before it arrives here, and neither shell without `local` can
// be asked.
func TestALocalPlusRUnfreezesAFreezeTheSameCallMade(t *testing.T) {
	out, errs, st := declRun(t, `f() { local -r y=1; local +r y; y=2; echo "in=[$y]"; }
f
echo "st=$?"`, func(s *Semantics) {
		removingReadonly(s)
		// The shadow axis is a different question and this shape meets it
		// first — the one shell that grants the removal grants the shadow
		// too, which is the arrangement being asked about here.
		s.DeclarationMayShadowAReadonly = Yes
	}, readonlyWording())
	if out != "in=[2]\nst=0\n" || errs != "" || st != 0 {
		t.Errorf("`local +r` over this call's own `local -r`:\n"+
			"got  out %q errs %q status %d\nwant out %q, nothing on stderr, status 0\n"+
			"the shadow has nothing to displace the second time, so the plus form is "+
			"what takes the attribute off", out, errs, st, "in=[2]\nst=0\n")
	}
}

// And the other side of the same rule, over a name that is *already* frozen:
// `typeset -r +x x` is silent and leaves the freeze standing in bash, ksh93
// and zsh alike — unanimous, so core, and the same whichever way the removal
// axis is answered. Both answers are asserted for exactly that reason:
// reading the last option *word*'s sign would make this a removal, and under
// the answer that grants one that is invisible — the `-r` beside it freezes
// the name straight back — so only the refusing answer shows it, as a
// diagnostic and a status the panel does not produce.
func TestAPlusWordBesideTheReadonlyLetterRemovesNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(*Semantics)
	}{
		{"removal granted", removingReadonly},
		{"removal refused", refusingRemoval},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := declRun(t, `typeset -r x=1
typeset -r +x x
echo "d=$?"
x=2
echo "st=$? x=[$x]"`, func(s *Semantics) {
				c.set(s)
				s.ReadonlyReassignmentFatal = No
			}, readonlyWording())
			const wantOut = "d=0\nst=1 x=[1]\n"
			const wantErr = "testsh: x: readonly variable\n"
			if out != wantOut || errs != wantErr || st != 0 {
				t.Errorf("`typeset -r +x x` over a frozen name:\n"+
					"got  out %q errs %q status %d\nwant out %q errs %q status 0\n"+
					"the plus belongs to the `x` letter; the `r` still freezes, nothing "+
					"is taken off, and the assignment's is the only refusal",
					out, errs, st, wantOut, wantErr)
			}
		})
	}
}

// A refused shadow that the dialect makes *fatal* keeps the fatal error's own
// status. The builtin reports 1 for a refused operand — which is right where
// it carries on, since the rest of the operands are still declared — and
// falling through to that after a fatal overwrote the status the error had
// set: dash exits 2 for one of these, and it was exiting 1.
//
// Both words, because they are two routes to the one gate and the fix has to
// be the same on each. `local` is the route a real shell reaches today; no
// dialect currently answers the two axes in the combination that reaches it
// through `typeset`, and the substrate still has to agree with itself about a
// combination it can be handed.
func TestAFatalShadowRefusalKeepsItsOwnStatus(t *testing.T) {
	for _, word := range []string{"local", "typeset"} {
		t.Run(word, func(t *testing.T) {
			out, errs, st := declRun(t, `typeset -r x=1
f() { `+word+` x=2; echo "in=[$x]"; }
f
echo never`, func(s *Semantics) {
				refusingRemoval(s)
				s.DeclarationMayShadowAReadonly = No
				s.ReadonlyReassignmentByDeclarationFatal = Yes
				// The dialect that answers 2 here, which is the whole
				// point: with the other answer the overwrite is invisible.
				s.FatalErrorStatusIsOne = No
			}, readonlyWording())
			if out != "" || st != 2 || !strings.Contains(errs, word+": x: readonly variable") {
				t.Errorf("a fatal shadow refusal through `%s`:\n"+
					"got  out %q errs %q status %d\nwant no output, the refusal on "+
					"stderr, status 2 — the builtin's 1 must not stand in front of "+
					"the fatal error's", word, out, errs, st)
			}
		})
	}
}

// The other half of this surface: the attribute a declaration *adds* inside a
// function is the call's, and goes away with it. bash, ksh93 and zsh all
// leave y writable after the return, so this is core and has no axis.
//
// The shadow already put back a freeze it *displaced*; a freeze the
// declaration itself added was never recorded, so a `local -r` froze the
// caller's name for the rest of the script (#1168, found beside it).
func TestAFreezeADeclarationAddsEndsWithTheCall(t *testing.T) {
	for _, word := range []string{"local", "typeset"} {
		t.Run(word, func(t *testing.T) {
			out, errs, st := declRun(t, `f() { `+word+` -r y=1; echo "in=[$y]"; }
f
y=2
echo "st=$? y=[$y]"`, refusingRemoval, readonlyWording())
			if out != "in=[1]\nst=0 y=[2]\n" || errs != "" || st != 0 {
				t.Errorf("`%s -r` in a function, then a write after it returns:\n"+
					"got  out %q errs %q status %d\nwant out %q, nothing on stderr, status 0\n"+
					"the attribute lasts as long as the call in every shell that has a scope",
					word, out, errs, st, "in=[1]\nst=0 y=[2]\n")
			}
		})
	}
}

// And the freeze a declaration *displaces* still comes back, which is the
// half that must not break while the other is fixed: a function that thawed a
// readonly for good would be a hole in the whole point of the attribute.
func TestADisplacedFreezeStillComesBack(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
f() { local x=2; echo "in=[$x]"; }
f
x=9
echo "st=$? out=[$x]"`, func(s *Semantics) {
		shadowingReadonly(s)
		s.ReadonlyReassignmentFatal = No
	}, readonlyWording())
	if out != "in=[2]\nst=1 out=[1]\n" || !strings.Contains(errs, "x: readonly variable") || st != 0 {
		t.Errorf("a write after a shadowing call returned:\n"+
			"got  out %q errs %q status %d\nwant out %q with the refusal on stderr, status 0",
			out, errs, st, "in=[2]\nst=1 out=[1]\n")
	}
}
