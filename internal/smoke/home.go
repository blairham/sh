// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/internal/blocks"
)

// The scratch home a session is given.
//
// It is a directory of its own, every time, and the real one is never named:
// the suite writes an rc file, a history file and a handful of files to
// complete against, and doing any of that in a person's home would be a
// remarkably rude test. It is also the only way the answers mean anything —
// a completion graded against whatever happens to be in someone's home is
// graded against a different question on every machine.

// Marks the suite waits on.
//
// Both prompts end in the same four characters, which is what lets a session
// synchronize whether or not the rc file was read: the anchor is the suffix,
// and which prefix arrived says whether the rc took effect. Nothing a person
// would type produces it, so a wait on it cannot be answered by the echo of a
// line.
const (
	promptAnchor = "::> "
	// envPrompt is set in the environment, so it is the prompt of a shell
	// that read no startup file at all. Without it, a shell that ignores its
	// rc has no recognizable prompt and every later check would fail for one
	// reason wearing ten hats.
	envPromptPrefix = "[env:"
	// rcPrompt is set by the rc file, and is how the suite knows the file was
	// read at the moment a prompt is drawn rather than by asking afterwards.
	rcPromptPrefix = "[rc:"
	// The directory the prompt escape should draw, given that the session
	// starts in its own home.
	cwdMark = "~"
	// What separates the two escapes inside the prompt's brackets. The user
	// escape's answer is this machine's login name rather than a constant, so
	// the row that grades it reads what lies between this and the closing
	// bracket and compares it against the system's own answer.
	promptFieldSep = "|"
)

// autoCdTarget is the directory in the scratch home that a bare name is asked
// to move into.
const autoCdTarget = "projects"

// cdSpellTarget is the directory the `cdspell` row misspells, and
// cdSpellTyped is the misspelling: one transposition, which is the cheapest
// mistake a person makes and the one bash corrects.
const (
	cdSpellTarget = "documents"
	cdSpellTyped  = "documnets"
)

// dirSpellFile is the file inside that misspelled directory the completion
// row reaches for, and dirSpellPrefix is what is typed before Tab.
//
// Inside cdSpellTarget rather than in a directory of its own, because the two
// rows are the same misspelling asked of two different keys and sharing the
// name is what makes that legible. The prefix is unique in there, so a Tab
// that works has one answer rather than a list.
const (
	dirSpellFile   = "dirspell-target.txt"
	dirSpellPrefix = "dirspell-ta"
)

// completionTarget is the file Tab is asked to complete, and completionPrefix
// is what is typed before pressing it.
//
// The prefix is unique in the directory on purpose: an ambiguous completion is
// a different feature — it lists the candidates and waits — and grading one
// question with the other's answer is how a suite reports a gap that is not
// there.
const (
	completionTarget = "smoke-completion-target.txt"
	completionPrefix = "smoke-completion-ta"
)

// home builds a scratch home for one dialect and answers its path.
//
// The rc file is written under the name that dialect should read. That is the
// assertion, not a convenience: a suite that wrote to whichever file the shell
// happens to read today would pass forever and say nothing.
func home(root string, d Dialect) (string, error) {
	dir := filepath.Join(root, d.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// A directory and some files, so completion has something realistic to
	// answer and so the target is not the only name in the room. The
	// directory is also what the `autocd` row moves into, which is why it is
	// named something a person would have rather than something a test would.
	if err := os.MkdirAll(filepath.Join(dir, autoCdTarget), 0o700); err != nil {
		return "", err
	}
	// And one more, whose name is worth misspelling: the `cdspell` row types
	// it with two letters swapped.
	if err := os.MkdirAll(filepath.Join(dir, cdSpellTarget), 0o700); err != nil {
		return "", err
	}
	// And something inside it, so that the misspelling is worth Tab as well
	// as worth `cd`: a directory with nothing in it completes to nothing
	// whether or not the spelling was corrected, which is a row that passes
	// for the wrong reason.
	if err := os.WriteFile(filepath.Join(dir, cdSpellTarget, dirSpellFile), []byte("scratch\n"), 0o600); err != nil {
		return "", err
	}
	for _, name := range []string{completionTarget, "notes.txt", "other-file.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("scratch\n"), 0o600); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, tickerName), []byte(tickerText), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, d.RCFile), []byte(rcText(d)), 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

// rcText is what a person's rc file has in it, cut down to what is worth
// grading: something exported, an alias, a function, a prompt, a hook that
// runs before every prompt, and two key rebindings — the plain one and the one
// written for the editing mode the file selects, which is the shape of the
// file that opened #1352.
//
// The `autocd` line is a real rc line rather than something typed later, which
// is where a person puts it and is also the only way the row means anything:
// an option set at the prompt would be graded in the same session that turned
// it on, and a startup file is the thing that has to work.
//
// The order of the last two lines is load-bearing rather than tidy. The mode
// comes first and the plain binding after it, so that both bindings are
// recorded in the keymap the mode makes current and both are live at the
// prompt. Written the other way round the plain one would land in the map that
// was current before the mode moved, and the row for it would fail for a
// reason that is not the row's — which is the arrangement a real file happens
// to have, and is worth its own row rather than being smuggled into these two.
//
// Each of them produces text that is *not* in the line that triggers it —
// `alias-42-ok` from typing `smokealias`. A mark that is also in the typed
// line is answered by the terminal echoing the keystrokes, so a suite built
// that way passes for a shell that runs nothing at all.
func rcText(d Dialect) string {
	return fmt.Sprintf(`# Written by the smoke suite. Not a person's file.
SMOKE_RC=yes
export SMOKE_RC
alias smokealias='echo alias-$((6 * 7))-ok'
smokefunc() { echo function-$((6 * 7))-ok; }
PS1='%s%s%s%s]%s'
PS2='%s'
%s
%s
%s
%s
%s
%s
%s
`, rcPromptPrefix, d.CwdEscape, promptFieldSep, d.UserEscape, promptAnchor, continuationPrompt,
		d.PromptHook, d.AutoCdOption, d.CdSpellOption, strings.Join(d.DirSpellOption, "\n"),
		d.CheckJobsOption, strings.Join(d.RebindKeyInViMode, "\n"), d.RebindKey)
}

// The foreground job the suspend checks use.
//
// A single external command, because that is the case ^Z is about — suspend an
// editor, do something else, come back to it — and because it is the case with
// one right answer. Whether a shell can suspend a loop it is running *itself*
// is a harder and separate question, and grading the two together would report
// a shell that does the ordinary thing correctly as broken.
//
// It says something every second rather than sleeping, because both halves of
// suspend need a positive mark: that it stopped is the ticks stopping, and
// that it resumed is a tick arriving after `fg`. A plain `sleep` would let a
// shell that suspended nothing pass the second half simply by the sleep ending.
const (
	tickerName = "ticker"
	tickerText = `#!/bin/sh
# Written by the smoke suite. A foreground job that says it is alive.
while :; do
	echo tick-$((6 * 7))
	sleep 1
done
`
)

// continuationPrompt is what an unfinished construct asks with. Set so that a
// line the suite got wrong announces itself as an unfinished construct rather
// than looking like a shell that stopped answering.
const continuationPrompt = "...more> "

// environment is what the shell is started with.
//
// Assembled rather than inherited, minus the one thing that cannot be: PATH,
// because a session that cannot find `sleep` is not testing job control. The
// real HOME is not passed on and neither is anything else the person running
// this happens to have set — an rc file that was read because the *user's*
// environment pointed at it is not evidence about the shell.
func environment(dir, path string, d Dialect) []string {
	return []string{
		"HOME=" + dir,
		"PWD=" + dir,
		"PATH=" + path,
		"TERM=xterm-256color",
		// A stable language, so an external command's message is the same
		// on every machine that runs this.
		"LANG=C",
		"LC_ALL=C",
		// The prompt of a shell that read no startup file. See envPromptPrefix.
		"PS1=" + envPromptPrefix + d.CwdEscape + promptFieldSep + d.UserEscape + "]" + promptAnchor,
		"PS2=" + continuationPrompt,
		// Where this session records what it ran. See blocksStoreDir.
		//
		// HISTFILE is deliberately not set here: an empty one turns the block
		// store off as well, and unset is not empty — it leaves the session
		// recording, which is the state these rows are about.
		"SH_BLOCKS_DIR=" + blocksStoreDir(dir),
		// And the output half, by name. Since #2274 neither half is something
		// a session gets for saying nothing, so a suite that wants to grade
		// the bodies has to ask for them — the `terminal` spelling, because
		// these sessions are on a pseudo-terminal and that is the mode a
		// person at one would choose.
		//
		// TestNoStoreUnlessOneWasAskedFor is the row that asks the opposite
		// question, and it takes SH_BLOCKS_DIR back out of this list rather
		// than reaching for a second environment.
		blocks.OutputVar + "=" + blocks.CaptureTerminalValue,
	}
}

// without is the environment with one variable taken out of it.
//
// Taken out rather than set to empty, and the two are not the same question:
// an empty SH_BLOCKS_DIR is a session that was told "no store" and an absent
// one is a session that was told nothing. Only the second can answer what a
// person who has never heard of this feature gets.
func without(env []string, name string) []string {
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, name+"=") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
