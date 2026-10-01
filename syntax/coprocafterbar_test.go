// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// **A `coproc` after a bar is refused at the word where the dialect does not
// take one there**, and parses where it does. See Dialect.CoprocAfterABar.
func TestACoprocAfterABar(t *testing.T) {
	t.Parallel()
	refuses := Core()
	refuses.Coproc = true
	refuses.PipeBothStreams = true
	takes := refuses
	takes.CoprocAfterABar = true
	for _, tc := range []struct {
		src       string
		line, col int
	}{
		{"echo | coproc true\n", 1, 8},
		{"echo hi | coproc { cat; }\n", 1, 11},
		{"echo |& coproc cat\n", 1, 9},
		{"echo |\ncoproc cat\n", 2, 1},
		{"echo | cat | coproc true\n", 1, 14},
		{"if echo | coproc cat; then :; fi\n", 1, 11},
	} {
		_, err := Parse(tc.src, refuses)
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("refuses: %q: err = %v, want a refusal", tc.src, err)
		} else if e.Kind != ErrUnexpected || int(e.Pos.Line) != tc.line || int(e.Pos.Col) != tc.col {
			t.Errorf("refuses: %q: %v at %d:%d, want unexpected at %d:%d",
				tc.src, e.Kind, e.Pos.Line, e.Pos.Col, tc.line, tc.col)
		}
		if _, err := Parse(tc.src, takes); err != nil {
			t.Errorf("takes: %q: %v, want it to parse", tc.src, err)
		}
	}
	// The controls, which parse in both: the first element of a pipeline, a
	// quoted spelling, and a group after the bar beginning a list of its own.
	// And a dialect without the word reads it as a command name anywhere.
	for _, src := range []string{
		"coproc true | cat\n",
		"echo | \"coproc\" true\n",
		"echo | { coproc cat; }\n",
		"echo | (coproc cat)\n",
	} {
		for name, d := range map[string]Dialect{"refuses": refuses, "takes": takes} {
			if _, err := Parse(src, d); err != nil {
				t.Errorf("%s: %q: %v, want it to parse", name, src, err)
			}
		}
	}
	if _, err := Parse("echo | coproc true\n", Core()); err != nil {
		t.Errorf("no coproc word: %v, want a command named coproc", err)
	}
}
