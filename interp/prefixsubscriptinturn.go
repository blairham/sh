// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A subscripted assignment prefix's subscript, read **in turn** — where the
// entry stands in the prefix, between its own value and the next entry's —
// and read once.
//
// `a[k]=v cmd` is an element write, and an element write has three pieces to
// read: the subscript's words, the value, and for an indexed name the
// subscript's arithmetic. This engine read the value in the prefix walk and
// left the subscript to the store the route makes once the walk is over, so
// the trace line — written by the walk — could only spell the subscript as the
// script typed it, and the one store that warns while tracing warned after
// every line. ksh93 reads the three pieces, stores, and then writes the line,
// with the subscript as it resolved and quoted as a word (#6582's rule).
// Measured 2026-10-28 on ksh93u+ 2012-08-01 (`/bin/ksh`), script files under
// `env -i PATH=/usr/bin:/bin` with `typeset -A a; b=(x y z); i=1` and `set -x`
// above (#6771):
//
//	a[""]=1 :                 the empty-key warning, `+ a['']=1`, `+ :`
//	a["p q"]=1 a[""]=2 a[$((1+1))]=3 :
//	                          `+ a['p q']=1`, the warning, `+ a['']=2`,
//	                          `+ a[2]=3`, `+ :`
//	a["p q"]=1 a[""]=2 /usr/bin/true
//	                          `+ /usr/bin/true`, `+ a['p q']=1`, the
//	                          warning, `+ a['']=2`
//	f() { :; }; a[""]=1 f     the warning, `+ a['']=1`, `+ f`
//	b[i++]=$((i*10)) :        `+ b[1]=10`, and `i` is 2 after
//	b[-1]=Q :                 `+ b[2]=Q`
//	k="x y"; a[$k]=1 :        `+ a['x y']=1`
//	a[$(echo s >&2; echo k)]=$(echo t >&2; echo v) :
//	                          `+ echo s`, `+ echo k`, `+ echo t`, `+ echo
//	                          v`, `+ a[k]=v` — the subscript's words, then
//	                          the value, then the line
//	b[1/0]=v :                the division alone, and no line
//
// So the order is the element write's own (Semantics.
// ElementSubscriptIsExpandedBeforeTheValue, interp/elementorder.go), and the
// line waits for it as a bare element write's does
// (Semantics.TraceElementSubscriptIsEvaluated). That axis is the one asked
// here: the question is the bare statement's, put to the same word in front of
// a command.
//
// The reading is not only the trace's. The same shell reads a persisting
// prefix — a special builtin's, a function's — ahead of the command's
// redirections and ahead of the entries behind it, whether or not it is
// tracing, measured the same day without `set -x`:
//
//	i=1; b[i++]=Q y=$i f; echo $i        `f` sees `y=2`, and `i` is 2
//	b[i++]=1 f >/nope/x; echo i=$i       the file's complaint, then `i=2`
//
// where this engine showed the body `y=1` and left `i` at 1 behind the failed
// redirection. Reading it in the walk for a traced command alone would have
// made `set -x` change both answers, so the ordered walk two columns make
// ahead of the redirections reads it for every command — and only one of the
// two reaches it, since the other refuses a subscripted prefix outright (see
// interp/prefixsubscript.go). A regular builtin's prefix is read by the store
// that shows it to the builtin, in the same order: `a[$(echo s >&2; echo
// k)]=$(echo t >&2; echo v) echo hi` writes `s` before `t` untraced as well,
// and `b[1/0]=v echo hi` is located as the builtin's, `S[2]:`, as a failed
// value of it is (interp/prefixfailurelocation.go).
//
// And a prefix in front of a program is read in the child, which is a copy of
// this shell here: `b[1/0]=v /usr/bin/true; echo after $?` writes the division
// and `after 1` in ksh93u+ and in zsh 5.9.2 (`division by zero`), and `i=1;
// b[i++]=v /usr/bin/true` leaves `i` at 1 in both, where this engine never
// read the subscript at all and ran the program at 0. See
// Runner.prefixExpansionShell. Not through `command`: that road is the
// builtin's, which expands its prefix in this shell and has never read the
// subscript, and a walk reading it in a copy there made `set -x` change the
// answer — so it keeps the line as written until that road reads its prefix
// in a child.
//
// The route's store then takes what was read rather than reading it again —
// the arrangement interp/declarationinturn.go makes for a declaration's
// element literal, through the same two fields — so `b[i++]=Q :` steps `i`
// once, and the warning the store makes under the empty key is owed to the
// walk's and swallowed (Runner.emptyKeyWarningsOwed).
//
// The **element** is not stored by the walk, only read: an entry behind it
// still reads the element the shell held, where ksh93 shows it the new one.
// That is the store's half of "in turn" and is a question of its own.

// prefixSubscriptRead is one subscripted prefix entry's subscript, as the
// walk read it.
type prefixSubscriptRead struct {
	assign *syntax.Assign
	// text is the subscript's words expanded: a table's key, or the
	// expression an indexed name evaluates.
	text string
	// keyed says the name was a table when the words were read, so text is
	// the whole of the subscript and there is no arithmetic to come.
	keyed bool
	// idx is the position the expression evaluated to, and placed says the
	// subscript is fully read — evaluated, or a key.
	idx    int
	placed bool
}

// readsPrefixSubscriptInTurn reports whether this prefix entry is an element
// write whose subscript the walk reads in turn: one the dialect stores at all,
// to a plain name, with one subscript holding an expression or a key.
//
// The other shapes — a chain, an element literal, a flag group or a range, a
// whole-array subscript, a dotted or referenced name, a frozen one — keep the
// store's own reading, which is what they had.
func (r *Runner) readsPrefixSubscriptInTurn(a *syntax.Assign) bool {
	if !prefixIsSubscripted(a) || r.subscriptedPrefixDropped(a) {
		return false
	}
	if len(a.Leading) > 0 || a.IsArray || a.Members != nil || a.Member != "" ||
		a.IndexFlags != nil || a.IndexRange != nil || a.EmptySubscript {
		return false
	}
	if wholeArraySubscript(a.IndexText) || pairIsWritten(a.IndexText) {
		return false
	}
	if strings.Contains(a.Name, ".") || r.isNameref(a.Name) || r.attrHas(a.Name, attrReadonly) {
		return false
	}
	return true
}

// prefixSubscriptReadFor is what the walk has read of this entry's subscript,
// or nil. A scan, on the terms prefixTraceAssigns gives.
func (r *Runner) prefixSubscriptReadFor(a *syntax.Assign) *prefixSubscriptRead {
	for _, s := range r.prefixSubscripts {
		if s.assign == a {
			return s
		}
	}
	return nil
}

// readPrefixSubscriptBeforeTheValue is a walk arriving at a prefix entry:
// where the entry reads its subscript in turn and the dialect expands an
// element write's subscript before its value, the words are read now. Reports
// whether the entry reads its subscript in turn at all, so the walk knows to
// place it once the value is in, and whether the words that were read
// expanded — false is a failure that has been said, and the value is not
// reached.
func (r *Runner) readPrefixSubscriptBeforeTheValue(a *syntax.Assign) (inTurn, expanded bool) {
	if !r.readsPrefixSubscriptInTurn(a) {
		return false, true
	}
	if r.subscriptExpandsBeforeTheValue(a) {
		return true, r.readPrefixSubscriptWords(a) != nil
	}
	return true, true
}

// readPrefixSubscriptWords expands the subscript's words, once, and records
// them. Nil where they would not expand, which has been said.
//
// The words are read the way the store reads them — a table's key through
// Runner.assocAssignKey, an indexed name's expression through
// Runner.indexedAssignSubscriptText — so the store taking the record and the
// store reading for itself cannot come to different text.
func (r *Runner) readPrefixSubscriptWords(a *syntax.Assign) *prefixSubscriptRead {
	if s := r.prefixSubscriptReadFor(a); s != nil {
		return s
	}
	// Located as a value of the prefix is, which is the command's to say.
	// See interp/prefixfailurelocation.go.
	defer r.speakForAPrefixValue()()
	s := &prefixSubscriptRead{assign: a}
	if r.assocDeclared(a.Name) {
		key, ok := r.assocAssignKey(a.Name, a.Subscript())
		if !ok {
			return nil
		}
		s.text, s.keyed = key, true
	} else {
		text, ok := r.indexedAssignSubscriptText(a)
		if !ok {
			return nil
		}
		s.text = text
	}
	r.prefixSubscripts = append(r.prefixSubscripts, s)
	return s
}

// placePrefixSubscript finishes reading the subscript once the value is in:
// the words, if they were not read ahead of it, and an indexed name's
// arithmetic. A failure is reported as the store reports it, ends the walk,
// and is false.
//
// Then the warning the store will make under a table's empty key, which is
// written here — ahead of the entry's line, where the reference writes it —
// and, where this shell is the one that will make the store, owed to it. A
// program's prefix is stored by its child, which this engine never makes, so
// nothing is owed there.
func (r *Runner) placePrefixSubscript(a *syntax.Assign, storedHere bool) bool {
	s := r.readPrefixSubscriptWords(a)
	if s == nil || s.placed {
		return s != nil
	}
	// The arithmetic's failure and the store's warning are located as a
	// value's failure is: the shell's for a store it keeps, the builtin's
	// where the prefix is the builtin's environment. See
	// interp/prefixfailurelocation.go.
	defer r.speakForAPrefixValue()()
	if !s.keyed {
		idx, ok := r.indexedAssignPosition(a, s.text)
		if !ok {
			return false
		}
		s.idx = idx
	}
	s.placed = true
	if !s.keyed || s.text != "" || r.diag().EmptyTableKeyTraced == "" || !r.tracing() {
		return true
	}
	r.writeEmptyTableKeyWarning()
	if storedHere {
		r.oweTheEmptyKeyWarning("")
	}
	return true
}

// prefixSubscriptTraced is the subscript an entry's trace line writes, where
// the walk has read it and the dialect writes what a subscript resolved to:
// the key, or the position — counted from the front, as a bare element
// write's line counts it (Runner.tracedElementPosition).
func (r *Runner) prefixSubscriptTraced(a *syntax.Assign) (string, bool) {
	s := r.prefixSubscriptReadFor(a)
	if s == nil || !s.placed {
		return "", false
	}
	if !r.ask(r.sem().TraceElementSubscriptIsEvaluated,
		"a traced subscript written as what it resolved to") {
		return "", false
	}
	if s.keyed {
		return s.text, true
	}
	return itoa(r.tracedElementPosition(a.Name, s.idx)), true
}

// takePrefixSubscriptRead hands the route's store what the walk read, so the
// store neither expands the words nor evaluates the expression a second
// time. See Runner.subscriptReadAhead and Runner.indexReadAhead.
func (r *Runner) takePrefixSubscriptRead(e *expandedAssign) {
	s := r.prefixSubscriptReadFor(e.assign)
	if s == nil || !s.placed {
		return
	}
	e.subscriptAhead, e.subscriptAheadSet = s.text, true
	if !s.keyed {
		e.indexAhead, e.indexAheadSet = s.idx, true
	}
}

// takeThePrefixWalkFrom keeps what a prefix walk made in the copy of this
// shell it expanded in — the values, the joins, the matches and the
// subscripts it read, which the route reads back rather than expanding again
// — together with a failure the walk ran into. Nothing else of the copy comes
// back: a side effect in a value or a subscript is the child's.
func (r *Runner) takeThePrefixWalkFrom(ex *Runner) {
	if ex == r {
		return
	}
	r.prefixTraceAssigns, r.prefixTraceValues = ex.prefixTraceAssigns, ex.prefixTraceValues
	r.prefixTraceJoins, r.prefixGlobMatches = ex.prefixTraceJoins, ex.prefixGlobMatches
	r.prefixSubscripts = ex.prefixSubscripts
	r.takeThePrefixChildsFailure(ex)
}
