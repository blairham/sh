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
	// The scalar view of an array is its first element, so a plain `$a` keeps
	// working. Storing it keeps getVar honest without a special case there.
	if len(elems) > 0 {
		r.setVar(name, elems[0])
	} else {
		r.setVar(name, "")
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
	switch idx := strings.TrimSpace(r.joinWord(e.Index)); idx {
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
