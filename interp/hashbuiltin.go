// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// hash consults the builtin table through lookupBuiltin, so like `eval` and
// `.` it is registered in an init rather than in the map literal Go would
// call a cycle.
func init() { builtins["hash"] = biHash }

// biHash is `hash`, over the table commandhash.go keeps.
//
// Every shell measured accepts a bare `hash` and `hash -r` — scripts call
// both defensively — and the bare form lists what PATH has resolved so far,
// in whichever of the three shapes the dialect writes (HashListingForm).
// Naming a command checks that it could run and remembers where: found is
// silent success, and a name that resolves to nothing is the dialect's
// question — three report it at status 1, ksh93 says nothing and reports
// success.
//
// The letters past `-r` are bash's, each one asked for separately, because a
// letter is either in this dialect's set or in
// Diagnostics.UnimplementedOptionLetters and one that is in neither reads as
// "no shell has this" (#2081).
func biHash(r *Runner, _ context.Context, args []string) int {
	if r.hashBuiltinIsRefused() {
		// Every spelling, including `hash -r` and a bare listing: measured,
		// bash answers all of them with one sentence at 1 while the option
		// is off. Before the options are read, so the refusal does not
		// depend on which letters the dialect has. zsh stops filling the
		// table and leaves the builtin open, which is the other axis.
		r.diagf("%s\n", Wording(r.diag().HashDisabled, "hash: hashing disabled"))
		return 1
	}
	if r.unspecified {
		return r.status
	}
	known := r.hashOptionLetters(args)
	if r.unspecified {
		return r.status
	}
	args, opts, optArg, _, code := r.builtinOptionsArg("hash", args, known)
	if code != 0 {
		return code
	}
	switch {
	case strings.ContainsRune(opts, 'p'):
		// `-p pathname name…`: an entry put there by hand. The path is taken
		// as written — measured, bash neither checks it nor resolves it —
		// and a `-p` with no name at all is a usage error rather than a
		// no-op.
		if len(args) == 0 {
			r.builtinUsageLine("hash")
			return orDefault(r.diag().BuiltinBadOptionStatus, 2)
		}
		status := 0
		for _, name := range args {
			// The path is taken as written, and a *directory* is the one
			// thing that stops it. Measured 2026-09-21 on bash 5.3.20 and
			// bash 3.2.57, the only column with the letter: `-p /nosuchfile`
			// and `-p /dev/null` are both remembered in silence at 0, so
			// neither existing nor being executable is asked about, while
			// `-p /tmp` and `-p .` are `hash: /tmp: Is a directory` at 1
			// with the table left empty. The complaint is per name —
			// `hash -p /tmp a b` prints it twice — which is why it is inside
			// the loop rather than in front of it (#4064).
			if r.hashPathIsADirectory(optArg['p']) {
				r.diagf("%s\n", Wording(r.diag().HashPathIsADirectory,
					"hash: %[1]s: Is a directory", optArg['p']))
				status = 1
				continue
			}
			r.putHashedCommand(name, optArg['p'], 0)
		}
		return status
	case strings.ContainsRune(opts, 't'):
		// `-t` before `-d`, measured: `hash -d -t ls` reports the path and
		// leaves the entry where it was. `-l` beside it is not a second
		// action but the *shape* of this one — measured, `hash -l -t ls` and
		// `hash -t -l ls` both write the reusable command rather than the
		// path, while `hash -l ls` with no `-t` prints nothing at all
		// because an operand without `-t` is a name to hash.
		return r.hashReportPaths(args, strings.ContainsRune(opts, 'l'))
	case strings.ContainsRune(opts, 'd'):
		if r.ask(r.sem().HashDefinesANamedDirectory, "`hash -d` naming a directory") {
			return r.hashNamedDirs(args, strings.ContainsRune(opts, 'L'),
				strings.ContainsRune(opts, 'r'))
		}
		if r.unspecified {
			return r.status
		}
		return r.hashForget(args)
	}
	if strings.ContainsRune(opts, 'r') {
		r.forgetEveryHashedCommand()
		if len(args) == 0 {
			return 0
		}
		// `hash -r name` is both, in order: measured, the table comes back
		// holding that one name at zero hits.
	}
	if len(args) == 0 {
		r.printCommandHash(strings.ContainsRune(opts, 'l'))
		return 0
	}
	status := 0
	for _, name := range args {
		// An operand written with a slash is not a name PATH could hold, and
		// what a shell does with one is its own question — asked only when
		// the operand really has a slash, because everything else here is
		// unanimous. See Semantics.HashIgnoresAnOperandWithASlash.
		pathname := strings.ContainsRune(name, '/')
		if pathname && r.ask(r.sem().HashIgnoresAnOperandWithASlash,
			"`hash` passing over an operand written with a slash") {
			continue
		}
		if r.unspecified {
			return r.status
		}
		pathAlone := r.ask(r.sem().HashSearchesPathAlone, "`hash` counting only what PATH holds")
		if r.unspecified {
			return r.status
		}
		if !pathAlone {
			// A builtin or a function could run, so it hashes — and there is
			// no path to remember for either.
			if _, ok := r.lookupBuiltin(name); ok {
				continue
			}
			if _, ok := r.funcs[name]; ok {
				continue
			}
		}
		// A shell that searches PATH alone has no way to reach a written
		// pathname, so the operand falls through to the missing-name
		// question rather than being remembered as itself.
		if !pathname || !pathAlone {
			if path, err := r.lookPath(name); err == nil {
				// Zero hits, not one: an explicit `hash ls` after `ls` has
				// run puts the count back to 0 in the one dialect that shows
				// it.
				r.putHashedCommand(name, path, 0)
				continue
			}
		}
		if r.ask(r.sem().HashReportsAMissingName, "`hash` reporting a name that resolves to nothing") {
			r.diagf("%s\n", Wording(r.diag().HashNotFound, "hash: %[1]s: not found", name))
			status = 1
		}
		if r.unspecified {
			return r.status
		}
	}
	return status
}

// hashPathIsADirectory is the one check `hash -p` makes on the path it was
// handed.
//
// Against the runner's own directory rather than the process's, because a
// relative path is measurably taken relative to the shell's cwd — `hash -p .`
// blames `.` — and interp may not read a process-wide one. A stat that fails
// for any other reason is not a directory, which is the same reading the
// measurement gives a path that is simply not there.
func (r *Runner) hashPathIsADirectory(path string) bool {
	if path == "" {
		return false
	}
	st, err := r.stat(r.absolute(path))
	return err == nil && st.IsDir()
}

// hashOptionLetters is the set `hash` takes in this dialect, the paired half
// of Diagnostics.UnimplementedOptionLetters.
//
// `r` is in every column and is not asked about. The rest are bash's, and the
// `p:` says the letter takes an argument — see builtinOptionsArg.
//
// **Each one is asked for only when the call spells it.** A bare `hash`, a
// `hash -r` and a `hash name` are unanimous, so they run in a shell that has
// chosen no dialect at all; a vector that was consulted anyway would refuse
// the three commands every column agrees about in order to settle a letter
// none of them used. That is the rule this tree keeps having to relearn —
// ask at the disagreement — and TestHashAsksNothingWhereThePanelAgrees pins
// it.
func (r *Runner) hashOptionLetters(args []string) string {
	known := "r"
	for _, o := range []struct {
		spelling string
		answer   Answer
		why      string
	}{
		{"l", r.sem().HashListsAsCommands, "`hash -l`"},
		{"p:", r.sem().HashTakesAPathToRemember, "`hash -p`"},
		{"d", r.hashLetterD(), "`hash -d`"},
		{"t", r.sem().HashReportsThePath, "`hash -t`"},
	} {
		if !hashLetterSpelled(args, o.spelling[0]) {
			continue
		}
		if r.ask(o.answer, o.why) {
			known += o.spelling
		}
		if r.unspecified {
			return known
		}
	}
	// `-L` is the listing-as-commands letter in the dialect whose `-d` names
	// a directory — `hash -dL` writes `hash -d a=/tmp` — and it is accepted
	// only beside that `-d`. Measured: that shell has no `-l` at all
	// (`hash -l` is `bad option`), and a bare `hash -L` lists the *command*
	// table as commands, which is a letter this builtin does not have yet
	// and is not this change's (#2191).
	if strings.ContainsRune(known, 'd') && hashLetterSpelled(args, 'L') {
		known += "L"
	}
	return known
}

// hashLetterSpelled says whether a letter appears in the leading `-` words of
// a call, which is where an option can be.
//
// Approximate on purpose and in the safe direction: a word that is the
// *argument* of `-p` and happens to start with a dash is read for letters
// too, so `hash -p -t x` asks about `-t` when it did not have to. Over-asking
// costs an axis question in a call nobody writes; under-asking would refuse a
// letter the dialect has.
func hashLetterSpelled(args []string, letter byte) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if len(a) < 2 || a[0] != '-' {
			return false
		}
		if strings.IndexByte(a[1:], letter) >= 0 {
			return true
		}
	}
	return false
}

// hashReportPaths is `-t`: what the table holds for each name.
//
// One name is the path by itself and two or more are `name<TAB>path`, which
// is measured and is a rule about how many were asked for rather than about
// the letter. A name the table does not hold is the dialect's not-found
// wording at status 1, and the rest of the operands are still answered.
//
// asCommands is `-l` given beside it, which replaces both shapes with the
// line that would put the entry back.
func (r *Runner) hashReportPaths(names []string, asCommands bool) int {
	if len(names) == 0 {
		return r.hashLetterNeedsAName('t')
	}
	status := 0
	for _, name := range names {
		path, ok := r.hashedCommandPath(name)
		if !ok {
			r.diagf("%s\n", Wording(r.diag().HashNotFound, "hash: %[1]s: not found", name))
			status = 1
			continue
		}
		// A lookup the table answered, so it counts — see hashCommandHit.
		r.hashCommandHit(name)
		if asCommands {
			r.printf("%s\n", hashAsCommand(name, path))
			continue
		}
		if len(names) == 1 {
			r.printf("%s\n", path)
			continue
		}
		r.printf("%s\t%s\n", name, path)
	}
	return status
}

// hashForget is `-d`: one name out of the table, where `-r` is all of them.
func (r *Runner) hashForget(names []string) int {
	if len(names) == 0 {
		return r.hashLetterNeedsAName('d')
	}
	status := 0
	for _, name := range names {
		if r.forgetHashedCommand(name) {
			continue
		}
		r.diagf("%s\n", Wording(r.diag().HashNotFound, "hash: %[1]s: not found", name))
		status = 1
	}
	return status
}

// hashLetterNeedsAName is `-d` or `-t` with nothing to apply it to.
//
// The shared "option requires an argument" wording, and **1 rather than the
// bad-option status**: measured, `hash -t` answers 1 and prints no usage line
// where `hash -p` answers 2 and prints one. The two are different shapes —
// `-p` takes a word of its own, while these two take the operands — so they
// fail differently, and the usage line follows the letter that has an
// argument rather than the letter that is missing one.
func (r *Runner) hashLetterNeedsAName(letter byte) int {
	d := r.diag()
	r.complainAboutOption("hash", "%s\n", Wording(d.OptionNeedsArgument,
		"%[1]s: -%[2]s: option requires an argument",
		r.builtinComplaintName("hash"), string(letter)))
	return 1
}

// printCommandHash writes the table, in this dialect's shape.
//
// `asCommands` is `-l`, whose whole point is that its output can be pasted
// back — and whose empty case prints *nothing*, where the bare listing
// announces itself in the one dialect that has words for it.
func (r *Runner) printCommandHash(asCommands bool) {
	names := r.hashedCommandNames()
	if r.unspecified {
		return
	}
	if len(names) == 0 {
		if asCommands {
			return
		}
		// One dialect announces the empty table, on standard output.
		if w := r.diag().HashEmptyTable; w != "" {
			r.printf("%s\n", w)
		}
		return
	}
	if asCommands {
		for _, name := range names {
			r.printf("%s\n", hashAsCommand(name, r.cmdHash[name].path))
		}
		return
	}
	if r.diag().HashListing == HashListingHitsAndPath {
		r.printf("hits\tcommand\n")
	}
	for _, name := range names {
		e := r.cmdHash[name]
		switch r.diag().HashListing {
		case HashListingHitsAndPath:
			r.printf("%4d\t%s\n", e.hits, e.path)
		case HashListingNameEqualsPath:
			r.printf("%s=%s\n", name, e.path)
		default:
			r.printf("%s\n", e.path)
		}
	}
}

// hashAsCommand is one entry written as the command that would put it back,
// which is what `-l` is for. `builtin` in front of it, so a function of the
// same name cannot intercept the line when it is pasted back.
func hashAsCommand(name, path string) string {
	return "builtin hash -p " + path + " " + name
}

// hashLetterD is whether `-d` is a letter this dialect has, which is two
// different questions with one answer: the letter is in the set if it forgets
// a name *or* if it names a directory, and no dialect does both. Written this
// way so a dialect answering only the second does not have to claim the first
// as well (#2191).
func (r *Runner) hashLetterD() Answer {
	if r.sem().HashDefinesANamedDirectory == Yes || r.sem().HashForgetsOneName == Yes {
		return Yes
	}
	if r.sem().HashDefinesANamedDirectory == No && r.sem().HashForgetsOneName == No {
		return No
	}
	return Unspecified
}

// hashNamedDirs is `hash -d` in the dialect where the letter names a
// directory rather than forgetting a command.
//
// Three shapes, measured on zsh 5.9.2, 2026-09-14 — see
// Semantics.HashDefinesANamedDirectory for the table:
//
//	hash -d name=dir …   define, quietly
//	hash -d name …       ask; silent when it is there, a complaint at 1 when
//	                     it is not
//	hash -d              list, sorted by name; `-L` writes the command back
//	hash -d -r           empty the table
//
// `-r` with an operand is `too many arguments` there, and the table is left
// alone — so the clearing and the defining are not two things one call may do.
func (r *Runner) hashNamedDirs(args []string, asCommands, clear bool) int {
	if clear {
		if len(args) > 0 {
			r.builtinUsageLine("hash")
			return orDefault(r.diag().BuiltinBadOptionStatus, 2)
		}
		r.namedDirs = nil
		return 0
	}
	if len(args) == 0 {
		for _, name := range r.namedDirNames() {
			if asCommands {
				r.printf("hash -d %s=%s\n", name, namedDirValue(r.namedDirs[name]))
				continue
			}
			r.printf("%s=%s\n", name, namedDirValue(r.namedDirs[name]))
		}
		return 0
	}
	status := 0
	for _, arg := range args {
		name, dir, isAssignment := strings.Cut(arg, "=")
		if !isAssignment {
			// A name on its own asks after an entry rather than making one.
			if _, ok := r.namedDir(name); !ok {
				r.diagf("%s\n", Wording(r.diag().HashNamedDirNotFound,
					"hash: no such directory name: %[1]s", name))
				status = 1
			}
			continue
		}
		if !validNamedDirName(name) {
			// The *name*, not the word it was written in: measured,
			// `hash -d a/b=/tmp` blames `a/b`.
			r.diagf("%s\n", Wording(r.diag().HashNamedDirBadName,
				"hash: invalid character in directory name: %[1]s", name))
			status = 1
			continue
		}
		r.putNamedDir(name, dir)
	}
	return status
}
