// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

// What this shell deliberately will not have of zsh's modules, and why.
//
// Every other absence in this package is unfinished work: a module is missing
// because nobody has written it, `zmodload` says so, and the day somebody does
// the gate opens by itself — that is the argument at the top of zmodload.go
// and it is the normal state. This file is the other kind. It holds the two
// absences that are a **decision**, each with the measurement it was taken on,
// so that the next reader does not spend a session rediscovering a cost
// somebody already paid and then reach the same answer.
//
// It exists because the failure it guards against has a shape. An absence
// nobody wrote down is indistinguishable from an oversight, and an oversight
// is filed, triaged, staffed and measured again — which is exactly what
// happened to both of these before they were recorded. A ledger is the cheap
// half of that: the entry carries the issue holding the evidence, what the
// row costs today, why the answer is *not this* rather than *not yet*, and —
// the part that keeps it from being an excuse — **what would change it**.
//
// The same terms `coverage.UnreachableByConstruction` and the sandbox ledger
// are kept on, and for the same reason: a ledger whose entries cannot go
// stale is a ledger nobody has to be right about. declined_test.go is what
// makes these ones able to. A declined module that turns up in the feature
// table, or a declined builtin its own module no longer names, fails there —
// so the day one of these is implemented, the decision has to be taken back
// out loud rather than left sitting under working code.
//
// **A suite file that skips itself reads exactly like a suite file that
// passed**, which is why each entry states the file and the figure. `make
// zsh-suite` runs the shell under test over zsh's own `Test/`, and a file
// whose first line is a `zmodload` it cannot have declines to run the rest of
// itself; both shells then print nothing much and the harness scores what
// they printed. The number beside each entry is what that file costs *while
// it refuses to run*, measured rather than reasoned, so a later reader can
// tell a row that was decided from a row nobody has looked at.

// declined is one deliberate absence.
//
// Four fields and all four required, which declined_test.go enforces. The
// issue is where the measurement lives, the cost is what the gap is worth
// today, why is the argument, and changes is the falsifier — a decision with
// no statable way of being wrong is a preference wearing a decision's clothes.
type declined struct {
	// issue is the number holding the measurement this was decided on.
	issue int
	// cost is what the gap is worth, measured, with the instrument named.
	cost string
	// why is the argument for *not this* rather than *not yet*.
	why string
	// changes is what would make the answer different. It is not a promise
	// and it is not a plan; it is the one sentence somebody could come back
	// and hold this entry against.
	changes string
}

// declinedModules is the modules `zmodload` refuses here on purpose.
//
// A module in this table is deliberately **absent from zmodloadFeatures**,
// which is what makes the refusal happen at all — see zmodloadLoad, where a
// module the table does not name is refused with the same sentence as one
// short of a feature. The test holds that absence, because an entry here
// beside an entry there would be a decision recorded against a module that
// loads.
var declinedModules = map[string]declined{
	"zsh/pcre": {
		issue: 4737,
		cost: "`V07pcre.ztst` is 2 differing lines, strict 0/1, 33.3% line " +
			"agreement — `-only V07pcre.ztst -jobs 1`, oracle " +
			"/opt/homebrew/bin/zsh 5.9.2, ours build/shells/zsh. It is the " +
			"smallest of the three files #4601 names, and both lines are " +
			"what the file costs while it declines to run.",
		why: "Go's `regexp` is RE2, which has no backtracking and therefore " +
			"cannot express two constructs this module's own suite uses. " +
			"Measured 2026-09-27 against zsh 5.9.2 and Go 1.26.1: " +
			"`[[ abcabc -pcre-match '(abc)\\1' ]]` is 0 with `$MATCH=abcabc` " +
			"there and `invalid escape sequence: \\1` from `regexp.Compile`, " +
			"and `[[ foobar -pcre-match 'foo(?=bar)' ]]` is 0 with " +
			"`$MATCH=foo` there and `invalid or unsupported Perl syntax: " +
			"(?=` here. `^ab+c$` compiles in both, which is the positive " +
			"control that says the reading is the pattern's and not the " +
			"probe's. Lookbehind, atomic groups, possessive quantifiers, " +
			"`\\K`, recursion and conditional groups are the same class, and " +
			"PCRE's `x` flag has no Go equivalent at all. " +
			"Closing the gap therefore means one of two things and neither " +
			"is proportionate. A backtracking engine written here is a " +
			"PCRE-syntax parser plus a VM plus the option letters — not a " +
			"*table* derived from a specification, which is what the " +
			"generate-don't-import rule is about (internal/widthgen, " +
			"internal/normgen) — and it reintroduces catastrophic " +
			"backtracking, which is a denial-of-service surface in a shell " +
			"that runs untrusted scripts under a sandbox policy. A " +
			"third-party engine is a dependency, and internal/depsurface " +
			"pins this module's runtime dependency surface at empty on " +
			"purpose (#2045). " +
			"And a partial module is the worst of the three: a `pcre_match` " +
			"that is right for ordinary patterns and silently wrong for a " +
			"backreference is the silent success zmodload.go's whole rule " +
			"exists to prevent, and `-pcre-match` is a *condition*, which " +
			"has no call site to refuse at.",
		changes: "A pure-Go backtracking regular-expression engine entering " +
			"this module's dependency surface as a deliberate, argued " +
			"addition to internal/depsurface's list — not a shell's worth " +
			"of engine written here, and not an RE2 approximation.",
	},
}

// declinedBuiltins is the builtins this shell deliberately does not register,
// keyed by name.
//
// The other side of declinedModules, and it does not hold a module shut: a
// missing builtin is `command not found` at the word that ran it, which is
// the loud half zmodloadHolds is built on, so the module a declined builtin
// belongs to goes on loading and a script finds out at the line that needs
// it. `zmodload zsh/zutil` is 0 here and `zregexparse` is `command not found`
// on the line that calls it — both true, and consistent for that reason.
//
// The test requires that the module still **names** each of these, which is
// the staleness check that matters: a builtin nobody's module claims is not a
// decision, it is a typo.
var declinedBuiltins = map[string]declined{
	"zregexparse": {
		issue: 4761,
		cost: "`V02zregexparse.ztst` is 6 differing lines, strict 0/1, " +
			"14.3% line agreement, static read 1/1 — `make zsh-suite " +
			"ARGS='-only V02zregexparse.ztst -jobs 1'`, oracle " +
			"/opt/homebrew/bin/zsh 5.9.2, ours build/shells/zsh, measured " +
			"on origin/main at a83fdfe7a. All six are lines we printed that " +
			"the reference never asked for. #4479's catalog quotes 1 and " +
			"has aged.",
		why: "There is no specification on CLEANROOM.md's green list to " +
			"implement from. `man zshmodules`'s entry for this builtin is " +
			"one sentence — *This implements some internals of the " +
			"_regex_arguments function* — with no synopsis line, where " +
			"`zparseopts` immediately below it has a full one; POSIX has " +
			"nothing; and the only other description of the grammar is the " +
			"implementation and the shipped `_regex_arguments`, both of " +
			"which the red list covers, the second as squarely as the " +
			"first. " +
			"What is left is black-box probing of an undocumented " +
			"state-machine language whose programs its one real caller " +
			"*generates*, so the cases a person thinks to probe are not the " +
			"cases it produces. Measured 2026-09-27 on zsh 5.9.2 under " +
			"`-f`: at least four exit statuses — `not enough arguments` at " +
			"1 for nought, one and two operands, a silent 2, a silent 1 " +
			"with both output parameters written `0`, and `invalid regex` " +
			"at 3 — two numeric output parameters, and a `:tag:desc:action` " +
			"form that is valid in some positions and refused in others. " +
			"Nothing on the interactive surface here reaches it either: " +
			"`_regex_arguments` is its only caller and this shell's " +
			"completion system does not run it (#1282). " +
			"The cheap half is deliberately not done, and that is the part " +
			"worth keeping. Registering the name so `whence -v` answers and " +
			"the arity refusal reads right is an hour's work and makes " +
			"things worse: `command not found` at 127 stops a caller, where " +
			"a wrong parse at 0, 1, 2 or 3 is a completion silently " +
			"computing something else. It would also enter the surface with " +
			"no case behind it, which is what " +
			"TestNothingEntersTheSurfaceWithoutACase exists to catch.",
		changes: "A completion system here that runs `_regex_arguments`, " +
			"which would give the builtin a caller and make the DSL " +
			"reachable rather than hypothetical — or a description of that " +
			"DSL's grammar arriving on the green list, in the manual or a " +
			"specification, so there is something to implement *from*.",
	},
}
