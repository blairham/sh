// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// `$_` across a function call, and the one column whose `$_` is not a
// last-argument parameter at all.
//
// `$_` is the previous command's last argument, and `mkdir -p "$d" && cd "$_"`
// is the idiom it exists for. It is written after *any* command, a function
// call included, and a wrapper function is exactly the shape that broke it
// here: `make_dir /tmp/x` then `cd "$_"` reached the body's last command —
// `:` — where all three references reach `/tmp/x`, so the `cd` went somewhere
// else at status 0 (#3134).
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, with `inner() { :; }` and a `peek` whose body prints `$_`,
// calls `inner zz`, and prints it again:
//
//	                	bash 5.3.20	ksh93u+ 2012-08-01	zsh 5.9.2
//	body, at entry  	`outer`    	`outer`           	`two`
//	body, after `inner zz`	`zz` 	`outer`           	`zz`
//	after `peek one two`	`two`  	`two`             	`two`
//
// Three columns, three readings, and they agree about exactly one row — the
// last. That is the unanimous half and it is core: whatever a body did, the
// caller reads the **call's** own last argument once it returns. The two rows
// above it are the axes below.
//
// dash and BusyBox ash have no `$_` at all — they hold whatever the
// environment brought and nothing when it brought nothing, because `_` is an
// ordinary name there — and this shell agrees with them, which is right and
// stays.

// underscoreAcrossAFunctionCall holds `$_` to the call rather than to the body.
//
// It is given what `$_` held *before* the call's own last argument was
// recorded, and gives back the closure that puts the call's value in place
// once the body has returned.
//
// The restore is unconditional because the panel is unanimous about it. What
// is asked is only the body's *entry* value, and only in a dialect that has
// the parameter at all: a shell that keeps no `$_` has nothing to decide, and
// asking there would report an unanswered axis on every function call in a
// script that never reads the name.
func (r *Runner) underscoreAcrossAFunctionCall(beforeArg string, beforeSet bool) func() {
	callArg, callSet := r.lastArg, r.lastArgSet
	if r.sem().UnderscoreTracksTheLastArgument == Yes &&
		!r.ask(r.sem().UnderscoreMovesBeforeAFunctionBody,
			"`$_` holding the call's own last argument inside the body") {
		// The body sees what the caller had. bash and ksh93 — measured on
		// the table above, where the first line of the body reads `outer`
		// and not `two`.
		r.lastArg, r.lastArgSet = beforeArg, beforeSet
	}
	return func() {
		// And the caller reads the call's, whatever the body left behind.
		// Unanimous, and the whole of #3134's first defect: without this the
		// body's last command leaked out through `$_`.
		r.lastArg, r.lastArgSet = callArg, callSet
	}
}

// The narrowed record needs no such bracket. It is not written until the
// statement that recorded it is over — see [Runner.takeInputLevelArgument] —
// so a body never sees the call's own argument and never has to be given back
// what it overwrote.

// noteInputLevelArgument records a command's last argument for the one dialect
// whose `$_` moves only between the commands the shell *reads*.
//
// ksh93 has `$_` — the row that said it kept none was measured through a
// `;`-list, where this shell writes nothing and looks like a shell without the
// parameter. It has one, and it moves under a rule none of the others have:
// only a **simple command standing alone on a line at the top level of the
// input** puts anything in it. Measured 2026-09-16, ksh93u+ 2012-08-01, over a
// script file, reading `$_` on the line after each:
//
//	echo a b                            	`b`
//	echo a \ <newline> b c              	`c`   	one command over two lines
//	echo a b;                           	`b`   	a trailing `;` is still one
//	true a b / /bin/echo a b / . /file  	its own last argument
//	f one two                           	`two` 	a function call is a command
//	echo a b; echo c d                  	nothing moves
//	echo a b && echo c d                	nothing moves
//	echo c d | cat                      	nothing moves
//	! echo c d                          	nothing moves
//	x=5 on a line of its own            	nothing moves — and it does not clear
//	inside a `for`, `if`, `{ }`, `( )`  	nothing moves
//	inside a function body              	nothing moves
//	inside `eval`'s text                	nothing moves; the `eval` line itself does
//
// Two consequences worth naming, because both are what a simpler reading gets
// wrong. `echo one two >/dev/null; echo "$_"` — the corpus's own probe for
// this parameter — answers **empty** in ksh93, and would answer `two` in a
// shell given bash's rule; the record has held that empty cell since the
// column was first measured. And a bare assignment does not empty `$_` here,
// where bash and zsh do, which falls out of the same rule rather than needing
// one of its own.
//
// The gate is cleared the moment it is used, which is what makes a *body*
// inherit nothing: a call is a lone top-level command, so it records its own
// last argument and then everything it runs is below the input level.
func (r *Runner) noteInputLevelArgument(argv []string) {
	if !r.atInputLevel {
		return
	}
	r.atInputLevel = false
	if len(argv) > 0 {
		// Held rather than written, because this shell moves `$_` when the
		// command is *over*: the body of a function and the text of an
		// `eval` both read what stood before the line they were started
		// from, and both leave the line's own last argument behind them.
		// Writing it here instead put the call's argument inside the call.
		r.pendingInputArg = underscorePending{arg: argv[len(argv)-1], armed: true}
	}
}

// underscorePending is a last argument waiting for its command to finish.
type underscorePending struct {
	arg   string
	armed bool
}

// takeInputLevelArgument hands back the argument this level recorded, once its
// command has finished.
func (r *Runner) takeInputLevelArgument() (string, bool) {
	if !r.pendingInputArg.armed {
		return "", false
	}
	arg := r.pendingInputArg.arg
	r.pendingInputArg = underscorePending{}
	return arg, true
}

// aLoneSimpleCommandOnItsLine reports whether this top-level statement is the
// kind ksh93 moves `$_` for: a simple command, with no other statement sharing
// its line.
//
// The line test is what stands in for "the shell read one command": `a; b` is
// two statements on one line and ksh93 moves nothing for either, where the
// same two on their own lines move it twice. A `&&` chain, a pipeline and a
// negation are one statement whose command is not a call, so the kind test
// covers those without a second rule.
func (r *Runner) aLoneSimpleCommandOnItsLine(stmts []*syntax.Stmt, at int) bool {
	st := stmts[at]
	if st.Background || st.Disown || st.Coprocess {
		// Started rather than run, so what it was handed is a fact about a
		// child. Measured: `echo a b &` moves nothing here.
		return false
	}
	p, ok := st.Expr.(*syntax.Pipeline)
	if !ok || p.Negated || len(p.Cmds) != 1 {
		// An `&&` chain is a BinaryExpr, a pipe has more than one command,
		// and a `!` is the pipeline's own — all three move nothing.
		return false
	}
	if _, ok := p.Cmds[0].(*syntax.SimpleCmd); !ok {
		return false
	}
	line := r.lineOf(st.Pos())
	for i, other := range stmts {
		if i == at {
			continue
		}
		at, ok := statementLine(r, other)
		if ok && at == line {
			return false
		}
	}
	return true
}

// statementLine is the line a statement starts on, and whether it has one.
//
// A [syntax.Pipeline] with no commands and no `!` cannot answer Pos — it
// reads `Cmds[0]` — and one exists: a dialect that takes a bare negation at
// either place parses text that leaves an empty pipeline behind. Nothing else
// on this path asks a neighboring statement where it is, which is why the
// guard lives here rather than on the node.
func statementLine(r *Runner, st *syntax.Stmt) (int, bool) {
	if p, ok := st.Expr.(*syntax.Pipeline); ok && !p.Negated && len(p.Cmds) == 0 {
		return 0, false
	}
	return r.lineOf(st.Pos()), true
}

// underscoreValue is what a read of `$_` answers with, for the dialects that
// move it.
//
// Two trackers rather than one flag, because the two readings are not the same
// parameter narrowed: bash and zsh record every simple command and ksh93
// records only the ones it read at the input level, so a single record with a
// filter over it would have to know which shell it was at write time. Each is
// written where it is true and the read picks.
func (r *Runner) underscoreValue() (string, bool) {
	if r.ask(r.sem().UnderscoreMovesOnlyBetweenInputCommands,
		"`$_` moving only between the commands the shell reads") {
		return r.inputLastArg, r.inputLastArgSet
	}
	return r.lastArg, r.lastArgSet
}

// underscoreWrittenValue is what a script's own assignment to `$_` left, and
// whether it is what a read of the name answers.
//
// `_` is a name a script may **write** in the dialects that do not stamp it
// before every command, and every write to it was being lost: the parameter is
// produced rather than stored, so setVarAs records the assignment in
// Runner.assigned — which is right, and is how SECONDS works — and the
// producer never looked.
//
// Whether it is readable is the stamping rule and not a second question, so it
// is Semantics.UnderscoreMovesOnlyBetweenInputCommands again. Measured
// 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME, on ksh93u+ 2012-08-01, which is the column that answers
// yes:
//
//	: alpha
//	_=TOP;              $_ is TOP
//	: beta;             $_ is beta      — the next command the shell reads
//	_=SECOND; echo "$_"                 — SECOND, two commands on one line
//	f(){ _=INFN; echo "$_"; }; f        — INFN inside, and `f` after the call
//
// So the write stands until the **next command the shell reads** stamps the
// name, which is exactly the record that axis already distinguishes: a shell
// that stamps before every command overwrites the write before anything can
// see it — bash and zsh both answer `read _ rest` with `rest` — and one with
// no producer at all keeps the value by storing it, which is what dash and
// BusyBox ash do.
//
// Runner.underscoreWritten is what says the write is newer than the stamp; the
// stamp clears it, so there is no ordering to keep in two places.
func (r *Runner) underscoreWrittenValue() (string, bool) {
	if !r.underscoreWritten {
		return "", false
	}
	if !r.ask(r.sem().UnderscoreMovesOnlyBetweenInputCommands,
		"`$_` moving only between the commands the shell reads") {
		return "", false
	}
	value, ok := r.assigned["_"]
	return value, ok
}

// underscoreAcrossABuiltinsOwnCommands is the same bracket for the two
// builtins that run commands the script wrote rather than commands of their
// own: `eval`, `.`, and the `source` a dialect registers beside it.
//
// It reuses the function call's restore rather than spelling a second one,
// because it is the same rule and was measured to be: the entry value splits
// exactly where UnderscoreMovesBeforeAFunctionBody splits, and the exit value
// is the call's own last argument wherever the axis is Yes. A second helper
// here is how this tree grows two spellings of one rule and then fixes only
// one of them.
//
// Not asked where the answer could not be read. A shell that keeps no `$_`
// has nothing to hold, and a shell whose `$_` moves only between the commands
// it *reads* never wrote inside the text in the first place — so a bracket
// there would hold a record that never moved, and asking would report an
// unanswered axis on every `eval` in a script that never reads the name.
func (r *Runner) underscoreAcrossABuiltinsOwnCommands(argv []string, beforeArg string, beforeSet bool) func() {
	if !builtinRunsTheScriptsOwnCommands(argv) ||
		r.sem().UnderscoreTracksTheLastArgument != Yes ||
		r.sem().UnderscoreMovesOnlyBetweenInputCommands == Yes {
		return func() {}
	}
	if !r.ask(r.sem().UnderscoreHoldsTheCallAcrossEvalAndSource,
		"`$_` holding the call's own last argument across `eval` and `.`") {
		return func() {}
	}
	return r.underscoreAcrossAFunctionCall(beforeArg, beforeSet)
}
