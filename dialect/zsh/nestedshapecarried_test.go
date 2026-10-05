// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedListKeepsItsShapeThroughALevel is #5978: whether a subscript
// counts elements or characters is decided by the shape the level below
// handed up, changed only by what the level in between does. Every row
// measured on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f), 2026-10-05.
//
// The first two rows are the pair the fix turns on: the same inner and the
// same one value surviving, and the level between — a filter or a single
// subscript — is the only thing that differs.
func TestANestedListKeepsItsShapeThroughALevel(t *testing.T) {
	const pre = `x=$'two\nthree'; y=three; s=hello; a=(hello)
`
	rows := []struct{ src, want string }{
		{`print -r -- ${${${(f)x}:#tw*}[1]}`, "three"},
		{`print -r -- ${${${(f)x}[2]}[1]}`, "t"},
		{`print -r -- ${${${(f)x}[2,2]}[1]}`, "three"},
		{`print -r -- ${${(M)${(f)x}:#th*}[1]}`, "three"},
		{`v=${${(M)${(f)x}:#th*}[1]}; print -r -- $v`, "three"},
		{`print -r -- ${${${=x}:#tw*}[1]}`, "three"},
		{`print -r -- ${${${${(f)x}:#tw*}}[1]}`, "three"},
		{`print -r -- ${#${${(f)x}:#tw*}}`, "1"},
		// The issue's own shape: a quoted command substitution split by
		// lines, filtered, and subscripted.
		{`v=${${(M)${(f)"$(print -l 'From a' 'Subject: q1' '' body)"}:#Subject: *}[1]}; print -r -- $v`, "Subject: q1"},
		{`v=${${(M)${(f)"$(print -l 'From a' 'Subject: q1')"}:#Sub*}[1]#Subject: }; print -r -- $v`, "q1"},
		// Controls: a string stays a string at any depth, a split that found
		// one field is a string, a join is one, and quoting joins.
		{`print -r -- ${${(f)y}[1]}`, "t"},
		{`print -r -- ${${${s}}[2]}`, "e"},
		{`print -r -- ${${(j:,:)${${(f)x}:#tw*}}[1]}`, "t"},
		{`print -r -- "${${${(f)x}:#tw*}[1]}"`, ""},
		{`print -r -- ${${${a}#h}[1]}`, "ello"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), pre+row.src+"\n")
		if out != row.want+"\n" {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, row.want+"\n")
		}
	}
}

// TestANestedConditionalIsTheShapeOfWhatItCameTo is #5985: a conditional on a
// nested inner is the shape of the word where the test fires and of the inner
// where it does not. Every row measured on zsh 5.9.2 under -f, 2026-10-05.
func TestANestedConditionalIsTheShapeOfWhatItCameTo(t *testing.T) {
	const pre = `x=$'two\nthree'; a=(hello); b=(p q); e=; s=hello
`
	rows := []struct{ src, want string }{
		{`print -r -- ${${${(f)x}:+$a}[1]}`, "hello"},
		{`print -r -- ${${${(f)x}+$a}[1]}`, "hello"},
		{`print -r -- ${${${(f)x}:+"$a"}[1]}`, "h"},
		{`print -r -- ${${${(f)x}:-$a}[1]}`, "two"},
		{`print -r -- ${${${(f)e}:-$a}[1]}`, "hello"},
		{`print -r -- ${${${(f)e}:+$a}[1]}`, ""},
		{`print -r -- ${${${(f)e}-$a}[1]}`, ""},
		{`print -r -- ${${${s}:+$a}[1]}`, "hello"},
		{`print -r -- ${${${s}:-zz}[1]}`, "h"},
		{`print -r -- ${${${(f)x}:+$b}[2]}`, "q"},
		{`print -r -- ${${${s}:+${(f)x}}[1]}`, "two"},
		{`print -r -- ${${${(f)e}:-xyz}[1]}`, "x"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), pre+row.src+"\n")
		if out != row.want+"\n" {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, row.want+"\n")
		}
	}
}
