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
// was remembered and nothing sent. `kshoptionprint` is the twelfth: it is the
// shape of both bare listings rather than a behavior, and until #4529 it was
// recorded while the listings went on writing the deviating names.
// `notify` is the thirteenth: it moves
// Semantics.FinishedJobNoticeArrivesAtOnce, which is whether a finished job's
// notice is written the moment the job ends or held for the next prompt, and
// until #4524 it was held in both states.
// `posixtraps` is the fourteenth: it moves
// Semantics.ExitTrapIsFunctionLocal backwards — the option on is that axis
// answering No — so an EXIT trap set inside a function fires when the shell
// exits rather than when the function returns. Until #4547 it fired at the
// return in both states, including under `emulate sh`, which turns the option
// on without anyone typing `setopt`.
// `rcexpandparam` is the fifteenth: it moves
// Semantics.ParamExpansionDistributesOverTheWord, which is whether `x${a}y`
// on a two-element array is one word or two, and until #4549 it was one in
// both states — with the `${^a}` spelling of the same distribution already
// built and already working beside it. Like `posixtraps` it is read at the
// moment the question is *asked* rather than at the definition: a word in a
// function defined while the option was off distributes when the function is
// called with it on, so the read is at expansion and not at parse.
// `autopushd` is the sixteenth: it moves
// Semantics.CdPushesTheDirectoryItLeaves, which makes a successful `cd` push
// the directory it came from onto the stack `dirs` prints, and until #4592 it
// pushed nothing in either state while `pushd` and `popd` kept a stack beside
// it. Like `posixtraps` and `rcexpandparam` it is read at a *moment* and the
// moment is the one that decides: the state when `cd` starts, not the state
// when `cd` finishes — a `chpwd` that turns it off during the `cd` does not
// take the push back.
// `chaselinks` and `chasedots` are the seventeenth and eighteenth, and they
// are the first here to move a *session switch* rather than an axis:
// interp.Runner.CdResolvesSymlinks and interp.Runner.CdResolvesDotDot. Every
// column's default is the same — `cd` keeps the path a directory was reached
// by — so there is no disagreement for an axis to record, and what this shell
// has is a pair of names for moving off that default. Until #4590 both were
// remembered and `cd` through a symbolic link published the logical path in
// either state. The wider of the two is read by **two** commands at two
// moments: `cd` when it moves, and `pwd` when it prints.
// `rcquotes` is the nineteenth and the first that is not a semantics question
// at all: it moves syntax.Dialect.DoubledQuoteInSingleQuotesIsALiteralQuote,
// so a doubled `'` inside a single-quoted string is one literal quote, and
// until #4591 `””` was an empty word in both states. It is the mirror of
// `rcexpandparam` above on the one question that separates them — this one is
// read when the word is **lexed**, so a function body defined while it was on
// keeps the reading when it is called with the option off.
// `globassign` is the twenty-first, behind `badpattern`: it moves
// Semantics.ScalarAssignmentValueIsGlobbed, so the right-hand side of a plain
// scalar assignment is a pattern and `a=*.txt` stores the names it matched.
// Until #4638 it was remembered and the six characters were stored in either
// state. Like `rcexpandparam` and `posixtraps` it is read at the **store**
// and not at the parse — a function body written while it was off globs when
// it is called with it on. Its own discriminator is not a state but a
// *route*: `typeset a=*.txt` keeps the characters with the option on, so the
// noun the rule is keyed on is the assignment and not the value.
func TestTheOptionsSomethingReadsAreNotRecordedOnly(t *testing.T) {
	for _, base := range []string{
		"histignorespace", "histignoredups", "promptsp", "promptcr",
		"interactivecomments", "banghist", "autolist", "debugbeforecmd",
		"longlistjobs", "cbases", "hup", "kshoptionprint", "notify",
		"posixtraps", "rcexpandparam", "errreturn", "autopushd",
		"chaselinks", "chasedots", "rcquotes", "badpattern", "globassign",
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
	if want := 122; recordedCount != want {
		t.Errorf("%d recorded names, want %d — docs/spec/semantics.md publishes the count", recordedCount, want)
	}
}
