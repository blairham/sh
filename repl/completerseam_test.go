// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"reflect"
	"testing"
)

// recordingCompleter keeps what it was asked and answers with a fixed list.
type recordingCompleter struct {
	asked  []Completion
	answer []string
}

func (c *recordingCompleter) Complete(req Completion) []string {
	c.asked = append(c.asked, req)
	return c.answer
}

// What a completer is told is the whole of the question: the line, where the
// cursor is in it, where the word starts, the word itself, whether it is a
// command, and the shell's directory.
//
// Asserted as one value rather than field by field, so a field that stopped
// being filled in is a failure rather than an untested field. The indices are
// bytes into Line, which is why one of these lines is not ASCII: the editor
// counts in runes, so the last case would say 10 and 8 if the conversion were
// missed, and every other line here agrees either way.
func TestACompleterIsToldWhereTheWordSitsInTheLine(t *testing.T) {
	for _, c := range []struct {
		name string
		line string
		pos  int
		want Completion
	}{
		{
			name: "the first word of a line is a command",
			line: "ec", pos: 2,
			want: Completion{Line: "ec", Point: 2, Start: 0, Word: "ec", Command: true, Dir: "/somewhere"},
		},
		{
			name: "an argument is not",
			line: "echo ap", pos: 7,
			want: Completion{Line: "echo ap", Point: 7, Start: 5, Word: "ap", Command: false, Dir: "/somewhere"},
		},
		{
			name: "after a pipe a command begins again",
			line: "ls | gr", pos: 7,
			want: Completion{Line: "ls | gr", Point: 7, Start: 5, Word: "gr", Command: true, Dir: "/somewhere"},
		},
		{
			name: "the cursor need not be at the end of the line",
			line: "echo ap two", pos: 7,
			want: Completion{Line: "echo ap two", Point: 7, Start: 5, Word: "ap", Command: false, Dir: "/somewhere"},
		},
		{
			name: "an empty word at a fresh prompt",
			line: "", pos: 0,
			want: Completion{Line: "", Point: 0, Start: 0, Word: "", Command: true, Dir: "/somewhere"},
		},
		{
			name: "the indices are bytes and not runes",
			line: "echo 日本 ap", pos: 10,
			want: Completion{Line: "echo 日本 ap", Point: 14, Start: 12, Word: "ap", Command: false, Dir: "/somewhere"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := &recordingCompleter{}
			e := &editor{
				line: []rune(c.line), pos: c.pos, comp: rec,
				workingDir: func() string { return "/somewhere" },
			}
			e.complete(rec)
			if len(rec.asked) != 1 {
				t.Fatalf("the completer was asked %d times, want 1", len(rec.asked))
			}
			if got := rec.asked[0]; !reflect.DeepEqual(got, c.want) {
				t.Errorf("asked\n got %#v\nwant %#v", got, c.want)
			}
		})
	}
}

// Several completers are consulted in order, and the first one with anything
// to say is the whole answer: the lists are not merged, and nothing after it
// is asked at all.
//
// Both halves are the claim. A merged list would put `beta` on the line, and a
// list that was chosen correctly but computed by asking everybody would have
// run the second completer's work for nothing — which for a completer that
// shells out is the difference between one round trip per Tab and all of them.
func TestTheFirstCompleterWithAnAnswerIsTheWholeAnswer(t *testing.T) {
	first := &recordingCompleter{answer: []string{"alpha"}}
	second := &recordingCompleter{answer: []string{"beta"}}

	e := &editor{line: []rune("a"), pos: 1}
	matches := e.complete(completers{first, second})

	if matches != nil {
		t.Errorf("one match should be filled in rather than listed, got %q", matches)
	}
	if got := string(e.line); got != "alpha " {
		t.Errorf("the line is %q, want %q", got, "alpha ")
	}
	if len(second.asked) != 0 {
		t.Errorf("the second completer was asked %d times, want 0", len(second.asked))
	}
}

// A completer with nothing to say leaves the next one to answer, and that is
// what makes the shell's own completion the fallback rather than the loser.
func TestACompleterThatDeclinesLeavesTheNextToAnswer(t *testing.T) {
	quiet := &recordingCompleter{}
	answering := &recordingCompleter{answer: []string{"gamma"}}

	e := &editor{line: []rune("g"), pos: 1}
	e.complete(completers{quiet, answering})

	if len(quiet.asked) != 1 {
		t.Errorf("the declining completer was asked %d times, want 1", len(quiet.asked))
	}
	if got := string(e.line); got != "gamma " {
		t.Errorf("the line is %q, want %q", got, "gamma ")
	}
}

// A nil in the list is skipped rather than a crash, because a caller building
// a list from configuration will have one.
func TestANilCompleterInTheListIsSkipped(t *testing.T) {
	e := &editor{line: []rune("d"), pos: 1}
	e.complete(completers{nil, &recordingCompleter{answer: []string{"delta"}}})
	if got := string(e.line); got != "delta " {
		t.Errorf("the line is %q, want %q", got, "delta ")
	}
}

// The shell puts the caller's completers in front of its own, so a caller can
// answer a word this shell would also have answered.
//
// `echo` is a builtin, so the shell's own completer has an answer for `ech`.
// The caller's wins, which is the whole of what extending completion means.
func TestACallersCompleterAnswersBeforeTheShellsOwn(t *testing.T) {
	mine := &recordingCompleter{answer: []string{"echoed-by-the-caller"}}
	s := Shell{Runner: newTestRunner(nil), Completers: []Completer{mine}}

	e := &editor{line: []rune("ech"), pos: 3, workingDir: s.workingDir}
	e.complete(s.completer(t.Context()))

	if got := string(e.line); got != "echoed-by-the-caller " {
		t.Errorf("the line is %q, want %q", got, "echoed-by-the-caller ")
	}
}

// And a caller that declines leaves this shell's own answer standing.
func TestTheShellsOwnCompleterAnswersWhenTheCallersDoNot(t *testing.T) {
	quiet := &recordingCompleter{}
	s := Shell{Runner: newTestRunner(nil), Completers: []Completer{quiet}}

	e := &editor{line: []rune("ech"), pos: 3, workingDir: s.workingDir}
	e.complete(s.completer(t.Context()))

	if got := string(e.line); got != "echo " {
		t.Errorf("the line is %q, want %q", got, "echo ")
	}
	if len(quiet.asked) != 1 {
		t.Errorf("the caller's completer was asked %d times, want 1", len(quiet.asked))
	}
}

// A shell with neither a Runner nor a completer of the caller's has none, and
// the editor's nil check has to see a nil interface rather than a typed one.
func TestAShellWithNothingToCompleteWithHasNoCompleter(t *testing.T) {
	if c := (Shell{}).completer(t.Context()); c != nil {
		t.Errorf("an empty shell has a completer: %#v", c)
	}
}

// The directory a completer is told is the shell's and not the process's.
//
// The two are the same until `cd` runs, which is why this sets one that no
// process is ever started in: a completer resolving a relative path against
// the wrong one is a completer that offers the files of whatever directory the
// embedding program happened to start in.
func TestACompleterIsToldTheShellsDirectoryRatherThanTheProcesss(t *testing.T) {
	r := newTestRunner(nil)
	r.Dir = "/not/where/this/test/runs"
	s := Shell{Runner: r}
	rec := &recordingCompleter{}

	e := &editor{line: []rune("x"), pos: 1, workingDir: s.workingDir}
	e.complete(rec)

	if len(rec.asked) != 1 {
		t.Fatalf("the completer was asked %d times, want 1", len(rec.asked))
	}
	if got := rec.asked[0].Dir; got != "/not/where/this/test/runs" {
		t.Errorf("the completer was told %q, want %q", got, "/not/where/this/test/runs")
	}
}

// A Shell with no Runner still builds an editor, and the directory it reports
// is empty rather than a panic.
func TestAShellWithoutARunnerReportsNoDirectory(t *testing.T) {
	if got := (Shell{}).workingDir(); got != "" {
		t.Errorf("the directory is %q, want %q", got, "")
	}
}

// A function is a completer, which is the shape a caller with one rule writes.
func TestCompleterFuncIsACompleter(t *testing.T) {
	var c Completer = CompleterFunc(func(req Completion) []string {
		return []string{req.Word + "-suffixed"}
	})
	if got := c.Complete(Completion{Word: "w"}); !reflect.DeepEqual(got, []string{"w-suffixed"}) {
		t.Errorf("got %q, want %q", got, []string{"w-suffixed"})
	}
}

// A caller's completer reaches the editor at a real terminal, and the word it
// answered with is the word the command runs with.
//
// The reader-driven tests above prove what the seam is asked and which answer
// wins. This proves the wiring: the editor is only built where there is a
// terminal, so a Shell field dropped on the way to it looks exactly like a
// caller that supplied nothing — which is the mutation that survived
// everything when driver's own prompt wiring was written inline.
//
// `printf` is given the completed word, so a completion that arrived wrong is
// a different line of output rather than a different-looking screen.
func TestACallersCompleterReachesTheEditorAtATerminal(t *testing.T) {
	mine := CompleterFunc(func(req Completion) []string {
		if req.Command || req.Word != "sur" {
			return nil
		}
		// Through Escape rather than by hand: the seam's answers are
		// replacement words in the line's own quoting, and a completer that
		// wrote the space out plainly would have offered two arguments.
		return []string{req.Escape("surprising name.txt")}
	})
	s := newSessionWith(t, func(sh *Shell) { sh.Completers = []Completer{mine} })

	s.typeLine("printf '<%s>' sur\t\n")
	waitFor(t, s.ran, "<surprising name.txt>", "the completed word, as one argument")
	s.end()
}

// Escape writes a name so that reading the completed line back gives the name
// again, in whichever quotation the word was already inside.
//
// The whole word each time, opening quote included, because that is what the
// editor replaces. The cases are the ones a completer gets wrong: a space, a
// quote of the other kind, a character the grammar reads as an expansion, and
// a name that has to survive being written inside a quotation it contains.
func TestACompletionEscapesALiteralNameForTheWordItReplaces(t *testing.T) {
	for _, c := range []struct{ name, word, literal, want string }{
		{"a space in an unquoted word", "sur", "surprising name.txt", `surprising\ name.txt`},
		{"a dollar in an unquoted word", "a", "a$b.txt", `a\$b.txt`},
		{"a tilde only counts at the start", "", "~x", `\~x`},
		{"inside double quotes a space needs nothing", `"file t`, "file two.txt", `"file two.txt`},
		{"inside double quotes a dollar still does", `"a`, "a$b", `"a\$b`},
		{"inside single quotes only the quote is special", "'quo", "quo'te.txt", `'quo'\''te.txt`},
		{"a directory is written out in full", "sub/o", "sub/other.txt", "sub/other.txt"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := (Completion{Word: c.word}).Escape(c.literal); got != c.want {
				t.Errorf("Escape(%q) in %q: got %q, want %q", c.literal, c.word, got, c.want)
			}
		})
	}
}

// The editor a session builds is given the shell's directory and the shell's
// completers, and this is the assertion that a field dropped in newEditor is a
// failure.
//
// The editor exists only where there is a terminal, so everything carried
// across that wiring is invisible to a reader-driven test of the editor and
// invisible to a test of the Shell: a dropped field looks exactly like a
// caller that said nothing. driver's own frontEnd has the same note on it,
// having lost two mutations to exactly this.
func TestTheEditorIsBuiltWithTheDirectoryAndTheCompleters(t *testing.T) {
	r := newTestRunner(nil)
	r.Dir = "/carried/across"
	mine := &recordingCompleter{answer: []string{"mine"}}
	e := Shell{Runner: r, Completers: []Completer{mine}}.newEditor(t.Context())

	if got := e.dir(); got != "/carried/across" {
		t.Errorf("the editor reports the directory %q, want %q", got, "/carried/across")
	}
	e.line, e.pos = []rune("m"), 1
	e.complete(e.comp)
	if got := string(e.line); got != "mine " {
		t.Errorf("the line is %q, want %q — the caller's completer did not reach the editor", got, "mine ")
	}
}
