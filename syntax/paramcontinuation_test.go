// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// paramSpanIn returns the first braced parameter expansion in the arguments of
// the only command of src, looking inside double quotes too.
func paramSpanIn(t *testing.T, src string, d syntax.Dialect) syntax.Span {
	t.Helper()
	cmd, ok := onlyCommand(t, src, d).(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	for _, w := range cmd.Args {
		for _, sp := range w.Spans {
			if sp.Kind == syntax.ParamExp {
				return sp
			}
		}
	}
	t.Fatalf("%q holds no parameter expansion", src)
	return syntax.Span{}
}

// A backslash-newline inside `${ }` is a line continuation, removed before the
// expansion is read, wherever it stands in it — inside the name, in front of
// the closing brace, between an operator's characters, in an operand.
//
// Measured 2026-09-16 from script files, and unanimous in bash 5.3, bash 3.2,
// zsh 5.9.2, ksh93u+ and dash for every shape each has: `xy=5; echo
// "[${x\⏎y}]"` is `[5]`, `x=; echo "[${x:\⏎-d}]"` is `[d]`, `x=a.b.c; echo
// "[${x%\⏎%.*}]"` is `[a]`. The scanner stepped over the pair and kept it, so
// every one of these was a bad substitution in every dialect, and the ones
// the pair did not break outright read wrong: `${x#\⏎*.}` stripped nothing
// (#3452).
//
// Asserted on the text and on the tree both: the text is what a diagnostic
// quotes and what an operand is read from, and the tree is what says the
// continuation no longer stands in the reading.
func TestALineContinuationIsRemovedFromAParameterExpansion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, value, param string
		op                      syntax.ParamOp
	}{
		{"inside the name", "echo ${x\\\ny}", "xy", "xy", syntax.ParamNone},
		{"before the closing brace", "echo ${x\\\n}", "x", "x", syntax.ParamNone},
		{"right after the opener", "echo ${\\\nx}", "x", "x", syntax.ParamNone},
		{"between the colon and the operator", "echo ${x:\\\n-d}", "x:-d", "x", syntax.ParamDefault},
		{"between the name and the operator", "echo ${x\\\n:-d}", "x:-d", "x", syntax.ParamDefault},
		{"between a trim's two characters", "echo ${x%\\\n%.*}", "x%%.*", "x", syntax.ParamTrimSuffixLong},
		{"at the front of a pattern", "echo ${x#\\\n*.}", "x#*.", "x", syntax.ParamTrimPrefix},
		{"at the front of a replacement's pattern", "echo ${x/\\\na/c}", "x/a/c", "x", syntax.ParamReplace},
		{"between a replacement's slashes", "echo ${x/\\\n/a/c}", "x//a/c", "x", syntax.ParamReplace},
		{"behind a length's hash", "echo ${#\\\nx}", "#x", "x", syntax.ParamNone},
		{"behind a positional", "echo ${1\\\n2}", "12", "12", syntax.ParamNone},
		{"behind a special parameter", "echo ${@\\\n}", "@", "@", syntax.ParamNone},
		{"inside an operand", "echo ${x-ab\\\ncd}", "x-abcd", "x", syntax.ParamDefault},
		{"inside a subscript", "echo ${a[\\\n1]}", "a[1]", "a", syntax.ParamNone},
		{"inside double quotes", "echo \"[${x\\\n}]\"", "x", "x", syntax.ParamNone},
		{"in a double-quoted operand", "echo ${x-\"a\\\nb\"}", "x-\"ab\"", "x", syntax.ParamDefault},
		{"several of them", "echo ${\\\nx\\\ny\\\n}", "xy", "xy", syntax.ParamNone},
		{"in a nested expansion", "echo ${x-${y\\\n}}", "x-${y}", "x", syntax.ParamDefault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := paramSpanIn(t, tc.src, syntax.Core())
			if sp.Value != tc.value {
				t.Errorf("%q holds %q, want %q", tc.src, sp.Value, tc.value)
			}
			e := sp.Param
			if e == nil || e.Bad {
				t.Fatalf("%q did not read", tc.src)
			}
			if e.Name != tc.param || e.Op != tc.op {
				t.Errorf("%q read as %q with operator %v, want %q with %v", tc.src, e.Name, e.Op, tc.param, tc.op)
			}
		})
	}
}

// What is not the expansion's own continuation stays where it is.
func TestALineContinuationThatIsNotTheExpansionsIsKept(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, value string }{
		// A backslash that is itself escaped makes no continuation: `unset x;
		// echo "[${x-a\\⏎b}]"` holds a backslash and a newline on the whole
		// panel.
		{"an escaped backslash", "echo ${x-a\\\\\nb}", "x-a\\\\\nb"},
		// And a third backslash begins one again: `${x-a\\\⏎b}` is `a\b`.
		{"an escaped backslash and then a continuation", "echo ${x-a\\\\\\\nb}", "x-a\\\\b"},
		// A single-quoted part is literal: `unset x; echo [${x-'a\⏎b'}]` is
		// `[a\⏎b]` in all five.
		{"a single-quoted operand", "echo ${x-'a\\\nb'}", "x-'a\\\nb'"},
		// A command substitution's text is a program, read by its own rules
		// when it runs, and a quoted here-document in it keeps the pair:
		// `unset x; echo "[${x-$(cat <<'E'⏎a\⏎b⏎E⏎)}]"` is `[a\⏎b]` in bash
		// 5.3, zsh, ksh93 and dash.
		{"a quoted here-document inside a command substitution", "echo ${x-$(cat <<'E'\na\\\nb\nE\n)}", "x-$(cat <<'E'\na\\\nb\nE\n)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if sp := paramSpanIn(t, tc.src, syntax.Core()); sp.Value != tc.value {
				t.Errorf("%q holds %q, want %q", tc.src, sp.Value, tc.value)
			}
		})
	}
}

// An unquoted here-document body reads its expansions through the same
// scanner: `x=a.b.c; cat <<E⏎[${x%\⏎%.*}]⏎E` is `[a]` on the whole panel.
func TestALineContinuationIsRemovedFromAParameterExpansionInABody(t *testing.T) {
	t.Parallel()
	spans, err := syntax.HeredocSpans("[${x%\\\n%.*}]\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for _, sp := range spans {
		if sp.Kind == syntax.ParamExp {
			if sp.Value != "x%%.*" {
				t.Errorf("value = %q, want %q", sp.Value, "x%%.*")
			}
			return
		}
	}
	t.Fatal("the body holds no parameter expansion")
}

// Under ParamContinuationNeedsAName a continuation is kept — and the expansion
// is refused with it — until a name has begun: directly behind the `${`, or
// its `#` or `!` prefix, or behind a positional or special parameter. Once an
// operator stands in front of it, or a name has begun, it is removed as
// everywhere else. In a body the flag is not asked: the continuations are
// gone there before the expansion is scanned.
func TestParamContinuationNeedsAName(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ParamContinuationNeedsAName = true
	d.ParamIndirection = true
	for _, tc := range []struct {
		src  string
		kept bool
	}{
		{"echo ${\\\nx}", true},
		{"echo ${#\\\nx}", true},
		{"echo ${!\\\nx}", true},
		{"echo ${#\\\n}", true},
		{"echo ${1\\\n}", true},
		{"echo ${12\\\n}", true},
		{"echo ${@\\\n}", true},
		{"echo ${?\\\n}", true},
		{"echo ${#1\\\n}", true},
		{"echo ${.\\\nsh}", true},
		{"echo \"${@\\\n:-d}\"", true},

		{"echo ${x\\\n}", false},
		{"echo ${ab\\\nc}", false},
		{"echo ${#x\\\n}", false},
		{"echo ${_\\\na}", false},
		{"echo ${@:\\\n-d}", false},
		{"echo ${1:-a\\\nb}", false},
		{"echo ${x\\\n\\\n}", false},
		{"echo ${x-${\\\ny}}", true},
	} {
		t.Run(tc.src, func(t *testing.T) {
			sp := paramSpanIn(t, tc.src, d)
			if held := strings.Contains(sp.Value, "\\\n"); held != tc.kept {
				t.Errorf("%q holds %q: continuation kept = %v, want %v", tc.src, sp.Value, held, tc.kept)
			}
		})
	}
	t.Run("a body", func(t *testing.T) {
		spans, err := syntax.HeredocSpans("[${\\\nx}]\n", d)
		if err != nil {
			t.Fatal(err)
		}
		for _, sp := range spans {
			if sp.Kind == syntax.ParamExp && sp.Value != "x" {
				t.Errorf("value = %q, want %q", sp.Value, "x")
			}
		}
	})
	t.Run("off", func(t *testing.T) {
		if sp := paramSpanIn(t, "echo ${\\\nx}", syntax.Core()); sp.Value != "x" {
			t.Errorf("value = %q, want %q", sp.Value, "x")
		}
	})
}
