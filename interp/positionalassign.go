// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
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

// truncatedPositionDigits reports how many digits of a decimal index the
// shell says it read when the index is too wide for a signed 64-bit integer,
// and whether it was.
//
// The count is one past the digit that went over, or that digit's own place
// where it is the last one written — so it is the length less one only while
// the run is at most a digit longer than what fits. Measured 2026-10-16 on zsh
// 5.9.2, `-f`, through `N=x` (the same count comes back for a numbered tilde):
//
//	9223372036854775808         18   the last digit went over
//	9999999999999999999         18   the same
//	10000000000000000000        19   the same, one digit later
//	99999999999999999999        19   the 19th went over, one more follows
//	999999999999999999999       19   and two more: still 19
//	100000000000000000000       20   the 20th went over
//	1000000000000000000000      20
//	00000000000000000003         —   leading zeros are not width
//	9223372036854775807          —   fits
//
// The length less one, which was the reading before, is right for the first
// five rows and wrong for the fifth and seventh (#6743).
func truncatedPositionDigits(digits string) (int, bool) {
	const limit = uint64(1<<63 - 1)
	var v uint64
	for i := 0; i < len(digits); i++ {
		d := uint64(digits[i] - '0')
		if v > (limit-d)/10 {
			if i < len(digits)-1 {
				return i + 1, true
			}
			return i, true
		}
		v = v*10 + d
	}
	return 0, false
}

// warnATruncatedPosition says so where a store names a position too wide for
// the shell's integer, in the dialect with a sentence for it.
//
// A warning and not a refusal, and the store is still declined here. The
// shell being modeled goes on to store at the index the truncated digits make,
// reduced again to the width of its own list index, and that is an artifact
// with no reading to model: measured 2026-10-16 on zsh 5.9.2, an index whose
// low 32 bits are negative puts the value in front of the list, one whose low
// bits are a large positive grows the list until the shell is killed, and
// `4294967297=x` stores at position 1 — while two neighboring cells of the
// negative half (`$#` plus one, under `+=(…)`) leave a list of what looks like
// unrelated memory. positionalAssignLimit is the bound on the second shape and
// nothing here is the first or third.
//
// Once per store, and on every route that stores: `N=x`, `N=(x)`, `N+=x`, a prefix to a builtin, a
// function or an external command, `for N in`, `read`, `read -A`, `set -A`,
// `printf -v`, `print -v`, `zparseopts -a` and `${(A)N::=x}`. `printf -v`
// warns twice, the first time where it asks whether the name is an array.
func (r *Runner) warnATruncatedPosition(name string) {
	wording := r.diag().PositionTruncated
	if wording == "" {
		return
	}
	if read, cut := truncatedPositionDigits(name); cut {
		// The number reader's warning and not the builtin's that handed it
		// the name: `zsh:1:` for `read`, `set -A` and `printf -v` alike,
		// which is arithmetic's own location for the same sentence.
		r.arithDiagf("%s\n", Wording(wording, "", read, name))
	}
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
		// An index this shell cannot represent, or one past
		// positionalAssignLimit. The value is expanded all the same, and the
		// warning comes after it: `99999999999999999999=$(echo side >&2)`
		// writes `side` and then the truncation, on zsh 5.9.2 (#6743).
		if a.IsArray {
			for _, e := range a.Elems {
				if e.Word != nil {
					r.expandWord(e.Word)
				}
			}
		} else {
			r.assignValue(a)
		}
		if !r.assignmentGaveUp() {
			r.warnATruncatedPosition(a.Name)
		}
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
	value := r.assignValue(a)
	if r.assignmentGaveUp() {
		// A value that would not expand stores nothing — see the indexed
		// store in Runner.assign (#6579).
		return
	}
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
	value := r.assignValue(a)
	if r.assignmentGaveUp() {
		// A value that would not expand stores nothing — see the indexed
		// store in Runner.assign (#6579).
		return
	}
	if a.Append {
		// The shell's own name, which is what `$0` reads at the top level.
		// A function or a sourced file can be what `$0` *answers* in this
		// dialect (see Semantics.DollarZeroNames), and the
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
	for _, e := range a.Elems {
		if e.Word == nil {
			// A literal standing where an element goes has no reading as a
			// *positional parameter*: the parameters are a flat list of
			// strings, so what would be stored is the element's own scalar
			// reading. Only one dialect can write one and none of them
			// splices positionals, so this is unreachable rather than a
			// choice; taking the scalar keeps the count right if it ever is.
			value, ok := r.nestedLiteral("", e.Nested.Elems)
			if !ok {
				return
			}
			words = append(words, value.scalar())
			continue
		}
		words = append(words, r.expandWord(e.Word)...)
	}
	if r.assignmentGaveUp() {
		// A word that would not expand costs the whole splice, as it costs
		// an array literal: measured 2026-10-15 on zsh 5.9.2 interactively,
		// `set -- p q; 1=(x $((1/0)))` leaves `p q`. It spliced in the
		// words that had expanded (#6579).
		return
	}
	if a.Append {
		words = append([]string{r.paramsExtendedTo(n)[n-1]}, words...)
	}
	r.spliceParamWords(n, words)
}

// spliceParamWords replaces parameter n with words, padding the list with
// empty parameters when n lies past its end. It is the splice itself, shared
// by `N=(words)` and by a builtin's whole-array store into a position — see
// Runner.StoreAWholeArrayAtAPosition.
func (r *Runner) spliceParamWords(n int, words []string) {
	params := r.paramsExtendedTo(n)
	out := make([]string, 0, len(params)-1+len(words))
	out = append(out, params[:n-1]...)
	out = append(out, words...)
	out = append(out, params[n:]...)
	r.Params = out
}

// StoreAWholeArrayAtAPosition is a registered builtin's whole-array store —
// or, with table set, its whole-association store — into a name that is a
// position rather than a name, and reports whether name was one. When it
// was, stop says the builtin ends here, with status as what it returns.
//
// It answers only in the dialect whose builtins fill a position by its number
// (ReadNameOperands holding NamesAndPositionals, the rule
// storeThroughPositional reads for a scalar), and there `set -A`, `set +A`,
// `read -A` and `zparseopts -a` treat `N` as `N=(words)` treats it: as
// `argv[N]`, which the words replace. Position 0 is `$0`, a slot of a fixed
// scalar kind, so the array is refused with the sentence `0=(x)` gets; a
// table is refused at every position with the kind sentence, as it is over
// the shell's own parameters. Each refusal ends the script, and the number is
// arrayStoreRefusalStatus's. Measured 2026-10-15 on zsh 5.9.2, `-f`, `env -i
// PATH=/usr/bin:/bin`, `-c` then a script file (#6648):
//
//	set -- a b; set -A 1 x y         x y b
//	set -- a b c; set +A 2 x y       a x y c — the plus spelling alike
//	set -- a b; set -A 1             b, the empty splice
//	set -- a b; set -A 4             a b and an empty $3: the list padded
//	set -- a b; read -A 4 <<< p      a b, an empty $3, then p
//	set -- -a b; zparseopts -a 2 a   -a -a
//	set -A 0 x                       0: attempt to assign array value to
//	                                 non-array, and the script ends
//	read -A 0, set +A 0, zparseopts -a 0 a   the same
//	zparseopts -A 1 a                1: can't change type of a special
//	                                 parameter, and the script ends
//	zparseopts -A 0 a                the same, naming 0
//
// `01` is position 1 in that shell as it is to `01=(x)`. An index too wide to
// represent stores nothing, as positionalAssignIndex declines it for the
// statement form.
func (r *Runner) StoreAWholeArrayAtAPosition(name string, values []string, table bool) (status int, took, stop bool) {
	took, refused := r.storeAWholeArrayAtAPosition(name, values, table)
	if !refused {
		return 0, took, false
	}
	return r.arrayStoreRefusalStatus(r.status), true, true
}

// storeAWholeArrayAtAPosition is StoreAWholeArrayAtAPosition without the
// number a builtin leaves, for the route that leaves its own: `${(A)N::=…}`,
// whose refusal over `0` is the expansion's and ends the script at 1.
func (r *Runner) storeAWholeArrayAtAPosition(name string, values []string, table bool) (took, refused bool) {
	if !r.namesAPosition(name) {
		return false, false
	}
	n, _ := positionalAssignIndex(name)
	switch {
	case table:
		r.reportATableToAFixedSlot(name)
	case n == 0:
		r.reportAnArrayValueToAFixedSlot(name)
	case n > 0:
		r.spliceParamWords(n, values)
		return true, false
	default:
		r.warnATruncatedPosition(name)
		return true, false
	}
	r.fatalQuiet()
	return true, true
}

// namesAPosition reports whether a builtin's store into name is a store into
// a position, which is StoreAWholeArrayAtAPosition's question asked without
// storing.
func (r *Runner) namesAPosition(name string) bool {
	if r.sem().ReadNameOperands != NamesAndPositionals {
		return false
	}
	_, ok := positionalAssignIndex(name)
	return ok
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
func (r *Runner) prefixAssignsPositional(ctx context.Context, a *syntax.Assign) bool {
	if _, ok := positionalAssignIndex(a.Name); !ok {
		return false
	}
	if value, ok := r.prefixTraceValue(a); ok {
		// Expanded once already, by the trace that is about to write this
		// line — see Runner.expandPrefixTraceValues. Handed to the store the
		// way the three other prefix routes take it, rather than expanded a
		// second time, which is what keeps `1=$(date) f` from running the
		// substitution twice under a trace: the double run #1915 fixed for a
		// scalar's value, and the reason this position had no trace line at
		// all until #3157.
		r.withPreparedValue(ctx, &expandedAssign{assign: a, value: value})
		return true
	}
	r.assign(ctx, a)
	return true
}

// setLoopName writes one pass of a loop's variable, which in one dialect may
// be a positional parameter's number rather than a name.
//
// It is the grammar flag [syntax.Dialect.ForNameMayBeAPositionalParameter]
// that decides whether such a header parses at all, so nothing here asks
// which dialect it is: a name of digits cannot reach this in a shell whose
// parser refused the header. What it writes is the same parameter `1=value`
// writes — measured 2026-09-15 on zsh 5.9.2, `set -- p q; for 1 in a b; do
// :; done` leaves `$1` as `b` and `$2` still `q` — so the store is the one
// assignPositional already owns rather than a second one beside it.
func (r *Runner) setLoopName(name, value string) {
	if n, ok := positionalAssignIndex(name); ok {
		// Quoted, because the value is a word the loop already expanded and
		// must not be read a second time: `for 1 in 'a\\0b'` held `a\0b`
		// here where zsh 5.9.2 holds the four bytes it was given, measured
		// 2026-10-01 — an unquoted literal had its backslashes taken as
		// escapes by the assignment's own expansion.
		quoted := &syntax.Word{Spans: []syntax.Span{{
			Kind: syntax.Literal, Quoting: syntax.SingleQuoted, Value: value,
		}}}
		r.assignPositional(&syntax.Assign{Name: name, Value: quoted}, n)
		return
	}
	// A pass that retypes a hidden name takes the word off it. See
	// Runner.storeForgetsARetypedHide.
	end := r.storeForgetsARetypedHide(name)
	r.setVar(name, value)
	end()
	r.markAFilledNameForAllexport(name)
}
