// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"errors"
	"fmt"
	"strings"
)

// An `exec-argv` rule refuses a program when certain words appear in its
// arguments, so that a policy can say "not `make check`" without saying "not
// `make`" (#6938):
//
//	deny exec-argv /**/make -- check
//	deny exec-argv /**/git  -- worktree remove
//	deny exec-argv /**/go   -- tool golangci-lint
//
// The maintainer approved this shape as proposed on 2026-10-29 (#6938): the
// selector word, the ` -- ` delimiter, deny only, literal words, and an
// ordered subsequence. What follows is the argument for it, kept so that a
// later change to any of those has the reasons in front of it.
//
// # The delimiter, and why it is a new selector rather than a longer `exec`
//
// A pattern is the rest of the line, which is what lets a path contain a
// space. An argument vector appended after a space would take that away, and
// it would take it away silently: `/Applications/My App/tool stash` would read
// as the program `/Applications/My` with three argument words, a rule that
// never fires. So the program and the words are split at the first ` -- ` —
// a `--` standing alone between whitespace — and a rule without one is
// refused rather than guessed at. The program pattern keeps every space it
// was written with; a program whose path itself holds ` -- ` is the one name
// this cannot write, and `?` stands in for a space there exactly as it does in
// any glob.
//
// The word is new rather than the delimiter being taught to `exec`, because
// every `exec` rule that parses today means what it says and a path may
// legally contain ` -- `. Reading an old line a new way is the misreading the
// `version` line exists to prevent; a selector this parser did not know is a
// fatal "unknown selector" in every older binary, which is the refusal that
// line asks for, without a version bump that every existing file would have
// to follow.
//
// # The match: these words, in this order, anywhere after the program
//
// Not a prefix of the arguments. A prefix is what the rule reads like, and it
// is the issue's own counterexample: `git -C dir stash` begins `-C`, so a
// prefix rule about `stash` lets it through, and so `make -j8 check` past a
// rule about `check`. Skipping options first needs to know which options take
// a value — `-C dir` — which is a fact about one program, and this package
// knows no programs.
//
// An ordered subsequence is the widening, and widening is the side a deny can
// afford: it refuses `git log --grep stash` as well, which is over-refusal the
// script sees, in the safe direction — the same trade fold.go makes for case.
// It is also why there is no `allow exec-argv`. Widening an allow grants, and
// "these words appear somewhere" is a grant `git -c alias.status='!sh' status`
// satisfies.
//
// The words are literal and compared exactly. A glob character in one is
// refused rather than read literally, because `check*` matching only the word
// `check*` would be a rule that never fires, and refused rather than given
// glob meaning because that is a second decision this does not need to make
// yet — it can be added later without changing any file that parses today.
//
// # What it cannot see
//
// This is a guardrail for a cooperative agent, not a boundary against a
// hostile script, and the design doc says so in as many words. The argument
// vector is the one the shell built, so everything that reaches the program
// some other way is out of reach: an option bundled with another (`commit
// -nm x` for `--no-verify`), an alias or a configuration file the program
// reads (`git -c alias.x=stash x`, a Makefile target that calls another), a
// launcher whose own program is what the shell execs (`env`, `xargs`, `nice`,
// `timeout`), a copy of the program under another name, and any interpreter
// the policy lets run, which is a child this gate never hears from at all.
//
// What it does see is everything the *shell* runs. When this shell is the one
// interpreting the command line — `-c`, a script, an agent's terminal request
// that arrived as a line, `eval`, a function, an alias, `command` — each inner
// exec is asked about with its own vector, so `cd x && git stash` is refused
// at the `git`.

// argvDelimiter separates an `exec-argv` rule's program pattern from its
// argument words.
const argvDelimiter = "--"

// splitArgvRule divides an `exec-argv` pattern at its first stand-alone `--`.
func splitArgvRule(rest string) (program string, words []string, err error) {
	at := -1
	for i := 0; i+len(argvDelimiter) <= len(rest); i++ {
		if rest[i:i+len(argvDelimiter)] != argvDelimiter {
			continue
		}
		before := i == 0 || isBlank(rest[i-1])
		after := i+len(argvDelimiter) == len(rest) || isBlank(rest[i+len(argvDelimiter)])
		if before && after {
			at = i
			break
		}
	}
	if at < 0 {
		return "", nil, fmt.Errorf(
			"exec-argv needs ` %s ` between the program and its arguments, as in `deny exec-argv /usr/bin/git %s stash`",
			argvDelimiter, argvDelimiter,
		)
	}
	program = strings.TrimRight(rest[:at], " \t")
	words = strings.Fields(rest[at+len(argvDelimiter):])
	if program == "" {
		return "", nil, errors.New("exec-argv needs a program pattern before `--`")
	}
	if len(words) == 0 {
		// One spelling per meaning: a rule about the program and no words is
		// `deny exec`, and two ways to write it is two lines a reviewer has
		// to recognize as the same rule.
		return "", nil, fmt.Errorf(
			"exec-argv needs at least one argument word after `--`; to refuse %s whatever its arguments, write `deny exec %s`",
			program,
			program,
		)
	}
	for _, w := range words {
		if strings.ContainsAny(w, `*?[\`) {
			return "", nil, fmt.Errorf(
				"argument word %q: argument words are literal, and a glob character in one would be a rule that never fires",
				w,
			)
		}
	}
	return program, words, nil
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' }

// argvMatches reports whether every word appears in argv after argv[0], in
// order, with anything between. argv[0] is skipped because the program is
// matched by the path the shell resolved, not by the name it was called by.
func argvMatches(words, argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	rest := argv[1:]
	for _, w := range words {
		i := 0
		for i < len(rest) && rest[i] != w {
			i++
		}
		if i == len(rest) {
			return false
		}
		rest = rest[i+1:]
	}
	return true
}
