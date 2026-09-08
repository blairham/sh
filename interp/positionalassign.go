// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"

	"github.com/blairham/sh/syntax"
)

// positionalAssignLimit bounds how far one assignment may extend the list.
//
// Not a modeled refusal. The reference shell grows the parameters to whatever
// the index says — `set -- a; 5000000=x` leaves `$#` at 5000000, measured
// 2026-09-07 — and refuses only an index too wide to represent, where it says
// `number truncated after 19 digits` and assigns nothing. There is no measured
// answer between those two, so this is a resource bound: past it the
// assignment does nothing, which is what the shell does for the index it
// cannot represent, rather than the process asking for an allocation the
// machine cannot serve.
const positionalAssignLimit = 1 << 20

// positionalAssignIndex reports the positional parameter an assignment's name
// points at, when the name is a run of decimal digits.
//
// Only one dialect's parser ever builds such an assignment — see
// [syntax.Dialect.PositionalAssignment] — so this is the whole of the test.
// isName rejects a leading digit everywhere, and the subscripted head insists
// on a name in front of the bracket, so no other route produces one.
func positionalAssignIndex(name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(name)
	if err != nil || n > positionalAssignLimit {
		// Wider than the machine's own integer, or wider than the bound
		// above. Reported as a target so the word does not fall through to
		// the variable table and quietly become an ordinary name; the
		// assignment itself is declined below.
		return -1, true
	}
	return n, true
}

// assignPositional performs `N=value`, the assignment whose name is a number.
//
// It is `argv[N]=value` under another spelling, and measured to be exactly
// that: `set -- z y w; 1=(a b)` and `set -- z y w; argv[1]=(a b)` both leave
// `a b y w`, and `set -- a b; 1+=(z)` and `argv[1]+=(z)` both leave `a z b`.
// So the four shapes are the four an element assignment has, and each was
// measured on zsh 5.9.2 on 2026-09-07:
//
//	set -- x;     1=abc      →  [abc]        the value replaces the element
//	set -- abc;   1+=x       →  [abcx]       the value joins it
//	set -- z y w; 1=(a b)    →  [a b y w]    the words replace the element
//	set -- a b;   1+=(z)     →  [a z b]      the words follow it
//
// An index past the end extends the list with empty parameters rather than
// failing: `set -- a b; 9=nine` leaves `$#` at 9. An index of 0 is `$0`, which
// is not one of the parameters and not a list — `0=abc` sets it, `0+=Z` joins
// it, and `0=(x y)` is `attempt to assign array value to non-array`.
//
// Nothing here is a dialect question. The construct exists in one shell and
// nowhere else, so what it *does* has a single answer and no axis to ask; the
// grammar flag is what keeps the other five from reaching this at all.
func (r *Runner) assignPositional(a *syntax.Assign, n int) {
	if n < 0 {
		// An index this shell cannot represent. See positionalAssignLimit.
		return
	}
	if n == 0 {
		r.assignDollarZero(a)
		return
	}
	if a.IsArray {
		r.spliceParams(n, a)
		return
	}
	params := r.paramsExtendedTo(n)
	value := r.expandAssignValue(a.Value)
	if a.Append {
		value = params[n-1] + value
	}
	params[n-1] = value
	r.Params = params
}

// assignDollarZero is `0=value`, which names the shell rather than one of its
// parameters.
//
// `$0` is not in the list — `set -- a b` leaves `$#` at 2 and `0=abc` does not
// change it — so it is written where the name lives instead. The array
// spelling is refused for the same reason: there is no list there to splice
// into, and the shell says so with the wording a scalar gets anywhere else.
func (r *Runner) assignDollarZero(a *syntax.Assign) {
	if a.IsArray {
		r.fatal("%s\n", Wording(r.diag().ArrayValueToNonArray,
			"%[1]s: attempt to assign array value to non-array", a.Name))
		return
	}
	value := r.expandAssignValue(a.Value)
	if a.Append {
		// The shell's own name, which is what `$0` reads at the top level.
		// A function or a sourced file can be what `$0` *answers* in this
		// dialect (see Semantics.DollarZeroNamesTheInnermostCall), and the
		// assignment does not write that: it replaces the name, and the
		// innermost call goes on answering ahead of it until it returns.
		value = r.Name + value
	}
	r.Name = value
}

// spliceParams is the array spelling, `N=(words)`, which replaces the one
// parameter the number names with however many words the parentheses hold.
//
// The count changes with it, which is what makes this a splice rather than a
// write: `set -- a b c; 2=()` leaves `a c` and `$#` at 2, and `set -- z y w;
// 1=(a b)` leaves `a b y w` and `$#` at 4. Appending keeps the element and
// puts the words after it — `set -- a b; 1+=(z)` is `a z b` — which is the
// same operation with the old value at the front of the replacement.
func (r *Runner) spliceParams(n int, a *syntax.Assign) {
	var words []string
	for _, w := range a.Elems {
		words = append(words, r.expandWord(w)...)
	}
	params := r.paramsExtendedTo(n)
	if a.Append {
		words = append([]string{params[n-1]}, words...)
	}
	out := make([]string, 0, len(params)-1+len(words))
	out = append(out, params[:n-1]...)
	out = append(out, words...)
	out = append(out, params[n:]...)
	r.Params = out
}

// paramsExtendedTo copies the positional parameters into a slice at least n
// long, padding with empty ones.
//
// A copy every time rather than an append in place, and deliberately not
// conditional on whether growth is needed. The list is passed by slice — a
// function call saves the caller's and puts it back, and `shift` re-slices
// the one it has — so a write in place would go through whatever else is
// holding the same backing array.
//
// Stated as the invariant rather than as a bug this prevents: mutation says
// no current caller can be made to observe the difference, since the two
// places that hold a second reference each replace it rather than read it
// again. The copy is a few words and is what keeps that from having to stay
// true.
func (r *Runner) paramsExtendedTo(n int) []string {
	size := len(r.Params)
	if n > size {
		size = n
	}
	out := make([]string, size)
	copy(out, r.Params)
	return out
}

// prefixAssignsPositional applies an assignment prefix that names a positional
// parameter, and reports whether it did.
//
// A prefix to a *builtin* or to a *function* is applied to this shell and
// stays applied, which is not what a prefix to a name does: `foo=bar` before
// either is taken back afterward, and `set -- a b; 1=X shift` leaves `b` — so
// the assignment happened and the shift then dropped it — while `set -- a b;
// f() { echo "in [$*]"; }; 1=X f y` prints `in [y]` and then `out [X b]`. The
// function gets its own parameters and the caller's carry the write.
//
// It is not taken back because there is nothing here that takes it back: the
// undo the builtin path keeps is over the variable table, and the parameters
// are not in it. That is the reading, not an accident of ours — the shell's
// own behavior is the same shape.
//
// An *external* command is the other side and is handled where it is decided,
// because the answer there is to do nothing at all: `set -- a b; 1=X /bin/echo
// hi` leaves `a b`, and `1=X /usr/bin/env` shows the child no `1` in its
// environment.
func (r *Runner) prefixAssignsPositional(a *syntax.Assign) bool {
	if _, ok := positionalAssignIndex(a.Name); !ok {
		return false
	}
	r.assign(a)
	return true
}
