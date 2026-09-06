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
	zshishHistory  = HistoryStyle{
		IgnoreSpaceOption:            "HIST_IGNORE_SPACE",
		IgnoreDupsOption:             "HIST_IGNORE_DUPS",
		Ignore:                       "HISTORY_IGNORE",
		IgnoreIsOnePattern:           true,
		PatternIgnoredStaysInSession: true,
	}
)

// ignores is the first half of ignored, for the cases that are only about
// whether a line is recorded at all. The second half — whether it may still be
// recalled — is what TestWhichIgnoredLinesStayRecallable is for, and reading it
// here would let a rule that answered the wrong half pass.
func (r historyRules) ignores(line, previous string) bool {
	ignored, _ := r.ignored(line, previous)
	return ignored
}

func rulesFor(t *testing.T, style HistoryStyle, vars map[string]string) historyRules {
	t.Helper()
	r := newTestRunner(vars)
	return historyRulesFrom(style, r.GetVar, nil, r.MatchPattern)
}

// optionsFor is rulesFor for a dialect that spells its rules as options: a
// stub namespace answering for the names given and reporting every other name
// unknown, which is what a real dialect's namespace does.
func optionsFor(t *testing.T, style HistoryStyle, on map[string]bool) historyRules {
	t.Helper()
	r := newTestRunner(nil)
	option := func(name string) (bool, bool) {
		state, known := on[name]
		return state, known
	}
	return historyRulesFrom(style, r.GetVar, option, r.MatchPattern)
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
	if !space.ignores(" echo hidden", "") {
		t.Error("a leading space did not hide the line")
	}
	if space.ignores("echo shown", "") {
		t.Error("a line with no leading space was hidden")
	}

	dups := historyRules{ignoreDups: true}
	if !dups.ignores("echo a", "echo a") {
		t.Error("the line before it was the same and it was kept")
	}
	// Measured: `echo a`, `echo a`, `echo b`, `echo a` leaves three entries
	// and not two in both shells. Only the *immediately* preceding line
	// counts, so the fourth is not a duplicate of the first.
	if dups.ignores("echo a", "echo b") {
		t.Error("a repeat with something in between was treated as a duplicate")
	}
	// The whole line and not part of one. `echo` after `echo a` is a
	// different command, and a rule that compared by containment would drop
	// every prefix of the line before it — which passes every other case
	// here, because they differ at the first character.
	if dups.ignores("echo", "echo a") {
		t.Error("a line contained in the one before it was treated as a duplicate")
	}
	if dups.ignores("echo a --now", "echo a") {
		t.Error("a line containing the one before it was treated as a duplicate")
	}
	// The first line of a session has nothing before it, and the file's last
	// line is not it: that was a different session and is already written.
	if dups.ignores("echo a", "") {
		t.Error("the first line of a session was treated as a duplicate")
	}

	// With nothing turned on, nothing is ignored — which is measured and is
	// not the obvious default: both shells record `echo a` typed twice as two
	// entries, because dropping the second is what the knob is for.
	if (historyRules{}).ignores("echo a", "echo a") {
		t.Error("a consecutive duplicate was dropped with no knob asking for it")
	}
	if (historyRules{}).ignores(" echo hidden", "") {
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
		if !rules.ignores(line, "") {
			t.Errorf("%q was recorded, want it matched", line)
		}
	}
	for _, line := range []string{"pwd x", "echo keep", "als"} {
		if rules.ignores(line, "") {
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
	if !one.ignores("scp a:b", "") {
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
	if rules.ignores(" ls", "") {
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
	if rules.ignores("anything at all", "") {
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

// A dialect that spells these two rules as options has them read from its
// option namespace and not from a variable.
//
// zsh 5.9.2 is that dialect: `setopt HIST_IGNORE_SPACE` and
// `setopt HIST_IGNORE_DUPS` are the two rules bash keeps in HISTCONTROL, and
// zsh has no HISTCONTROL at all — measured, setting it there changes nothing.
func TestWhatTheIgnoreOptionsSay(t *testing.T) {
	for _, tc := range []struct {
		name        string
		on          map[string]bool
		space, dups bool
	}{
		{name: "neither", on: map[string]bool{
			"HIST_IGNORE_SPACE": false, "HIST_IGNORE_DUPS": false,
		}},
		{name: "space", on: map[string]bool{
			"HIST_IGNORE_SPACE": true, "HIST_IGNORE_DUPS": false,
		}, space: true},
		{name: "dups", on: map[string]bool{
			"HIST_IGNORE_SPACE": false, "HIST_IGNORE_DUPS": true,
		}, dups: true},
		{name: "both", on: map[string]bool{
			"HIST_IGNORE_SPACE": true, "HIST_IGNORE_DUPS": true,
		}, space: true, dups: true},
		// A namespace that has never heard of the name leaves the rule off.
		// This is the case the second result of DialectOption exists for: a
		// lookup that answered a bare false would be indistinguishable, and a
		// lookup that answered a bare true would turn a knob nobody has.
		{name: "unknown to this shell", on: map[string]bool{}},
	} {
		got := optionsFor(t, zshishHistory, tc.on)
		if got.ignoreSpace != tc.space || got.ignoreDups != tc.dups {
			t.Errorf("%s gave space=%v dups=%v, want %v and %v",
				tc.name, got.ignoreSpace, got.ignoreDups, tc.space, tc.dups)
		}
	}
}

// A namespace that reports the name unknown is not believed about its state.
//
// The two results are separate answers and the second is the one that decides:
// a lookup saying "I do not have this name, and by the way it is on" is
// contradictory, and the rule must stay off rather than take the half of the
// answer that happens to be a bool. Nothing in the panel answers that way
// today — zsh's namespace returns false for both when it does not have a name
// — which is exactly why it needs a test: without one, dropping the `known`
// check is invisible, and a mutation run said so.
func TestAnUnknownNameIsNotBelievedAboutItsState(t *testing.T) {
	r := newTestRunner(nil)
	option := func(string) (bool, bool) { return true, false }
	got := historyRulesFrom(zshishHistory, r.GetVar, option, r.MatchPattern)
	if got.ignoreSpace || got.ignoreDups {
		t.Errorf("a namespace reporting the name unknown turned on space=%v dups=%v",
			got.ignoreSpace, got.ignoreDups)
	}
}

// A dialect that names no options is not asked about any.
//
// The empty name must not reach the namespace: a lookup handed "" that
// answered `true, true` — and one could, since a dialect writes it — would
// turn both rules on for bash, which has neither option.
func TestAnUnnamedOptionIsNotAsked(t *testing.T) {
	asked := []string{}
	r := newTestRunner(map[string]string{"HISTCONTROL": ""})
	option := func(name string) (bool, bool) {
		asked = append(asked, name)
		return true, true
	}
	got := historyRulesFrom(bashishHistory, r.GetVar, option, r.MatchPattern)
	if len(asked) != 0 {
		t.Errorf("asked the namespace about %q; bash names no options", asked)
	}
	if got.ignoreSpace || got.ignoreDups {
		t.Errorf("a namespace answering yes to everything turned on space=%v dups=%v",
			got.ignoreSpace, got.ignoreDups)
	}
}

// A session with no shell to ask has no options, which is not the same as a
// shell whose options are off — but it must behave like one rather than panic
// or guess.
func TestNoNamespaceLeavesTheOptionRulesOff(t *testing.T) {
	r := newTestRunner(nil)
	got := historyRulesFrom(zshishHistory, r.GetVar, nil, r.MatchPattern)
	if got.ignoreSpace || got.ignoreDups {
		t.Errorf("with no namespace: space=%v dups=%v, want both off",
			got.ignoreSpace, got.ignoreDups)
	}
}

// Which ignored lines a session can still recall, and it is the pattern knob
// alone that decides.
//
// Re-measured on 2026-09-06 because implementing zsh's two option-spelled
// rules was the first time anything asked. One zsh, three probes, `fc -l` read
// before exiting:
//
//	HISTORY_IGNORE='echo hidden'  `fc -l` lists `echo hidden`
//	setopt HIST_IGNORE_SPACE      `fc -l` does not list ` echo hidden`
//	setopt HIST_IGNORE_DUPS       `fc -l` does not list the repeat
//
// bash 5.3.15, bash 3.2.57 and bash-as-sh drop the line from `history` under
// all three of their rules. So the panel agrees about a blank and a repeat,
// and the axis is the pattern knob's alone. Before this, the axis was read as
// the dialect's, with the "kept" observation filed against HIST_IGNORE_SPACE —
// a knob it is not true of.
func TestWhichIgnoredLinesStayRecallable(t *testing.T) {
	// A dialect that keeps what its patterns reject. Every rule turned on at
	// once, so each case is decided by which rule fired and not by which
	// rules were available.
	keeps := historyRules{
		ignoreSpace: true, ignoreDups: true,
		patterns:    []string{"echo hidden"},
		keepIgnored: true,
		match:       func(pattern, line string) bool { return pattern == line },
	}
	for _, tc := range []struct{ line, previous, why string }{
		{" echo x", "", "a leading blank"},
		{"echo a", "echo a", "a repeat of the line before"},
	} {
		ignored, recallable := keeps.ignored(tc.line, tc.previous)
		if !ignored {
			t.Errorf("%s: the line was recorded", tc.why)
		}
		if recallable {
			t.Errorf("%s: the line stayed recallable, and no shell in the panel keeps it", tc.why)
		}
	}
	if ignored, recallable := keeps.ignored("echo hidden", ""); !ignored || !recallable {
		t.Errorf("a pattern match gave ignored=%v recallable=%v, want true and true",
			ignored, recallable)
	}

	// The same rules in a dialect that forgets what its patterns reject.
	forgets := keeps
	forgets.keepIgnored = false
	if ignored, recallable := forgets.ignored("echo hidden", ""); !ignored || recallable {
		t.Errorf("a pattern match gave ignored=%v recallable=%v, want true and false",
			ignored, recallable)
	}

	// A line no rule rejected is not recallable-by-exception: it is simply
	// recorded, and the second result must not be read without the first.
	if ignored, recallable := keeps.ignored("echo kept", ""); ignored || recallable {
		t.Errorf("an unignored line gave ignored=%v recallable=%v, want false and false",
			ignored, recallable)
	}
}

// The order the two namespaces are read in, written down rather than left to
// whichever branch runs second.
//
// No shell in the panel has both a HISTCONTROL and the two options, so this is
// a rule for a dialect that does not exist yet rather than a measurement. It
// is here because "the variable wins the last word" is the sort of thing that
// is true until somebody reorders two `if` blocks.
func TestTheVariableGetsTheLastWord(t *testing.T) {
	both := HistoryStyle{
		Control:           "HISTCONTROL",
		IgnoreSpaceOption: "HIST_IGNORE_SPACE",
		IgnoreDupsOption:  "HIST_IGNORE_DUPS",
	}
	r := newTestRunner(map[string]string{"HISTCONTROL": "ignoreboth"})
	option := func(string) (bool, bool) { return false, true }
	got := historyRulesFrom(both, r.GetVar, option, r.MatchPattern)
	if !got.ignoreSpace || !got.ignoreDups {
		t.Errorf("the options were read after the variable: space=%v dups=%v",
			got.ignoreSpace, got.ignoreDups)
	}
}

// The rules are read through the shell's own option namespace, not through a
// second copy of one.
//
// A real zsh Runner, its `setopt` registered, asked the way the session asks
// — which is what says the folding `setopt` does reaches this too, and that a
// dialect may write the spelling its documentation uses.
func TestTheOptionRulesComeFromTheShell(t *testing.T) {
	for _, spelling := range []string{
		"HIST_IGNORE_SPACE", "hist_ignore_space", "histignorespace", "Hist_Ignore_Space",
	} {
		style := HistoryStyle{IgnoreSpaceOption: spelling}
		r := newTestRunner(nil)
		on, known := r.DialectOption(spelling)
		if known {
			t.Fatalf("the bare test runner knows %q; this test needs a shell that does not", spelling)
		}
		rules := historyRulesFrom(style, r.GetVar, r.DialectOption, r.MatchPattern)
		if rules.ignoreSpace {
			t.Errorf("%q read as on from a shell that does not have it (on=%v)", spelling, on)
		}
	}
}
