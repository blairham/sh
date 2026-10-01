// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `!` after a bar is refused at the `!`, whatever the dialect's other
// negation answers are (#5256). The negation is written in front of a whole
// pipeline, so after a bar it is a reserved word standing where a command
// belongs; it ran as a command named `!` before this.
//
// Every dialect knob near the question is turned in the columns, so a refusal
// written to depend on one of them — the toggle, the bare reach, the
// both-streams bar — fails a column rather than passing all of them by luck.
func TestABangAfterABarIsRefusedAtTheBang(t *testing.T) {
	t.Parallel()
	toggles := Core()
	toggles.RepeatedNegationToggles = true
	bare := Core()
	bare.BareNegationReach = BareNegationAtEitherPlace
	both := Core()
	both.PipeBothStreams = true
	cols := []struct {
		name string
		d    Dialect
	}{{"core", Core()}, {"toggles", toggles}, {"bare reach", bare}, {"both streams", both}}
	for _, tc := range []struct {
		src       string
		line, col int
	}{
		{"echo | ! true\n", 1, 8},
		{"echo | !\n", 1, 8},
		{"echo | ! true | cat\n", 1, 8},
		{"! true | ! true\n", 1, 10},
		{"echo | cat | ! true\n", 1, 14},
		{"echo |\n! true\n", 2, 1},
		{"if echo | ! true; then :; fi\n", 1, 11},
	} {
		for _, c := range cols {
			_, err := Parse(tc.src, c.d)
			e, ok := err.(*Error)
			if !ok {
				t.Errorf("%s: %q: err = %v, want a refusal", c.name, tc.src, err)
				continue
			}
			if e.Kind != ErrUnexpected || int(e.Pos.Line) != tc.line || int(e.Pos.Col) != tc.col {
				t.Errorf("%s: %q: %v at %d:%d, want unexpected at %d:%d",
					c.name, tc.src, e.Kind, e.Pos.Line, e.Pos.Col, tc.line, tc.col)
			}
		}
	}
	// The controls, which parse in every column: a `!` that is not the bare
	// reserved word, one that begins a pipeline of its own inside a group,
	// and the leading one the grammar has always taken.
	for _, src := range []string{
		"echo | !true\n",
		"echo | \"!\" true\n",
		"echo | (! true)\n",
		"echo | { ! true; }\n",
		"! true | cat\n",
		"true && ! false\n",
		"echo; ! false\n",
	} {
		for _, c := range cols {
			if _, err := Parse(src, c.d); err != nil {
				t.Errorf("%s: %q: %v, want it to parse", c.name, src, err)
			}
		}
	}
}

// `|&` joining both streams is a bar like any other, so a `!` after it is
// refused too.
func TestABangAfterABothStreamsBar(t *testing.T) {
	t.Parallel()
	both := Core()
	both.PipeBothStreams = true
	if _, err := Parse("echo |& ! true\n", both); err == nil {
		t.Errorf("`echo |& ! true` parsed where |& is a bar, want a refusal")
	}
}
