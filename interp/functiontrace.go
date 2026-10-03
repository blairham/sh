// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A *function* marked for tracing: `functions -t f` and `functions -T f`, and
// the same two letters on a `-f` declaration line.
//
// This is not the trace *attribute* interp/functionattribute.go records. That
// one is bash's `declare -ft`, whose whole meaning is which traps a call
// inherits, and it is listed back as a `declare -ft NAME` row after the body.
// The mark here turns the **xtrace option** on for the length of a call and
// is written *inside* the body, as a comment line under the opening brace.
// The two tables are kept apart on purpose: a shell that marked one and read
// the other would give a traced function the DEBUG and RETURN traps of a
// shell it is not. Measured 2026-09-28 on zsh 5.9.2, a DEBUG trap set at the
// top level already fires inside an unmarked function there, so the mark
// decides nothing about traps in this column.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// from script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME.
// Every row below is one of those files.
//
//   - **The mark turns the real option on, not a private flag.** `f() { [[ -o
//     xtrace ]] && print ON }; functions -t f; f` writes `ON`, and
//     `$options[xtrace]` inside the body is `on`. So there is nothing here to
//     carry beside Runner.xtrace; the mark sets it and the ordinary reader
//     finds it.
//   - **`-t` reaches what the body calls and `-T` does not.** With `g`
//     unmarked and called from `f`: `-t f` traces `+f:0> g` and then `+g:0>
//     print G`, where `-T f` traces the call line and leaves g's body silent.
//     A function that holds its own mark traces wherever it is called from —
//     `-T f` calling `g` calling `-t h` traces h.
//   - **Both letters can be held at once and `T` wins.** `functions -t f;
//     functions -T f` leaves g untraced, and so does the same pair the other
//     way round, and so does `functions -tT f`.
//   - **The letters are separate records for a listing.** `-T f` then
//     `functions -t` writes nothing, `functions -T` writes the body, and
//     `functions -tT` writes it too — a union, which is the reading
//     Runner.SetMarkedFunctions already measured for `-u` and `-U`.
//   - **`+t` takes that letter off and leaves the other.** `-t f; +T f` still
//     lists under `functions -t`.
//   - **A redefinition clears both.** `functions -t f; f() { print B }; f`
//     writes a plain `B`, and so does the same through `unfunction`.
//   - **A name that is not a function is a silent 1** and the names beside it
//     are still marked: `functions -t f nosuch` answers 1 and f traces. A
//     name `autoload` left waiting *is* a function for this: `autoload -Uz
//     nm; functions -t nm` is 0.

// functionTraceMode is what entering one function does to the trace.
type functionTraceMode uint8

const (
	// functionTraceUnmarked is every function in every other column: the
	// call changes nothing about the option.
	functionTraceUnmarked functionTraceMode = iota
	// functionTraceBody is `-t`: the body traces and so does everything it
	// calls, because nothing turns the option back off.
	functionTraceBody
	// functionTraceBodyAlone is `-T`: the body traces and a function it
	// calls starts with the option off, unless that one holds a mark of its
	// own.
	functionTraceBodyAlone
)

// functionTraceMarkMode is the mark this name holds, read through the
// dialect's letters.
//
// Empty letters — every column but one — is functionTraceUnmarked without
// touching the map, so a shell without the notion never pays for it.
func (r *Runner) functionTraceMarkMode(name string) functionTraceMode {
	held := r.funcTraceMarks[name]
	if held == "" {
		return functionTraceUnmarked
	}
	if strings.ContainsAny(held, r.sem().FunctionTraceLettersBoundToTheBody) {
		return functionTraceBodyAlone
	}
	return functionTraceBody
}

// markFunctionTrace writes the letters this line gave a name, and takes off
// the ones it wrote under a plus.
//
// One name at a time and one letter at a time, because the two signs may
// appear on the same line: `functions -t +T f` holds `t` and not `T`, which
// is f.lastSign's rule and the same one every other declaration letter
// follows.
func (r *Runner) markFunctionTrace(name, written, removed string) {
	held := r.funcTraceMarks[name]
	for _, c := range written {
		if !strings.ContainsRune(held, c) {
			held += string(c)
		}
	}
	for _, c := range removed {
		held = strings.ReplaceAll(held, string(c), "")
	}
	if held == "" {
		delete(r.funcTraceMarks, name)
		return
	}
	if r.funcTraceMarks == nil {
		r.funcTraceMarks = map[string]string{}
	}
	r.funcTraceMarks[name] = held
}

// forgetFunctionTrace drops a name's marks, for a definition that replaces
// the body and for a removal that takes it away.
//
// Measured rather than assumed, and it is the half a table like this usually
// misses: `functions -t f; f() { print B }; f` writes a bare `B` in zsh
// 5.9.2, so the mark belongs to the *body* that was marked and not to the
// name.
func (r *Runner) forgetFunctionTrace(name string) {
	delete(r.funcTraceMarks, name)
	// The nested-scope mark goes with the body as the trace marks do:
	// measured 2026-10-02 on zsh 5.9.2, `functions -W f; f(){ g=3 }; f`
	// warns about nothing (#5155).
	delete(r.warnNestedFuncs, name)
}

// redefinesTheRunningBody reports a definition of name written directly in
// name's own body while it runs, which is the one definition that keeps the
// mark. Measured 2026-10-01 on zsh 5.9.2, each with the mark set before the
// call and `which` after it:
//
//	f() { f() { echo inner } }               kept, under -t and -T alike
//	c() { c() { …C2 }; c() { …C3 } }         kept, by both
//	f() { g }; g() { f() { echo X } }        cleared: g's body, not f's
//	a() { () { a() { echo A2 } } }           cleared: a nameless body
//	b() { eval 'b() { echo B2 }' }           cleared: text eval runs
//	d() { unfunction d; d() { echo D2 } }    cleared, by the unfunction
//	g() { h() { echo H } }, h marked         cleared: another name's
//
// So it is the innermost frame being the function itself, read the way a
// location is, and not the name being anywhere on the stack.
func (r *Runner) redefinesTheRunningBody(name string) bool {
	n := len(r.frames)
	if n == 0 || r.frames[n-1].Name != name || r.locationIsInsideEvalText() {
		return false
	}
	return true
}

// functionsHoldingATraceMark is the population a listing narrowed by these
// letters writes: the names holding **any** of them, in listing order.
//
// A union, measured: with `f` marked `-T` alone, `functions -tT` writes it.
// The same reading Runner.functionsHoldingAttributes records for bash's
// attribute letters and SetMarkedFunctions for zsh's autoload marks, so the
// third table here does not invent a fourth rule.
// functionsWarningNested is the functions `functions -W` has marked, in
// listing order.
func (r *Runner) functionsWarningNested() []string {
	var out []string
	for _, name := range r.scriptFuncNames() {
		if r.warnNestedFuncs[name] {
			out = append(out, name)
		}
	}
	return out
}

func (r *Runner) functionsHoldingATraceMark(letters string) []string {
	var out []string
	for _, name := range r.scriptFuncNames() {
		if strings.ContainsAny(r.funcTraceMarks[name], letters) {
			out = append(out, name)
		}
	}
	return out
}

// traceLettersWritten is the trace letters this declaration wrote, split by
// sign: the ones setting the mark and the ones taking it off.
//
// The sign is the *last* one the line gave that letter, which is lastSign's
// rule throughout these builtins.
func (r *Runner) traceLettersWritten(f declareFlags) (written, removed string) {
	for _, c := range r.sem().FunctionTraceLetters {
		plus, ok := f.lastSign(c)
		if !ok {
			continue
		}
		if plus {
			removed += string(c)
		} else {
			written += string(c)
		}
	}
	return written, removed
}

// setFunctionTraceMarks is the operand form: each name is marked with the
// letters the line wrote, and the status is 1 if any of them is not a
// function.
//
// Silent, and the names beside a bad one are still marked — measured,
// `functions -t f nosuch` answers 1 with nothing said and f traces
// afterwards. That is the answer a `-f` listing already gives a name it does
// not hold, which is why reportedFunc is asked rather than r.funcs directly:
// a name `autoload` left waiting is a function here.
func (r *Runner) setFunctionTraceMarks(names []string, written, removed string) int {
	status := 0
	for _, name := range names {
		if _, ok := r.reportedFunc(name); !ok {
			status = 1
			continue
		}
		r.markFunctionTrace(name, written, removed)
	}
	return status
}

// tracedFunctionBody is the body a listing writes for a marked function: the
// dialect's marker line put under the opening brace, at the listing's own
// indent.
//
// Inside the body rather than after it, which is the whole reason this is not
// Runner.functionAttributeLine: the one column with the mark writes
//
//	f () {
//		# traced
//		print A
//	}
//
// and the indent moves with `functions -x2`, so the line is built from
// Runner.functionLayout rather than from a tab.
//
// The body a name still waiting prints is left alone: there is no opening
// brace in it to put a line under.
func (r *Runner) tracedFunctionBody(name, body string) string {
	marker := r.diag().TracedFunctionListingLine
	if marker == "" || r.funcTraceMarks[name] == "" {
		return body
	}
	brace := strings.IndexByte(body, '\n')
	if brace < 0 {
		return body
	}
	indent := r.functionLayout.Indent
	if indent == "" {
		indent = "\t"
	}
	return body[:brace+1] + indent + marker + "\n" + body[brace+1:]
}

// functionCallRestoresTheTrace reports whether a call saves the xtrace option
// and puts it back on the way out.
//
// Read rather than asked, so a core with no dialect in front of it — which is
// every embedding of this package that has not chosen a shell — goes on
// behaving the way the three columns that do not restore behave, instead of
// being refused for calling a function. See Semantics.FunctionCallRestoresTheTrace.
func (r *Runner) functionCallRestoresTheTrace() bool {
	return r.sem().FunctionCallRestoresTheTrace == Yes
}
