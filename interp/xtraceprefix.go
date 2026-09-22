// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Where `set -x` writes an assignment that stands in front of a command.
//
// `A=3 f zz` is one command, and the assignment is part of it. Every shell in
// the panel says so in its trace and none of them says it the same way; this
// shell said nothing at all, so a traced script showed the command and not the
// one thing that changed how it ran (#3133). `PATH=/opt/bin make`, `LC_ALL=C
// sort` and `DEBUG=1 run` are the construct a person reaches for `set -x`
// *about*, and the line that was missing is the only place the value appears.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, with `f() { :; }` and `set -x` above:
//
//	shell                     	A=3 f zz             	B=4 /usr/bin/env true
//	bash 5.3.20, as-sh, 3.2.57	`+ A=3` / `+ f zz`   	`+ B=4` / `+ /usr/bin/env true`
//	ksh93u+ 2012-08-01        	`+ A=3` / `+ f zz`   	`+ /usr/bin/env true` / `+ B=4`
//	zsh 5.9.2                 	`+p> A=3 +p> f zz`   	`+p> B=4 /usr/bin/env true`
//	dash 0.5.12               	`+ A=3 f zz`         	`+ B=4 /usr/bin/env true`
//	BusyBox ash 1.37.0        	`+ A=3 f zz`         	`+ B=4 /usr/bin/env true`
//
// Three shapes, and the two columns that look alike in the left-hand cell are
// not alike: ksh93 writes the command word *first* and the assignment once its
// value has expanded, which the right-hand cell is what shows. A bare `C=5` on
// a line of its own is traced correctly in every column here already, which is
// what says the gap was the prefix position and not assignments.
//
// Decoration, so it is on Diagnostics beside TraceStyle, TraceQuoting and
// TraceArrayLiteral rather than on Semantics: it decides what is *written* and
// not what happens. #3133 proposed Semantics; every other field that answers
// "how does this shell spell a trace" is here, and splitting the answer across
// two vectors would make the next such question ask which one it belonged to.
type TracePrefixAssignment int

const (
	// TracePrefixOwnLineBefore writes one line per assignment, ahead of the
	// command's own line: `+ A=3` then `+ f zz`, and `+ D=6` `+ E=7` `+ :`
	// for a run of them. All three bash columns, and the substrate's own —
	// it is the shape three of the seven columns keep and the only one that
	// makes the assignment findable by reading down the left margin.
	TracePrefixOwnLineBefore TracePrefixAssignment = iota

	// TracePrefixOwnLineAfter writes one line per assignment *after* the
	// command's, which is ksh93 — with an exception that is not an
	// exception to this field but a consequence of what the prefix does.
	// See Runner.tracePrefixFollowsTheCommand.
	TracePrefixOwnLineAfter

	// TracePrefixOnTheCommandLine writes the assignments in front of the
	// command's words on one line, exactly as the script spells it:
	// `+ A=3 f zz`. dash and BusyBox ash.
	TracePrefixOnTheCommandLine

	// TracePrefixOnTheCommandLineRepeatingThePrefix is zsh, which puts the
	// assignments and the command on one line and writes the trace prefix a
	// second time between them — `+x.sh:3> A=3 +x.sh:3> f zz` — but only
	// when the command is one it runs itself. Measured 2026-09-16 on zsh
	// 5.9.2, with `g(){ :; }` and the same script file:
	//
	//	A=1 echo a          	two prefixes	a builtin
	//	D=4 builtin echo d  	two prefixes
	//	E=5 g               	two prefixes	a function
	//	C=3 /bin/echo c     	one prefix  	an external
	//	F=6 nosuchcmd_zz    	one prefix  	nothing to run
	//	B=2 command echo b  	one prefix  	and the `command` word is dropped
	//
	// So the second prefix stands where this shell is about to do the work,
	// and the single-prefix line is what it hands to a process. `command`
	// counts as handing it over even when what it names is a builtin, which
	// is the one row a "does this run in this shell" reading gets wrong and
	// why tracePrefixRepeatsBeforeTheCommand asks separately.
	TracePrefixOnTheCommandLineRepeatingThePrefix
)

// TracePrefixAppendIsTheJoinedValue writes an **appending** prefix as the
// value it came to, with no operator: `v=14; v+=5 cmd` traces `+ v=145`.
//
// bash alone, and only in the prefix position — measured 2026-09-16 over a
// script file with `PS4='+ '`:
//
//	                	`v+=5 true`	a bare `w+=2`
//	bash 5.3.20     	`+ v=145`  	`+ w+=2`
//	ksh93u+ 2012-08-01	`+ v+=5`   	`+ w+=2`
//	zsh 5.9.2       	`+ v+=5`   	`+ w+=2`
//
// So it is not that bash spells an append differently: the same shell keeps
// the operator for the statement and drops it for the prefix, which is the
// contrast that makes this a fact about the position. dash and BusyBox ash
// have no `+=` at all and answer nothing.
//
// A bool rather than an enum because the panel splits two ways and the
// majority answer — the assignment as written — is the zero value.
//
// tracesItsPrefix reports whether this command has an assignment prefix that
// `set -x` has anything to write for.
//
// Read once per command and kept in a local, because it is asked twice — once
// where the command word would have been traced and once where the values are
// known — and the answer must not move between them. `x=1 set +x` is the
// command that would move it.
func (r *Runner) tracesItsPrefix(assigns []*syntax.Assign, argv []string) bool {
	if !r.tracing() || len(argv) == 0 {
		return false
	}
	for _, a := range assigns {
		if a.Operand {
			// An argument to a declaration builtin, not a prefix to it:
			// `typeset x=1` is traced as the command it is.
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok && a.IsArray {
			// `1=(p q)`, which splices the parameter list rather than
			// writing one parameter. The splice reads the words for itself
			// and there is no single value to render, so the line this
			// dialect would write for it is unmeasured and none is written.
			continue
		}
		return true
	}
	return false
}

// tracePrefixFollowsTheCommand reports whether this command's prefix is
// written after the command's own line rather than before or on it.
//
// The field says "after" and this says *when* after applies, because ksh93 —
// the only column with that shape — divides its own commands in two, and the
// line it divides them on is whether the assignment survives the command:
//
//	A=1 echo a          	`+ echo a` then `+ A=1`   	regular builtin
//	A=1 read -r v       	`+ read -r v` then `+ A=1`	regular builtin
//	A=1 builtin echo d  	the command then `+ A=1`  	forced regular
//	A=1 command echo b  	the command then `+ A=1`  	through `command`
//	A=1 /bin/echo c     	the command then `+ A=1`  	an external
//	A=1 :               	`+ A=1` then `+ :`        	special builtin
//	A=1 eval :          	`+ A=1` then `+ eval :`   	special builtin
//	A=1 export Z=3      	`+ A=1` then the export   	special builtin
//	A=1 g               	`+ A=1` then `+ g`        	a function
//
// Measured 2026-09-16, ksh93u+ 2012-08-01, script file under `env -i`. The
// two that come first are exactly the two positions where a prefix in this
// dialect is an ordinary assignment that stays assigned — see
// Semantics.AssignmentPrefixPersistsOnSpecialBuiltin and
// AssignmentPrefixPersistsAfterAFunction — so the shell is writing it where a
// bare assignment would be written, and writing the *environment* of a
// command that is about to get one after the command it belongs to.
//
// Asked on the command's kind rather than on those two axes, because the
// kinds are what was measured: a dialect that answered the axes differently
// from ksh93 would otherwise inherit a trace order nobody has measured for it.
func (r *Runner) tracePrefixFollowsTheCommand(argv []string) bool {
	if r.diag().TracePrefixAssignment != TracePrefixOwnLineAfter || len(argv) == 0 {
		return false
	}
	if r.IsSpecialBuiltinHere(argv[0]) {
		return false
	}
	if _, ok := r.funcs[argv[0]]; ok {
		return false
	}
	return true
}

// tracePrefixRepeatsBeforeTheCommand is zsh's second trace prefix: whether
// this command gets one between the assignments and its words. See
// TracePrefixOnTheCommandLineRepeatingThePrefix for the measurement.
func (r *Runner) tracePrefixRepeatsBeforeTheCommand(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	if argv[0] == "command" {
		// Measured: `B=2 command echo b` is one prefix even though `echo` is
		// a builtin this shell would run. The precommand word is what the
		// shell is answering, and it drops the word itself from the line as
		// well.
		return false
	}
	if _, ok := r.funcs[argv[0]]; ok {
		return true
	}
	if _, ok := r.lookupBuiltin(argv[0]); ok {
		return true
	}
	return r.reservedBuiltin(argv[0])
}

// expandPrefixTraceValues expands this command's prefix values once, for the
// trace, and keeps them for the route that applies them.
//
// Once is the whole of it: the three routes a prefixed command takes — a
// function, a builtin, an external — each call Runner.prefixValue for
// themselves, and a trace that expanded a second time would run `x=$(date)
// cmd`'s substitution twice. So the values are computed here and read back
// through Runner.prefixValue, which finds them by the assignment's own
// pointer.
//
// Only when the command is being traced. Without that guard this would move
// every prefixed command's expansion earlier, past the redirections, for the
// overwhelming majority of runs that have no `set -x` on at all.
func (r *Runner) expandPrefixTraceValues(assigns []*syntax.Assign) {
	for _, a := range assigns {
		if a.Operand {
			continue
		}
		if _, held := r.prefixTraceValue(a); held {
			// Already expanded, by the ordered walk one dialect makes before
			// it opens the command's redirections — see
			// interp/prefixredirorder.go. Expanding again would run a
			// substitution in the value twice.
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok && a.IsArray {
			// The splice form, which has no one value to expand. See
			// Runner.tracesItsPrefix.
			continue
		}
		if r.readonly[a.Name] {
			// A frozen name is refused, and no column in the panel writes a
			// trace line for a prefix that did not happen. Measured
			// 2026-09-16 with `readonly x=1; set -x; x=2 /usr/bin/env true`:
			// bash complains and traces the command alone, ksh93 traces the
			// command and then complains, and zsh and dash give the command
			// up — and not one of the four writes `x=2`.
			//
			// It also keeps the one column that checks the prefix *before*
			// it evaluates anything from evaluating it here to print it, so
			// `x=$((1/0)) cmd` still says nothing about the division there.
			// See Runner.refusePrefixesEarly.
			continue
		}
		if r.subscriptedPrefixDropped(a) {
			// A subscripted name the dialect refuses, which is refused
			// before this runs and gets no trace line: bash writes the
			// identifier complaint and then `+ f`, with the word it refused
			// nowhere in the line. Measured 2026-09-16 on `set -x; a[1]=w f`
			// and on `set -x; w=1 a[2]=z f`, where `w=1` is still written.
			continue
		}
		// The value first and the two slices afterwards, in that order: the
		// lookup reads them as a pair, and an assignment recorded before its
		// value would be found with nothing beside it.
		value := r.prefixExpansion(a)
		r.prefixTraceAssigns = append(r.prefixTraceAssigns, a)
		r.prefixTraceValues = append(r.prefixTraceValues, value)
	}
}

// prefixTraceValue is the value expanded for this assignment's trace, and
// whether there is one. A scan rather than a lookup: a prefix is one, two or
// three assignments.
func (r *Runner) prefixTraceValue(a *syntax.Assign) (string, bool) {
	for i, held := range r.prefixTraceAssigns {
		if held == a {
			return r.prefixTraceValues[i], true
		}
	}
	return "", false
}

// prefixTraceWords renders the assignments that have a value, in order.
func (r *Runner) prefixTraceWords(assigns []*syntax.Assign, d Diagnostics) []string {
	words := make([]string, 0, len(assigns))
	for _, a := range assigns {
		value, ok := r.prefixTraceValue(a)
		if !ok {
			continue
		}
		if a.Append && d.TracePrefixAppendIsTheJoinedValue {
			// One column writes an appending prefix as the assignment it
			// came to rather than as the assignment it was — see the field.
			plain := *a
			plain.Append = false
			words = append(words, r.traceAssign(&plain, r.prefixJoined(a, value), nil, d))
			continue
		}
		words = append(words, r.traceAssign(a, value, nil, d))
	}
	return words
}

// tracePrefixAndCommand writes a prefixed command's trace in the two shapes
// that put the assignment ahead of the command: bash's own lines, and the one
// line dash, BusyBox ash and zsh write.
func (r *Runner) tracePrefixAndCommand(c *syntax.SimpleCmd, argv []string) {
	d := r.diag()
	words := r.prefixTraceWords(c.Assigns, d)
	if len(words) == 0 {
		// Every assignment was refused or is one this shell does not write a
		// line for. The command is traced as it would be with no prefix at
		// all, which is what the panel shows for `readonly x=1; x=2 cmd`.
		r.traceCommand(argv)
		return
	}
	if d.TracePrefixAssignment == TracePrefixOwnLineBefore ||
		d.TracePrefixAssignment == TracePrefixOwnLineAfter {
		// The "after" column reaches here for the commands it writes the
		// assignment *first* for — a special builtin or a function — and it
		// writes them the way the "before" column does, one line each. See
		// Runner.tracePrefixFollowsTheCommand.
		r.awaitTraceTurn()
		for _, w := range words {
			r.traceLine(w, d)
		}
		r.releaseTraceTurn()
		r.traceCommand(argv)
		return
	}
	line := strings.Join(words, " ") + " "
	if d.TracePrefixAssignment == TracePrefixOnTheCommandLineRepeatingThePrefix &&
		r.tracePrefixRepeatsBeforeTheCommand(argv) {
		line += r.tracePrefix()
	}
	line += strings.Join(r.traceCommandWords(argv, d), " ")
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.tracef("%s%s\n", r.tracePrefix(), line)
}

// tracePrefixAfterTheCommand writes the assignment lines ksh93 puts behind
// the command it already traced.
func (r *Runner) tracePrefixAfterTheCommand(assigns []*syntax.Assign) {
	d := r.diag()
	words := r.prefixTraceWords(assigns, d)
	if len(words) == 0 {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	for _, w := range words {
		r.traceLine(w, d)
	}
}

// prefixEntryHasATraceableValue reports whether this prefix entry is one a
// value is expanded and recorded for at all — the three skips
// expandPrefixTraceValues makes, said once so that the ordered walk in
// interp/prefixredirorder.go makes the same three.
//
// The splice form has no one value; a frozen name is refused and no column
// writes a line for a prefix that did not happen; and a subscripted word the
// dialect refuses is refused rather than expanded.
func (r *Runner) prefixEntryHasATraceableValue(a *syntax.Assign) bool {
	if _, ok := positionalAssignIndex(a.Name); ok && a.IsArray {
		return false
	}
	return !r.readonly[a.Name] && !r.subscriptedPrefixDropped(a)
}

// tracesEachPrefixEntryOnItsOwnLine reports whether this dialect writes one
// trace line per prefix entry ahead of the command, which is the only shape
// the ordered walk can interleave a refusal into.
//
// The other two shapes put the whole prefix on the command's own line, where
// there is no "between two entries" for a complaint to land in.
func (r *Runner) tracesEachPrefixEntryOnItsOwnLine() bool {
	switch r.diag().TracePrefixAssignment {
	case TracePrefixOwnLineBefore, TracePrefixOwnLineAfter:
		return true
	}
	return false
}
