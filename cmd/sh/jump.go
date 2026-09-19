// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// `cd` with a memory: going to a directory by naming a piece of it, ranked by
// how much recent use it has had.
//
// The ranking is [blocks.Store.RankDirs] and the data is the block store,
// which has recorded where every line was accepted since it existed. What is
// here is the command: which of the ranked directories a person's words mean,
// and the refusals for every corner this does not do.
//
// # Why it is in this binary and at a prompt
//
// The same three constraints dialecthint.go is written under, and they land
// the same way. cmd/sh/blocks.go already answered most of this for the other
// half of the store, and it answered it for a command rather than a flag:
//
//	A builtin would have to live in interp, which has no history, no store
//	and no business acquiring either — the substrate does not know that a
//	prompt exists — and it is not a dialect's builtin either, because no
//	shell in the panel has one and inventing a `blocks` command inside the
//	bash dialect would make that dialect not-bash. When there is an
//	interactive shell built on this substrate, the command belongs to that
//	shell.
//
// This binary is that shell. `cmd/bash` claims to *be* bash and a `z` there
// would be the same category of lie as a flag bash rejects; `cmd/sh` claims
// to be nothing, which is what makes it where a substrate seam gets a way in.
//
// **And a prompt only**, which is what keeps `sh -dialect bash` and `./bash`
// the same language. A builtin a script can call is a difference the corpus
// grades and the two binaries would answer differently; a builtin registered
// when the editor starts a line is unreachable from `-c` and from a script
// file, so the drift is confined to the thing this is for. `StartLine` is the
// seam that runs there and nowhere else — it is called once before each new
// line an interactive session reads, and a Register is idempotent, so it is
// installed by being installed again.
//
// A function defined in a startup file that calls `z` still works: a function
// body resolves its command names when it runs, and by then the prompt has
// started a line.
//
// # The name
//
// `z` is **a proposal of this change's author and not a decision the
// maintainer has made** — #1312 leaves it open, saying only that changing
// `cd` is a compatibility question and probably wrong and that a new name is
// probably right. The argument for this one is that several unrelated tools
// converged on the single letter for this exact gesture, which makes it a
// domain fact rather than any one project's expression, and it is one string
// here to change if the answer is different.
const jumpCommand = "z"

const jumpUsage = "Usage: z [-l] word ..."

// jumpLimit is how many records the ranking reads.
//
// The same number and the same argument as blocksLimit: bounded, so that this
// costs the same after a year of use, and larger than any window the decay
// leaves worth counting.
const jumpLimit = blocksLimit

// withDirJump installs the ranked jump on every line an interactive session
// of this binary reads, chaining whatever the dialect already cleared there
// rather than replacing it.
//
// A function of its own so a test can look at what the wiring produced, for
// dialecthint.go's reason: a registration dropped on the floor inside run()
// is invisible, because the only way to find out is to be at a prompt.
func withDirJump(sh driver.Shell) driver.Shell {
	outer := sh.StartLine
	b := boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: sh.Session}
	session := sh.Session
	sh.StartLine = func(r *interp.Runner) {
		if outer != nil {
			outer(r)
		}
		r.Register(jumpCommand, jumpBuiltin(b, session))
	}
	return sh
}

// jumpBuiltin is the command, with the gate and the session this invocation
// was built with closed over.
//
// The store is opened per call rather than held, because where it lives is a
// variable a person can set at the prompt and mean — the same rule repl's own
// blocksDir follows — and opening one is resolving a path rather than opening
// a file.
func jumpBuiltin(b boundary.Boundary, session string) interp.Builtin {
	return func(r *interp.Runner, ctx context.Context, args []string) int {
		list, words, code := jumpOptions(r, args)
		if code != 0 {
			return code
		}
		dir := blocks.DirFrom(r.GetVar)
		if dir == "" {
			// The default state rather than a corner, since #2274: a store
			// exists where somebody named one and nowhere else. Refused by
			// name, with the one thing that fixes it, because a jump that
			// quietly did nothing — or quietly became `cd` — is the failure
			// this command is most exposed to.
			r.Diagnosef("%s: no block store to rank: SH_BLOCKS_DIR names one, and nothing else does\n",
				jumpCommand)
			return 1
		}
		ranked := blocks.Open(dir, b, session).RankDirs(ctx, jumpLimit, time.Now())
		if list {
			return jumpList(r, ranked, words)
		}
		return jumpTo(r, ctx, ranked, words)
	}
}

// jumpOptions reads the leading options, refusing what this does not do by
// name.
func jumpOptions(r *interp.Runner, args []string) (list bool, words []string, code int) {
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for _, letter := range word[1:] {
			if letter != 'l' {
				r.Diagnosef("%s: -%c: unknown option\n", jumpCommand, letter)
				_, _ = fmt.Fprintf(r.Err(), "%s\n", jumpUsage)
				return false, nil, 2
			}
			list = true
		}
	}
	if len(rest) == 0 && !list {
		// A bare `z` is where the other tools go home, and going home is
		// `cd`. Refused by name rather than given a meaning nobody asked
		// for: this command exists to rank, and there is nothing to rank a
		// word against.
		r.Diagnosef("%s: needs a word to match\n", jumpCommand)
		_, _ = fmt.Fprintf(r.Err(), "%s\n", jumpUsage)
		return false, nil, 2
	}
	return list, rest, 0
}

// jumpList writes the ranking, most used first.
//
// The visit count beside the score, because a score on its own is a number
// nobody can check: two directories a hundredth apart look arbitrary until
// the counts say one was used twice this morning and the other nine times
// last month. This is the only way to see what the ranking thinks, which is
// what makes the half-life in internal/blocks something a person can argue
// with.
func jumpList(r *interp.Runner, ranked []blocks.DirRank, words []string) int {
	for _, d := range jumpMatches(ranked, words) {
		if _, err := fmt.Fprintf(r.Out(), "%8.2f  %5d  %s\n", d.Score, d.Visits, d.Dir); err != nil {
			return 1
		}
	}
	return 0
}

// jumpTo goes to the best-ranked directory the words name.
//
// The stat is here rather than in the ranking, and it walks in rank order and
// stops: a directory that has been deleted is skipped, so the answer is the
// best one that is still there rather than the best one that ever was. That
// is the corner #1312 names — "a deleted directory must not be returned
// silently, and it must not be a hang" — and the two refusals below are the
// difference between *nothing matched* and *everything that matched is gone*,
// which are different things to have to fix.
func jumpTo(r *interp.Runner, ctx context.Context, ranked []blocks.DirRank, words []string) int {
	matched := jumpMatches(ranked, words)
	for _, d := range matched {
		if d.Dir == r.Dir {
			// Already here. Skipped rather than accepted, because the
			// directory a person is standing in is the one they are least
			// likely to have asked to go to, and accepting it would hide
			// the match they meant behind a jump that does nothing.
			continue
		}
		if info, err := os.Stat(d.Dir); err != nil || !info.IsDir() {
			continue
		}
		cd, ok := r.Builtin("cd")
		if !ok {
			// Nothing in the panel is without one, and a dialect that
			// withdrew it has said this command cannot work.
			r.Diagnosef("%s: no `cd` in this shell\n", jumpCommand)
			return 1
		}
		// Through `cd` rather than by assigning r.Dir, which is the whole
		// reason Runner.Builtin is exported: OLDPWD, PWD, the physical or
		// logical reading of the path and whatever the dialect made of the
		// command are all its, and a second implementation of them here is a
		// second place for them to be wrong.
		return cd(r, ctx, []string{d.Dir})
	}
	if len(matched) == 0 {
		r.Diagnosef("%s: no ranked directory matches %s\n", jumpCommand, strings.Join(words, " "))
		return 1
	}
	r.Diagnosef("%s: every ranked directory matching %s is gone\n",
		jumpCommand, strings.Join(words, " "))
	return 1
}

// jumpMatches is the ranked directories a person's words name, in rank order
// but with the ones whose *last* segment answers the last word first.
//
// The matching rule, which is this change's author's proposal and is written
// here rather than only in a commit message because it is what a person has
// to be able to predict:
//
//   - Every word must appear in the path, case-insensitively, and in the
//     order they were written. `z gh sh` means a path with `gh` somewhere and
//     `sh` after it.
//   - A path whose **last segment** holds the final word comes first. That is
//     what makes `z sh` land in `…/blairham/sh` rather than in
//     `…/sh/internal/blocks`, which is the thing that would otherwise make
//     the command feel random — a word is nearly always the name of the place
//     meant, not of something it is inside.
//
// Two tiers rather than a filter on the last segment, because `z blairham`
// has to mean something: the word is a real part of the path and there is no
// directory of that name to land in, so the ranking answers rather than
// refusing.
//
// No words is every directory, which is what `-l` alone asks for.
func jumpMatches(ranked []blocks.DirRank, words []string) []blocks.DirRank {
	if len(words) == 0 {
		return ranked
	}
	var base, anywhere []blocks.DirRank
	last := strings.ToLower(words[len(words)-1])
	for _, d := range ranked {
		if !matchesInOrder(d.Dir, words) {
			continue
		}
		if strings.Contains(strings.ToLower(lastSegment(d.Dir)), last) {
			base = append(base, d)
			continue
		}
		anywhere = append(anywhere, d)
	}
	return append(base, anywhere...)
}

// matchesInOrder reports whether every word appears in the path in the order
// written, case-insensitively and without overlapping.
func matchesInOrder(dir string, words []string) bool {
	rest := strings.ToLower(dir)
	for _, w := range words {
		i := strings.Index(rest, strings.ToLower(w))
		if i < 0 {
			return false
		}
		rest = rest[i+len(w):]
	}
	return true
}

// lastSegment is the final component of a path, and the whole path for one
// with no separator in it.
//
// Written here rather than taken from path/filepath because a recorded `Cwd`
// is a string from somebody else's session: filepath.Base answers "." for an
// empty path and strips a trailing separator, and both of those are it
// tidying up an argument rather than reading what is there.
func lastSegment(dir string) string {
	if i := strings.LastIndexByte(dir, os.PathSeparator); i >= 0 {
		return dir[i+1:]
	}
	return dir
}
