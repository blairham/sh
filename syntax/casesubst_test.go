// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Where a command substitution ends is a question about the grammar.
//
// Counting parentheses cannot answer it: a `case` arm's `)` closes nothing,
// so the count reaches zero early and takes half the arm with it. Every shell
// in the panel runs the first of these.
//
// The worse half is that it did not always *fail*. Where the leftovers
// happened to be a command, two scripts installed on this machine parsed into
// a tree nobody wrote and said nothing — which only reading the tree back
// showed.
func TestASubstitutionEndsWhereItsContentsDo(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an arm inside one", `x=$(case a in a) echo yes;; esac)`},
		{"an arm with several patterns", `x=$(case a in a|b) echo yes;; esac)`},
		{"an arm inside a loop inside one", `x=$(for f in a; do case $f in a) echo m;; esac; done)`},
		{"two arms", `x=$(case a in a) echo p;; b) echo q;; esac)`},
		// The nesting that counting got right, which has to keep working.
		{"a substitution inside an arm", `case a in a) echo "$(echo inner)";; esac`},
		{"a subshell inside one", `x=$( (echo hi) )`},
		{"parentheses that really do pair", `x=$(echo "(a)")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := syntax.Parse(tc.src, syntax.Core()); err != nil {
				t.Errorf("parse %q: %v", tc.src, err)
			}
		})
	}
}

// An unfinished one is still unfinished, and says so rather than being read
// as something else — which is what a prompt needs in order to ask for more.
func TestAnUnfinishedSubstitutionIsIncomplete(t *testing.T) {
	p := syntax.NewParser(`x=$(echo hi`, syntax.Core())
	p.Parse()
	if p.Err() == nil {
		t.Fatal("accepted an unterminated substitution")
	}
	if !p.Incomplete() {
		t.Error("not reported as incomplete, so a prompt would refuse it rather than ask for more")
	}
}

// The two spellings are one node and not one syntax: this one ends at its
// closing backquote and the other where its contents end. A substitution
// holding a here-document whose delimiter never matches is terminated by the
// first and not by the second — which is a real script on this machine.
func TestTheOlderSpellingIsWrittenBackAsItself(t *testing.T) {
	src := "x=`cat <<EOF\nbody\n EOF`\n"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	printed := syntax.Print(f)
	if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
		t.Fatalf("printed source does not parse: %v\n  gave: %q", err, printed)
	}
}
