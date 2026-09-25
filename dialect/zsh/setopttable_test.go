// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// And none of them is recorded any more, which is a claim about the table
// rather than about behavior — so it is read off the table.
//
// The honest split #856 established is that `recorded` means remembered and
// not acted on. An option something reads must leave that set, or the count
// in docs/spec/semantics.md is a promise the code no longer keeps.
//
// The two history names are read by a session before it records a line; the
// two prompt names are read by the line editor before it draws a prompt. Those
// two joined the list in #2515, where the reason they had been left behind —
// that `emulate` reset every name outside the recorded set — stopped being
// true. `interactivecomments` is the fifth and is read earlier still: the
// editor asks for it before it *parses* the line (#2537). `banghist` is the
// sixth and the most recent: it is the switch the history expander reads, so
// `unsetopt banghist` at a prompt really does stop `!!` being rewritten
// (#3093). `octalzeroes` is the seventh and is not read by a session at all:
// it moves Semantics.ArithLeadingZeroIsOctal, which is what makes `$(( 010 ))`
// eight rather than ten, and it was accepted and inert until #2884.
// `debugbeforecmd` is the eighth: it moves
// Semantics.DebugTrapRunsBeforeTheCommand, which is where the DEBUG trap
// fires, and until #4473 both states of it produced the same output.
// `longlistjobs` is the ninth: it moves Semantics.JobNoticeNamesThePID, which
// is whether a job notice names the job's pid, and until #4491 it named none
// in either state. `cbases` is the tenth: it moves
// Semantics.IntegerBaseMarkIsCSpelled, which is whether `$(( [#16] 108 ))`
// writes `0x6C` or `16#6C`, and until #4502 it wrote the second in both
// states. `hup` is the eleventh: it moves
// interp.Runner.SendsHangupToJobsAtExit — the switch bash already reaches
// under `shopt -s huponexit` — so a session that is leaving really does send
// SIGHUP to the jobs it abandons and really does say how many. Until #4509 it
// was remembered and nothing sent.
func TestTheOptionsSomethingReadsAreNotRecordedOnly(t *testing.T) {
	for _, base := range []string{
		"histignorespace", "histignoredups", "promptsp", "promptcr",
		"interactivecomments", "banghist", "autolist", "debugbeforecmd",
		"longlistjobs", "cbases", "hup", "kshoptionprint",
	} {
		o, _, ok := resolveOptionName(base)
		if !ok {
			t.Fatalf("%s is not in the table at all", base)
		}
		if o.recorded {
			t.Errorf("%s is still marked recorded, but a session reads it", base)
		}
		if o.set == nil {
			t.Errorf("%s cannot be moved, so `setopt %s` would refuse", base, base)
		}
	}
	// The count the spec publishes, read from the table rather than from the
	// prose. A name moving in or out without the document following is the
	// failure this catches.
	recordedCount := 0
	for _, o := range zshOptions {
		if o.recorded {
			recordedCount++
		}
	}
	if want := 133; recordedCount != want {
		t.Errorf("%d recorded names, want %d — docs/spec/semantics.md publishes the count", recordedCount, want)
	}
}
