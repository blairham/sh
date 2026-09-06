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
