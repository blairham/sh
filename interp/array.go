// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// arrayBase is the index the first element answers to. bash and ksh93 count
// from 0 and zsh from 1, which is measured — and it means a subscript cannot
// be used as a slice offset without asking.
func (r *Runner) arrayBase() int {
	if r.ask(r.sem().ArrayBaseIsZero, "arrays being indexed from zero") {
		return 0
	}
	return 1
}

func (r *Runner) setArray(name string, elems []string) {
	if r.Arrays == nil {
		r.Arrays = map[string][]string{}
	}
	r.Arrays[name] = elems
	// A plain `$a` has to keep working. The first element is stored rather
	// than the scalar view, because *which* view it is depends on a dialect
	// and building an array must not need one: getVar asks, and only when
	// the answer could differ.
	if len(elems) > 0 {
		r.setVar(name, elems[0])
	} else {
		r.setVar(name, "")
	}
}

// arrayScalar is what a plain `$a` gives when `a` is an array.
//
// Two answers: every element joined by a space, or the first element alone.
// Asked only when there is more than one element, because with none or one the
// two agree — and only when a scalar is actually read, because building an
// array is not a question about how it would be flattened.
func (r *Runner) arrayScalar(elems []string) string {
	switch {
	case len(elems) == 0:
		return ""
	case len(elems) == 1:
		return elems[0]
	case r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array"):
		return strings.Join(elems, " ")
	default:
		return elems[0]
	}
}

// setArrayElem assigns one element, growing the array if the index is past
// the end — which is what makes `a[5]=x` on an empty array legal.
func (r *Runner) setArrayElem(name string, idx int, value string) {
	if r.Arrays == nil {
		r.Arrays = map[string][]string{}
	}
	i := idx - r.arrayBase()
	if i < 0 {
		r.diagf("%s[%d]: index out of range\n", name, idx)
		return
	}
	cur := r.Arrays[name]
	for len(cur) <= i {
		cur = append(cur, "")
	}
	cur[i] = value
	r.setArray(name, cur)
}

// arrayElems returns an array's elements, treating a plain variable as a
// one-element array — which is what makes `x=v; echo ${x[0]}` work in bash.
func (r *Runner) arrayElems(name string) ([]string, bool) {
	// Produced first, for the same reason a produced scalar is read ahead of
	// the stored table: the record is the answer, and a copy left in Arrays
	// would be the previous pipeline's.
	if elems, ok := r.pipelineStatuses(name); ok {
		return elems, true
	}
	if a, ok := r.Arrays[name]; ok {
		return a, true
	}
	if v, ok := r.getVar(name); ok {
		return []string{v}, true
	}
	return nil, false
}

// arraySubscript answers `${a[i]}`, `${a[@]}` and `${a[*]}`.
//
// The whole-array forms are the reason this returns a slice: `"${a[@]}"` is
// one field per element, exactly as `"$@"` is one per parameter, and joining
// them would lose an element that contains a space.
func (r *Runner) arraySubscript(e *syntax.ParamExpr) ([]string, bool) {
	if e.Index == nil {
		return nil, false
	}
	elems, ok := r.arrayElems(e.Name)
	if !ok {
		return nil, true
	}
	switch idx := r.subscriptText(e.Index); idx {
	case "@", "*":
		return elems, true
	default:
		n, err := r.parseNum(idx)
		if err != nil {
			return nil, true
		}
		i := n - r.arrayBase()
		if i < 0 || i >= len(elems) {
			return nil, true
		}
		return []string{elems[i]}, true
	}
}

// subscriptText reads a subscript without letting it expand as a pattern.
//
// `${a[*]}` is the whole array and `${a[@]}` is its elements, and the two were
// behaving differently for a reason that had nothing to do with either: the
// subscript went through ordinary expansion, where `*` is a pattern that
// matched no file and became the empty string. `@` is not a pattern, so it
// survived and `*` did not.
//
// A subscript written as a plain literal is taken as written. Anything else —
// `${a[$i]}`, `${a[i+1]}` — still expands, because it has to.
func (r *Runner) subscriptText(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal {
		return strings.TrimSpace(w.Spans[0].Value)
	}
	return strings.TrimSpace(r.joinWord(w))
}
