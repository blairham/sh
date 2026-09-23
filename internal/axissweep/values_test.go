// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import "testing"

// TestPresetUseAsksEveryAxis. The value of this check is that it is total and
// costs no processes, so the thing to pin is that it really does cover the
// whole vector and really does read every dialect that claims to be a panel
// member — five of them since ash gained a column (#2263).
func TestPresetUseAsksEveryAxis(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) < 300 {
		t.Fatalf("only %d axes asked about", len(uses))
	}
	for _, u := range uses {
		for _, want := range []string{"bash", "zsh", "ksh", "dash", "ash"} {
			if _, ok := u.Held[want]; !ok {
				t.Fatalf("%s: %s was not asked what it holds", u.Field, want)
			}
		}
		if got, want := len(u.Held), len(Targets()); got != want {
			t.Fatalf("%s: %d vectors asked, want one per target (%d)", u.Field, got, want)
		}
	}
}

// TestAnAxisEveryDialectAnswersAlikeIsReported is the claim the check makes.
// SplitCommandSubstitution is the standing example — its own comment says
// "true everywhere measured, including zsh" — so if this stops being flagged,
// either the axis was triaged or the check stopped working, and the two must
// not look alike.
func TestAnAxisEveryDialectAnswersAlikeIsReported(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	var unanimous int
	for _, u := range uses {
		if u.Unanimous {
			unanimous++
			if len(u.Held) == 0 {
				t.Fatalf("%s: unanimous with nothing held", u.Field)
			}
		}
	}
	if unanimous == 0 {
		t.Skip("no axis is answered alike by all four dialects; nothing to check here")
	}
}

// TestTheTriageIsReadBackFromTheFieldItAnswers. The verdicts from #2060 are
// worth nothing if they live anywhere but the axis: a note in a pull request
// is invisible to the next sweep, which then re-opens the same 25 entries.
// The standing examples are the two the issue argued about — one reached by a
// run-time option, one by POSIX mode.
func TestTheTriageIsReadBackFromTheFieldItAnswers(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FunctionLocalTraps":     "TrapsGoBackAtTheReturn",
		"ForNameWhenTheLoopRuns": "ForNameEndsTheScriptAsASyntaxError",
	}
	seen := map[string]bool{}
	for _, u := range uses {
		value, ok := want[u.Field]
		if !ok {
			continue
		}
		seen[u.Field] = true
		if u.Explained[value] == "" {
			t.Errorf("%s: nothing says who holds %s, and its own comment does", u.Field, value)
		}
	}
	for field := range want {
		if !seen[field] {
			t.Errorf("%s: not swept at all", field)
		}
	}
	if u := findUse(uses, "FunctionLocalTraps"); u != nil && u.Why == "" {
		t.Error("FunctionLocalTraps: answered alike by all four and nothing says why it is an axis")
	}
}

// TestATriageLineIsOnlyReadWhereItIsWritten guards the parse against the two
// ways it could be too loose: a sentence that merely begins with the word, and
// a note that swallows the paragraph after it.
func TestATriageLineIsOnlyReadWhereItIsWritten(t *testing.T) {
	t.Parallel()
	got := parseNotes("Unanimous answers are common here.\n" +
		"unexhibited SomeValue: held by ksh93, measured.\n" +
		"Still the same note.\n" +
		"\n" +
		"unanimous: because the pair is the measurement.\n" +
		"\n" +
		"A closing paragraph that answers nothing.\n")
	if want := "held by ksh93, measured. Still the same note."; got.Value["SomeValue"] != want {
		t.Errorf("value note is %q, want %q", got.Value["SomeValue"], want)
	}
	if want := "because the pair is the measurement."; got.Unanimous != want {
		t.Errorf("unanimous note is %q, want %q", got.Unanimous, want)
	}
	if len(got.Value) != 1 {
		t.Errorf("%d value notes, want the one that is written", len(got.Value))
	}
}

func findUse(uses []ValueUse, field string) *ValueUse {
	for i := range uses {
		if uses[i].Field == field {
			return &uses[i]
		}
	}
	return nil
}

// TestAnUnpinnedVerdictIsReadBackPerDialect. The flip half's verdicts are
// per dialect — an axis bash never consults is one zsh may lean on — so the
// parse has to keep them apart, and a reason written for every dialect has to
// answer for the ones with no line of their own (#2057).
func TestAnUnpinnedVerdictIsReadBackPerDialect(t *testing.T) {
	t.Parallel()
	got := parseNotes("unpinned zsh: the axis is never consulted there.\n" +
		"unpinned: and this one answers for anybody else.\n")
	if want := "the axis is never consulted there."; got.Unpinned["zsh"] != want {
		t.Errorf("zsh verdict is %q, want %q", got.Unpinned["zsh"], want)
	}
	if want := "and this one answers for anybody else."; got.Unpinned[""] != want {
		t.Errorf("the shared verdict is %q, want %q", got.Unpinned[""], want)
	}
	if got := verdict(got, "zsh"); got != "the axis is never consulted there." {
		t.Errorf("zsh reads %q, want its own line rather than the shared one", got)
	}
	if got := verdict(got, "bash"); got != "and this one answers for anybody else." {
		t.Errorf("bash reads %q, want the shared line", got)
	}
	if n := parseNotes("unpinned fish: not one of the four.\n"); len(n.Unpinned) != 0 {
		t.Errorf("read a verdict for a dialect that does not exist: %v", n.Unpinned)
	}
}

// TestTheFlipVerdictsAreOnTheAxesTheyAnswer is the same claim the preset one
// makes, for the pairs triaged under #2057 that cannot be pinned by any row.
//
// A list of pairs rather than a map keyed by field, and that is not a style
// choice: an axis can be unpinned in two dialects for *opposite* reasons —
// the shell that has the option and the shell that does not — and a map held
// one dialect per field, so the second entry was a compile error and the
// second reason went unwritten.
func TestTheFlipVerdictsAreOnTheAxesTheyAnswer(t *testing.T) {
	t.Parallel()
	notes, err := FieldNotes()
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ field, dialect string }{
		{"PrintfEmptyIsNotANumber", "zsh"},
		{"SetBTurnsOffBraceExpansion", "zsh"},
		{"ValuelessDeclarationHidesTheOuterValue", "zsh"},
		// The terminal family (#2058). Every one of these is a fact about a
		// shell somebody is *watching* — a job announced, an editing mode
		// chosen, a menu laid out, a prompt asked again, a profile read —
		// and the corpus runs every case under `-c` or a script with no
		// controlling terminal, `TERM=dumb` and an empty scratch `$HOME`.
		// Re-measured 2026-09-13; the pairs that had been pinned since the
		// first run are deliberately not listed here.
		{"AnnouncesBackgroundJob", "bash"},
		{"AnnouncesBackgroundJobWithoutTheMonitor", "zsh"},
		{"AutoCdAnnouncesTheSubstitution", "zsh"},
		{"InteractiveScriptAnnouncesJobs", "bash"},
		{"InteractiveCommandStringAnnouncesJobs", "zsh"},
		{"InteractiveSelectsEmacs", "bash"},
		{"InteractiveMonitorNeedsATerminal", "zsh"},
		{"FinishedJobNoticeNeedsAPrompt", "bash"},
		{"FinishedJobNoticeNeedsAPrompt", "zsh"},
		{"InteractiveStartupFileWhenLogin", "bash"},
		{"InteractiveOptionLetters", "zsh"},
		{"SelectAssumesUnboundedWidth", "bash"},
		{"SelectEofEndsPromptLine", "zsh"},
		{"PromptAsksAgainAfterARefusedToken", "bash"},
		{"PromptAsksAgainAfterARefusedToken", "zsh"},
		{"PromptCommentsNeedTheOption", "zsh"},
		{"LoginProfileWhenNonInteractive", "bash"},
		// The startup family (#2059), and the reason the list is pairs and
		// not a map keyed by field: a shell that *has* the thing and a shell
		// that does not are unpinned for opposite reasons, and both have to
		// be answered. Three of the bash entries are an absence — the empty
		// string means no such name, and the sweep's only other value for a
		// string axis is a word it invented, so the flip is unobservable by
		// construction. Four are the administrator's copies, which live
		// where a corpus row must not write. The names a shell does hold are
		// pinned by the `startup/…` rows Case.Files made writable.
		{"StartupDirectoryVariable", "bash"},
		{"StartupDirectoryVariable", "zsh"},
		{"UnconditionalStartupFile", "bash"},
		{"LateLoginStartupFile", "bash"},
		{"NonInteractiveStartupVariable", "zsh"},
		{"StartupFileOptions.SuppressAll", "bash"},
		{"StartupFileOptions.SuppressSystem", "bash"},
		{"StartupFileOptions.SuppressSystem", "zsh"},
		{"StartupFileOptions.SuppressLogin", "zsh"},
		{"StartupFileOptions.SuppressInteractive", "zsh"},
		{"StartupFileOptions.NameInteractive", "zsh"},
		{"SystemStartupFiles.Unconditional", "bash"},
		{"SystemStartupFiles.Login", "bash"},
		{"SystemStartupFiles.Interactive", "bash"},
		{"SystemStartupFiles.LateLogin", "bash"},
	} {
		if verdict(notes[pair.field], pair.dialect) == "" {
			t.Errorf("%s: nothing says why no corpus row objects in %s", pair.field, pair.dialect)
		}
	}
}
