// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"strings"
	"testing"
)

// A large listing is asked about rather than printed.
//
// Columns made a long one four times shorter and did not stop it pushing the
// prompt off the screen: a directory of a thousand files is still hundreds of
// rows. Both shells with a line editor stop at a hundred matches and ask.
func TestALargeListingIsAskedAboutFirst(t *testing.T) {
	for _, c := range []struct {
		name    string
		matches int
		keys    string
		want    bool
		asked   bool
	}{
		{"below the threshold, no question", 99, "", true, false},
		{"at the threshold, asked", 100, "y", true, true},
		{"and above it", 500, "y", true, true},
		{"declined", 100, "n", false, true},
		{"declined in capitals", 100, "N", false, true},
		{"accepted in capitals", 100, "Y", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, out := queryEditor(c.keys, "Display all %[1]d possibilities? (y or n)", false, true)
			got := e.confirmList(names(c.matches), drawPrompt("P> "))
			if got != c.want {
				t.Errorf("printed = %v, want %v", got, c.want)
			}
			if asked := strings.Contains(out.String(), "possibilities"); asked != c.asked {
				t.Errorf("asked = %v, want %v (%q)", asked, c.asked, out.String())
			}
		})
	}
}

// The question is the dialect's, and one of the two counts the rows the
// matches would take as well as the matches themselves.
func TestTheQuestionIsTheDialects(t *testing.T) {
	e, out := queryEditor("y", "zsh: do you wish to see all %[1]d possibilities (%[2]d lines)? ", true, false)
	e.width = func() int { return 80 }
	e.confirmList(names(120), drawPrompt("P> "))
	got := out.String()
	if !strings.Contains(got, "all 120 possibilities (") {
		t.Errorf("said %q, want the number of matches", got)
	}
	// 120 names eight wide plus two make ten columns each, so eight fit in
	// eighty and 120 of them take fifteen rows.
	if !strings.Contains(got, "(15 lines)") {
		t.Errorf("said %q, want the rows the matches would take", got)
	}
}

// One shell takes the first key it is given and treats everything but `y` as
// no. The other waits for one of two, and rings the bell at anything else.
func TestAnAnswerThatIsNeitherYesNorNo(t *testing.T) {
	t.Run("the first key decides", func(t *testing.T) {
		e, out := queryEditor("q", "ask %[1]d %[2]d", true, false)
		if e.confirmList(names(100), drawPrompt("P> ")) {
			t.Error("printed, want the first key to decline")
		}
		if strings.Contains(out.String(), "\a") {
			t.Errorf("said %q, want no bell", out.String())
		}
	})
	t.Run("or the bell, and asked again", func(t *testing.T) {
		// Two keys that are not answers, then one that is.
		e, out := queryEditor("qxy", "ask %[1]d %[2]d", false, true)
		if !e.confirmList(names(100), drawPrompt("P> ")) {
			t.Error("did not print, want the `y` after the two to be taken")
		}
		if n := strings.Count(out.String(), "\a"); n != 2 {
			t.Errorf("rang the bell %d times, want 2 — once per key that was not an answer", n)
		}
	})
}

// The key that answered is written back by one of them and not the other.
func TestTheAnswerIsEchoedOrNot(t *testing.T) {
	for _, echo := range []bool{true, false} {
		e, out := queryEditor("y", "ask %[1]d %[2]d", echo, false)
		e.confirmList(names(100), drawPrompt("P> "))
		// After the question, and before the newline that ends it.
		if got := strings.Contains(out.String(), "ask 100"); !got {
			t.Fatalf("said %q, want the question", out.String())
		}
		if echoed := strings.Contains(out.String(), "y"); echoed != echo {
			t.Errorf("echo=%v: said %q, want the key written back = %v", echo, out.String(), echo)
		}
	}
}

// A dialect with no answer asks nothing and prints, which is what the two
// without a line editor of their own leave it at.
func TestNoQuestionMeansPrint(t *testing.T) {
	e, out := queryEditor("", "", false, false)
	if !e.confirmList(names(1000), drawPrompt("P> ")) {
		t.Error("did not print, want a shell with no question to print")
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}

// Input ending while the question is unanswered is nobody to print for.
func TestAQuestionNobodyAnswers(t *testing.T) {
	e, _ := queryEditor("", "ask %[1]d %[2]d", false, true)
	if e.confirmList(names(100), drawPrompt("P> ")) {
		t.Error("printed, want an unanswered question to decline")
	}
}

func queryEditor(keys, query string, echo, strict bool) (*editor, *strings.Builder) {
	out := &strings.Builder{}
	return &editor{
		in: strings.NewReader(keys), out: out,
		listQuery: query, listQueryEchoes: echo, listQueryStrict: strict,
	}, out
}

// names makes n matches of a fixed width, so the rows they take are known.
func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("name%04d", i)
	}
	return out
}

// The answer decides whether the listing happens, which is the whole point of
// asking — and the tab handler is where the two meet.
//
// Driven through readLine rather than confirmList directly, because a mutant
// that dropped the answer at the call site and listed regardless passed every
// test that only called confirmList.
func TestDecliningStopsTheListingAndAcceptingDoesNot(t *testing.T) {
	for _, c := range []struct {
		name, answer string
		listed       bool
	}{
		{"declined", "n", false},
		{"accepted", "y", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := &strings.Builder{}
			e := &editor{
				in:  strings.NewReader("a\t\t" + c.answer + "\r"),
				out: out,
				// Matches sharing only `a`, so the first Tab has no common
				// prefix to insert and the second asks about the listing.
				comp:            fakeCompleter{cmds: shortNames(120)},
				listQuery:       "ask %[1]d %[2]d",
				listQueryStrict: true,
			}
			line, err := e.readLine(drawPrompt("P> "))
			if err != nil {
				t.Fatal(err)
			}
			if line != "a" {
				t.Fatalf("line = %q, want the typed line back", line)
			}
			if !strings.Contains(out.String(), "ask 120") {
				t.Fatalf("said %q, want the question", out.String())
			}
			// `a119` appears only in a listing: it is not a prefix of
			// anything the editor would otherwise draw.
			if listed := strings.Contains(out.String(), "a119"); listed != c.listed {
				t.Errorf("listed = %v, want %v (%q)", listed, c.listed, out.String())
			}
		})
	}
}

// shortNames share only their first character, so completing one of them
// inserts nothing and the second Tab is what asks.
func shortNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("a%d", i)
	}
	return out
}
