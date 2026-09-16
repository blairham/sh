// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// DeclarationCommandWord decides whether a declaration utility's operand is an
// assignment only when the utility's name was written as the command word, or
// whenever the utility runs — and the three readings split the rows three
// ways, which is what makes it an enum rather than an Answer.
//
// The literal row is the control: it keeps the value whole under every
// reading, so a runner that ignored the axis entirely would fail every other
// row under two of the three.
func TestADeclarationIsRecognizedByTheWordAsTheAxisSays(t *testing.T) {
	for _, row := range []struct {
		name, line string
		// splits names the readings under which the value is split, so a row
		// listed for neither is a row that keeps `x y` whole everywhere.
		splits []DeclarationCommandWordReading
	}{
		{"the literal word", `export v=$b`, nil},
		{"an expansion", `cmd=export; $cmd v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord, DeclarationByWrittenWord,
		}},
		{"an empty expansion in front", `e=; $e export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord, DeclarationByWrittenWord,
		}},
		{"a backslash", `\export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord,
		}},
		{"single quotes", `'export' v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord,
		}},
		{"double quotes", `"export" v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord,
		}},
		{"a quoted part", `ex'port' v=$b`, []DeclarationCommandWordReading{
			DeclarationByUnquotedLiteralWord,
		}},
		{"a quoted part of the value is not the word", `export v="x y"`, nil},
	} {
		for _, reading := range readings() {
			t.Run(row.name, func(t *testing.T) {
				want := "[x y]"
				for _, r := range row.splits {
					if r == reading {
						want = "[x]"
					}
				}
				if got := declarationValue(t, row.line, func(s *Semantics) {
					s.DeclarationCommandWord = reading
				}); got != want {
					t.Errorf("reading %d: %s wrote %q, want %q", reading, row.line, got, want)
				}
			})
		}
	}
}

// CommandPrefixKeepsADeclaration is asked only where a declaration utility
// stands behind `command`, and it is asked under each reading of how the
// words have to be written — so the two axes are exercised together, which is
// how the panel reaches them.
//
// `command -p` is the row that is not a second axis: under the reading keyed
// on the utility's name the flag is part of the prefix, because it changes
// where the utility is looked for and not which one runs, and under the
// readings keyed on the written word it is not the prefix as written.
func TestACommandPrefixKeepsADeclarationWhereTheAxisSaysSo(t *testing.T) {
	for _, row := range []struct {
		name, line string
		// keptUnder names the readings under which this spelling is still a
		// declaration when the prefix axis says the rule survives at all.
		keptUnder []DeclarationCommandWordReading
	}{
		{"one prefix", `command export v=$b`, readings()},
		{"two prefixes", `command command export v=$b`, readings()},
		{"a quoted prefix", `\command export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUtilityName, DeclarationByWrittenWord,
		}},
		{"a quoted utility", `command \export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUtilityName, DeclarationByWrittenWord,
		}},
		{"an expanded prefix", `c=command; $c export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUtilityName,
		}},
		{"an expanded utility", `cmd=export; command $cmd v=$b`, []DeclarationCommandWordReading{
			DeclarationByUtilityName,
		}},
		{"the -p flag", `command -p export v=$b`, []DeclarationCommandWordReading{
			DeclarationByUtilityName,
		}},
	} {
		for _, reading := range readings() {
			for _, keeps := range []Answer{Yes, No} {
				t.Run(row.name, func(t *testing.T) {
					want := "[x]"
					for _, r := range row.keptUnder {
						if r == reading && keeps == Yes {
							want = "[x y]"
						}
					}
					if got := declarationValue(t, row.line, func(s *Semantics) {
						s.DeclarationCommandWord = reading
						s.CommandPrefixKeepsADeclaration = keeps
					}); got != want {
						t.Errorf("reading %d, prefix %v: %s wrote %q, want %q",
							reading, keeps, row.line, got, want)
					}
				})
			}
		}
	}
}

// A `command` that has no declaration utility behind it never asks the axis,
// which is what keeps an unanswered one off the common path: the shell below
// leaves it Unspecified, and a script that would consult it reports rather
// than guessing.
func TestTheCommandPrefixAxisIsAskedOnlyBehindADeclaration(t *testing.T) {
	sem := PosixSemantics()
	sem.DeclarationCommandWord = DeclarationByWrittenWord
	sem.CommandPrefixKeepsADeclaration = Unspecified
	out, status := run(t, "b='x y'\ncommand echo v=$b",
		func(r *Runner) { r.Semantics = &sem })
	if got := strings.TrimSpace(out); got != "v=x y" || status != 0 {
		t.Errorf("wrote %q at %d, want %q at 0", got, status, "v=x y")
	}
	out, _ = run(t, "b='x y'\ncommand export v=$b\necho \"[$v]\"",
		func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "run through `command`") || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("a declaration behind the prefix wrote %q, want the unanswered axis reported", out)
	}
	if strings.Contains(out, "[x") {
		t.Errorf("a declaration behind the prefix wrote %q, want the command refused rather than run", out)
	}
}

func readings() []DeclarationCommandWordReading {
	return []DeclarationCommandWordReading{
		DeclarationByUtilityName,
		DeclarationByUnquotedLiteralWord,
		DeclarationByWrittenWord,
	}
}

// declarationValue runs one declaration line with `b` holding two words and
// reports what the declared parameter ended up with — `[x y]` where the
// operand was an assignment and `[x]` where it was split into fields.
func declarationValue(t *testing.T, line string, answer func(*Semantics)) string {
	t.Helper()
	sem := PosixSemantics()
	answer(&sem)
	out, _ := run(t, "b='x y'\n"+line+"\n"+`echo "[$v]"`,
		func(r *Runner) { r.Semantics = &sem })
	return strings.TrimSpace(out)
}
