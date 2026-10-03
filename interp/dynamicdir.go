// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"
)

// callForADirectory is Runner.CallFunction, reached through a variable set at
// init: a tilde is expanded on the way to running every builtin, and naming
// the call directly makes the builtin table's initialization depend on
// itself.
var callForADirectory func(r *Runner, ctx context.Context, name string, args ...string) (bool, error)

func init() { callForADirectory = (*Runner).CallFunction }

// A **dynamic named directory** is `~[name]`: a directory a script's own
// function names, asked at the moment the tilde is expanded, and the same
// function asked the other way when a prompt shortens the working directory.
//
// The protocol is one shell's and the core keeps it behind a hook, the way
// UserHomeDir keeps the user database: Runner.DynamicDirectoryFunctions
// names the functions to ask, in order, and nil means the bracketed tilde is
// not a construct at all. Each is called with two operands and answers in the
// `reply` array, status 0 meaning it answered:
//
//	n NAME   reply=(DIRECTORY)        `~[NAME]` expands to DIRECTORY
//	d PATH   reply=(NAME LENGTH)      the first LENGTH bytes of PATH are
//	                                  `~[NAME]` in a prompt's `%~`
//
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`), with D01prompt.ztst's function and a second
// one listed in `$zsh_directory_name_functions`:
//
//	print ~[barmy]/anything      $mydir/foo/bar/anything
//	cd foo/bar/rod; print -P %~  ~[barmy]/rod
//	print ~[scuzzy]/rubbish      no directory expansion: ~[scuzzy], and the
//	                             script ends, as a pattern that matches
//	                             nothing does — or, under `nonomatch`, the
//	                             word as written
//	print x~[F]                  a pattern: only a word's leading tilde
//	reply=(); return 0           not an answer: the word as written
//	zsh_directory_name and a function in the array both answering
//	                             the first one's answer
//	the home claiming more       `~/bar` and not `~[F]/bar`: what decides
//	                             is how much of the path each claims
//	the home claiming as much    `~/bar` — the home wins a tie
//	`hash -d` claiming as much   `~[F]/bar` — a named directory does not
//	`hash -d` claiming more      `~yy`
//	a named directory against a function claiming the whole path
//	                             `~[<parent>:l]`, however long that draws
//
// With no function at all the tilde is still a failed one: `print ~[x]/a` is
// `no directory expansion: ~[x]`.
// DynamicDirectoryFunctions is declared on Runner beside UserHomeDir.

// callDynamicDirectory asks the dialect's functions one question and answers
// the first reply that came with status 0. The caller's status is left as it
// was: the call is part of expanding a word, not a command of its own.
func (r *Runner) callDynamicDirectory(mode, arg string) ([]string, bool) {
	if r.DynamicDirectoryFunctions == nil {
		return nil, false
	}
	saved := r.status
	defer func() { r.status = saved }()
	for _, fn := range r.DynamicDirectoryFunctions(r) {
		ran, err := callForADirectory(r, r.ctx, fn, mode, arg)
		if err != nil || !ran || r.status != 0 {
			continue
		}
		reply, _ := r.arrayElems("reply")
		if len(reply) > 0 && reply[0] != "" {
			return reply, true
		}
	}
	return nil, false
}

// dynamicDirectoryTilde answers the name of a `~[name]` tilde, and whether
// the name was the bracketed form at all — a bracketed name that no function
// answered has been reported, where nomatch says so, and is not a user name to
// go looking for afterwards.
func (r *Runner) dynamicDirectoryTilde(name string) (dir string, ok, bracketed bool) {
	if r.DynamicDirectoryFunctions == nil || len(name) < 2 ||
		name[0] != '[' || name[len(name)-1] != ']' {
		return "", false, false
	}
	inner := name[1 : len(name)-1]
	if reply, answered := r.callDynamicDirectory("n", inner); answered {
		return reply[0], true, true
	}
	// Refused where a pattern that matched nothing would be — the same two
	// routes glob.go asks, in the same order — and left as written where
	// `nonomatch` says so.
	axis := r.ask(r.sem().GlobNoMatchIsError, "an unmatched pattern being an error")
	if (r.MatchOption(UnmatchedPatternIsError) || (axis && !r.MatchOption(UnmatchedPatternIsEmpty))) &&
		r.ctl != controlExit && r.ctl != controlAbandon {
		// Refused once: an assignment's value is read for its tilde twice,
		// and the refusal leaves the tilde there to be read again (#5657).
		// See Runner.refuseTilde, which guards the other refusal the same
		// way.
		r.diagf("no directory expansion: ~%s\n", name)
		r.failedExpansion()
	}
	return "", false, true
}

// dynamicDirectoryPrefix is the other direction, for a prompt: the spelling
// `~[NAME]` and the rest of the path, where a function claims a leading part
// of it.
func (r *Runner) dynamicDirectoryPrefix(dir string) (spelled string, claimed int, ok bool) {
	reply, answered := r.callDynamicDirectory("d", dir)
	if !answered || len(reply) < 2 {
		return "", 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(reply[1]))
	if err != nil || n < 0 || n > len(dir) {
		return "", 0, false
	}
	return "~[" + reply[0] + "]" + dir[n:], n, true
}
