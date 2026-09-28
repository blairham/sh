// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Semantics.UnrunSimpleCommandReadsItsWords: whether `set -n` reads the words
// of a simple command it will not run far enough to raise what reading them
// raises.
//
// Tests name the axis and never a shell.

// unrunGrammar is what these probes need: the flag group a name-flagged
// assignment is written with, and the assignment operators inside `${ }`.
func unrunGrammar(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ParamAssignAlways = true
}

// unrunSem answers the axis by name, with the three refusals the probes reach
// turned on. Which preset answers Yes is the dialect packages' claim.
func unrunSem(a Answer) Semantics {
	s := permissive()
	s.UnrunSimpleCommandReadsItsWords = a
	s.EqualsExpansion = Yes
	s.FatalErrorStatusIsOne = Yes
	return s
}

func unrunRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, "set -n\n"+src, unrunGrammar, withSem(unrunSem(a)))
}

// The three constructs the reading refuses, each with the answer on and off.
func TestTheWordsOfAnUnrunCommandAreReadWhereTheAxisSaysSo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a `=word` head with no command behind it", "echo =nosuchcommand_xyz\n", "nosuchcommand_xyz"},
		{"an expansion the grammar could not read", "echo ${9nope}\n", "bad substitution"},
		{"a name-flagged assignment onto no name", ": ${(P)::=y}\n", "not an identifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := unrunRun(t, Yes, tc.src)
			if !strings.Contains(out, tc.want) || st != 1 {
				t.Errorf("Yes: out %q status %d, want %q at 1", out, st, tc.want)
			}
			if out, st := unrunRun(t, No, tc.src); out != "" || st != 0 {
				t.Errorf("No: out %q status %d, want silence at 0", out, st)
			}
		})
	}
}

// And the refusal ends the script, exactly as it does when the command runs:
// the line after it is not read either.
func TestAnUnrunWordsRefusalEndsTheScript(t *testing.T) {
	out, st := unrunRun(t, Yes, "echo =nosuchcommand_xyz\necho =nosuchcommand_zzz\n")
	if strings.Contains(out, "nosuchcommand_zzz") {
		t.Errorf("out %q, want only the first refusal", out)
	}
	if !strings.Contains(out, "nosuchcommand_xyz") || st != 1 {
		t.Errorf("out %q status %d, want the first refusal at 1", out, st)
	}
}

// It is a property of the **position**: a command inside a compound is never
// reached, because `set -n` declines to walk into one at all. Each of these is
// silent with the axis on, which is what says the reading is the top-level
// list's and not the construct's.
func TestAnUnrunCommandInsideACompoundIsNotRead(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a group", "{ echo =nosuchcommand_xyz; }\n"},
		{"a subshell", "( echo =nosuchcommand_xyz )\n"},
		{"an if", "if true; then echo =nosuchcommand_xyz; fi\n"},
		{"a while loop", "while false; do echo =nosuchcommand_xyz; done\n"},
		{"a for loop", "for i in a; do echo =nosuchcommand_xyz; done\n"},
		{"a case arm", "case a in a) echo =nosuchcommand_xyz;; esac\n"},
		{"a function body", "f() { echo =nosuchcommand_xyz; }\n"},
		// And the control for the whole group: an assignment's value is not
		// a `=word` head, so the same characters in a word that is not one
		// are silent wherever they stand.
		{"an assignment's value", "v==nosuchcommand_xyz\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := unrunRun(t, Yes, tc.src); out != "" || st != 0 {
				t.Errorf("out %q status %d, want silence at 0", out, st)
			}
		})
	}
}

// And the shapes a list is made of *are* read, which is the other half of the
// same claim: the reading follows the list and stops at the compound.
func TestAnUnrunCommandReachedThroughAListIsRead(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"after a separator", "true; echo =nosuchcommand_xyz\n"},
		{"the right side of an and", "true && echo =nosuchcommand_xyz\n"},
		{"under a negation", "! echo =nosuchcommand_xyz\n"},
		{"with a redirection", "echo =nosuchcommand_xyz > /dev/null\n"},
		{"after a compound that was not read", "{ true; }; echo =nosuchcommand_xyz\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := unrunRun(t, Yes, tc.src)
			if !strings.Contains(out, "nosuchcommand_xyz") || st != 1 {
				t.Errorf("out %q status %d, want the refusal at 1", out, st)
			}
		})
	}
}

// The chain's own short-circuit is read from a status nothing set, so a right
// side the chain skips is not read either.
// The one that needs a rule of its own, measured rather than inferred: a
// timed command is silent where a negated one is not.
//
// Asserted on the refusal rather than on the whole output, because a `time`
// clause writes its report under this vector's diagnostics whatever `set -n`
// says — which is a question about the report and not about the reading.
func TestATimedUnrunCommandIsNotRead(t *testing.T) {
	out, st := unrunRun(t, Yes, "time echo =nosuchcommand_xyz\n")
	if strings.Contains(out, "nosuchcommand_xyz") || st != 0 {
		t.Errorf("out %q status %d, want the refusal unraised at 0", out, st)
	}
}

func TestAnUnrunChainSkipsTheSideItWouldNotRun(t *testing.T) {
	if out, st := unrunRun(t, Yes, "true || echo =nosuchcommand_xyz\n"); out != "" || st != 0 {
		t.Errorf("out %q status %d, want the right side skipped", out, st)
	}
}

// The controls that make the whole thing narrow: nothing is expanded, nothing
// is evaluated, nothing is matched, no value is fetched and nothing is stored.
// Each of these refuses when the command runs and is silent here.
func TestReadingAnUnrunCommandsWordsExpandsNothing(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a command substitution", "echo $(echo RAN >&2)\n"},
		{"a parameter with a word to complain with", "echo ${nope:?boom}\n"},
		{"an arithmetic expansion", "echo $(( 1/0 ))\n"},
		{"a pattern that matches nothing", "echo /nonexistentdir_xyz*/zzz\n"},
		{"a tilde naming no user", "echo ~nosuchuser_xyz\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := unrunRun(t, Yes, tc.src); out != "" || st != 0 {
				t.Errorf("out %q status %d, want silence at 0", out, st)
			}
		})
	}
}

// And the axis is not asked of a command that could not refuse however it is
// answered, which is what keeps `set -n` working in a vector that has not
// chosen: an ordinary file reads through with nothing said.
func TestAnUnrunCommandWithNothingToRefuseAsksNothing(t *testing.T) {
	s := CoreSemantics()
	out, st := runGrammar(t, "set -n\necho hi\ntrue && false\nif true; then :; fi\n",
		unrunGrammar, withSem(s))
	if out != "" || st != 0 {
		t.Errorf("out %q status %d, want the core to read the file unasked", out, st)
	}
	// And it *is* asked where one of the shapes stands, rather than being
	// answered for the vector.
	if _, st := runGrammar(t, "set -n\necho ${9nope}\n", unrunGrammar, withSem(s)); st != 2 {
		t.Errorf("status %d, want the core to refuse by name", st)
	}
}

// The two rows that carry a message at status 0, and they are a **fork**
// rather than a rule about pipelines: a non-last pipeline element and a
// backgrounded statement are each a shell of their own, so the refusal ends
// that shell and this one never sees a status.
//
// The control is the last element, which runs in this shell: a refusal there
// ends the script at 1, which is what says the difference is the fork and not
// the pipe.
func TestAnUnrunWordsRefusalInAForkCostsThisShellNothing(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		status    int
	}{
		{"a non-last pipeline element", "echo =nosuchcommand_xyz | cat\n", 0},
		{"a longer pipeline", "echo =nosuchcommand_xyz | cat | cat\n", 0},
		{"a backgrounded statement", "echo =nosuchcommand_xyz &\n", 0},
		{"the last element, which is this shell", "cat </dev/null | echo =nosuchcommand_xyz\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := unrunRun(t, Yes, tc.src)
			if !strings.Contains(out, "nosuchcommand_xyz") {
				t.Errorf("out %q, want the refusal written", out)
			}
			if st != tc.status {
				t.Errorf("status %d, want %d", st, tc.status)
			}
		})
	}
}

// unrunArrayGrammar is the grammar the two shapes below need: an array
// literal whose first element decides whether the `[sub]=` heads are
// subscripts, and an unbraced `$name[…]` subscript.
func unrunArrayGrammar(d *syntax.Dialect) {
	unrunGrammar(d)
	d.ArraySubscript = true
	d.ArrayLiteralShapeFollowsTheFirstElement = true
	d.BareSubscript = true
}

// unrunArraySem answers the axis and the two the shapes below reach: arrays
// are indexed from **one**, so a written `[0]` is below the first element,
// and an unbraced `[…]` is a subscript.
func unrunArraySem(a Answer) Semantics {
	s := unrunSem(a)
	s.ArrayBaseIsZero = No
	s.BareSubscriptIsASubscript = Yes
	return s
}

func unrunArrayRun(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, "set -n\n"+src, unrunArrayGrammar, withSem(unrunArraySem(a)))
}

// The fourth shape: a `[` an unbraced `$name` never closed. It needs no value
// to answer — the brackets are a subscript and this one does not terminate.
func TestAnUnrunCommandsUnclosedBareSubscriptIsRead(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a bracket at the end of the word", "printf x $a[\n"},
		{"a bracket the word never closes", "printf x $a[1 2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := unrunArrayRun(t, Yes, tc.src)
			if !strings.Contains(out, "subscript") || st != 1 {
				t.Errorf("Yes: out %q status %d, want the refusal at 1", out, st)
			}
			if out, st := unrunArrayRun(t, No, tc.src); out != "" || st != 0 {
				t.Errorf("No: out %q status %d, want silence at 0", out, st)
			}
		})
	}
}

// The fifth: an array literal's subscript, which is the one thing this mode
// **evaluates**. A command of nothing but assignments has them read; a
// command word in front of them takes the reading away.
func TestAnUnrunAssignmentsLiteralSubscriptIsRead(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"below the first element", "a=([0]=x)\n", true},
		{"below it, with a line in front", "true; a=([0]=x)\n", true},
		{"an unset name is zero", "a=([q]=y)\n", true},
		{"an expression that will not evaluate", "a=([1/0]=y)\n", true},
		// The controls. A subscript that lands is silent, and so is every
		// literal whose assignment is a command's prefix rather than the
		// whole of it.
		{"at the first element", "a=([1]=x)\n", false},
		{"quotes come off the subscript", "a=([\"1\"]=y)\n", false},
		{"a bare literal names no subscript", "a=(x y)\n", false},
		{"a prefix to a command word", "a=([0]=x) true\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := unrunArrayRun(t, Yes, tc.src)
			switch {
			case tc.refused && st != 1:
				t.Errorf("out %q status %d, want a refusal at 1", out, st)
			case tc.refused && out == "":
				t.Error("refused in silence, want the dialect's sentence")
			case !tc.refused && (out != "" || st != 0):
				t.Errorf("out %q status %d, want silence at 0", out, st)
			}
			if !tc.refused {
				return
			}
			if out, st := unrunArrayRun(t, No, tc.src); out != "" || st != 0 {
				t.Errorf("No: out %q status %d, want silence at 0", out, st)
			}
		})
	}
}

// And nothing in the literal is expanded to reach that subscript: no
// substitution runs, and a parameter between the brackets contributes
// nothing — set or not, since nothing set it.
func TestReadingAnUnrunLiteralsSubscriptExpandsNothing(t *testing.T) {
	// A parameter in the subscript is not expanded, so the text is **empty**
	// — the wording names no subscript at all. Never the parameter's value,
	// and never the name between the brackets either, which is what a reading
	// that took every span of the word would have left behind.
	for _, src := range []string{
		"q=3; a=([$q]=y)\n",
		"a=([$q]=y)\n",
		"a=([$(echo 0)]=y)\n",
	} {
		out, st := unrunArrayRun(t, Yes, src)
		if st != 1 || !strings.Contains(out, "a[]") {
			t.Errorf("%q: out %q status %d, want `a[]` refused at 1", src, out, st)
		}
	}
	// And a value in the literal is not expanded at all: a subscript that
	// lands leaves the whole element untouched.
	if out, st := unrunArrayRun(t, Yes, "a=([1]=$(echo RAN >&2))\n"); out != "" || st != 0 {
		t.Errorf("out %q status %d, want silence at 0 with nothing run", out, st)
	}
}
