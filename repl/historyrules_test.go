// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The knobs that keep the list worth walking.
//
// Measured on 2026-09-05 against bash 5.3.15 and zsh 5.9.2 under a
// pseudo-terminal, each with a throwaway HOME and a HISTFILE of its own, by
// typing at the prompt and then reading both the file and what the up arrow
// recalls. The two are separate questions and the shells answer the second
// differently, which is the whole reason there is an axis here.

// bashish and zshish are the two dialect answers, spelled here rather than
// imported: repl is the substrate and may not know its successors, so the test
// names the fields and the dialect packages assert their own values.
var (
	bashishHistory = HistoryStyle{Control: "HISTCONTROL", Ignore: "HISTIGNORE"}
	zshishHistory  = HistoryStyle{Ignore: "HISTORY_IGNORE", IgnoreIsOnePattern: true, IgnoredStaysInSession: true}
)

func rulesFor(t *testing.T, style HistoryStyle, vars map[string]string) historyRules {
	t.Helper()
	r := newTestRunner(vars)
	return historyRulesFrom(style, r.GetVar, r.MatchPattern)
}

// HISTCONTROL is a colon-separated list, and the words in it are the three
// bash has.
func TestWhatHistcontrolSays(t *testing.T) {
	for _, tc := range []struct {
		value             string
		space, dups, none bool
	}{
		{value: "ignorespace", space: true},
		{value: "ignoredups", dups: true},
		{value: "ignoreboth", space: true, dups: true},
		{value: "ignorespace:ignoredups", space: true, dups: true},
		// A word this shell does not have leaves the rules it does have
		// alone: bash accepts `erasedups`, this shell does not erase, and
		// refusing the whole value over it would turn off a rule that was
		// asked for in the same breath.
		{value: "erasedups:ignorespace", space: true},
		{value: "", none: true},
		{value: "nonsense", none: true},
	} {
		got := rulesFor(t, bashishHistory, map[string]string{"HISTCONTROL": tc.value})
		if got.ignoreSpace != tc.space || got.ignoreDups != tc.dups {
			t.Errorf("HISTCONTROL=%q gave space=%v dups=%v, want %v and %v",
				tc.value, got.ignoreSpace, got.ignoreDups, tc.space, tc.dups)
		}
		if tc.none && (got.ignoreSpace || got.ignoreDups) {
			t.Errorf("HISTCONTROL=%q turned something on", tc.value)
		}
	}
}

// A leading space hides a line, and a duplicate is the line immediately before
// it and not any earlier one.
func TestWhatTheRulesIgnore(t *testing.T) {
	space := historyRules{ignoreSpace: true}
	if !space.ignored(" echo hidden", "") {
		t.Error("a leading space did not hide the line")
	}
	if space.ignored("echo shown", "") {
		t.Error("a line with no leading space was hidden")
	}

	dups := historyRules{ignoreDups: true}
	if !dups.ignored("echo a", "echo a") {
		t.Error("the line before it was the same and it was kept")
	}
	// Measured: `echo a`, `echo a`, `echo b`, `echo a` leaves three entries
	// and not two in both shells. Only the *immediately* preceding line
	// counts, so the fourth is not a duplicate of the first.
	if dups.ignored("echo a", "echo b") {
		t.Error("a repeat with something in between was treated as a duplicate")
	}
	// The whole line and not part of one. `echo` after `echo a` is a
	// different command, and a rule that compared by containment would drop
	// every prefix of the line before it — which passes every other case
	// here, because they differ at the first character.
	if dups.ignored("echo", "echo a") {
		t.Error("a line contained in the one before it was treated as a duplicate")
	}
	if dups.ignored("echo a --now", "echo a") {
		t.Error("a line containing the one before it was treated as a duplicate")
	}
	// The first line of a session has nothing before it, and the file's last
	// line is not it: that was a different session and is already written.
	if dups.ignored("echo a", "") {
		t.Error("the first line of a session was treated as a duplicate")
	}

	// With nothing turned on, nothing is ignored — which is measured and is
	// not the obvious default: both shells record `echo a` typed twice as two
	// entries, because dropping the second is what the knob is for.
	if (historyRules{}).ignored("echo a", "echo a") {
		t.Error("a consecutive duplicate was dropped with no knob asking for it")
	}
	if (historyRules{}).ignored(" echo hidden", "") {
		t.Error("a leading space hid a line with no knob asking for it")
	}
}

// The patterns are the dialect's own, matched against the whole line.
func TestWhatThePatternsIgnore(t *testing.T) {
	// Measured: `HISTIGNORE=ls*:pwd` drops `ls -la` and `pwd` and keeps
	// `pwd x` — the pattern is anchored at both ends rather than searched
	// for inside the line.
	rules := rulesFor(t, bashishHistory, map[string]string{"HISTIGNORE": "ls*:pwd"})
	// `lsof -h` is matched too, and that is measured rather than a corner
	// this implementation invented: `ls*` is a glob and not a word, so real
	// bash given `HISTIGNORE=ls*` drops `lsof -h` as well. Worth pinning,
	// because a pattern language that quietly anchored on a word boundary
	// would pass every other case in this test.
	for _, line := range []string{"ls", "ls -la", "pwd", "lsof -h"} {
		if !rules.ignored(line, "") {
			t.Errorf("%q was recorded, want it matched", line)
		}
	}
	for _, line := range []string{"pwd x", "echo keep", "als"} {
		if rules.ignored(line, "") {
			t.Errorf("%q was ignored, want it kept", line)
		}
	}

	// zsh's is one pattern and not a list, so a colon inside it is part of
	// the pattern. Reading it as a separator would break every pattern with
	// a path in it.
	one := rulesFor(t, zshishHistory, map[string]string{"HISTORY_IGNORE": "scp *:*"})
	if len(one.patterns) != 1 {
		t.Errorf("HISTORY_IGNORE became %q, want one pattern", one.patterns)
	}
	if !one.ignored("scp a:b", "") {
		t.Error("the colon in the pattern was read as a separator")
	}
}

// A variable a dialect does not have is not a variable that half works.
//
// Measured: ksh93 with HISTIGNORE set records the lines anyway. So a dialect
// that names no variable must not pick one up because another dialect uses it.
func TestADialectWithNoKnobsReadsNothing(t *testing.T) {
	rules := rulesFor(t, HistoryStyle{}, map[string]string{
		"HISTCONTROL":    "ignoreboth",
		"HISTIGNORE":     "*",
		"HISTORY_IGNORE": "*",
	})
	if rules.ignoreSpace || rules.ignoreDups || len(rules.patterns) != 0 {
		t.Errorf("a dialect with no knobs read %+v", rules)
	}
	if rules.ignored(" ls", "") {
		t.Error("a line was ignored by a dialect with nothing to ignore it with")
	}

	// And each dialect reads only its own: bash has no HISTORY_IGNORE and
	// zsh has no HISTCONTROL.
	if r := rulesFor(t, bashishHistory, map[string]string{"HISTORY_IGNORE": "*"}); len(r.patterns) != 0 {
		t.Error("the dialect with HISTIGNORE read HISTORY_IGNORE as well")
	}
	if r := rulesFor(t, zshishHistory, map[string]string{"HISTCONTROL": "ignoreboth"}); r.ignoreSpace || r.ignoreDups {
		t.Error("the dialect with no HISTCONTROL read one")
	}
}

// With no interpreter to ask, a pattern matches nothing rather than
// everything. The failure that matters is the destructive one.
func TestWithNoMatcherAPatternMatchesNothing(t *testing.T) {
	rules := historyRules{patterns: []string{"*"}}
	if rules.ignored("anything at all", "") {
		t.Error("a session with no matcher dropped a line")
	}
}

// The axis: an ignored line is gone in one dialect and merely unwritten in the
// other.
//
// Driven through the recorder the loop actually installs, because the two
// halves — what the up arrow can reach and what the file gets — are separate
// lists and this is where they part.
func TestAnIgnoredLineIsDroppedOrMerelyUnwritten(t *testing.T) {
	for _, tc := range []struct {
		name      string
		style     HistoryStyle
		vars      map[string]string
		recalled  []string
		writtenTo []string
	}{
		{
			// Measured: with HISTCONTROL=ignorespace, the up arrow at the
			// next prompt recalls the line *before* the hidden one, and
			// bash's own `history` cannot see it either.
			name:      "the dialect that forgets it",
			style:     bashishHistory,
			vars:      map[string]string{"HISTCONTROL": "ignorespace"},
			recalled:  []string{"echo kept"},
			writtenTo: []string{"echo kept"},
		},
		{
			// Measured: with HISTORY_IGNORE, the ignored line is what the up
			// arrow recalls, and the file does not have it.
			name:      "the dialect that keeps it",
			style:     zshishHistory,
			vars:      map[string]string{"HISTORY_IGNORE": " *"},
			recalled:  []string{"echo kept", " echo hidden"},
			writtenTo: []string{"echo kept"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &editor{}
			var added []string
			sh := Shell{Runner: newTestRunner(tc.vars), Dialect: syntax.Core(), History: tc.style}
			record := sh.recording(e, &added)
			record("echo kept")
			record(" echo hidden")

			if strings.Join(e.history, "|") != strings.Join(tc.recalled, "|") {
				t.Errorf("the up arrow can reach %q, want %q", e.history, tc.recalled)
			}
			if strings.Join(added, "|") != strings.Join(tc.writtenTo, "|") {
				t.Errorf("the file would get %q, want %q", added, tc.writtenTo)
			}
		})
	}
}

// What a duplicate is measured against is the list, which is what makes one
// implementation serve both dialects: in the one that keeps ignored lines they
// are what the next line is compared with, and in the one that drops them they
// are not there to compare with.
func TestADuplicateIsMeasuredAgainstTheList(t *testing.T) {
	e := &editor{}
	if got := e.newest(); got != "" {
		t.Errorf("an empty history's newest line is %q, want nothing", got)
	}
	e.remember("echo a")
	e.remember("echo b")
	if got := e.newest(); got != "echo b" {
		t.Errorf("newest is %q, want the last one remembered", got)
	}
}

// The whole point, end to end: `ls` typed over and over does not fill either
// list, and everything else still does.
func TestTheKnobsKeepTheListWorthWalking(t *testing.T) {
	e := &editor{}
	var added []string
	sh := Shell{
		Runner:  newTestRunner(map[string]string{"HISTCONTROL": "ignoreboth", "HISTIGNORE": "ls*"}),
		Dialect: syntax.Core(),
		History: bashishHistory,
	}
	record := sh.recording(e, &added)
	for _, line := range []string{"ls", "ls -la", "make check", "make check", " secret thing", "git push"} {
		record(line)
	}
	want := []string{"make check", "git push"}
	if strings.Join(e.history, "|") != strings.Join(want, "|") {
		t.Errorf("the list is %q, want %q", e.history, want)
	}
	if strings.Join(added, "|") != strings.Join(want, "|") {
		t.Errorf("the file would get %q, want %q", added, want)
	}
}

// A credential is refused whatever the knobs say, and stays recallable — the
// rule that predates them, restated here because the two now share a path.
func TestACredentialIsStillRecallableAndStillNotWritten(t *testing.T) {
	e := &editor{}
	var added []string
	sh := Shell{Runner: newTestRunner(nil), Dialect: syntax.Core(), History: bashishHistory}
	line := "export AWS_ACCESS_KEY_ID=" + fakeKeyID
	sh.recording(e, &added)(line)
	if len(e.history) != 1 || e.history[0] != line {
		t.Errorf("the list is %q, want the line still recallable", e.history)
	}
	if len(added) != 0 {
		t.Errorf("the file would get %q, want nothing", added)
	}
}

// A bare newline at the prompt writes nothing.
//
// The recall list and the write list are two lists now, and this is the rule
// they both need: dropping it from only one put a blank line in the file for
// every time somebody pressed return, which `load` then skipped on the way
// back in — so nothing inside the session could see it and the file grew
// anyway.
func TestABareNewlineIsNotRecorded(t *testing.T) {
	e := &editor{}
	var added []string
	sh := Shell{Runner: newTestRunner(nil), Dialect: syntax.Core(), History: bashishHistory}
	record := sh.recording(e, &added)
	for _, line := range []string{"", "   ", "\t", "echo real"} {
		record(line)
	}
	if len(e.history) != 1 || e.history[0] != "echo real" {
		t.Errorf("the list is %q, want only the line that was typed", e.history)
	}
	if len(added) != 1 || added[0] != "echo real" {
		t.Errorf("the file would get %q, want only the line that was typed", added)
	}
}
