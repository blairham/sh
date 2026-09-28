// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `-` on its own is an operand in three of the panel and an option in the
// fourth, which eats it and leaves the builtin one operand fewer.
//
// Invisible until something looks at the operands: `unset -` is quiet in bash
// because its bare form validates nothing, not because the dash was eaten.
// `unalias -` is where the difference shows, because a shell that eats the
// dash is then left with nothing to unalias.
func TestALoneDashIsAnOperandOrAnOption(t *testing.T) {
	for _, c := range []struct {
		name   string
		option Answer
		asked  bool
	}{
		{"kept, so the builtin is asked about it", No, true},
		{"eaten, so the builtin was given nothing to ask about", Yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Asserted on whether the dash reached the builtin rather than
			// on a wording: what a shell says about being given nothing is
			// its own, and this is about which operands it was given.
			out := loneDashRun(t, "unalias -\n", c.option)
			if asked := strings.Contains(out, "-: not found"); asked != c.asked {
				t.Errorf("said %q; the dash reached the builtin = %v, want %v", out, asked, c.asked)
			}
		})
	}
}

// Only a dash *on its own*, and only where the options are still being read.
func TestOnlyADashOnItsOwn(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// `--` ends the options, so the dash after it is an operand even in
		// the dialect that would otherwise eat one.
		{"a dash after the end of options", "unalias -- -\n", "-: not found"},
		// And the dash is eaten wherever it stands among the options, so
		// what follows it is what the builtin is asked about. Measured:
		// `unset - X` unsets `X` in the shell that eats the dash.
		{"a dash with something after it", "unalias - nope\n", "nope: not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := loneDashRun(t, c.src, Yes); !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
		})
	}
}

// A shell with no answer refuses rather than guessing, because the two
// answers give the builtin different operands.
func TestALoneDashWithNoAnswerIsRefused(t *testing.T) {
	out, st := loneDashRun2(t, "unalias -\n", Unspecified)
	if !strings.Contains(out, "lone `-`") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
	// Refused rather than reported and carried on: naming the axis and then
	// running the builtin anyway would leave it working on operands the two
	// answers disagree about, which is the thing being refused.
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	if strings.Contains(out, "not found") {
		t.Errorf("said %q, want the builtin not to have been asked about anything", out)
	}
}

func loneDashRun(t *testing.T, src string, option Answer) string {
	t.Helper()
	out, _ := loneDashRun2(t, src, option)
	return out
}

func loneDashRun2(t *testing.T, src string, option Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = option
	// A second axis this reaches, and not the one under test: whether the
	// builtin says anything about an alias it does not have.
	sem.UnaliasReportsNotFound = Yes
	// And a third, which decides whether `unalias` has an `-s` at all. No,
	// because the lone dash is the question here and three of the four
	// answer that way.
	sem.SuffixAliases = No
	// And a fourth, for the same reason: `-m` is the other letter `unalias`
	// asks about before it reads a word.
	sem.AliasOperandsCanBePatterns = No
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}

// TestALoneDashEndsTheOptionsRatherThanBeingSkipped: the dash is eaten **and
// the option scan stops there**, which is not the same rule as eating it and
// carrying on.
//
// The two readings agree wherever nothing but operands follows the dash,
// which is every row the axis was first measured from: `unalias -` and
// `unalias - nope` above cannot tell them apart, because there is no option
// behind the dash for the skip to read. What tells them apart is an option
// word *behind* it, and then the skip does that option's work where the
// reference hands the builtin an operand.
//
// Measured 2026-09-28 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m`: *not a Go executable*, from script files under `env -i`:
//
//	v=1; unset - -v v        -v: invalid parameter name
//	read - -r x              not an identifier: -r
//	hash - -r                no such command: -r
//	unalias - -m 'x*'        no such hash table element: -m, then x*
//	alias a=b; alias - -L    1, and silent — exactly as `alias -- -L` is
func TestALoneDashEndsTheOptionsRatherThanBeingSkipped(t *testing.T) {
	for _, c := range []struct{ name, src, ended string }{
		{
			// The letter behind the dash is a *pattern* option to this
			// builtin, so the skip matches nothing and says nothing where
			// the end names the word it was handed.
			"a letter behind the dash is an operand",
			"unalias - -m\n", "-m: not found",
		},
		{
			// And the word behind *that* is then an operand too, rather
			// than the pattern the letter was waiting for.
			"and so is what would have been its argument",
			"unalias - -m nope\n", "nope: not found",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := loneDashPatternRun(t, c.src, Yes); !strings.Contains(out, c.ended) {
				t.Errorf("said %q, want %q in it", out, c.ended)
			}
			// The control: the same letter with no dash in front of it is
			// still read as the option it spells, so what moved is the dash
			// and not the letter.
			bare := strings.Replace(c.src, "- -m", "-m", 1)
			if out := loneDashPatternRun(t, bare, Yes); strings.Contains(out, "-m: not found") {
				t.Errorf("without the dash: said %q, want the letter read as an option", out)
			}
		})
	}
}

// loneDashPatternRun is loneDashRun with `unalias`'s pattern letter present,
// which is what puts an option *behind* the dash for the scan to reach.
func loneDashPatternRun(t *testing.T, src string, option Answer) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = option
	sem.UnaliasReportsNotFound = Yes
	sem.SuffixAliases = No
	sem.AliasOperandsCanBePatterns = Yes
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestEchoReadsALoneDashAsTheEndOfItsOptions: `echo` has its own option
// reader rather than the shared one, so it had never asked the axis at all —
// and the same shell's `print`, which does ask, ate the word. A lone `-`
// handed to `echo` was printed as an operand (#5026).
//
// The `- -n` row is the discriminating one: a reading where the word is
// merely *dropped* would print `hi` with no newline, having read the `-n`
// behind it.
func TestEchoReadsALoneDashAsTheEndOfItsOptions(t *testing.T) {
	for _, c := range []struct{ name, src, option, operand string }{
		{"the word is eaten", `echo - hi`, "hi\n", "- hi\n"},
		{"and the options end there", `echo - -n hi`, "-n hi\n", "- -n hi\n"},
		{"wherever among the options it stands", `echo -n - hi`, "hi", "- hi"},
		{"a second dash is an operand", `echo - - hi`, "- hi\n", "- - hi\n"},
		{"on its own it leaves nothing but the newline", `echo -`, "\n", "-\n"},
		// The controls. `--` is an ordinary operand to `echo` in every
		// column, which is what says this is the word being exactly one
		// dash; and a dash after an operand is past the options already.
		{"two dashes is not this", `echo -- hi`, "-- hi\n", "-- hi\n"},
		{"nor is a dash behind an operand", `echo x - y`, "x - y\n", "x - y\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, func(r *Runner) { r.Semantics.LoneDashIsAnOption = Yes })
			if out != c.option {
				t.Errorf("eaten: %s = %q, want %q", c.src, out, c.option)
			}
			out, _ = run(t, c.src, func(r *Runner) { r.Semantics.LoneDashIsAnOption = No })
			if out != c.operand {
				t.Errorf("kept: %s = %q, want %q", c.src, out, c.operand)
			}
		})
	}
}
