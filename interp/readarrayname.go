// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ReadArrayDefaultStyle is what the `-A` spelling of `read`'s array letter
// fills when the line names no parameter at all.
//
// The two shells with the letter disagree, so this cannot ride the letter the
// way the rest of the array rules do. Measured 2026-09-26, `-f` for zsh and a
// script file otherwise, each row read back with `typeset -p`:
//
//	read -A <<<'a b c'          zsh 5.9.2     reply=(a b c), REPLY not created
//	read -A <<<'a b c'          ksh93u+ 2012  REPLY='a b c', reply untouched
//	read -a  <<<'a b c'         bash 5.3.20   `-a: option requires an argument`
//
// The zsh reading is the neighbor of the bare-read rule and is deliberately
// *not* the same parameter: a bare `read` fills the scalar `REPLY` and
// `read -A` fills the array `reply`, and the shell is case-sensitive about
// which. Both were measured with the other pre-set, so neither row is reading
// back what it wrote a moment earlier:
//
//	reply=(x y); read -A <<<'hello world'   reply=(hello world)
//	reply=scalar; read <<<'plain'           reply=scalar, REPLY=plain
//
// The ksh93 reading is not a second default name. `read -A` there answers
// exactly as a bare `read` does — one scalar holding the line, stripped of
// leading and trailing `IFS` whitespace and split no further — so the letter
// contributes nothing once there is no name for it to apply to, and `reply`
// is a parameter ksh93 has never heard of.
//
// bash cannot be asked. Its spelling is the lowercase `-a`, whose name is the
// option's own argument, so a line with no name for the array does not reach
// this question: it is refused by the option parser first.
type ReadArrayDefaultStyle int

const (
	// ReadArrayDefaultUnspecified is no answer, and is refused like any
	// other.
	ReadArrayDefaultUnspecified ReadArrayDefaultStyle = iota
	// ReadArrayDefaultIsTheArrayReply fills the array `reply` — lowercase,
	// and a different parameter from the `REPLY` a bare `read` fills. The
	// line is split into elements exactly as it would be for a name the
	// script wrote. zsh.
	ReadArrayDefaultIsTheArrayReply
	// ReadArrayDefaultIsAPlainRead drops the letter: with no name to apply
	// it to, the builtin reads as a bare `read` does and leaves one scalar
	// holding the line. ksh93.
	ReadArrayDefaultIsAPlainRead
)

func (s ReadArrayDefaultStyle) String() string {
	switch s {
	case ReadArrayDefaultIsTheArrayReply:
		return "ReadArrayDefaultIsTheArrayReply"
	case ReadArrayDefaultIsAPlainRead:
		return "ReadArrayDefaultIsAPlainRead"
	}
	return "ReadArrayDefaultUnspecified"
}

// readArrayDefaultName is the array `read -A` fills when the line names no
// parameter, and whether the letter names an array at all.
//
// The second return is false for the reading that drops the letter, which is
// not the same as an empty name: an empty name is a name the script wrote and
// is judged as one.
func (r *Runner) readArrayDefaultName() (string, bool) {
	switch r.sem().ReadArrayDefault {
	case ReadArrayDefaultIsTheArrayReply:
		return "reply", true
	case ReadArrayDefaultIsAPlainRead:
		return "", false
	case ReadArrayDefaultUnspecified:
	}
	r.diagf("%s\n", r.unanswered("what `read -A` fills when the line names no parameter"))
	r.status = 2
	r.unspecified = true
	return "", false
}

// readCountedWhole is a counted `read` in the dialect where the count takes
// the first name and nothing else: the text goes to the array, or to the first
// name, whole, and every other name is left as it was. An array the name
// already holds is emptied and takes the text as its one element; a name that
// is not one takes it as a scalar, which is what an element operand is. See
// Semantics.ReadCountFillsTheFirstNameWhole.
func (r *Runner) readCountedWhole(array string, names []string, text string, status int,
	echo readEcho, defaulted bool, refused *bool, badName string, bad bool,
) int {
	if array != "" {
		if _, held := r.arraysLookup(array); held {
			if st, stop := r.readStoreTheArray(array, []string{text}, status, echo, len(names), defaulted, refused); stop {
				return st
			}
			return r.readRefusedOrStatus(*refused, status, badName, bad)
		}
		if st, stop := r.readFill(array, text, len(names), defaulted, refused, echo); stop {
			return st
		}
		return r.readRefusedOrStatus(*refused, status, badName, bad)
	}
	if len(names) > 0 {
		if st, stop := r.readFill(names[0], text, len(names)-1, defaulted, refused, echo); stop {
			return st
		}
	}
	return r.readRefusedOrStatus(*refused, status, badName, bad)
}

// readArrayIntoATableKey is `read -A 'a[k]'` over a table, in the dialect
// where an array value placed in an element nests there, and reports whether
// it stored. Measured 2026-10-16 on ksh93u+ 2012-08-01, `typeset -A a` first:
//
//	read -A 'a[k]' <<< 'p q'                  a=([k]=(p q) )
//	read -A 'a[]' <<< 'p q'                   a=(['']=(p q) ), and under
//	                                          `set -x` read: warning: adding
//	                                          empty subscript
//	read -A 'a["p q"]' <<< x                  a=(['"p q"']=(x) ) — the key
//	                                          as written, as `read 'a[…]'`
//	a[k]=old; read -A 'a[k]' <<< 'x y'        [k]=(x y)
//	a[k]=(o1 o2); read -A 'a[k]' <<< x        [k]=(x o2) — written over in
//	                                          place, the rest kept
//
// An indexed array's element is not this — see readArrayIntoAnIndexedElement,
// where ksh93's answer takes the element out and fills the array from its
// first element on, and zsh's splices (#6869).
func (r *Runner) readArrayIntoATableKey(array string, elems []string) bool {
	if r.sem().SubscriptedArrayLiteral != SubscriptedArrayLiteralNests {
		return false
	}
	base, _, key, ok := r.subscriptOperandParts(array, r.outputOperandBracketsAreLexed(array))
	if !ok || !isPlainName(base) || !r.assocDeclared(base) {
		return false
	}
	var nested Array
	if held, there := r.assocsAt(base)[key]; there && held.Nested != nil {
		nested = held.Nested.clone()
	} else {
		nested = NewArray(len(elems))
	}
	for i, e := range elems {
		nested.Set(r.arrayBase()+i, Scalar(e))
	}
	r.storeAssocElement(base, key, Element{Nested: nested})
	return true
}
