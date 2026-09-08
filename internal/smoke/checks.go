// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"
)

// A check is one row of the table.
//
// The order is the order a person does these things: the shell starts and
// reads its file, a prompt appears, something is typed, something is recalled,
// something is put in the background, something is suspended, and the session
// ends. That is what makes it a session rather than a list of assertions — a
// feature that only works when nothing else has happened yet is one this would
// catch and a set of isolated tests would not.
type check struct {
	name   string
	proves string
	// endsTheSession says there is no session left afterwards, so nothing is
	// run after it.
	endsTheSession bool
	run            func(ctx context.Context, s *session, st *state) (Outcome, string)
}

// state is what one check leaves for the next.
//
// Only what genuinely carries: whether a job was suspended decides whether
// resuming it is a failure or a check that could not be reached, and pretending
// otherwise would report `fg` as broken every time ^Z is.
type state struct {
	suspended bool
}

// sessionRestarted forgets what the previous session was in the middle of.
func (st *state) sessionRestarted() { st.suspended = false }

// A probe is a line to type and the mark its *output* carries.
//
// The two are one value because the invariant between them is the one this
// whole suite rests on: the mark must not be text that is in the line. A
// terminal echoes what is typed, so a wait on a mark the line contains is
// answered by the echo — and the check then passes for a shell that drew back
// keystrokes and ran nothing, which is what a shell with a broken run loop
// looks like. Every line here is written as an expression so that what is
// typed and what is printed are different text: `echo alias-$((6 * 7))-ok`
// goes in and `alias-42-ok` comes out.
//
// The invariant is checked rather than remembered — see the test.
type probe struct{ line, mark string }

var (
	// Asked of an exported variable rather than of the prompt, so that "the
	// file was read" and "the prompt was set from it" stay two findings.
	rcProbe       = probe{`printf 'rc=%s\n' "$SMOKE_RC"`, "rc=yes"}
	aliasProbe    = probe{"smokealias", "alias-42-ok"}
	functionProbe = probe{"smokefunc", "function-42-ok"}
	// The before-every-prompt hook, read rather than printed. The rc file
	// installs a hook that counts; this asks whether it ever ran, and the
	// answer is a number the hook produced and the line does not contain.
	//
	// It is the row #1458 would have failed: the variable was accepted, kept
	// and never read, so the count stayed at nothing and the prompt drew as
	// though the file had said nothing at all — at status 0 and in silence.
	hookProbe = probe{`printf 'hook=%s\n' "$((SMOKE_HOOK > 0))"`, "hook=1"}
	// Uppercased by the second stage, so the mark can only come from the
	// pipeline having run.
	pipelineProbe = probe{"echo pipe-$((6 * 7)) | tr a-z A-Z", "PIPE-42"}
	recallProbe   = probe{"echo recall-$((6 * 7))", "recall-42"}
	searchProbe   = probe{"echo needle-$((6 * 7))-found", "needle-42-found"}
	// A line after the one being searched for, so the search has to find what
	// it was asked for rather than the last thing typed — which up-arrow
	// would also have found.
	chaffProbe = probe{"echo chaff-$((6 * 7))", "chaff-42"}
	// The line typed *after* a fatal expansion, so its mark can only appear
	// if the session outlived the error. Measured through a pseudo-terminal
	// in bash 5.3, zsh 5.9.2, ksh93u+ and dash: all four print the
	// diagnostic and draw the next prompt (#1124).
	survivedProbe = probe{"echo survived-$((6 * 7))", "survived-42"}
	// The foreground job the suspend rows use; see tickerText.
	tickProbe = probe{"./" + tickerName, "tick-42"}
	// Typed in three pieces with a Tab in the middle, so the line here is
	// what a person would have had to type without one. The mark is produced
	// by a test on the *completed* name: a Tab that did nothing leaves a name
	// that does not exist, the test is false, and nothing is printed.
	completionProbe = probe{completionOpens + completionTarget + completionCloses, "completed-42"}
	// The whole line, as it would have to be typed without `^_`. The row
	// types it, kills it with `^U` and takes the kill back, so the mark is
	// printed only if the line came back.
	undoProbe = probe{"echo undo-$((6 * 7))-ok", "undo-42-ok"}
	// A line to run after a half-typed key has been given up on. Its only job
	// is to prove the shell is still reading.
	escapeProbe = probe{"echo escaped-$((6 * 7))-ok", "escaped-42-ok"}
	// The body of the loop typed over several lines. It is the only line of
	// the construct that produces anything, so it is the only one that can
	// carry a mark — `for i in 1 2`, `do` and `done` print nothing, and a
	// wait on any of them would be answered by the echo of the keystrokes.
	multiLineProbe = probe{"echo loop-$((6 * 7))-ok", "loop-42-ok"}
	// The window's size as the shell reports it, which is the whole of what
	// `checkwinsize` promises and is not askable anywhere but here: a session
	// with no terminal has no window, so a `-c` probe shows every shell
	// reporting both variables unset and cannot tell an implementation from
	// an absence (#1445). The session's terminal is opened 24x80, so the mark
	// is the only pair of numbers that can be right.
	windowSizeProbe = probe{`printf 'win=%sx%s\n' "${COLUMNS-unset}" "${LINES-unset}"`, "win=80x24"}
	// Where a bare directory name left the shell. Typed after the name
	// itself, so what is graded is the directory the *next* line runs in
	// rather than anything the substitution printed — bash writes `cd --
	// projects` before moving and zsh writes nothing, so the sentence is not
	// a mark two shells could share.
	autoCdProbe = probe{`printf 'moved=%s\n' "${PWD##*/}"`, "moved=projects"}
	// Where a misspelled `cd` left the shell. One line for both answers,
	// because the two shells answer it differently on purpose: the one with
	// the option corrects and lands in `documents`, the one without it
	// refuses and stays in its own home, whose basename is the dialect's
	// name. So the mark is per dialect and the *line* is shared — see
	// cdSpellProbe.
	cdSpellLine = `printf 'spell=%s\n' "${PWD##*/}"`
	// The two rebinding rows. Each line is typed in two pieces with the
	// rebound key between them, so the mark appears only if the key moved the
	// cursor back to the start — the tail is typed first and the head after
	// it, which is the same shape repl/bindings_test.go uses for an override.
	//
	// A mark that is not in either piece, because a wait on text the line
	// contains is answered by the terminal's echo — see the type's comment.
	rebindProbe   = probe{`echo "rebound-$((6 * 7))-ok"`, "rebound-42-ok"}
	rebindViProbe = probe{`echo "vibound-$((6 * 7))-ok"`, "vibound-42-ok"}
)

// typedAroundKey is one probe's line split so the rebound key is pressed
// between the two halves: the tail first, then the key, then the head.
//
// The tail alone is not a command — `-ok"` on its own is an unfinished quote —
// so a shell where the key did nothing does not print the mark, and does not
// run anything either. It says the continuation prompt instead, which names
// itself, so a failure reads as a failure rather than as a timeout.
func typedAroundKey(p probe, key string) string {
	const split = `-ok"`
	head := strings.TrimSuffix(p.line, split)
	return split + key + head + "\r"
}

// multiLineLoop is the loop as a person types it: four lines, three of which
// leave the construct unfinished.
//
// A loop rather than a quoted string or a here-document, because the three
// spellings fail apart: a quote leaves the *lexer* unfinished, and this leaves
// the *grammar* unfinished with every word complete, which is the case a
// dialect's own reading of `for` decides (#1298).
func multiLineLoop() []string {
	return []string{"for i in 1 2", "do", multiLineProbe.line, "done"}
}

const (
	completionOpens  = "[ -f "
	completionCloses = " ] && echo completed-$((6 * 7))"
	// The line `M-.` reaches back for, and the mark the line after it prints.
	//
	// Not a probe, because the two halves belong to different lines: this one
	// prints nothing, and what proves the key worked is the *next* line —
	// which cannot contain the argument, because the whole point is that it
	// was never typed there. Quoted so that the word the key carries over is
	// one word and expands to text neither line ever held.
	lastArgSeed = `: "lastarg-$((6 * 7))-ok"`
	lastArgMark = "lastarg-42-ok"
)

// probes is every one of them, for the test that holds the invariant.
func probes() []probe {
	return []probe{
		rcProbe, aliasProbe, functionProbe, hookProbe, pipelineProbe,
		recallProbe, searchProbe, chaffProbe, tickProbe, completionProbe,
		undoProbe, escapeProbe, multiLineProbe,
		rebindProbe, rebindViProbe,
		windowSizeProbe, autoCdProbe,
		cdSpellProbe(Bash()), cdSpellProbe(Zsh()),
	}
}

// cdSpellProbe is what this dialect should say after a misspelled `cd`.
//
// A shell with the option lands in the corrected directory; a shell without
// one never left home. Both are that shell's own answer, so the row grades
// each against itself rather than grading zsh against bash's feature list.
func cdSpellProbe(d Dialect) probe {
	if d.CdSpellOption == "" {
		return probe{cdSpellLine, "spell=" + d.Name}
	}
	return probe{cdSpellLine, "spell=" + cdSpellTarget}
}

func checks() []check {
	return []check{
		{
			name:   "rc file is read",
			proves: "the shell reads the startup file its dialect names, at an interactive prompt",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(rcProbe); err != nil {
					return Fail, "~/" + s.dialect.RCFile +
						" sets SMOKE_RC and the session does not have it: " + err.Error()
				}
				return Pass, "~/" + s.dialect.RCFile + " was read"
			},
		},
		{
			name:   "alias from the rc file",
			proves: "an alias defined in the rc file expands at the prompt",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(aliasProbe); err != nil {
					return Fail, err.Error()
				}
				return Pass, "smokealias expanded"
			},
		},
		{
			name:   "function from the rc file",
			proves: "a function defined in the rc file is callable at the prompt",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(functionProbe); err != nil {
					return Fail, err.Error()
				}
				return Pass, "smokefunc ran"
			},
		},
		{
			name:   "hook before every prompt",
			proves: "the hook this dialect runs before each prompt was installed from the rc file and fires",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(hookProbe); err != nil {
					return Fail, "~/" + s.dialect.RCFile +
						" installs a hook that counts and the count is still nothing: " + err.Error()
				}
				return Pass, "the rc file's hook ran before the prompt"
			},
		},
		{
			name:   "a prompt is drawn",
			proves: "the shell prompts at all, which every other row depends on",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// Answered from what was on screen before anything was typed,
				// because that is when a prompt is a prompt. It is the one
				// row the session's own synchronization already settled;
				// asking again later would only ask whether the second prompt
				// is like the first.
				return Pass, "prompted with " + quote(s.prompt) + ": " + s.startupDrawn()
			},
		},
		{
			name:   "PS1 from the rc file",
			proves: "a prompt set in the rc file is the one drawn",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				switch {
				case strings.Contains(s.startup, rcPromptPrefix):
					return Pass, "the rc file's PS1 is what was drawn"
				case strings.Contains(s.startup, envPromptPrefix):
					return Fail, "PS1 from the environment was drawn, so the rc file's assignment never happened: " +
						s.startupDrawn()
				default:
					return Fail, "neither PS1 was drawn; the shell used its own default: " + s.startupDrawn()
				}
			},
		},
		{
			name:   "the prompt's directory escape",
			proves: `a prompt escape is rendered rather than drawn literally — \w and %~`,
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// The session starts in its own home, so the ~-abbreviated
				// working directory is exactly `~` — one right answer, rather
				// than a path that differs per run.
				for _, prefix := range []string{rcPromptPrefix, envPromptPrefix} {
					if strings.Contains(s.startup, prefix+cwdMark+promptFieldSep) {
						return Pass, "drew " + quote(cwdMark) + " for " + s.dialect.CwdEscape
					}
				}
				if !drewOneOfOurPrompts(s) {
					return Blocked, "no PS1 of ours was drawn at all, so there was no escape to render: " +
						s.startupDrawn()
				}
				return Fail, s.dialect.CwdEscape + " was not rendered as " + quote(cwdMark) + ": " + s.startupDrawn()
			},
		},
		{
			name:   `the prompt's user escape`,
			proves: `a prompt escape answered from the system rather than from the session — \u and %n`,
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// Beside the directory escape rather than folded into it,
				// because the two are answered from different places and only
				// this one was broken: the directory comes from the session's
				// own PWD and the login name from the password database. This
				// suite reported 22 of 22 while `\u` drew nothing, which is
				// #1446 — the default `\u@\h:\w\$ ` of most distributions
				// rendered `@host:~$`, and an empty prompt component announces
				// nothing.
				//
				// Compared against the system's own answer rather than against
				// a name: asserting the field is merely non-empty passes
				// against a hardcoded string, and asserting a literal name
				// passes only on the machine that wrote it.
				//
				// The session's environment carries no USER or LOGNAME, which
				// is deliberate and is the shape the fault lived in — `env
				// -i`, `sudo -i`, a container, a cron job. A prompt drawing
				// the right name here is drawing it from the system.
				u, err := user.Current()
				if err != nil {
					return Blocked, "this process has no user to compare against: " + err.Error()
				}
				for _, prefix := range []string{rcPromptPrefix, envPromptPrefix} {
					if strings.Contains(s.startup, prefix+cwdMark+promptFieldSep+u.Username+"]") {
						return Pass, "drew " + quote(u.Username) + " for " + s.dialect.UserEscape
					}
				}
				if !drewOneOfOurPrompts(s) {
					return Blocked, "no PS1 of ours was drawn at all, so there was no escape to render: " +
						s.startupDrawn()
				}
				return Fail, s.dialect.UserEscape + " was not rendered as " + quote(u.Username) + ": " + s.startupDrawn()
			},
		},
		{
			name:   "the window's size reaches $COLUMNS and $LINES",
			proves: "the shell keeps the two variables a startup file reads abreast of the terminal",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(windowSizeProbe); err != nil {
					return Fail, "the terminal is 80x24 and the shell does not say so: " + err.Error()
				}
				return Pass, "$COLUMNS and $LINES are the terminal's"
			},
		},
		{
			name:   "a bare directory name is a cd",
			proves: "the option the rc file set is read by command lookup, not merely remembered",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.typeLine(autoCdTarget); err != nil {
					return Fail, err.Error()
				}
				err := s.runProbe(autoCdProbe)
				// Back where the session started whatever happened, because
				// every row after this one is written against the scratch
				// home: a check that moved the shell and left it there would
				// fail the next completion row for this row's reason.
				if cdErr := s.typeLine("cd"); cdErr != nil && err == nil {
					return Fail, "the shell would not go home again: " + cdErr.Error()
				}
				if err != nil {
					return Fail, quote(autoCdTarget) + " did not move the shell: " + err.Error()
				}
				return Pass, "typing " + quote(autoCdTarget) + " moved the shell into it"
			},
		},
		{
			name:   "a misspelled cd",
			proves: "the shell corrects a one-letter slip where it has an option for that, and refuses where it has none",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.typeLine("cd " + cdSpellTyped); err != nil {
					return Fail, err.Error()
				}
				err := s.runProbe(cdSpellProbe(s.dialect))
				// Home again whatever happened, because every row after this
				// one is written against the scratch home.
				if cdErr := s.typeLine("cd"); cdErr != nil && err == nil {
					return Fail, "the shell would not go home again: " + cdErr.Error()
				}
				if err != nil {
					return Fail, quote("cd "+cdSpellTyped) + " did not leave the shell where " +
						s.dialect.Name + " leaves it: " + err.Error()
				}
				if s.dialect.CdSpellOption == "" {
					return Pass, "refused the misspelling, which is this shell's own answer"
				}
				return Pass, "corrected " + quote(cdSpellTyped) + " to " + quote(cdSpellTarget)
			},
		},
		{
			name:   "a pipeline runs",
			proves: "the ordinary thing a person types works through a terminal, not only through -c",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if !haveExternal("tr") {
					return Blocked, "no tr on PATH to pipe into"
				}
				if err := s.runProbe(pipelineProbe); err != nil {
					return Fail, err.Error()
				}
				return Pass, "two stages ran and the second saw the first's output"
			},
		},
		{
			name:   "a loop typed over several lines",
			proves: "a construct spread over several lines is continued at the prompt, not run a line at a time",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				lines := multiLineLoop()
				for i, line := range lines {
					if err := s.send(line + "\r"); err != nil {
						return Fail, err.Error()
					}
					if i == len(lines)-1 {
						// The last line closes the construct, so what comes
						// next is the loop running rather than another
						// question.
						break
					}
					// A wait per line rather than one at the end, because
					// *where* the shell stopped continuing is the finding: a
					// header read as a whole command and a body read as one
					// look identical from the far side of the construct.
					// Safe to wait on a redrawn continuation prompt, which
					// this may be: nothing has left raw mode, so no keystroke
					// can have been dropped, and the assertion the row rests
					// on is the mark below.
					if err := s.screen.Await(continuationPrompt, budget); err != nil {
						return Fail, "the shell did not ask for more after " + quote(line) +
							", so it took an unfinished construct as a whole command: " + err.Error()
					}
				}
				if err := s.screen.Await(multiLineProbe.mark, budget); err != nil {
					return Fail, "the loop did not run: " + err.Error()
				}
				return Pass, "the construct was continued over " + strconv.Itoa(len(lines)) +
					" lines and the body ran"
			},
		},
		{
			name:   "a fatal error costs the line and not the session",
			proves: "a mistyped variable name does not close the terminal",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// `${x?word}` rather than `set -u`, because an option set
				// here would stick: every later row would be running under
				// a rule this one turned on, and the first of them to read
				// an unset name would fail for this row's reason.
				if err := s.typeLine(`echo X${SMOKE_NOPE?gone}`); err != nil {
					return Fail, err.Error()
				}
				if err := s.runProbe(survivedProbe); err != nil {
					return Fail, "the session did not survive a fatal expansion: " + err.Error()
				}
				return Pass, "the prompt came back and the next line ran"
			},
		},
		{
			name:   "Tab completes a filename",
			proves: "completion offers the files in the directory, not only commands",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				for _, keys := range []string{
					completionOpens + completionPrefix, "\t", completionCloses + "\r",
				} {
					if err := s.send(keys); err != nil {
						return Fail, err.Error()
					}
				}
				if err := s.screen.Await(completionProbe.mark, budget); err != nil {
					return Fail, "Tab did not complete " + quote(completionPrefix) + " to " +
						quote(completionTarget) + ": " + err.Error()
				}
				return Pass, "completed " + quote(completionPrefix) + " to a real file"
			},
		},
		{
			name:   "up-arrow recalls",
			proves: "the previous line comes back, which is the most-used key at a prompt",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(recallProbe); err != nil {
					return Fail, "the line to recall did not run: " + err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				// The escape sequence a real terminal sends, in one write:
				// the editor reads it as a sequence, and three separate
				// writes would let it see an escape on its own.
				if err := s.send("\x1b[A\r"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(recallProbe.mark, budget); err != nil {
					return Fail, "up-arrow did not bring back the previous line: " + err.Error()
				}
				return Pass, "the previous line came back and ran again"
			},
		},
		{
			name:   "C-r searches history",
			proves: "an earlier line can be found by what is in it",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.runProbe(searchProbe); err != nil {
					return Fail, "the line to search for did not run: " + err.Error()
				}
				if err := s.runProbe(chaffProbe); err != nil {
					return Fail, err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				if err := s.send("\x12needle\r"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(searchProbe.mark, budget); err != nil {
					return Fail, "C-r did not find the earlier line: " + err.Error()
				}
				return Pass, "C-r found a line two back and ran it"
			},
		},
		{
			name:   "M-. inserts the last argument",
			proves: "the argument just typed can be used again without retyping it",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// The line to reach back into. It prints nothing, so the only
				// wait is for the prompt after it.
				if err := s.typeLine(lastArgSeed); err != nil {
					return Fail, "the line to reach back into did not run: " + err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				if err := s.send("echo \x1b.\r"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(lastArgMark, budget); err != nil {
					return Fail, "M-. did not bring the last argument over: " + err.Error()
				}
				return Pass, "M-. inserted the previous line's last argument"
			},
		},
		{
			name:   "^_ takes a kill back",
			proves: "a line killed by mistake is recoverable without retyping it",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				// Typed, killed whole, and taken back. The mark is printed
				// only if what came back is what was killed.
				if err := s.send(undoProbe.line + "\x15\x1f\r"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(undoProbe.mark, budget); err != nil {
					return Fail, "^_ did not put back what ^U took: " + err.Error()
				}
				return Pass, "^U took the line and ^_ gave it back"
			},
		},
		{
			name:   "a key rebound in the rc file",
			proves: "a key binding written in a startup file is the one the editor honors",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				// ^G is the key, because no shell here acts on it by itself —
				// so the mark cannot be the editor's own dispatch having done
				// the work.
				if err := s.send(typedAroundKey(rebindProbe, "\a")); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(rebindProbe.mark, budget); err != nil {
					return Fail, "the rc file's binding for ^G did not move the cursor: " + err.Error()
				}
				return Pass, "^G ran the action the rc file bound it to"
			},
		},
		{
			name:   "a key rebound for the editing mode the rc file selects",
			proves: "a binding written for a vi keymap is live once the rc file has selected vi editing",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				// ^O, and a different key from the row above on purpose: this
				// one is bound only in the keymap the mode makes current, so
				// a pass says the mode was accepted *and* that the binding
				// written for it is the live one. That pair is the whole of
				// what a real bash rc file asks — `set -o vi` and then `bind
				// -m vi-insert` (#1352).
				if err := s.send(typedAroundKey(rebindViProbe, "\x0f")); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(rebindViProbe.mark, budget); err != nil {
					return Fail, "the rc file's vi-keymap binding for ^O did not move the cursor: " + err.Error()
				}
				return Pass, "^O ran the action bound in the keymap vi editing selects"
			},
		},
		{
			name:   "^C ends a half-typed key",
			proves: "an Escape typed by accident does not leave the shell unable to answer",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if err := s.atPrompt(); err != nil {
					return Fail, err.Error()
				}
				// An Escape names no key on its own, so the editor is part-way
				// through one and waiting — which is what all three real
				// shells do too. ^C is the way out, and without it there is no
				// keystroke that gets the prompt back.
				if err := s.send("\x1b"); err != nil {
					return Fail, err.Error()
				}
				if err := s.send("\x03"); err != nil {
					return Fail, err.Error()
				}
				if err := s.runProbe(escapeProbe); err != nil {
					return Fail, "the shell did not come back after ^C: " + err.Error()
				}
				return Pass, "^C ended the half-typed key and the shell went on reading"
			},
		},
		{
			name:   "a background job is listed",
			proves: "& starts a job and jobs lists it as running",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if !haveExternal("sleep") {
					return Blocked, "no sleep on PATH to put in the background"
				}
				if err := s.typeLine("sleep 30 &"); err != nil {
					return Fail, err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, "the shell did not come back after starting a background job: " + err.Error()
				}
				if err := s.runLine("jobs", s.dialect.JobRunning); err != nil {
					return Fail, "jobs did not list a running job: " + err.Error()
				}
				// Tidied up here rather than left to the session ending, so
				// the rows after this one see an empty table.
				if err := s.typeLine("kill %1"); err != nil {
					return Fail, err.Error()
				}
				return Pass, "the job was listed as " + s.dialect.JobRunning
			},
		},
		{
			name:   "^Z suspends",
			proves: "the foreground job stops and the terminal comes back — the hourly one",
			run: func(_ context.Context, s *session, st *state) (Outcome, string) {
				if !haveExternal("sleep") {
					return Blocked, "no sleep on PATH to build a foreground job from"
				}
				if err := s.runProbe(tickProbe); err != nil {
					return Fail, "the foreground job did not start: " + err.Error()
				}
				if err := s.send("\x1a"); err != nil {
					return Fail, err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, "^Z did not hand the terminal back: " + err.Error()
				}
				// A prompt is not enough on its own: a shell that drew one
				// while the job kept running would pass. The job says
				// something every second, so silence is the assertion.
				if !s.screen.Quiet(tickProbe.mark, stoppedFor) {
					return Fail, "a prompt came back but the job kept running: " + s.drawn()
				}
				st.suspended = true
				if err := s.runLine("jobs", s.dialect.JobStopped); err != nil {
					return Fail, "the job stopped but jobs does not list it as " +
						s.dialect.JobStopped + ": " + err.Error()
				}
				return Pass, "the job stopped and is listed as " + s.dialect.JobStopped
			},
		},
		{
			name:   "fg resumes",
			proves: "the suspended job runs again in the foreground",
			run: func(_ context.Context, s *session, st *state) (Outcome, string) {
				if !st.suspended {
					return Blocked, "nothing was suspended to resume"
				}
				if err := s.typeLine("fg"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(tickProbe.mark, budget); err != nil {
					return Fail, "fg did not get the job going again: " + err.Error()
				}
				// Put down before the next row: it is an endless loop in the
				// foreground.
				if err := s.send("\x03"); err != nil {
					return Fail, err.Error()
				}
				st.suspended = false
				return Pass, "the job ran again after fg"
			},
		},
		{
			name:   "a running job holds the exit",
			proves: "the option a startup file sets is read, and leaving is refused once while a job is still going",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				if !haveExternal("sleep") {
					return Blocked, "no sleep on PATH to leave running"
				}
				// The table has to be clear of *stopped* jobs first, because
				// a stopped one wins the sentence in both shells — measured —
				// and a leftover from the suspend rows would have this graded
				// against the wrong wording. `jobs` is also what clears the
				// warning's told-once flag, so it has to come before the
				// command that starts the job rather than after it.
				if err := s.typeLine("jobs"); err != nil {
					return Fail, err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, "the shell did not come back after jobs: " + err.Error()
				}
				if strings.Contains(s.drawn(), s.dialect.JobStopped) {
					return Blocked, "a job left " + s.dialect.JobStopped +
						" by an earlier row would be the one reported: " + s.drawn()
				}
				if err := s.typeLine("sleep 30 &"); err != nil {
					return Fail, err.Error()
				}
				if err := s.atPrompt(); err != nil {
					return Fail, "the shell did not come back after starting a background job: " +
						err.Error()
				}
				// Typed, not run through runLine: `exit` is the one line in
				// this suite whose success is the shell *not* doing what it
				// was told.
				if err := s.typeLine("exit"); err != nil {
					return Fail, err.Error()
				}
				if err := s.screen.Await(s.dialect.RunningJobsAtExit, budget); err != nil {
					return Fail, "the shell did not say " + quote(s.dialect.RunningJobsAtExit) +
						" when told to leave with a job still running: " + err.Error()
				}
				// It said so and it is still here, which is the half that
				// makes the sentence mean anything: a shell that printed the
				// warning and left anyway would pass on the wording alone.
				if err := s.atPrompt(); err != nil {
					return Fail, "the shell said so and then left anyway: " + err.Error()
				}
				// Only what was drawn *after* the sentence counts as the
				// listing, and that is not fussiness: zsh's sentence is
				// `you have running jobs.`, so a search of the whole screen
				// for the word this shell lists a running job with finds it
				// inside the warning and reports a table that is not there.
				held := s.screen.Text()
				after := held[strings.LastIndex(held, s.dialect.RunningJobsAtExit)+
					len(s.dialect.RunningJobsAtExit):]
				listed := strings.Contains(after, s.dialect.JobRunning)
				if listed != s.dialect.ListsJobsAtExit {
					return Fail, fmt.Sprintf(
						"the job table under the warning: listed=%v, want %v: %s",
						listed, s.dialect.ListsJobsAtExit, quote(Readable(after)))
				}
				// Put down before the next row, and with a command rather
				// than a second `exit` — which is the thing that would work.
				if err := s.typeLine("kill %1"); err != nil {
					return Fail, err.Error()
				}
				return Pass, "the exit was held and the shell said " + quote(s.dialect.RunningJobsAtExit)
			},
		},
		{
			name: "every newline reached the terminal whole",
			proves: "the next prompt starts at column 0, because nothing was written " +
				"while the terminal had its newline translation off",
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// Asked of the whole session and asked late, because the
				// failure is a race and any one command may win it: a
				// pseudo-terminal's copy of a command's output can arrive
				// after the shell has gone back to raw mode, and then the
				// newline in it moves down without returning the carriage
				// and the next prompt is drawn wherever the output ended
				// (#1356).
				//
				// The bytes and not the layout, because the layout is what a
				// person sees and the bytes are what says why: one missing
				// carriage return, in a stream where every other newline has
				// one.
				drawn := s.screen.Text()
				if bare := bareLineFeeds(drawn); bare > 0 {
					return Fail, fmt.Sprintf("%d of the newlines drawn this session had no carriage "+
						"return in front of them, so the prompt after them starts where the "+
						"output ended: %s", bare, screenTail(drawn))
				}
				if !strings.Contains(drawn, "\r\n") {
					// Nothing was measured, which is not a pass. A session
					// that drew no line at all would otherwise report one.
					return Blocked, "nothing with a newline in it was drawn this session"
				}
				return Pass, "every newline drawn was preceded by a carriage return"
			},
		},
		{
			name:           "exit leaves cleanly",
			proves:         "the session ends when told to, with the status the shell was left holding",
			endsTheSession: true,
			run: func(_ context.Context, s *session, _ *state) (Outcome, string) {
				// A command that succeeds first, because `exit` with no
				// argument leaves with the status of the last one — and the
				// row before this interrupted a job, so a shell doing exactly
				// the right thing would exit 130 and be graded as broken.
				if err := s.typeLine(":"); err != nil {
					return Fail, err.Error()
				}
				if err := s.typeLine("exit"); err != nil {
					return Fail, err.Error()
				}
				code, err := s.waitForExit()
				switch {
				case err != nil:
					return Fail, err.Error()
				case code != 0:
					return Fail, "the shell exited " + strconv.Itoa(code) + ", want 0"
				}
				return Pass, "exited 0"
			},
		},
	}
}

// stoppedFor is how long a suspended job is watched for signs of life.
//
// It is an absence, so it is a duration and not a mark, and it is the one wait
// in this suite that cannot be replaced by one: there is no output that means
// "this process is not running". A second is four of the job's own periods,
// which is the ratio that matters rather than the number.
const stoppedFor = time.Second

// haveExternal reports whether the machine has a command the session needs.
//
// A missing one blocks the row rather than failing it: a machine without
// `sleep` says nothing about this shell's job control, and reporting it as a
// gap would be the suite lying about its own subject.
func haveExternal(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func quote(s string) string { return `"` + Readable(s) + `"` }

// bareLineFeeds counts the newlines with nothing returning the carriage in
// front of them.
//
// That is what a newline written while the terminal has OPOST off looks like,
// and it is the whole of #1356: the shell puts the terminal back in its own
// line discipline to run a command, and a copy of that command's output
// arriving after raw mode came back is written with the translation already
// gone.
func bareLineFeeds(drawn string) int {
	n := 0
	for i := 0; i < len(drawn); i++ {
		if drawn[i] == '\n' && (i == 0 || drawn[i-1] != '\r') {
			n++
		}
	}
	return n
}

// screenTail is the end of what was drawn, for a failure to read.
func screenTail(drawn string) string { return quote(Readable(LastLines(drawn, 3))) }

// drewOneOfOurPrompts reports whether either prompt this suite sets reached
// the screen, which is what separates an escape that rendered wrongly from a
// shell that never used our PS1 at all.
func drewOneOfOurPrompts(s *session) bool {
	return strings.Contains(s.startup, rcPromptPrefix) || strings.Contains(s.startup, envPromptPrefix)
}
