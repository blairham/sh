// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// short is the core plus the short-loop family. The flag is named here and the
// shell that sets it is not: what belongs to a dialect lives under dialect/.
//
// CloseBraceAlwaysReserved travels with it in every preset that has either, and
// it is what lets `{ echo $i }` close without a terminator — so leaving it out
// would test the family through a brace rule it never meets in practice.
func short() syntax.Dialect {
	d := syntax.Core()
	d.ShortLoop = true
	d.CloseBraceAlwaysReserved = true
	return d
}

func onlyCommand(t *testing.T, src string, d syntax.Dialect) syntax.Command {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if len(f.Stmts) == 0 {
		t.Fatalf("parse %q: no statements", src)
	}
	last := f.Stmts[len(f.Stmts)-1]
	pipe, ok := last.Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("parse %q: not one command", src)
	}
	return pipe.Cmds[0]
}

// A header that has ended may be followed straight by the body, and the body is
// one command.
//
// One and not a list, which is the construct rather than a shortcut: a second
// command needs a separator, and a separator there belongs to whatever encloses
// the loop.
func TestAShortBodyIsOneCommand(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		cond int // statements in a while/until condition, 0 for a `for`
		body int
	}{
		{"a while with an arithmetic condition", "while (( i < 2 )) echo hi", 1, 1},
		{"an until with the same", "until (( i > 2 )) echo hi", 1, 1},
		{"a condition that is a test", "while [[ -n $x ]] echo hi", 1, 1},
		{"a brace group as the one command", "while (( 0 )) { echo hi; }", 1, 1},
		{"a subshell as the one command", "while (( 0 )) (echo hi)", 1, 1},
		{"a for over a parenthesized list", "for i (a b) echo $i", 0, 1},
		{"the same with a brace group", "for i (a b) { echo $i; }", 0, 1},
		{"a separator before the body is allowed", "for i (a b); echo $i", 0, 1},
		{"and after an `in` list, where one is required", "for i in a b; echo $i", 0, 1},
		{"a C-style header, which ends itself", "for ((i=0;i<2;i++)) echo $i", 0, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := onlyCommand(t, c.src, short())
			switch x := cmd.(type) {
			case *syntax.LoopClause:
				if len(x.Cond) != c.cond || len(x.Body) != c.body {
					t.Errorf("%q: %d condition and %d body statements, want %d and %d",
						c.src, len(x.Cond), len(x.Body), c.cond, c.body)
				}
			case *syntax.ForClause:
				if len(x.Body) != c.body {
					t.Errorf("%q: %d body statements, want %d", c.src, len(x.Body), c.body)
				}
			case *syntax.ForArithClause:
				if len(x.Body) != c.body {
					t.Errorf("%q: %d body statements, want %d", c.src, len(x.Body), c.body)
				}
			default:
				t.Fatalf("%q: parsed as %T", c.src, cmd)
			}
		})
	}
}

// One command and not a list, which only a *following* statement can show: with
// nothing after the loop, "one" and "as many as there are" read the same.
//
// Reading a list here leaves the text meaning something else rather than
// failing — `for i (a b) echo $i; echo end` would print end twice — and the
// whole suite stayed green when the body was widened to a list, because every
// other case here ends at the loop.
func TestWhatFollowsAShortBodyIsOutsideTheLoop(t *testing.T) {
	for _, c := range []struct {
		src   string
		stmts int // statements at the top level
		body  int
	}{
		{"for i (a b) echo $i; echo end", 2, 1},
		{"while (( i < 2 )) echo hi; echo end", 2, 1},
		{"for i (a b) { echo $i; }; echo end", 2, 1},
		{"for i in a b; echo $i; echo end", 2, 1},
		{"for ((i=0;i<2;i++)) echo $i; echo end", 2, 1},
		// A newline separates as a `;` does, and the same rule applies.
		{"for i (a b) echo $i\necho end", 2, 1},
	} {
		t.Run(c.src, func(t *testing.T) {
			f, err := syntax.Parse(c.src, short())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(f.Stmts) != c.stmts {
				t.Fatalf("%d statements at the top level, want %d", len(f.Stmts), c.stmts)
			}
			var body int
			switch x := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(type) {
			case *syntax.ForClause:
				body = len(x.Body)
			case *syntax.LoopClause:
				body = len(x.Body)
			case *syntax.ForArithClause:
				body = len(x.Body)
			default:
				t.Fatalf("parsed as %T", x)
			}
			if body != c.body {
				t.Errorf("%d body statements, want %d", body, c.body)
			}
		})
	}
}

// The body may be left out altogether, which is the same rule with nothing
// after the header.
//
// It is reached far more often than it looks. `while cond; { … }` is *this*
// and not a short body: the `;` keeps the condition list going, so the brace
// group joins what is tested and the body has nothing left. Reading it the
// other way makes `while (( i < 2 )); { i=$((i+1)) }` terminate, where the
// shell that has the construct counts up without stopping.
func TestAnOmittedBodyLeavesTheHeaderTheWholeLoop(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		cond int
	}{
		{"a condition and nothing else", "while false", 1},
		{"an until the same way", "until true", 1},
		{"a separator puts what follows in the condition", "while true; { echo hi; break; }", 2},
		{"and keeps taking statements after that", "while false; echo A; echo B", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, ok := onlyCommand(t, c.src, short()).(*syntax.LoopClause)
			if !ok {
				t.Fatalf("%q: not a loop", c.src)
			}
			if len(x.Cond) != c.cond {
				t.Errorf("%q: %d condition statements, want %d", c.src, len(x.Cond), c.cond)
			}
			if len(x.Body) != 0 {
				t.Errorf("%q: %d body statements, want none", c.src, len(x.Body))
			}
		})
	}
}

// A `for` or `select` whose header ended itself may be left with no body too,
// and the parenthesized list is one of the two ways it ends.
func TestAForHeaderThatEndsItselfNeedsNoBody(t *testing.T) {
	for _, c := range []struct {
		name  string
		src   string
		items int
		has   bool
	}{
		{"a parenthesized list", "for i (a b)", 2, true},
		{"an empty parenthesized list", "for i ()", 0, true},
		{"no list at all", "for i", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, ok := onlyCommand(t, c.src, short()).(*syntax.ForClause)
			if !ok {
				t.Fatalf("%q: not a for", c.src)
			}
			if x.HasItems != c.has || len(x.Items) != c.items {
				t.Errorf("%q: HasItems=%v with %d items, want %v and %d",
					c.src, x.HasItems, len(x.Items), c.has, c.items)
			}
			if len(x.Body) != 0 {
				t.Errorf("%q: %d body statements, want none", c.src, len(x.Body))
			}
		})
	}
}

// The parenthesized list says exactly what `in` says, so the two spellings give
// the same tree and only the header text remembers which was written.
func TestTheParenthesizedListIsTheInList(t *testing.T) {
	paren, ok := onlyCommand(t, "for i (a b) echo $i", short()).(*syntax.ForClause)
	if !ok {
		t.Fatal("not a for")
	}
	in, ok := onlyCommand(t, "for i in a b; echo $i", short()).(*syntax.ForClause)
	if !ok {
		t.Fatal("not a for")
	}
	if !paren.HasItems || !in.HasItems || len(paren.Items) != len(in.Items) {
		t.Fatalf("item lists differ: %d against %d", len(paren.Items), len(in.Items))
	}
	for i := range paren.Items {
		if got, want := syntax.PrintWord(paren.Items[i]), syntax.PrintWord(in.Items[i]); got != want {
			t.Errorf("item %d = %q, want %q", i, got, want)
		}
	}
	// The header is what a trace prints, and it is kept as written.
	if paren.Header != "for i (a b)" {
		t.Errorf("Header = %q, want %q", paren.Header, "for i (a b)")
	}
}

// A redirection after a *closed* short body belongs to the loop, exactly as
// one after `done` does. #614's neighboring finding is why this is asserted
// rather than assumed: a compound node that has no redirection list does not
// refuse a redirection, it silently means something else.
//
// Where the body is one command the redirection is that command's, which is
// the ordinary rule and not a rule about loops. The two are not a matter of
// taste — measured 2026-09-05 with the loop variable in the target, and the
// answer is visible in the filesystem: `for i (a b) > /tmp/f$i` writes
// /tmp/fa and /tmp/fb, so the redirection ran twice with `$i` set and is the
// body; `for i (a b) { echo hi } > /tmp/f$i` writes /tmp/f, so that one was
// expanded once before the loop and is the loop's.
func TestAShortLoopTakesARedirection(t *testing.T) {
	for _, c := range []struct {
		src       string
		onTheLoop int
		body      int
	}{
		{"for i (a b) { echo $i; } >/dev/null", 1, 1},
		{"while (( 0 )) { echo hi; } >/dev/null", 1, 1},
		{"for i (a b) >/dev/null", 0, 1},
		{"for i (a b) echo $i >/dev/null", 0, 1},
		// Nothing closed here either, so `>/dev/null` is the condition's last
		// command and the loop has no body at all.
		{"while false >/dev/null", 0, 0},
	} {
		cmd := onlyCommand(t, c.src, short())
		var got, body int
		switch x := cmd.(type) {
		case *syntax.ForClause:
			got, body = len(x.Redirs), len(x.Body)
		case *syntax.LoopClause:
			got, body = len(x.Redirs), len(x.Body)
		default:
			t.Fatalf("%q: parsed as %T", c.src, cmd)
		}
		if got != c.onTheLoop || body != c.body {
			t.Errorf("%q: %d redirections on the loop and %d body statements, want %d and %d",
				c.src, got, body, c.onTheLoop, c.body)
		}
	}
}

// The header has to end itself, and a word cannot. `while true { … }` reads `{`
// as another word of `true`, so the loop never sees a body and the `}` has
// nothing to close — which is what the shell with the construct reports too.
func TestAWordHeaderTakesNoShortBody(t *testing.T) {
	for _, src := range []string{
		"while true { echo hi; }",
		"until false { echo hi; }",
		// `if` has no short form at all, in any dialect.
		"if true; { echo yes; }",
		"if true { echo yes; }",
		// The list `for` still needs its separator: with nothing between, `{`
		// is another item.
		"for i in a b { echo $i; }",
		// And with a list and no separator there is no header end to find.
		"for i in a b",
	} {
		if _, err := syntax.Parse(src, short()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
}

// Without the flag every shape above is refused, and the long forms are
// untouched. The refusals are the half worth pinning: a grammar that accepts
// the union of dialects would let a script written for one silently mean
// something in another.
func TestTheShortFamilyNeedsTheFlag(t *testing.T) {
	for _, src := range []string{
		"while (( 0 )) echo hi",
		"until (( 0 )) echo hi",
		"while (( 0 )) { echo hi; }",
		"for i (a b) { echo $i; }",
		"for i (a b) echo $i",
		"for i in a b; echo $i",
		"select x (a b) { echo $x; }",
		"for ((i=0;i<2;i++)) echo $i",
		"while false",
		"for i",
		"for i (a b)",
		// The parenthesized list with an ordinary `do … done` after it. This
		// is the row the *list* half of the flag is pinned by, and the only
		// one that separates reading the parentheses from reading the body:
		// every other paren form above is refused by the body rule as well,
		// so dropping the flag from the item test alone left them all still
		// failing and the whole suite green.
		"for i (a b) do echo $i; done",
		"for i (a b); do echo $i; done",
		"select x (a b) do echo $x; done",
	} {
		if _, err := syntax.Parse(src, syntax.Core()); err == nil {
			t.Errorf("%q parsed without ShortLoop, want a syntax error", src)
		}
	}
	for _, src := range []string{
		"while false; do :; done",
		"for i in a b; do echo $i; done",
		"for i; do echo $i; done",
		"for i in a b; { echo $i; }",
		"for ((i=0;i<2;i++)) { echo $i; }",
		"select x in a b; do echo $x; done",
	} {
		if _, err := syntax.Parse(src, syntax.Core()); err != nil {
			t.Errorf("%q no longer parses without ShortLoop: %v", src, err)
		}
	}
}

// Printing, and the reason this test exists at all: a construct the parser
// accepts and the printer cannot write back is a silent corruption.
//
// Two properties, the same two the corpus round trip asks for — what comes
// back parses, and printing it again gives the same text. The corpus cannot
// ask them here, because it round-trips under Core and every case below is a
// syntax error there.
func TestPrintingAShortLoop(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A body that was written short has a long spelling and gets it. The
		// text is not the input and is not meant to be; the tree is.
		{"while (( i < 2 )) echo hi", "while (( i < 2 )); do echo hi; done"},
		{"until (( i > 2 )) echo hi", "until (( i > 2 )); do echo hi; done"},
		{"for i (a b) { echo $i; }", "for i in a b; do echo $i; done"},
		{"for i (a b) echo $i", "for i in a b; do echo $i; done"},
		// The parenthesized list with an ordinary body, which is the one form
		// where the list half of the flag is reached on its own.
		{"for i (a b) do echo $i; done", "for i in a b; do echo $i; done"},
		{"for i (a b); do echo $i; done", "for i in a b; do echo $i; done"},
		{"select x (a b) { echo $x; }", "select x in a b; do echo $x; done"},
		{"for ((i=0;i<2;i++)) echo $i", "for ((i=0; i<2; i++)); do echo $i; done"},
		// An *omitted* body has no long spelling — `do done` is refused
		// everywhere — so these keep the short one, and the parenthesized
		// list comes back because `for i in a b` with no body does not parse.
		{"while false", "while false"},
		{"until true", "until true"},
		{"while true; { echo hi; break; }", "while true; { echo hi; break; }"},
		{"for i (a b)", "for i (a b)"},
		{"for i", "for i"},
		{"select x (a b)", "select x (a b)"},
		{"for ((i=0;i<2;i++))", "for ((i=0; i<2; i++))"},
		// A redirection on the loop survives the long spelling.
		{"for i (a b) { echo $i; } >/dev/null", "for i in a b; do echo $i; done > /dev/null"},
	} {
		t.Run(c.src, func(t *testing.T) {
			d := short()
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.Print(f)
			if printed != c.want {
				t.Errorf("printed %q, want %q", printed, c.want)
			}
			g, err := syntax.Parse(printed, d)
			if err != nil {
				t.Fatalf("printed source does not parse: %v\n  gave: %q", err, printed)
			}
			if again := syntax.Print(g); again != printed {
				t.Errorf("printing is not settled:\n  once:  %q\n  twice: %q", printed, again)
			}
		})
	}
}

// An omitted body carrying a redirection is the one shape with no source form,
// and the printer's job there is to lose nothing rather than to round-trip.
//
// No text means it. A redirection written after a header with nothing else is
// the *body* — one command that only redirects — which is measured rather than
// chosen: `for i (a b) > /tmp/f$i` writes /tmp/fa and /tmp/fb, so it ran once
// per iteration with the loop variable set. So the parser never builds this
// tree, and what is printed for it reads back as the body form.
//
// The printer still has to write the items and the redirection, because a
// tree is not only what this parser produced — a caller assembling one would
// otherwise get a loop whose redirection had silently gone, which is the
// failure #614 describes. What it must not do is print the long form, where
// `do` would stand with nothing after it.
func TestPrintingAnOmittedBodyThatRedirects(t *testing.T) {
	f, err := syntax.Parse("for i (a b)", short())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	loop, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.ForClause)
	if !ok {
		t.Fatal("not a for")
	}
	target, err := syntax.Parse("echo /dev/null", short())
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	word := target.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Args[1]
	loop.Redirs = []*syntax.Redirect{{Op: syntax.TokGreat, Text: "/dev/null", Word: word}}

	printed := syntax.Print(f)
	if want := "for i (a b) > /dev/null"; printed != want {
		t.Fatalf("printed %q, want %q", printed, want)
	}
	// It parses, and it still says `for i` over `a b` writing to /dev/null.
	// What moved is which node owns the redirection, and only for a tree no
	// input produces.
	g, err := syntax.Parse(printed, short())
	if err != nil {
		t.Fatalf("printed source does not parse: %v", err)
	}
	back, ok := g.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.ForClause)
	if !ok {
		t.Fatal("did not read back as a for")
	}
	if back.Name != "i" || len(back.Items) != 2 || len(back.Body) != 1 {
		t.Errorf("read back as %q over %d items with %d body statements, want i, 2 and 1",
			back.Name, len(back.Items), len(back.Body))
	}
	if len(back.Body) == 1 {
		if got := syntax.PrintCommand(back.Body[0].Expr.(*syntax.Pipeline).Cmds[0]); got != " > /dev/null" {
			t.Errorf("the redirection came back as %q", got)
		}
	}
}

// The long spelling a short body prints back as parses under a dialect that has
// no short loops at all, which is what makes the choice safe: only the omitted
// body needs the flag to be read again.
func TestAShortBodyPrintsBackToTheCommonForm(t *testing.T) {
	for _, src := range []string{
		"while (( i < 2 )) echo hi",
		"for i (a b) { echo $i; }",
		"select x (a b) echo $x",
		"for ((i=0;i<2;i++)) echo $i",
	} {
		f, err := syntax.Parse(src, short())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := syntax.Print(f)
		if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
			t.Errorf("%q printed as %q, which does not parse without the flag: %v",
				src, printed, err)
		}
	}
}
