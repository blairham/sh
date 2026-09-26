// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// positionSem answers both readings of "this word is a tilde context" — the
// narrow rule about an assignment's *shape* and the wider one about the first
// unquoted `=` — on, so that a row that keeps its tilde keeps it for the
// position rather than because no rule claimed it. Which preset answers each
// is the dialect packages' claim.
func equalsContextSem(narrow, wide Answer) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode = narrow
		sem.TheFirstUnquotedEqualsInAWordOpensATildeContext = wide
		r.Semantics = &sem
		r.Vars = map[string]string{"HOME": "/h"}
	}
}

// An element of an array literal is not a tilde context however it is shaped,
// under either reading (#4564). Measured on bash 5.3.20, whose column holds
// the narrow rule: `a=(FOO=~/x); echo "${a[0]}"` is `FOO=~/x` while `echo
// FOO=~/x` on the same line is a path.
func TestAnArrayLiteralsElementIsNotATildeContext(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		narrow, wide    Answer
	}{
		{"the shape, in a literal", `a=(FOO=~/x); echo "${a[@]}"`, "FOO=~/x", Yes, No},
		{"the wider rule, in a literal", `a=(--opt=~); echo "${a[@]}"`, "--opt=~", No, Yes},
		{"both readings on", `a=(FOO=~/x --opt=~); echo "${a[@]}"`, "FOO=~/x --opt=~", Yes, Yes},
		// **The controls, and they are what make the rows above a position
		// rather than a rule nobody turned on.** The same two words as an
		// ordinary argument do expand, so each reading is seen to fire.
		{"the shape, as an argument", `echo FOO=~/x`, "FOO=/h/x", Yes, No},
		{"the wider rule, as an argument", `echo --opt=~`, "--opt=/h", No, Yes},
		// And a tilde that opens the element is an ordinary word's tilde and
		// is not this question at all.
		{"a tilde opening the element", `a=(~/x); echo "${a[@]}"`, "/h/x", Yes, Yes},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src+"\n", equalsContextSem(c.narrow, c.wide))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("%s = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The word naming the command is not a tilde context either, and **which word
// that is is decided by where it is written** rather than by what the words in
// front of it came to.
//
// The word is seen through the diagnostic for a command that is not there,
// which is why every row runs a name nothing can resolve: what the shell
// reports is the word it was going to run.
func TestTheWordNamingTheCommandIsNotATildeContext(t *testing.T) {
	// `mod` is a word this runner is told the next written word still names
	// the command behind, and it is also a function that writes its operands
	// back — so a row can see which word the position reached without the
	// shell having to fail to find one.
	const mod = `mod(){ printf "[%s]" "$@"; echo; }; `
	for _, c := range []struct{ name, src, want string }{
		{"the first word", `--opt=~/x`, "--opt=~/x"},
		{"behind a modifier", mod + `mod --opt=~/x`, `[--opt=~/x]`},
		{"behind two modifiers", mod + `mod mod --opt=~/x`, `[mod][--opt=~/x]`},
		// **The discriminating row.** A word that becomes the command name
		// only because what stood in front of it expanded to nothing is
		// *not* in that position: it is the second word written, and it
		// expands. A rule read off the expanded argument list answers this
		// one the other way round and agrees with every row above.
		{"a word an empty expansion promoted", `e=; $e --opt=~/x`, "--opt=/h/x"},
		// A modifier the scan cannot read stops it, since nothing in front
		// of the word can be identified.
		{"behind a word that is not a literal", mod + `e=mod; $e --opt=~/x`, `[--opt=/h/x]`},
		// And the control: the same word one place further along is an
		// argument and expands.
		{"an argument of the same command", mod + `mod q --opt=~/x`, `[q][--opt=/h/x]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			setup := equalsContextSem(No, Yes)
			out, _ := run(t, c.src+"\n", func(r *Runner) {
				setup(r)
				r.SetCommandWordModifier("mod")
			})
			if !strings.Contains(out, c.want) {
				t.Errorf("%s reported %q, want it to name %q", c.src, out, c.want)
			}
		})
	}
}

// The flag is spent by the word pipeline, so a word expanded *inside* a
// command word does not inherit the position: `$(echo a=~)` written as a
// command name expands in the reference, because the substitution's body is a
// command line of its own.
func TestTheCommandWordsPositionDoesNotReachInsideIt(t *testing.T) {
	out, st := run(t, "v=$(echo --opt=~/x)\necho \"$v\"\n", equalsContextSem(No, Yes))
	if st != 0 {
		t.Fatalf("status %d, out %q", st, out)
	}
	if got := strings.TrimSpace(out); got != "--opt=/h/x" {
		t.Errorf("got %q, want the substitution's own word to have expanded", got)
	}
}
