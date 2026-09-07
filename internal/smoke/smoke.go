// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package smoke drives a realistic interactive session through a real
// terminal and reports, feature by feature, what worked.
//
// It exists because every gap on the `daily-driver` label was found by hand:
// somebody opened a pty, typed for ten minutes and wrote down what was
// missing. That is an excellent way to find the first ten and a terrible way
// to find out whether they are fixed, because it costs an afternoon every time
// and because it is a different afternoon each time. This is the same ten
// minutes, repeatable, in about twenty seconds.
//
// # Why a table rather than a pass
//
// A single assertion that stops the run answers one question and hides nine.
// A shell whose rc file is never read fails its alias, its function and its
// prompt for one reason, and a suite that stops at the first of them reports a
// shell about which nothing else is known. So every check is graded on its
// own, the session is put back at a prompt between them, and the session is
// started again if it cannot be — a check that wedges the shell costs that
// check and nothing after it.
//
// # Why the assertions are written against shells we do not have yet
//
// Most rows fail today, and that is the intended reading. The checks are
// written against what bash and zsh do, not against what this implementation
// currently does, and the ones known to be failing carry the issue that owns
// them. Weakening an assertion until it passes would produce a green table
// about a shell nobody can use — which is precisely the failure this is meant
// to prevent, since a green suite is what would let the gaps go unnoticed in
// the first place. When one is fixed, its row flips and the report says so
// out loud.
//
// # Why it is not a required check
//
// It runs real binaries on a real pseudo-terminal, calls external commands,
// and asserts on timing — a suspended job resumes or it does not. None of
// that is deterministic in the way `make check` is, and the answer also
// depends on what the machine has installed. `make wild` is not a gate for the
// same reason. It is a target you run.
package smoke

import (
	"context"
	"os"
	"sort"
)

// Dialect is one shell this suite knows how to drive.
//
// It is a value rather than a switch for the reason the substrate's own
// dialects are: what differs between bash and zsh here is data — the file each
// reads, the escape each spells the working directory with, the prompt each
// draws when nothing has told it otherwise — and a third shell should be a
// literal rather than a branch.
type Dialect struct {
	// Name is the shell's name, and is what it is invoked as.
	Name string
	// RCFile is the startup file this shell should read at an interactive,
	// non-login prompt: bash's ~/.bashrc, zsh's ~/.zshrc.
	RCFile string
	// CwdEscape draws the working directory in a prompt, ~-abbreviated:
	// bash's \w, zsh's %~.
	CwdEscape string
	// DefaultPrompt is the tail of what this shell prompts with when no
	// prompt parameter took effect. It is what the suite falls back to
	// synchronizing on, so that a shell honoring neither PS1 nor its rc file
	// is still one the other checks can be run against.
	DefaultPrompt string
	// RebindKey is the rc-file line that binds `^G` to the action that moves
	// the cursor to the start of the line, in this shell's own spelling and
	// this shell's own name for that action: `bind` and `beginning-of-line`
	// in one, `bindkey` and `beginning-of-line` in the other — the same name
	// here by coincidence, since both inherit readline's vocabulary, and not
	// everywhere (`previous-history` against `up-line-or-history`).
	RebindKey string

	// RebindKeyInViMode is the pair of rc-file lines that select vi editing
	// and bind `^O` to the same action in the keymap that mode makes current.
	//
	// Two lines rather than one, because it is two questions and a real rc
	// file asks both: whether the mode is accepted at all, and whether a
	// binding written for that mode's keymap is the live one afterwards. It
	// is the shape of the file that opened #1352 — `set -o vi` and then
	// `bind -m vi-insert`.
	RebindKeyInViMode []string

	// JobRunning and JobStopped are the words this shell lists a job's state
	// with. They differ, and the difference is the point of having a column
	// per dialect: bash writes `Running` and `Stopped`, zsh writes `running`
	// and `suspended`, and a suite that looked for bash's wording in zsh's
	// output would report a working `jobs` as broken.
	JobRunning string
	JobStopped string
}

// Bash and Zsh are the two the suite drives. Both are measured facts about the
// shells being imitated, taken from their manuals and from running them.
func Bash() Dialect {
	return Dialect{
		Name: "bash", RCFile: ".bashrc", CwdEscape: `\w`, DefaultPrompt: "$ ",
		// The quoted form, which is the one that takes a whole sequence —
		// measured, bash reads the left side of an unquoted `keyseq:function`
		// as the name of a single key.
		RebindKey: `bind '"\C-g": beginning-of-line'`,
		RebindKeyInViMode: []string{
			"set -o vi",
			`bind -m vi-insert '"\C-o": beginning-of-line'`,
		},
		JobRunning: "Running", JobStopped: "Stopped",
	}
}

func Zsh() Dialect {
	// zsh draws the host and a percent sign when nothing has said otherwise,
	// spells its prompt escapes with a percent rather than a backslash, and
	// lists a job in lower case.
	return Dialect{
		Name: "zsh", RCFile: ".zshrc", CwdEscape: "%~", DefaultPrompt: "% ",
		// The caret notation, which is this shell's and not the other's.
		RebindKey: "bindkey '^G' beginning-of-line",
		// `bindkey -v` rather than `set -o vi`, which is how this shell's own
		// rc files spell it.
		RebindKeyInViMode: []string{
			"bindkey -v",
			"bindkey -M viins '^O' beginning-of-line",
		},
		JobRunning: "running", JobStopped: "suspended",
	}
}

// Outcome is how one check came out.
type Outcome int

const (
	// Pass: the shell did what a shell does.
	Pass Outcome = iota
	// Fail: it did not.
	Fail
	// Blocked: the check could not be reached, because something it needs
	// was itself broken or missing. Counted with the failures and never with
	// the passes — a check that did not run is not a check that succeeded —
	// but reported separately, because "the rc file was never read" and "the
	// alias did not work" are one finding and not two.
	Blocked
)

func (o Outcome) String() string {
	switch o {
	case Pass:
		return "PASS"
	case Blocked:
		return "BLOCKED"
	default:
		return "FAIL"
	}
}

// Result is one row of the table.
type Result struct {
	// Feature is what was asked.
	Feature string
	// Proves is the one line saying why the row is worth having.
	Proves string
	// Outcome and Detail are the answer and how it was reached.
	Outcome Outcome
	Detail  string
	// Known is the issue that owns this row's failure, empty where none
	// does. A row with an issue that fails is expected; a row with an issue
	// that *passes* is the news this suite exists to deliver, and a row with
	// no issue that fails is a regression.
	Known string
}

// Expected reports whether this row's outcome is the one the tree currently
// admits to.
func (r Result) Expected() bool {
	if r.Known != "" {
		return r.Outcome != Pass
	}
	return r.Outcome == Pass
}

// Fixed reports that a row known to be failing has started passing.
func (r Result) Fixed() bool { return r.Known != "" && r.Outcome == Pass }

// Regressed reports a failure nothing is known to be working on.
func (r Result) Regressed() bool { return r.Known == "" && r.Outcome != Pass }

// Report is one dialect's whole session.
type Report struct {
	Dialect string
	// Binary is what was driven, so a report can be traced to a build.
	Binary  string
	Results []Result
	// Restarts counts the times the session had to be started again because
	// a check left the shell unable to prompt. It is worth printing: a suite
	// that restarts on every row is measuring its own recovery as much as
	// the shell.
	Restarts int
	// Startup is what the shell drew before anything was typed — the prompt,
	// and anything it complained about on the way to drawing it.
	Startup string
	// Err is set where the session could not be started at all, in which
	// case there are no results to read.
	Err error
}

// Failures counts rows that did not pass, blocked ones included.
func (rep Report) Failures() (n int) {
	for _, r := range rep.Results {
		if r.Outcome != Pass {
			n++
		}
	}
	return n
}

// Unexpected counts rows nothing is known to be working on that did not pass.
// It is what an exit status should be built from: a known gap is not news.
func (rep Report) Unexpected() (n int) {
	for _, r := range rep.Results {
		if r.Regressed() {
			n++
		}
	}
	return n
}

// Fixed lists the rows that have started passing since the known list was
// written.
func (rep Report) Fixed() (fixed []Result) {
	for _, r := range rep.Results {
		if r.Fixed() {
			fixed = append(fixed, r)
		}
	}
	return fixed
}

// Config is what one sweep needs.
type Config struct {
	// Bin is the shell binary to drive.
	Bin string
	// Root is where the scratch homes are made. Empty makes one under the
	// system's temporary directory and removes it afterwards.
	Root string
	// Path is the PATH the session gets. Empty inherits the caller's, which
	// is the one case where inheriting is right: a shell that cannot find
	// `sleep` is not being asked about job control.
	Path string
}

// Run drives one dialect end to end and answers what happened.
//
// It never returns an error for a check that failed — that is the report. The
// error field is for a session that could not be started, which is the one
// thing that leaves nothing to say.
func Run(ctx context.Context, d Dialect, cfg Config) Report {
	rep := Report{Dialect: d.Name, Binary: cfg.Bin}

	root := cfg.Root
	if root == "" {
		dir, err := os.MkdirTemp("", "sh-smoke-")
		if err != nil {
			rep.Err = err
			return rep
		}
		defer func() { _ = os.RemoveAll(dir) }()
		root = dir
	}
	dir, err := home(root, d)
	if err != nil {
		rep.Err = err
		return rep
	}
	path := cfg.Path
	if path == "" {
		path = os.Getenv("PATH")
	}

	s := &session{dialect: d, home: dir, bin: cfg.Bin, path: path}
	if err := s.start(ctx); err != nil {
		rep.Err = err
		rep.Startup = s.startupDrawn()
		s.stop()
		return rep
	}
	defer s.stop()
	rep.Startup = s.startupDrawn()

	st := &state{}
	for _, c := range checks() {
		outcome, detail := c.run(ctx, s, st)
		rep.Results = append(rep.Results, Result{
			Feature: c.name, Proves: c.proves,
			Outcome: outcome, Detail: detail, Known: known[c.name],
		})
		if c.endsTheSession {
			break
		}
		// Back to a prompt before the next row, or a fresh session. A check
		// that left the shell unable to prompt must not be allowed to fail
		// the ones after it — that is the whole design.
		if err := s.recover(); err != nil {
			s.stop()
			rep.Restarts++
			if err := s.start(ctx); err != nil {
				rep.Err = err
				return rep
			}
			st.sessionRestarted()
		}
	}
	return rep
}

// known is the row-to-issue map: what fails today and who owns it.
//
// It is written from the issues in flight rather than fitted to a run, which
// is the difference between a list that means something and a list that is a
// second copy of the output. A row here that starts passing is reported as
// fixed; a row not here that fails is reported as a regression.
//
// It is empty, and that is the state it is meant to reach: every row this
// suite grades passes in both dialects. An empty map is not a suite with
// nothing to say — a failure now reports as a regression rather than as a
// known gap, which is the stricter reading and the one worth having.
//
// The rows that were here are worth remembering as the shape of the thing.
// Four belonged to #807, which read no interactive startup file at all, so an
// alias, a function, an export and a PS1 in a real ~/.bashrc or ~/.zshrc did
// nothing. One belonged to #812: `0x12` fell through the editor's control-byte
// guard, so C-r was dropped and the search text was typed into the line and
// run as a command. Both are fixed.
//
// Several rows the daily-driver label listed as gaps were never here, because
// they already passed when the suite was written — filename completion (#809),
// up-arrow recall, and ^Z and `fg` for a foreground external command (#813).
// Measuring first is what kept them out.
var known = map[string]string{}

// Features is every row this suite grades, in the order it grades them.
func Features() []string {
	names := make([]string, 0, len(checks()))
	for _, c := range checks() {
		names = append(names, c.name)
	}
	return names
}

// SortedKnown is the known list as ordered pairs, for printing.
func SortedKnown() [][2]string {
	pairs := make([][2]string, 0, len(known))
	for k, v := range known {
		pairs = append(pairs, [2]string{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	return pairs
}
