// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"sort"
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
//   - A bare `help` writes every topic's synopsis, one per line. The real
//     shell's own listing opens with its version and four lines about itself
//     and then packs the synopses into two truncated columns; the version
//     line is a claim we must not make, and what is left is the same
//     information one topic per line — which is also what its own
//     `help -s ''` writes.
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
	// This one is not gated on Builtin, deliberately: four of its names —
	// `dirs`, `popd`, `pushd` and `suspend` — are functions this dialect's
	// prelude defines rather than Go builtins, so asking the builtin registry
	// about them would drop four topics the shell really has.
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
		patterns = []string{""}
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
