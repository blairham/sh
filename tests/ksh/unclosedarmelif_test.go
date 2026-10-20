// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// The whole sentence each refusal is written as, line and token both.
// Measured 2026-10-16 on ksh93u+ 2012-08-01 at /bin/ksh with `-c`; the rows
// marked as such were also run from a script file and on standard input.
func kshRefusal(t *testing.T, d syntax.Dialect, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, d)
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	return ksh.Diagnostics().ParseFailure(err)
}

// A `case` arm whose command list the input ran out in is numbered at the
// line its `case` was read on (#6681). Between arms, and inside a pattern
// list, it is the line the input ran out on, as for every other construct.
// See syntax.Dialect.UnclosedCaseArmIsNumberedAtItsCase.
func TestAnUnclosedCaseArmIsNumberedAtItsCase(t *testing.T) {
	for _, c := range []struct {
		src  string
		line string
		tok  string
	}{
		{"case a in a) :\necho ok", "1", "case"}, // also file and stdin
		{"case a in a) :\necho ok\necho k", "1", "case"},
		{"echo x\ncase a in a) :\necho ok", "2", "case"}, // also file and stdin
		{"echo x\n\ncase a in a) :\necho ok", "3", "case"},
		{"case a in\na) :\necho ok", "1", "case"},
		{"case a in\na)\n:\necho ok", "1", "case"},
		{"case a in\na)", "1", "case"},
		{"case a\nin a) :\necho", "1", "case"},
		{"case a in a) <x\necho ok", "1", "case"},
		{"case a in a) {v}<x\necho", "1", "case"},
		{"case a in a) : ;\necho", "1", "case"},
		{"case x in x) ;\necho", "1", "case"},
		{"case a in a) : |\necho", "1", "case"},
		{"case a in a) : &&\necho", "1", "case"},
		{"case a in a) { :; }\necho", "1", "case"},
		{"case a in a) :;; b) :\necho ok", "1", "case"},
		{"case a in a) :;;\nb) :\necho ok", "1", "case"},
		{"if :; then case a in a) :\necho ok", "1", "case"},
		{"if :; then\ncase a in a) :\necho ok", "2", "case"},
		{"f() case a in a) :\necho", "1", "case"},
		// The innermost `case`, which is the one whose arm is open.
		{"case a in\na)\ncase b in\nb) :\necho", "3", "case"}, // also file and stdin
		// Another construct inside the arm is what ran out, at the end.
		{"case a in a) if :; then :\necho ok", "2", "then"},
		{"case a in a) { :\necho ok", "2", "{"},
		{"case a in a) while :; do :\necho", "2", "do"},
		// Between arms and inside a pattern list it is the end of the input.
		{"case a in a) :;;\n", "2", "case"},
		{"case a in a) :;;\nb", "2", "case"},
		{"case a in a) :;&\necho", "2", "case"},
		{"case a in a) :\necho ok;;\necho", "3", "case"},
		{"case a in a) :;;\nb) :;;\n", "3", "case"},
		{"case a in\na", "2", "case"},
		{"case a in\n(a", "2", "case"},
		{"case a in\na|", "2", "case"},
		{"case a in\n\n", "3", "case"},
		{"case a\nin", "2", "case"},
	} {
		want := "syntax error at line " + c.line + ": `" + c.tok + "' unmatched"
		if got := kshRefusal(t, ksh.Dialect(), c.src); got != want {
			t.Errorf("%q: got %q, want %q", c.src, got, want)
		}
	}
}

// An `elif` the input ran out in is named by the `if` it continues, and the
// body after its `then` by the `then` (#6682). See
// syntax.Dialect.ElifIsNamedByItsIf.
func TestAnUnclosedElifIsNamedByItsIf(t *testing.T) {
	for _, c := range []struct {
		src  string
		line string
		tok  string
	}{
		{"if :; then :; elif :\necho ok", "2", "if"}, // also file and stdin
		{"if :; then :; elif\necho ok", "2", "if"},
		{"if :; then :; elif", "1", "if"},
		{"if :; then if :; then :; elif :\necho ok", "2", "if"},
		{"while :; do if :; then :; elif :\necho", "2", "if"},
		{"if :\nthen :\nelif :\necho", "4", "if"},
		{"if :; then :; elif :; then :; elif :\necho", "2", "if"},
		{"if :; then :; elif : ||\necho", "2", "if"},
		{"if :; then :; elif :; then\necho ok", "2", "then"}, // also file and stdin
		{"if :; then :; elif :; then", "1", "then"},
		{"if :; then :; elif :; then :\necho", "2", "then"},
		// What already outranks a keyword still does.
		{"if :; then :; elif :; then ;\necho", "2", ";"},
		{"if :; then :; elif <x\necho", "2", "newline"},
		{"if :; then :; elif { :\necho", "2", "{"},
		// And an `else` is named as it is after an `if`.
		{"if :; then :; elif :; then :; else :\necho", "2", "else"},
		{"if :; then :; else :\necho ok", "2", "else"},
	} {
		want := "syntax error at line " + c.line + ": `" + c.tok + "' unmatched"
		if got := kshRefusal(t, ksh.Dialect(), c.src); got != want {
			t.Errorf("%q: got %q, want %q", c.src, got, want)
		}
	}
}

// Both readings are this dialect's alone: without them the grammar numbers
// the arm at the end of the input and names the `elif` by its own keyword.
func TestTheArmLineAndTheElifNameAreTheDialects(t *testing.T) {
	d := ksh.Dialect()
	d.UnclosedCaseArmIsNumberedAtItsCase = false
	d.ElifIsNamedByItsIf = false
	for src, want := range map[string]string{
		"case a in a) :\necho ok":          "syntax error at line 2: `case' unmatched",
		"if :; then :; elif :\necho ok":    "syntax error at line 2: `elif' unmatched",
		"if :; then :; elif :; then\necho": "syntax error at line 2: `elif-then' unmatched",
	} {
		if got := kshRefusal(t, d, src); got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}
