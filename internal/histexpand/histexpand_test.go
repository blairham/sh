// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package histexpand

import (
	"errors"
	"testing"
)

// The rows below are not invented. Each was run against bash 5.3.20 on
// 2026-09-15 with the same one-line history seeded — a script holding
//
//	set -o history
//	set -H
//	echo one two three
//	<the row>
//
// — and the "want" column is what bash echoed to **standard error**, which is
// the expanded line it was about to run. A row whose want is the input is one
// bash echoed nothing for, which is how it says the expansion changed nothing.
//
// That is the only honest way to write this table. Reading the manual would
// have produced a table that agrees with the manual.
func TestTheDesignatorsBashWasMeasuredExpanding(t *testing.T) {
	h := List{Lines: []string{"echo one two three"}, First: 1}
	for _, c := range []struct{ in, want string }{
		// Event designators.
		{"echo !!", "echo echo one two three"},
		{"echo !1", "echo echo one two three"},
		{"echo !-1", "echo echo one two three"},
		{"echo !e", "echo echo one two three"},
		{"echo !?two?", "echo echo one two three"},
		// Word designators, on the previous event with no event written.
		{"echo !$", "echo three"},
		{"echo !^", "echo one"},
		{"echo !*", "echo one two three"},
		{"echo !:0", "echo echo"},
		{"echo !:1", "echo one"},
		{"echo !:2-3", "echo two three"},
		// Quick substitution, and the `s` modifier it is shorthand for.
		{"^one^ONE^", "echo ONE two three"},
		{"echo !!:s/one/1/", "echo echo 1 two three"},
		// The quoting rules, which are the scanner's and not the parser's.
		{"echo '!!'", "echo '!!'"},
		{`echo "!!"`, `echo "echo one two three"`},
		{`echo a\!b`, `echo a\!b`},
		// The three contexts where a `!` is something else. `[!` opens a
		// bracket expression's negation and `${!` an indirect expansion; a
		// `!` against one of the operators a command line is built from, or
		// at the end of the line, is ordinary text.
		{"echo [!a]", "echo [!a]"},
		{"echo ${!v}", "echo ${!v}"},
		{"echo end!", "echo end!"},
		{"echo hi ! there", "echo hi ! there"},
		{"echo x!;y", "echo x!;y"},
		{"echo x!=y", "echo x!=y"},
		{"echo x!(y)", "echo x!(y)"},
	} {
		got, err := Expand(c.in, h, Default)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got.Line != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.in, got.Line, c.want)
		}
		// And the other half of every row: a line bash echoed is a line it
		// says changed, so the flag and the text have to agree.
		if want := c.in != c.want; got.Changed != want {
			t.Errorf("%q: Changed = %v, want %v", c.in, got.Changed, want)
		}
	}
}

// A `!` inside double quotes expands and a `'` inside those double quotes
// opens nothing — measured, `echo "it's !!"` is expanded where `echo '!!'` is
// not. The pair is the whole reason the scanner tracks two states instead of
// one "in quotes" flag.
func TestASingleQuoteInsideDoubleQuotesOpensNothing(t *testing.T) {
	h := List{Lines: []string{"echo LINE"}, First: 1}
	got, err := Expand(`echo "it's !!"`, h, Default)
	if err != nil {
		t.Fatal(err)
	}
	if want := `echo "it's echo LINE"`; got.Line != want {
		t.Errorf("got %q, want %q", got.Line, want)
	}
}

// The modifiers that take a path apart, measured on the same shell against
// `echo /a/b/c.txt other`.
func TestThePathModifiers(t *testing.T) {
	h := List{Lines: []string{"echo /a/b/c.txt other"}, First: 1}
	for _, c := range []struct{ in, want string }{
		{"echo !:1:h", "echo /a/b"},
		{"echo !:1:t", "echo c.txt"},
		{"echo !:1:r", "echo /a/b/c"},
		{"echo !:1:e", "echo .txt"},
	} {
		got, err := Expand(c.in, h, Default)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got.Line != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got.Line, c.want)
		}
	}
}

// `:p` is the one modifier whose result is not run. It is how a person looks
// at what a reference resolves to before trusting it with `rm`, so the flag
// has to reach the caller rather than being folded into the text.
func TestPrintModifierAsksForTheLineNotToRun(t *testing.T) {
	h := List{Lines: []string{"rm -rf /tmp/scratch"}, First: 1}
	got, err := Expand("!!:p", h, Default)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Print {
		t.Error("Print is false, so the line would have run")
	}
	if want := "rm -rf /tmp/scratch"; got.Line != want {
		t.Errorf("got %q, want %q", got.Line, want)
	}
}

// A reference nothing matches is an error and not an empty expansion, which is
// the difference between a shell that says so and a shell that silently runs
// `echo` with no arguments.
func TestAReferenceNothingMatchesIsRefused(t *testing.T) {
	h := List{Lines: []string{"echo one"}, First: 1}
	for _, in := range []string{"echo !nosuch", "echo !?nothinglikethis?", "echo !9"} {
		_, err := Expand(in, h, Default)
		var nf *NotFound
		if !errors.As(err, &nf) {
			t.Errorf("%q: err = %v, want a NotFound", in, err)
		}
	}
	// And a substitution whose left side is not there fails rather than
	// leaving the line alone: measured, `^nope^x^` is `substitution failed`
	// in bash, ksh93 and zsh alike and the line does not run.
	_, err := Expand("^nope^x^", h, Default)
	var sf *SubstFailed
	if !errors.As(err, &sf) {
		t.Fatalf("err = %v, want a SubstFailed", err)
	}
	// bash names the modifier it rewrote the line into and ksh93 names what
	// was typed, so the error carries both.
	if sf.Ref != ":s^nope^x^" || sf.Bare != "^nope^x^" {
		t.Errorf("Ref = %q, Bare = %q", sf.Ref, sf.Bare)
	}
}

// An empty history is not an error to look at, it is an error to *index*: the
// engine must not hand back an empty string for `!!` in a fresh session.
func TestAnEmptyHistoryRefusesEveryReference(t *testing.T) {
	var h List
	h.First = 1
	if _, err := Expand("echo !!", h, Default); err == nil {
		t.Error("`!!` against an empty history was granted")
	}
	// And a line with no reference in it still goes through untouched.
	got, err := Expand("echo plain", h, Default)
	if err != nil {
		t.Fatal(err)
	}
	if got.Line != "echo plain" || got.Changed {
		t.Errorf("got %q changed=%v", got.Line, got.Changed)
	}
}

// The numbering is absolute and does not restart when old entries fall off the
// front, which is why List carries First. A list that had lost its first two
// entries and answered `!3` with its own third line would be quietly running
// the wrong command.
func TestTheNumberingSurvivesATrimmedList(t *testing.T) {
	h := List{Lines: []string{"echo three", "echo four"}, First: 3}
	got, err := Expand("!3", h, Default)
	if err != nil {
		t.Fatal(err)
	}
	if want := "echo three"; got.Line != want {
		t.Errorf("got %q, want %q", got.Line, want)
	}
	if _, err := Expand("!1", h, Default); err == nil {
		t.Error("`!1` was granted against a list whose first entry is 3")
	}
}

// `histchars` moves all three characters, and an empty value turns the whole
// feature off. Both were measured before anything read the parameter — see
// docs/spec/history.md.
func TestHistcharsMovesTheCharacters(t *testing.T) {
	h := List{Lines: []string{"echo one two"}, First: 1}
	c := Chars{Event: ',', Quick: '%', Comment: '#'}
	got, err := Expand("echo ,,", h, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := "echo echo one two"; got.Line != want {
		t.Errorf("got %q, want %q", got.Line, want)
	}
	// And the character it moved *from* is now ordinary text.
	got, err = Expand("echo !!", h, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Line != "echo !!" || got.Changed {
		t.Errorf("got %q changed=%v, want the old character left alone", got.Line, got.Changed)
	}
	// An empty value is the off switch.
	got, err = Expand("echo !!", h, Chars{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Line != "echo !!" {
		t.Errorf("got %q, want an empty histchars to expand nothing", got.Line)
	}
}
