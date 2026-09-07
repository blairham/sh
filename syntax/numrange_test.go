// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// numRange is the core with the one flag this file is about.
func numRange() Dialect {
	d := Core()
	d.NumericRangePattern = true
	return d
}

// TestANumericRangeIsAWordAndNotARedirection — `<->` and its three bounded
// spellings are pattern text where the dialect has them, and the `<` is the
// redirection it is everywhere else where it does not.
//
// The four shapes are one operator with either bound left out, so all four
// are asserted rather than the bare one standing in for the rest.
func TestANumericRangeIsAWordAndNotARedirection(t *testing.T) {
	on, off := numRange(), Core()
	for _, src := range []string{
		`[[ 1 = <-> ]]`,
		`[[ 1 = <1-9> ]]`,
		`[[ 1 = <2-> ]]`,
		`[[ 1 = <-9> ]]`,
		`case 42 in <->) echo num;; esac`,
		`echo <->`,
		`[[ $s = <->" "<-> ]]`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
		if _, err := Parse(src, off); err == nil {
			t.Errorf("%s: parsed without the flag, where the `<` is a redirection", src)
		}
	}
	// Mid-word and at the end of one, taken into the word rather than
	// ending it.
	for _, tc := range []struct{ src, want string }{
		{`echo x<->`, "x<->"},
		{`echo <->x`, "<->x"},
	} {
		if got := lastWordText(t, parsed(t, tc.src, on), tc.src); got != tc.want {
			t.Errorf("%s: last word = %q, want %q", tc.src, got, tc.want)
		}
	}
	// And the one of those two where "did it parse" cannot answer: without
	// the flag `echo <->x` is `echo` reading the file `-` and writing the
	// file `x`, which parses perfectly well and is a different program.
	cmd := onlySimpleCommand(t, parsed(t, `echo <->x`, off), `echo <->x`)
	if len(cmd.Redirs) != 2 || len(cmd.Args) != 1 {
		t.Errorf("`echo <->x` without the flag: %d words and %d redirections, want 1 and 2",
			len(cmd.Args), len(cmd.Redirs))
	}
}

// parsed is Parse with the failure reported rather than returned, for the
// assertions that need the tree and not only the verdict.
func parsed(t *testing.T, src string, d Dialect) *File {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return f
}

// TestOnlyTheMeasuredShapeIsARange — everything else keeps the `<` it always
// had. Measured 2026-09-05 on zsh 5.9.2: `echo <1` reads the file `1`,
// `echo <a-b>` is a redirection followed by a parse error at the `>`, and
// `<1-2-3>`, `<-->` and `<>` are parse errors there too. The shape is the
// whole of the disambiguation.
func TestOnlyTheMeasuredShapeIsARange(t *testing.T) {
	on := numRange()
	// A redirection with a target still parses; it is just not a pattern.
	if _, err := Parse(`echo <1`, on); err != nil {
		t.Errorf("`echo <1` should be a redirection from the file 1: %v", err)
	}
	for _, src := range []string{
		`echo <a-b>`,
		`echo <1-2-3>`,
		`echo <-->`,
		`echo <->>`,
		`echo <`,
		// The `-` is required and it is a `-` rather than "some
		// separator": dropping its test from numericRangeAt admits any
		// single character in its place, and that survived the whole suite
		// until these rows. Measured on zsh 5.9.2, each in a script file of
		// its own under `env -i`: all four are parse errors there.
		`echo <12>`,
		`echo <a>`,
		`echo <1x2>`,
		`echo <1.2>`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: read as a range, where only `<` digits `-` digits `>` is one", src)
		}
	}
	// And a quoted one is four characters: quoting is what decides whether
	// text is a pattern, here as everywhere.
	for _, src := range []string{`echo "<->"`, `echo '<->'`, `echo \<-\>`} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: a quoted range should be ordinary text: %v", src, err)
		}
	}
}

// TestDigitsInFrontOfANumericRangeAreNotADescriptor — digits with no gap
// before a `<` are a file descriptor, and stop being one when the operator
// turns out to be a pattern. The same for the braced name the shell would
// pick a descriptor for.
//
// Asserted as the word the scanner produced rather than as "it parsed",
// because `echo 2<->` parses either way: as one word, or as `echo` with a
// redirection, which is a different program with the same text.
func TestDigitsInFrontOfANumericRangeAreNotADescriptor(t *testing.T) {
	on := numRange()
	for _, tc := range []struct{ src, want string }{
		{`echo 2<->`, "2<->"},
		{`echo {a}<->`, "{a}<->"},
		{`echo 10<->`, "10<->"},
	} {
		got := lastWordText(t, parsed(t, tc.src, on), tc.src)
		if got != tc.want {
			t.Errorf("%s: last word = %q, want %q", tc.src, got, tc.want)
		}
	}
	// The boundary: with an ordinary redirection after them the digits are a
	// descriptor again, so the command has one word and a redirection.
	cmd := onlySimpleCommand(t, parsed(t, `echo 2<f`, on), `echo 2<f`)
	if len(cmd.Args) != 1 || len(cmd.Redirs) != 1 {
		t.Errorf("`echo 2<f`: %d words and %d redirections, want 1 and 1", len(cmd.Args), len(cmd.Redirs))
	}
	// And a range missing only its closing `>` is a redirection from a file
	// whose name happens to hold a dash. "Did it parse" cannot say so —
	// both readings parse — so it is asserted as the shape, which is what
	// makes it the row that catches a scanner reading a width past the end
	// of the input. Measured on zsh 5.9.2: `cat <1-2` prints the contents of
	// a file called `1-2`.
	open := onlySimpleCommand(t, parsed(t, `echo <1-2`, on), `echo <1-2`)
	if len(open.Args) != 1 || len(open.Redirs) != 1 {
		t.Errorf("`echo <1-2`: %d words and %d redirections, want 1 and 1",
			len(open.Args), len(open.Redirs))
	}
}

// lastWordText is the text of a simple command's final word.
func lastWordText(t *testing.T, f *File, src string) string {
	t.Helper()
	cmd := onlySimpleCommand(t, f, src)
	if len(cmd.Args) == 0 {
		t.Fatalf("%s: no words", src)
	}
	w := cmd.Args[len(cmd.Args)-1]
	var b []byte
	for _, s := range w.Spans {
		b = append(b, s.Value...)
	}
	return string(b)
}

// onlySimpleCommand is the one command a single-line source parsed to.
func onlySimpleCommand(t *testing.T, f *File, src string) *SimpleCmd {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%s: %d statements, want 1", src, len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok {
		t.Fatalf("%s: %T, want a pipeline", src, f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%s: %T, want a simple command", src, pipe.Cmds[0])
	}
	return cmd
}

// TestANumericRangePrintsBackAsAPattern — the printer's promise is that
// printed source *means* the same thing, and escaping a range's `<` breaks it
// silently: `echo \\<-\\>` parses and prints three characters where the source
// named every numbered file. The same rule `*` and `{1..3}` already have.
//
// It cannot be caught by the corpus's round trip, because every case that
// writes a bare range is a syntax error under the dialect that round trip
// uses. Asserted here on the whole printed line.
func TestANumericRangePrintsBackAsAPattern(t *testing.T) {
	on := numRange()
	for _, tc := range []struct{ src, want string }{
		{`echo <->`, "echo <->"},
		{`echo <1-9>`, "echo <1-9>"},
		{`echo 2<->`, "echo 2<->"},
		{`echo a<->b`, "echo a<->b"},
		{`[[ 1 = <-> ]]`, "[[ 1 = <-> ]]"},
		// Quoting survives too, and means the printed form is quoted: a
		// quoted range is ordinary text and has to stay ordinary text.
		{`echo "<->"`, `echo "<->"`},
		{`echo '<->'`, `echo '<->'`},
		// And the shapes that are not ranges keep the escaping they need:
		// `<` is a redirection operator, so a literal one in a word is not
		// safe bare.
		{`echo "<-"`, `echo "<-"`},
		{`echo "a<b"`, `echo "a<b"`},
	} {
		got := Print(parsed(t, tc.src, on))
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
		}
		if _, err := Parse(got, on); err != nil {
			t.Errorf("%s: printed %q, which does not parse: %v", tc.src, got, err)
		}
	}
	// The one that says the escaping is still there when it is needed: a
	// literal `<` that is not part of a range comes back escaped, so the
	// printed word is still one word.
	f := &File{Stmts: []*Stmt{{Expr: &Pipeline{Cmds: []Command{&SimpleCmd{Args: []*Word{
		{Spans: []Span{{Kind: Literal, Value: "echo"}}},
		{Spans: []Span{{Kind: Literal, Value: "a<b"}}},
	}}}}}}}
	if got := Print(f); got != `echo a\<b` {
		t.Errorf("a bare `<` printed as %q, want %q", got, `echo a\<b`)
	}
}

// numRangeInGroup is the core with the two flags a range inside a
// parenthesised group needs from a *condition*: the range itself, and the
// bare pattern groups that give a `(` to be inside of.
func numRangeInGroup() Dialect {
	d := numRange()
	d.PatternAlternation = true
	return d
}

// numRangeInWordGroup adds the third flag, which the other two routes into
// the same scanner need: a `(` that starts a word where an *argument* may
// stand, which is also how a `case` arm's pattern gets one — the arm hands
// its patterns the argument position after taking its own paren.
func numRangeInWordGroup() Dialect {
	d := numRangeInGroup()
	d.GlobQualifiers = true
	return d
}

// TestANumericRangeIsReachedFromInsideALeadingGroup — a group that *starts*
// a pattern is read by the scanner that stops at `;`, `<`, `>` and `&`, and
// a range's `<` is none of those: it is pattern text, so the group has to
// carry it through.
//
// The discriminating pair is the nesting and nothing else. A bare range
// parsed, and a group without one parsed; only the range inside the group
// did not, which is the shape every powerlevel10k config's version gate is
// written in (#1217).
func TestANumericRangeIsReachedFromInsideALeadingGroup(t *testing.T) {
	on := numRangeInGroup()
	for _, src := range []string{
		// The controls: each half alone, which always worked.
		`[[ $k == <1-> ]]`,
		`[[ $k == (a|b.*) ]]`,
		`[[ $k == a(<6->) ]]`,
		// The gap: a range inside a group that starts the operand, in all
		// four spellings.
		`[[ $k == (<->) ]]`,
		`[[ $k == (<1-9>) ]]`,
		`[[ $k == (<6->) ]]`,
		`[[ $k == (<-9>) ]]`,
		// An alternation of them, which is the real gate.
		`[[ $k == (5.<1->*|<6->.*) ]]`,
		// And a group nested inside the leading one, whose `<` the outer
		// scanner is the one that has to carry.
		`[[ $k == (x(<6->)) ]]`,
		`[[ $k == ((<6->)|x) ]]`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where both flags are on: %v", src, err)
		}
	}
}

// TestTheSameScannerIsReachedFromACaseArmAndAnArgument — the other two
// routes into scanGroupSpans. They are asserted separately because they
// need a *different* flag from the condition: a `case` arm and an argument
// reach the scanner through the leading-paren rule rather than through
// `inPattern`, so a fix that only satisfied the condition would leave these
// refusing. That is what says the gap was the scanner's and not `[[ ]]`'s.
func TestTheSameScannerIsReachedFromACaseArmAndAnArgument(t *testing.T) {
	on := numRangeInWordGroup()
	for _, src := range []string{
		// A `case` arm's own paren, then a group: `((`.
		`case $k in ((<6->)) echo up;; esac`,
		`case $k in ((5.<1->*|<6->.*)) echo v;; esac`,
		// The control that always worked, in the same position.
		`case $k in ((a|b)) echo alt;; esac`,
		// An argument whose word a group starts.
		`echo (<5-6>)`,
		`echo (v<5-6>)`,
		`echo (v5|v6)`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the word-leading group flag is on: %v", src, err)
		}
	}
}

// TestOnlyARangeSurvivesALeadingGroupsOperators — the exception is the range
// shape and nothing wider. Every other `<` still ends the word where a group
// starts one, which is the guard that keeps this from being "a leading group
// keeps its operators" — a rule the shells measurably do not have.
//
// Measured on zsh 5.9.2, 2026-09-07, each probe in a script file of its own
// under `env -i`: `(a<b)`, `(a>b)`, `(a;b)`, `(a&b)`, `(<a-b>)`,
// `(<1-2-3>)`, `(<-->)`, `(<>)` and `(<1)` are all parse errors there, and
// only the nine-row set makes the assertion discriminating — a `Parse`
// succeeding on any of them would read as a fix.
func TestOnlyARangeSurvivesALeadingGroupsOperators(t *testing.T) {
	on := numRangeInGroup()
	for _, src := range []string{
		// The four characters that end the word, unchanged.
		`[[ $k == (a<b) ]] && echo hit`,
		`[[ $k == (a>b) ]] && echo hit`,
		`[[ $k == (a;b) ]] && echo hit`,
		`[[ $k == (a&b) ]] && echo hit`,
		// And the near-ranges, which are not ranges: the shape is `<`
		// digits `-` digits `>` exactly, so each of these is still a `<`
		// that ends the word and leaves the `)` with nowhere to go.
		`[[ $k == (<a-b>) ]] && echo hit`,
		`[[ $k == (<1-2-3>) ]] && echo hit`,
		`[[ $k == (<-->) ]] && echo hit`,
		`[[ $k == (<>) ]] && echo hit`,
		`[[ $k == (<1) ]] && echo hit`,
		// The two that say each half of the shape is load-bearing, and the
		// two the mutation run needed: `(<1-2)` is what a dropped
		// closing-`>` test would admit and `(<12>)` what a dropped `-`
		// test would. Both are parse errors in zsh 5.9.2 as well.
		`[[ $k == (<1-2) ]] && echo hit`,
		`[[ $k == (<12>) ]] && echo hit`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where the `<` is not a range and ends the word", src)
		}
	}
}

// TestARangeInALeadingGroupPrintsBackAsAPattern — printed source has to mean
// the same thing, and the round trip is where a scanner that took the group
// as text but printed it escaped would show up.
//
// The condition route only. The other two routes lose the group's
// parentheses to escaping — `echo (v5|v6)` prints as `echo \(v5\|v6\)`
// and a `case` arm's `((a|b))` as `\(a\|b\))`, which parses, runs and
// answers the opposite question at status 0. That is #1221, it is on
// `origin/main` without this change and without a range in sight, so it is
// filed rather than asserted here.
func TestARangeInALeadingGroupPrintsBackAsAPattern(t *testing.T) {
	on := numRangeInGroup()
	for _, tc := range []struct{ src, want string }{
		{`[[ $k == (<6->) ]]`, `[[ $k == (<6->) ]]`},
		{`[[ $k == (5.<1->*|<6->.*) ]]`, `[[ $k == (5.<1->*|<6->.*) ]]`},
		{`[[ $k == (x(<6->)) ]]`, `[[ $k == (x(<6->)) ]]`},
		{`[[ $k == ((<6->)|x) ]]`, `[[ $k == ((<6->)|x) ]]`},
	} {
		got := Print(parsed(t, tc.src, on))
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
		}
		if _, err := Parse(got, on); err != nil {
			t.Errorf("%s: printed %q, which does not parse: %v", tc.src, got, err)
		}
	}
}
