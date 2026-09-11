// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// argumentGrammar is zshLike plus the flag that makes a bare `(` a group.
//
// Both are needed and neither is optional for this subject: GlobQualifiers is
// what lets a `(` where an argument may stand belong to the word at all, and
// PatternAlternation is what says the group it opens is readable. The one
// shell with `${(z)v}` has both, so a test that dropped either would be
// measuring a grammar nobody runs — and it would pass by splitting the
// parenthesis off for a reason that has nothing to do with position.
func argumentGrammar() Dialect {
	d := zshLike()
	d.PatternAlternation = true
	return d
}

// A `(` that starts a token belongs to the word where an *argument* may
// stand, and is an operator where a command may begin.
//
// Every row is measured on zsh 5.9.2, the only shell whose grammar has the
// flag this splitter serves, so its answer is the specification here rather
// than one column of a comparison. `${(z)v}` is the spelling measured.
//
// The rows are chosen to separate the readings a splitter could give. The
// first block is argument position reached by every route there is — a plain
// word, a quoted one, a substitution, a redirection in the middle, a second
// group after the first. The second is command position held by every route
// that holds it, and three of those rows are the reason this cannot be a
// state machine over the token stream: `a="x"` has to be known to be an
// assignment, `then` to be a keyword, and a redirection's target word not to
// be a command name. None of the three is visible in a token stream (#1514).
func TestALeadingParenBelongsToTheWordWhereAnArgumentMayStand(t *testing.T) {
	d := argumentGrammar()
	d.PipeBothStreams = true
	d.Select = true

	for _, tc := range []struct {
		name string
		src  string
		want []string
	}{
		// --- argument position: the parenthesis is part of the word ---
		{"after a command word", "a (b c) d", []string{"a", "(b c)", "d"}},
		{"after a command word and nothing else", "echo (b c)", []string{"echo", "(b c)"}},
		{"after an argument", "a 1 (b c)", []string{"a", "1", "(b c)"}},
		{"after a quoted argument", `a "x" (b c)`, []string{"a", `"x"`, "(b c)"}},
		{"after an expansion", "a $x (b c)", []string{"a", "$x", "(b c)"}},
		{"after a substitution", "a $(x) (b c)", []string{"a", "$(x)", "(b c)"}},
		{"after a process substitution", "a <(x) (b c)", []string{"a", "<(x)", "(b c)"}},
		{"after a group of its own", "a b (c d) (e f)", []string{"a", "b", "(c d)", "(e f)"}},
		// A redirection does not end the command, so what follows one is
		// still an argument — including the redirection's own target.
		{"across a redirection", "a >f (b c) d", []string{"a", ">", "f", "(b c)", "d"}},
		{"as a redirection's target", "a > (b c)", []string{"a", ">", "(b c)"}},
		{"across a descriptor", "a 2>&1 (b c)", []string{"a", "2>&", "1", "(b c)"}},
		// An assignment in front of a command word does not stop the word
		// being one, and a word that only looks like an assignment because
		// it is not first is an ordinary argument.
		{"after an assignment and a command", "a=1 b (c d)", []string{"a=1", "b", "(c d)"}},
		{"after an argument holding an equals", "a b=c (d e)", []string{"a", "b=c", "(d e)"}},
		{"after a negation and a command", "! a (b c)", []string{"!", "a", "(b c)"}},
		{"after a loop's word list", "for x in a (b c)", []string{"for", "x", "in", "a", "(b c)"}},
		{"and the group need not be closed", "a (b c", []string{"a", "(b c"}},

		// --- command position: the parenthesis is an operator ---
		{"at the start", "(b c) d", []string{"(", "b", "c", ")", "d"}},
		{"after a terminator", "a; (b c) d", []string{"a", ";", "(", "b", "c", ")", "d"}},
		{"after an and-or", "a && (b c) d", []string{"a", "&&", "(", "b", "c", ")", "d"}},
		{"after an or", "a || (b c)", []string{"a", "||", "(", "b", "c", ")"}},
		{"after a pipe", "a | (b c)", []string{"a", "|", "(", "b", "c", ")"}},
		{"after a both-streams pipe", "a |& (b c)", []string{"a", "|&", "(", "b", "c", ")"}},
		{"after a background operator", "a & (b c)", []string{"a", "&", "(", "b", "c", ")"}},
		// The two rows a token stream cannot answer: `a="x"` is an
		// assignment rather than a command name, and `then` is a keyword
		// rather than one.
		{"after an assignment alone", `a="x" (b c)`, []string{`a="x"`, "(", "b", "c", ")"}},
		{"after a keyword", "if a; then (b c); fi", []string{"if", "a", ";", "then", "(", "b", "c", ")", ";", "fi"}},
		{"after a loop's keyword", "while a; do (b c); done", []string{"while", "a", ";", "do", "(", "b", "c", ")", ";", "done"}},
		{"inside a brace group", "{ (b c) }", []string{"{", "(", "b", "c", ")", "}"}},
		// A redirection written *first* is still command position: nothing
		// has named a command yet.
		{"after a leading redirection", ">f (b c)", []string{">", "f", "(", "b", "c", ")"}},
		{"after a bare time", "time (b c)", []string{"time", "(", "b", "c", ")"}},
		{"inside a condition", "[[ (a) ]]", []string{"[[", "(", "a", ")", "]]"}},
		{
			"in a select's word list", "select x in (b c); do :; done",
			[]string{"select", "x", "in", "(b c)", ";", "do", ":", ";", "done"},
		},

		// --- neither: a `(` that does not start a token was never this ---
		{"a glob qualifier is mid-word", "a *(N) b", []string{"a", "*(N)", "b"}},
		{"and so is a pattern group", "echo a(b|c)", []string{"echo", "a(b|c)"}},
		{"a closing parenthesis alone", "a ) b", []string{"a", ")", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShellWords(tc.src, d, ShellSplit{}); !equalWords(got, tc.want) {
				t.Errorf("ShellWords(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The words a dialect takes away in front of a command hold command position
// too, and each of them is its own grammar rule rather than a word in a list.
//
// They are here rather than in the table above because each needs a flag of
// its own, and because they are the rows that say why the splitter asks the
// parser: `repeat 2` and `foreach x` are two *words* deep and still not an
// argument, which no rule about the previous token can express.
func TestTheWordsThatHoldCommandPositionAreTheParsersOwn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		enable func(*Dialect)
		src    string
		want   []string
	}{
		{
			"a precommand", func(d *Dialect) { d.ReservedPrecommands = map[string]bool{"nocorrect": true} },
			"nocorrect (b c)",
			[]string{"nocorrect", "(", "b", "c", ")"},
		},
		{
			"a count, which is a word and not a command", func(d *Dialect) { d.Repeat = true },
			"repeat 2 (b c)",
			[]string{"repeat", "2", "(", "b", "c", ")"},
		},
		{
			"a coprocess", func(d *Dialect) { d.Coproc = true },
			"coproc (b c)",
			[]string{"coproc", "(", "b", "c", ")"},
		},
		{
			// The name after `foreach` is a name and the `(` after *it*
			// opens the loop's word list, so neither is an argument.
			"a loop whose list is parenthesized", func(d *Dialect) { d.Foreach = true },
			"foreach x (a b)",
			[]string{"foreach", "x", "(", "a", "b", ")"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := argumentGrammar()
			tc.enable(&d)
			if got := ShellWords(tc.src, d, ShellSplit{}); !equalWords(got, tc.want) {
				t.Errorf("ShellWords(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// Driving the parser must not make the splitter read a here-document's body,
// and must not make an unfinished value an error.
//
// Both are properties of splitting a *value* rather than parsing a program,
// and both are things a parser does by default — so each has a row here,
// beside the one in the main table, because they are what the two switches in
// ShellWords exist for.
func TestTheSplitterReadsAValueRatherThanAProgram(t *testing.T) {
	d := argumentGrammar()
	for _, tc := range []struct {
		name string
		src  string
		opt  ShellSplit
		want []string
	}{
		{
			// The body lines are more of the same value, not a body.
			"a here-document operator takes no body", "a <<EOF\nbody\nEOF\nb",
			ShellSplit{NewlineIsBlank: true},
			[]string{"a", "<<", "EOF", "body", "EOF", "b"},
		},
		{
			"and a quoted delimiter takes none either", "a <<'EOF'\nbody\nEOF",
			ShellSplit{NewlineIsBlank: true},
			[]string{"a", "<<", "'EOF'", "body", "EOF"},
		},
		{"input that ends inside a quote is not an error", `a "b c`, ShellSplit{}, []string{"a", `"b c`}},
		{"nor inside a substitution", "a $(b c", ShellSplit{}, []string{"a", "$(b c"}},
		{"nor inside an expansion", "a ${b", ShellSplit{}, []string{"a", "${b"}},
		// Text that is not a program at all still comes back as its words.
		// The parser refuses each of these; the splitter keeps what the
		// lexer read and lexes the rest plainly.
		{"a value that is not a program", "a ;; b", ShellSplit{}, []string{"a", ";;", "b"}},
		{"an unbalanced keyword", "done a b", ShellSplit{}, []string{"done", "a", "b"}},
		{"an operator with nothing after it", "a &&", ShellSplit{}, []string{"a", "&&"}},
		{"an operator with nothing before it", "&& a", ShellSplit{}, []string{"&&", "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShellWords(tc.src, d, tc.opt); !equalWords(got, tc.want) {
				t.Errorf("ShellWords(%q, %+v) = %q, want %q", tc.src, tc.opt, got, tc.want)
			}
		})
	}
}
