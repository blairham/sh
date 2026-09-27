// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A `$( … )` body the grammar refuses at a token settles the read: no closing
// parenthesis is looked for, so the line never reads and nothing written
// before the substitution on it can run. See
// [Dialect.SubstitutionBodyRefusalEndsTheRead].

// bodyRefusalCounts is the grammar that keeps counting parentheses after a
// refused body — every dialect's answer before one opts in.
func bodyRefusalCounts() Dialect {
	d := Core()
	d.SubstitutionBodyRefusalEndsTheRead = false
	return d
}

// bodyRefusalSettles differs from it in that one field, which is what makes
// every row below a statement about the field.
func bodyRefusalSettles() Dialect {
	d := bodyRefusalCounts()
	d.SubstitutionBodyRefusalEndsTheRead = true
	return d
}

func TestARefusedSubstitutionBodySettlesTheLineWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// settled is whether the line is refused once the flag is on. Every
		// row reads with the flag off, which is the control on each of them:
		// the counting loop finds the closer and the line stands.
		settled bool
	}{
		{"a body refused at an operator", "echo b; v=$(&&); echo a\n", true},
		{"a body refused at a terminator", "echo b; v=$(;;); echo a\n", true},
		{"a body refused at a reserved word", "echo b; v=$(fi); echo a\n", true},
		{"another reserved word", "echo b; v=$(done); echo a\n", true},
		{"and a third", "echo b; v=$(esac); echo a\n", true},
		// The controls. A body with nothing wrong in it is untouched, and so
		// is one the read merely ran out of — the second is the carve-out in
		// Lexer.bodyRefusalSettlesTheRead and the reason the flag is not "a
		// body that will not parse".
		{"a body that parses", "echo b; v=$(echo hi); echo a\n", false},
		{"a body whose arm holds the closer", "x=$(case a in a) echo yes;; esac)\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src, bodyRefusalCounts()); err != nil {
				t.Errorf("the counting dialect refused the line: %v", err)
			}
			_, err := Parse(tc.src, bodyRefusalSettles())
			if got := err != nil; got != tc.settled {
				t.Errorf("with the flag on: refused=%v (%v), want %v", got, err, tc.settled)
			}
		})
	}
}

// The construct runs out at the end of the **line** it opened on and not at
// the end of the file, which is where the shell that settles a read this way
// reports it — see the measurement on the flag.
func TestASettledSubstitutionRunsOutAtTheEndOfItsLine(t *testing.T) {
	_, err := Parse("echo b\nv=$(&&)\necho a\necho c\n", bodyRefusalSettles())
	if err == nil {
		t.Fatal("the line read, so there is no position to check")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("came back as %T, want an *Error", err)
	}
	// The opener is on line 2 and the read ran out at the end of that line,
	// so EndLine — the line *after* the input the construct was given — is 3.
	// At the end of the file it would be 5. See Error.EndLine.
	if e.Pos.Line != 2 {
		t.Errorf("the substitution is blamed at line %d, want 2 — where it opened", e.Pos.Line)
	}
	if e.EndLine != 3 {
		t.Errorf("the read ran out past line %d, want 3 — the end of the line the substitution "+
			"opened on, not the end of the file", e.EndLine)
	}
	// And the body's own refusal travels with it, which is what the dialect
	// that writes two complaints needs. See Error.BodyRefusal.
	if e.BodyRefusal == nil {
		t.Fatal("the body's own refusal was dropped, so a dialect writing both complaints has one")
	}
	if got := e.BodyRefusal.Pos.Line; got != 2 {
		t.Errorf("the body's refusal is at line %d, want 2", got)
	}
	if !strings.Contains(e.BodyRefusal.Error(), "&&") {
		t.Errorf("the body's refusal does not name the token it stopped on: %v", e.BodyRefusal)
	}
}
