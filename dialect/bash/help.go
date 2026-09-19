// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `help` is this shell's only self-documenting builtin, and there is no other
// name for it: a script or a person reaching for it used to get `command not
// found` at 127 (#2623).
//
// # What is here, and what deliberately is not
//
// A topic in the real shell answers with a **synopsis** and then a paragraph
// of **description**. The first is behavior — the option letters a builtin
// takes, the shape of a keyword — and this shell already carries it, measured,
// in builtinhelp.go, where it is the same string a bad option's usage line is
// built from. The second is documentation prose written by another project,
// which CLEANROOM.md's red list covers as squarely as its source; reproducing
// it would relicense a paragraph at a time.
//
// So: **`help -s` is byte-identical and the rest is the synopsis alone.** That
// is the same bargain `--help` already strikes here — see builtinhelp.go,
// which says so about the same text — rather than a new one, and it is a
// deliberate partial in the shape `set -o posix` and `shopt -s extdebug` are:
// what a script can act on is right, and what is missing is named.
//
//   - `help -s NAME` is exactly what the real shell writes.
//   - `help NAME` is that same line, where the real shell follows it with the
//     description. Better than 127 and not the whole answer.
//   - `help -d` and `help -m` are **refused by name**, because both are asked
//     *for* the description: `-d` is the one-line summary and `-m` is the
//     man-page layout built around it. A letter accepted and answered with a
//     synopsis would hand a script a success it did not earn, which is the
//     one thing a partial may not do.
//   - A bare `help` writes every topic's synopsis in **two columns**, cut to
//     the column with a `>` where they were cut, which is what the real
//     shell's listing does and is measured rather than chosen — see
//     writeHelpListing for the geometry. What is **not** reproduced is the
//     eight-line header above it: a version line is a claim we must not make
//     and the four sentences under it are another project's prose. This was
//     one topic per line for a while, on the reasoning that the header ruled
//     the whole listing out; the header and the layout are different kinds of
//     thing, and treating them as one cost 28 lines a call (#3055).
//   - `help -s ''` is the one-per-line form, and it is a **different answer**
//     from no operand at all rather than the same one: measured, the empty
//     operand writes 77 lines of `name: synopsis` where no operand writes 47.
//
// # Measured
//
// bash 5.3.15, 2026-09-13, `env -i`:
//
//	help -s true                 true: true
//	help -s cd pwd               one line each, in the order asked
//	help -s 'sh*'                a `Shell commands matching keyword` header,
//	                             a blank line, then shift and shopt
//	help -s ''                   every topic, one per line, sorted
//	help -s                      the two-column listing, as a bare `help`
//	help nosuchthing             no help topics match `nosuchthing'. …    1
//	help -q true                 help: -q: invalid option, then the usage  2
//	help -s -- cd                the operand after `--`
//	help -sd true                the *last* letter wins: `-d`'s answer
//
// The pattern header is written only when the operand is a pattern rather
// than a name: `help -s cd` has no header and `help -s 'c*'` does. Measured
// both ways.

// helpKeywordSynopses are the topics that are not builtins: the shell's own
// grammar, written the way the real shell writes it.
//
// Measured with `help -s ”` on bash 5.3.15, and kept to the constructs this
// parser actually has — a topic listed for a construct we cannot read would
// be documentation of somebody else's shell. `variables` is deliberately
// absent: its line is a sentence about what the topic contains rather than a
// synopsis, so it is description and not behavior.
func helpKeywordSynopses() map[string]string {
	return map[string]string{
		"!":         "! PIPELINE",
		"%":         "job_spec [&]",
		"(( ... ))": "(( expression ))",
		"[[ ... ]]": "[[ expression ]]",
		"case":      "case WORD in [PATTERN [| PATTERN]...) COMMANDS ;;]... esac",
		"coproc":    "coproc [NAME] command [redirections]",
		"for":       "for NAME [in WORDS ... ] ; do COMMANDS; done",
		"for ((":    "for (( exp1; exp2; exp3 )); do COMMANDS; done",
		"function":  "function name { COMMANDS ; } or name () { COMMANDS ; }",
		"if":        "if COMMANDS; then COMMANDS; [ elif COMMANDS; then COMMANDS; ]... [ else COMMANDS; ] fi",
		"select":    "select NAME [in WORDS ... ;] do COMMANDS; done",
		"time":      "time [-p] pipeline",
		"until":     "until COMMANDS; do COMMANDS-2; done",
		"while":     "while COMMANDS; do COMMANDS-2; done",
		"{ ... }":   "{ COMMANDS ; }",
	}
}

// helpOtherSynopses are the topics whose synopsis is not already in
// builtinHelp(), and the two reasons a name is here are different.
//
// Six of them — `:`, `true`, `false`, `test`, `[` and `echo` — are measured as
// having **no `--help` answer at all**: they read the word as an ordinary
// operand, which is what builtinhelp.go says about them and why they are not
// in that table. They are still help topics, so they are in this one.
//
// The other six do answer `--help` in the real shell with exactly these
// lines, measured 2026-09-13, and wiring that up is a change to what `--help`
// does rather than to what `help` does. They are here so that this builtin is
// complete now and that change can be made on its own evidence.
//
// **No name may be in both tables.** Two tables of synopses for one shell is
// how the two come to disagree, so the halves are disjoint by construction
// and TestTheTwoSynopsisTablesAreDisjoint keeps them that way.
func helpOtherSynopses() map[string]string {
	return map[string]string{
		":":     ":",
		"[":     "[ arg... ]",
		"echo":  "echo [-neE] [arg ...]",
		"false": "false",
		"test":  "test [expr]",
		"true":  "true",
		"bind": "bind [-lpsvPSVX] [-m keymap] [-f filename] [-q name] [-u name] " +
			"[-r keyseq] [-x keyseq:shell-command] " +
			"[keyseq:readline-function or readline-command]",
		"dirs": "dirs [-clpv] [+N] [-N]",
		"history": "history [-c] [-d offset] [n] or history -anrw [filename] " +
			"or history -ps arg [arg...]",
		// `logout` had no topic at all, which is one of the two names the
		// real shell lists and this one did not — measured 2026-09-18,
		// `help -s logout` is `logout: logout [n]` there and `no help
		// topics match` here. The other is `variables`, deliberately absent
		// for the reason above.
		"logout":  "logout [n]",
		"popd":    "popd [-n] [+N | -N]",
		"pushd":   "pushd [-n] [+N | -N | dir]",
		"suspend": "suspend [-f]",
	}
}

// helpTopics is every topic this shell answers for, as `name: synopsis`.
//
// The builtins come from the same table the usage lines and `--help` come
// from, so a builtin that gains or loses a synopsis gains or loses a topic
// with it and the three can never disagree. Only builtins this shell still
// has are listed: a dialect that unregisters one must not go on documenting
// it.
func helpTopics(r *interp.Runner) map[string]string {
	topics := make(map[string]string, len(helpKeywordSynopses())+64)
	for name, synopsis := range helpKeywordSynopses() {
		topics[name] = synopsis
	}
	// This one is not gated on Builtin, deliberately: three of its names —
	// `dirs`, `popd` and `pushd` — are functions this dialect's prelude
	// defines rather than Go builtins, so asking the builtin registry about
	// them would drop three topics the shell really has. `suspend` sits here
	// beside them rather than in builtinHelp because its entry is a synopsis
	// and not a help block; it is a Go builtin since #2557, so the gate would
	// pass either way.
	for name, synopsis := range helpOtherSynopses() {
		topics[name] = synopsis
	}
	for name, line := range builtinHelp() {
		if _, ok := r.Builtin(name); !ok {
			// A builtin another dialect took away. Documenting it would be
			// documenting a shell this is not.
			continue
		}
		topics[name] = strings.TrimPrefix(line, name+": ")
	}
	return topics
}

// biHelp is the builtin.
func biHelp(r *interp.Runner, _ context.Context, args []string) int {
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		for _, c := range a[1:] {
			switch c {
			case 's':
				// The one letter this shell can honor exactly, and it asks
				// for the half that is here. Nothing to record: with no
				// description to leave out, `-s` and no letter at all write
				// the same line.
			case 'd', 'm':
				// The two letters that ask *for* the description — `-d` is
				// the one-line summary and `-m` the man-page layout built
				// around it. Refused by name rather than answered with the
				// synopsis, which is the rule the whole file turns on: a
				// letter taken and answered with something else hands a
				// script a success it did not earn.
				r.Diagnosef("help: -%c: not implemented\n", c)
				return 2
			default:
				// Two lines, and only the first carries the shell's own
				// location prefix — the shape `.` with no operand already
				// has. Measured: `help -q` writes the complaint and then
				// this builtin's usage line.
				r.Diagnosef("help: -%c: invalid option\n%s\n", c, builtinUsage()["help"])
				return 2
			}
		}
	}
	topics := helpTopics(r)
	patterns := args[i:]
	if len(patterns) == 0 {
		// No operand at all is the **listing**, which is a different answer
		// from the one an empty operand gives. Measured 2026-09-18 on bash
		// 5.3.20: `help -s ''` writes 77 lines of `name: synopsis` and
		// `help -s` writes 47, the same two columns a bare `help` writes.
		writeHelpListing(r, topics)
		return 0
	}
	status := 0
	for _, p := range patterns {
		if !writeHelpTopic(r, topics, p) {
			status = 1
		}
	}
	return status
}

// writeHelpTopic writes one operand's answer, reporting whether anything
// matched.
//
// Three rules, each measured on bash 5.3.15 and each with a case that
// separates it from the rule beside it:
//
//   - **An exact topic name wins**, and writes one line with no header.
//     `help -s time` is `time` alone in a shell that also has `times`, and
//     `help -s read` is `read` alone beside `readarray` and `readonly`.
//   - **An operand with no metacharacter in it is a prefix match**, and
//     writes no header: `help -s sh` writes `shift` and `shopt`, `help -s ec`
//     writes `echo`, and `help -s c` writes all nine topics beginning with
//     `c`. Without this rule `help -s sh` would answer nothing.
//   - **An operand with one is a pattern anchored at both ends**, and writes
//     the header. `help -s '*pt'` is `compopt` and `shopt` and **not**
//     `getopts`, which is what says the pattern is not the prefix rule with a
//     `*` appended — that reading matches `getopts` and was the first thing
//     written here. `help -s 'c*d'` is `cd` and `command`.
//
// So the header hangs on the spelling rather than on the result, because the
// spelling is what chose the rule.
func writeHelpTopic(r *interp.Runner, topics map[string]string, pattern string) bool {
	if line, ok := topics[pattern]; ok {
		// A topic named exactly, which wins over the prefix match and gets no
		// header.
		_, _ = fmt.Fprintf(r.Out(), "%s: %s\n", pattern, line)
		return true
	}
	glob := strings.ContainsAny(pattern, "*?[")
	names := make([]string, 0, len(topics))
	for name := range topics {
		if glob {
			if r.MatchPattern(pattern, name) {
				names = append(names, name)
			}
			continue
		}
		if strings.HasPrefix(name, pattern) {
			names = append(names, name)
		}
	}
	if glob {
		// **Before the match, not after it.** Measured: `help -s 'z*'` writes
		// the header and the blank line and *then* complains that nothing
		// matched, at 1. Writing it only where something was found would put
		// the header on the wrong side of the one case that distinguishes
		// the two orders.
		_, _ = fmt.Fprintf(r.Out(), "Shell commands matching keyword `%s'\n\n", pattern)
	}
	if len(names) == 0 {
		r.Diagnosef("help: no help topics match `%s'.  Try `help help' or `man -k %s' or `info %s'.\n",
			pattern, pattern, pattern)
		return false
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = fmt.Fprintf(r.Out(), "%s: %s\n", name, topics[name])
	}
	return true
}

// helpListingWidth is the terminal width the listing is laid out in.
//
// `COLUMNS`, where the script has set it to something usable, and 80
// otherwise. Measured 2026-09-18 on bash 5.3.20 with the shell on a pipe and
// no terminal anywhere: an unset `COLUMNS`, a value that is not a number, a
// negative one, `0`, `1`, `3` and every value up to `7` all lay out exactly as
// `80` does, and `8` is the first that is used — two columns of four. `81`
// lays out as `80`, which is the halving rounding down rather than a second
// fallback.
//
// The shell's own parameter rather than the terminal's size, for the reason
// interp/prompt.go gives about the same name: a script may have assigned it,
// and what it assigned is what the layout is being asked about.
func helpListingWidth(r *interp.Runner) int {
	const fallback = 80
	value, ok := r.GetVar("COLUMNS")
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 8 {
		return fallback
	}
	return n
}

// writeHelpListing writes every topic's synopsis in two columns, which is what
// a bare `help` answers.
//
// It was one topic per line here, on the reasoning that the real shell's
// listing opens with a version line and four sentences about itself — both of
// which this shell must not reproduce, the first being a claim we cannot make
// and the second being another project's prose. That reasoning still holds for
// the **header**, and it never applied to the **layout**: how many columns a
// listing has, how wide a cell is and what marks a truncated one are
// measurements about a program's output rather than sentences somebody wrote,
// and they are what made this shell's listing 75 lines against 47 (#3055).
//
// The geometry, measured 2026-09-18 at COLUMNS 8, 10, 12, 40, 60, 80, 100 and
// 200, reading the character positions out of the bytes:
//
//   - a column is `COLUMNS / 2` wide, rounding down;
//   - each line opens with one space;
//   - the left cell is cut to `COLUMNS/2 - 2` characters and padded to that
//     width, then two spaces;
//   - the right cell is cut to `COLUMNS/2 - 3` characters and the line ends
//     where it ends, so the longest line is `COLUMNS - 2`;
//   - a cut cell's last character is `>`;
//   - a row with no right cell is not padded, which is the last row when the
//     count is odd.
//
// The order is **column-major**: with 77 topics and 39 rows, the left column
// is the first 39 in sorted order and the right column is the rest.
func writeHelpListing(r *interp.Runner, topics map[string]string) {
	names := make([]string, 0, len(topics))
	for name := range topics {
		names = append(names, name)
	}
	sort.Strings(names)

	half := helpListingWidth(r) / 2
	rows := (len(names) + 1) / 2
	for row := 0; row < rows; row++ {
		line := " " + helpCell(topics[names[row]], half-2)
		if right := row + rows; right < len(names) {
			line += strings.Repeat(" ", half-2-len(helpCell(topics[names[row]], half-2))) +
				"  " + helpCell(topics[names[right]], half-3)
		}
		_, _ = fmt.Fprintf(r.Out(), "%s\n", line)
	}
}

// helpCell is one synopsis cut to a column, with `>` where it was cut.
//
// A width with no room for the marker is the whole of the degenerate case,
// and it is reachable: COLUMNS=8 makes the right column five characters and
// the marker is then most of a cell.
func helpCell(text string, width int) string {
	if width < 1 {
		width = 1
	}
	if len(text) <= width {
		return text
	}
	return text[:width-1] + ">"
}
