// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Input that runs out with a construct still open is its own kind of failure,
// and it carries the whole state rather than a sentence.
//
// It has to, because the panel does not word one diagnosis four ways — each
// shell names a *different part* of that state. This test names the fields
// rather than the shells, which is the rule for a test in this package; what
// each dialect makes of them lives in dialect/.
func TestUnterminatedCarriesTheStateItFailedIn(t *testing.T) {
	for _, tc := range []struct {
		src       string
		construct string
		line      int
		innermost string
		expected  string
		lastToken string
	}{
		{`if`, "if", 1, "if", "then", "if"},
		{`if true; then echo x`, "if", 1, "then", "fi", "x"},
		{`if true; then echo x; else echo y`, "if", 1, "else", "fi", "y"},
		{`for i in a b; do echo hi`, "for", 1, "for", "done", "hi"},
		{`while true; do echo hi`, "while", 1, "do", "done", "hi"},
		{`until false; do echo hi`, "until", 1, "do", "done", "hi"},
		{`case a in a) echo x`, "case", 1, "case", ";;", "x"},
		{`{ echo x`, "{", 1, "{", "}", "x"},
		// The innermost is the inner loop and the construct is the one it
		// began in, which is the pair one shell names and another does not.
		{"if true; then\nfor i in a; do echo x", "for", 2, "for", "done", "x"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, syntax.Core())
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("got %v, want a *syntax.Error", err)
			}
			if se.Kind != syntax.ErrUnterminated {
				t.Fatalf("kind = %v, want ErrUnterminated", se.Kind)
			}
			if se.Construct != tc.construct || se.ConstructLine != tc.line {
				t.Errorf("construct = %q on line %d, want %q on line %d",
					se.Construct, se.ConstructLine, tc.construct, tc.line)
			}
			if se.Innermost != tc.innermost {
				t.Errorf("innermost = %q, want %q", se.Innermost, tc.innermost)
			}
			if se.Expected != tc.expected {
				t.Errorf("expected = %q, want %q", se.Expected, tc.expected)
			}
			if se.LastToken != tc.lastToken {
				t.Errorf("lastToken = %q, want %q", se.LastToken, tc.lastToken)
			}
		})
	}
}

// A construct that closes leaves nothing behind, which is what makes the stack
// safe to read at the point of failure.
func TestClosedConstructsAreNotRemembered(t *testing.T) {
	src := "if true; then echo x; fi\nfor i in a; do echo y; done\n{ echo z; }\nif"
	_, err := syntax.Parse(src, syntax.Core())
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("got %v, want a *syntax.Error", err)
	}
	if se.Construct != "if" || se.ConstructLine != 4 {
		t.Errorf("construct = %q on line %d, want if on line 4", se.Construct, se.ConstructLine)
	}
}
