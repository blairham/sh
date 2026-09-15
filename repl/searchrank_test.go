// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"strings"
	"testing"
)

// The ranked subsequence half of `C-r`, driven through the editor exactly as
// the contiguous half is.
//
// Every assertion here is about a query that used to ring the bell. The
// contiguous pass is unchanged and its tests are in search_test.go; what these
// pin is that the fallback is reached, that it ranks rather than merely
// matching, and that reaching it costs the contiguous case nothing.

// The headline: a query that describes a line finds it.
//
// `gco` is the case #1311 names, and it is the one people give when they call
// a shell dated — before this it matched nothing at all, because no entry
// contains the three characters together.
func TestASubsequenceQueryFindsTheLineItDescribes(t *testing.T) {
	history := []string{"ls -la", "git checkout origin/main", "make check"}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12gco\r")
	if line != "git checkout origin/main" {
		t.Errorf("C-r gco gave %q, want the line its characters describe", line)
	}
}

// A contiguous match wins outright, however well a subsequence match would
// have scored.
//
// This is the compatibility rule stated as a test rather than as a comment: a
// query that is a substring of anything must reach the same line it always
// reached, so the ranking can only ever decide a search the old code answered
// with a bell.
func TestAContiguousMatchBeatsABetterLookingSubsequence(t *testing.T) {
	history := []string{
		// A subsequence match with everything going for it: three characters
		// at word starts, at the front of a short line.
		"git checkout origin",
		// An unremarkable line that simply contains the query.
		"echo the-gco-marker-here",
	}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12gco\r")
	if line != "echo the-gco-marker-here" {
		t.Errorf("C-r gco gave %q, want the entry that contains the query", line)
	}
}

// Contiguity is the first thing the ranking looks at: a run of adjacent
// matched characters beats a scattered one.
func TestARunOfAdjacentCharactersOutranksAScatteredMatch(t *testing.T) {
	history := []string{
		// s, t, a, t scattered across the words.
		"set the alarm at two",
		// `stat` whole, so every character after the first is contiguous.
		"git dostat",
	}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12stat\r")
	if line != "git dostat" {
		t.Errorf("C-r stat gave %q, want the contiguous run", line)
	}
}

// A character after a word break scores higher than one inside a word.
func TestCharactersAtWordStartsOutrankOnesInsideAWord(t *testing.T) {
	history := []string{
		// The three characters are all buried inside words.
		"axxbxxcxx zzz",
		// Each begins a word: after a space, after a dash, after a slash.
		"a b-c",
	}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12abc\r")
	if line != "a b-c" {
		t.Errorf("C-r abc gave %q, want the one whose characters start words", line)
	}
}

// Among matches that are otherwise alike, the more recent one wins — which is
// what a history search is for.
func TestRecencyBreaksATie(t *testing.T) {
	history := []string{"a-b-c older", "a-b-c newer"}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12abc\r")
	if line != "a-b-c newer" {
		t.Errorf("C-r abc gave %q, want the newer of two equal matches", line)
	}
}

// And shortness is the last tie-break, once structure and recency have
// nothing left to say.
func TestShortnessIsTheLastTieBreak(t *testing.T) {
	e := &editor{history: []string{
		"a-b-c and a great deal more text besides to make this the longer one",
		"a-b-c",
	}}
	// Scored directly rather than through a search, because the two differ
	// only in length and the shorter one is also the newer: a search would
	// pass on recency alone and say nothing about the tie-break under it.
	long := rankedMatch{index: 1, score: 10, length: 60}
	short := rankedMatch{index: 1, score: 10, length: 5}
	if !short.better(long) {
		t.Error("the longer entry won a tie that only length could break")
	}
	if i, _ := e.rankBack([]rune("abc"), len(e.history)-1); i != 1 {
		t.Errorf("rankBack chose %d, want the shorter, newer entry", i)
	}
}

// The cursor lands on the first matched character, which is the same meaning
// the contiguous pass's offset has.
func TestTheCursorLandsOnTheFirstMatchedCharacter(t *testing.T) {
	history := []string{"echo hello && git checkout main"}
	e, line, _ := searching(t, HistoryStyle{}, history, "\x12gco\r")
	if line != history[0] {
		t.Fatalf("C-r gco gave %q", line)
	}
	if want := strings.Index(history[0], "git"); e.pos != want {
		t.Errorf("the cursor is at %d, want %d — the start of the match", e.pos, want)
	}
}

// A rune offset and not a byte one. The editor's cursor sits between
// characters, so a match after a multi-byte one would otherwise be drawn
// several columns to the right of where it is.
func TestTheOffsetIsARuneCountNotAByteCount(t *testing.T) {
	// Five runes and thirteen bytes before the match begins.
	history := []string{"日本語で git checkout main"}
	e, _, _ := searching(t, HistoryStyle{}, history, "\x12gco\r")
	if e.pos != 5 {
		t.Errorf("the cursor is at %d, want 5 runes in — a byte offset would be 13", e.pos)
	}
}

// Smart case, on this pass only: an all-lowercase query ignores case, and one
// uppercase character means the case was meant.
func TestSmartCaseOnTheRankedPass(t *testing.T) {
	e := &editor{history: []string{"Git CheckOut Origin"}}
	// Asserted on the matcher rather than through a search, because a failed
	// step keeps the line that last matched on the screen — measured, and
	// tested in search_test.go — so an accepted line cannot tell "this query
	// matched" from "an earlier, shorter query did".
	if i, _ := e.rankBack([]rune("gco"), 0); i != 0 {
		t.Error("an all-lowercase query did not ignore case")
	}
	if i, _ := e.rankBack([]rune("gcO"), 0); i != -1 {
		t.Error("a query with an uppercase character matched an entry spelled differently")
	}
	if i, _ := e.rankBack([]rune("GCO"), 0); i != 0 {
		t.Error("a query matching the entry's own case did not match")
	}
	// And folding never overrules the contiguous pass, which is the surface the
	// panel shells were measured for. With an entry that contains the query as
	// written, that entry wins — the case-folded one is not even consulted.
	both := &editor{history: []string{"Git CheckOut Origin", "run checkout now"}}
	if i, _ := both.findBack([]rune("checkout"), 1); i != 1 {
		t.Errorf("reached entry %d, want the one that contains the query as written", i)
	}
	// The folded reading is still there underneath, for a query nothing
	// contains: this is additive, and only ever turns a bell into a match.
	if i, _ := both.findBack([]rune("checkout"), 0); i != 0 {
		t.Errorf("reached entry %d, want the case-folded match once nothing contains the query", i)
	}
}

// A repeated `C-r` keeps going back through subsequence matches too, rather
// than showing the best one again.
func TestARepeatedStepWalksBackThroughRankedMatches(t *testing.T) {
	// Three entries the ranking scores identically, so recency orders them and
	// what is under test is the walk rather than the score.
	history := []string{"a-b-c oldest", "a-b-c middle", "a-b-c newest"}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12abc\r")
	if line != "a-b-c newest" {
		t.Fatalf("the first C-r gave %q", line)
	}
	_, line, _ = searching(t, HistoryStyle{}, history, "\x12abc\x12\r")
	if line != "a-b-c middle" {
		t.Errorf("a second C-r gave %q, want the next match back", line)
	}
	_, line, _ = searching(t, HistoryStyle{}, history, "\x12abc\x12\x12\r")
	if line != "a-b-c oldest" {
		t.Errorf("a third C-r gave %q, want the oldest match", line)
	}
}

// Contiguity outranks a word start, which is the order #1311 puts them in and
// is worth a test because the two pull opposite ways on a real query.
//
// `gco` over `git commit -a` takes the run `co`; over `git checkout origin` it
// takes three word starts. The first wins, and a reader who expected the
// second should read the priority list rather than this scoring.
func TestContiguityOutranksWordStarts(t *testing.T) {
	history := []string{"git checkout origin", "git commit -a"}
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12gco\r")
	if line != "git commit -a" {
		t.Errorf("C-r gco gave %q, want the contiguous run in `commit`", line)
	}
	// Reversing the order shows recency is not what decided it.
	history = []string{"git commit -a", "git checkout origin"}
	_, line, _ = searching(t, HistoryStyle{}, history, "\x12gco\r")
	if line != "git commit -a" {
		t.Errorf("with the order reversed, C-r gco gave %q, want the same line", line)
	}
}

// Extending a query does not jump forward to a more recent line, which is the
// measured behavior the contiguous pass already had and which the window on
// the ranked pass is there to keep.
func TestExtendingARankedQueryDoesNotJumpForward(t *testing.T) {
	history := []string{"git checkout main", "ls -la", "git clone x"}
	// `C-r gc` lands on the newest, then `C-r` steps back to the older one;
	// typing `o` must stay there rather than returning to `git clone x`.
	_, line, _ := searching(t, HistoryStyle{}, history, "\x12gc\x12o\r")
	if line != "git checkout main" {
		t.Errorf("extending the query gave %q, want it to stay where the walk got to", line)
	}
}

// An empty query is the contiguous pass's to answer, and the ranked pass
// declines it rather than ranking every line in the history.
//
// #1311 guesses in passing that "real ctrl-r shows the last line". The
// measurement already in this tree says otherwise and is what the code does:
// `C-r` on a half-typed line draws the search prompt with *that line* still
// after it, in both bash 5.3.15 and zsh 5.9.2 (search.go, recorded
// 2026-09-05). Nothing here changes it.
func TestAnEmptyQueryIsNotRanked(t *testing.T) {
	e := &editor{history: []string{"one", "two"}}
	if i, _ := e.rankBack(nil, 1); i != -1 {
		t.Errorf("an empty query ranked entry %d, want it declined", i)
	}
	// The contiguous pass does answer it — every entry contains the empty
	// string — so a step with no query lands on the newest entry.
	if i, off := e.findBack(nil, 1); i != 1 || off != 0 {
		t.Errorf("an empty query reached entry %d at %d, want the newest at 0", i, off)
	}
}

// A query whose characters are not all there, in order, still finds nothing —
// the fallback is a subsequence match and not a fuzzy one.
func TestAQueryThatIsNotASubsequenceStillFindsNothing(t *testing.T) {
	e := &editor{history: []string{"git checkout origin/main"}}
	if i, _ := e.rankBack([]rune("gcz"), 0); i != -1 {
		t.Errorf("rankBack found entry %d for a query that is not a subsequence", i)
	}
	if i, _ := e.findBack([]rune("gcz"), 0); i != -1 {
		t.Errorf("findBack found entry %d for a query that is not a subsequence", i)
	}
	// And a search that reaches nothing rings the bell.
	_, _, out := searching(t, HistoryStyle{}, []string{"nothing alike"}, "\x12zzz\r")
	if !strings.Contains(out, bell) {
		t.Error("a query matching nothing rang no bell")
	}
}

// The greedy placement is not the answer, which is why the score is a dynamic
// program rather than one more pass.
//
// Over `xaxxab`, taking the earliest `a` puts the match at rune 1 and leaves
// the `b` four characters away — two points, one for each bare character. The
// second `a` scores the same on its own and makes the `b` contiguous, which is
// worth eight more. Only scoring every placement can prefer it.
func TestTheBestPlacementIsFoundRatherThanTheFirstOne(t *testing.T) {
	var sc searchScorer
	score, start, length := sc.score([]rune("ab"), "xaxxab", false)
	if length != 6 {
		t.Errorf("the entry is %d runes, want 6", length)
	}
	greedy := 2 * matchBase
	if score <= greedy {
		t.Errorf("scored %d, want more than the %d the greedy placement gives", score, greedy)
	}
	if want := 2*matchBase + matchContiguous; score != want {
		t.Errorf("scored %d, want %d — one bare character and one contiguous", score, want)
	}
	if start != 4 {
		t.Errorf("the match starts at %d, want 4 — the greedy placement would say 1", start)
	}
}

// What a keystroke costs, by history size.
//
// #1311 asks for the cost against a realistic history rather than an assurance
// that it is fine, so both passes are timed and the sizes bracket what anybody
// actually has: a default HISTSIZE is a few thousand, and 50,000 is a person
// who has turned it up and never trimmed it.
//
// The contiguous number is the one an ordinary query pays, and it is the one
// that has not moved. The ranked number is paid only by a query that matches
// nothing contiguously — and only by the keystroke that makes it so, since
// while a query is still a substring of something the search never reaches
// this pass at all.
func BenchmarkReverseSearchContiguous(b *testing.B) {
	query := []rune("git checkout")
	for _, n := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			e := &editor{history: syntheticHistory(n)}
			if i, _ := e.findBack(query, len(e.history)-1); i < 0 {
				b.Fatal("no match, so this is timing the wrong path")
			}
			b.ResetTimer()
			for b.Loop() {
				e.findBack(query, len(e.history)-1)
			}
		})
	}
}

func BenchmarkReverseSearchRanked(b *testing.B) {
	query := []rune("gcom")
	for _, n := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			e := &editor{history: syntheticHistory(n)}
			if i, _ := e.rankBack(query, len(e.history)-1); i < 0 {
				b.Fatal("no match, so this is timing the wrong path")
			}
			b.ResetTimer()
			for b.Loop() {
				e.rankBack(query, len(e.history)-1)
			}
		})
	}
}

// syntheticHistory is a history of plausible command lines.
func syntheticHistory(n int) []string {
	shapes := []string{
		"git checkout origin/main", "make check", "ls -la /usr/local/bin",
		"go test ./internal/blocks/ -run TestSomething", "cd ../sh-repl-1311",
		"grep -rn 'something' --include='*.go' .", "echo $PATH | tr : '\\n'",
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s # %d", shapes[i%len(shapes)], i)
	}
	return out
}
