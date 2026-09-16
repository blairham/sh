// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
)

// The order a pathname expansion comes back in, where the dialect has a
// parameter that says so. bash 5.3's is `GLOBSORT`; no other column in the
// panel has one, which is why the parameter is named by the dialect rather
// than spelled here — see [Semantics.SortOrderVariable].
//
// Measured 2026-09-16 under `LC_ALL=C` against bash 5.3.20, and the rows that
// decide each rule are written beside it. Every one of them answers at status
// 0 and says nothing, including the refusals: a value this shell does not
// recognize is the default order and no diagnostic.

// globSortKey is the property of a match the order is taken from.
type globSortKey uint8

const (
	// globSortName is the default and is the order every other column gives.
	globSortName globSortKey = iota
	globSortSize
	globSortMtime
	globSortAtime
	globSortCtime
	globSortBlocks
	// globSortNumeric compares two names as numbers where both of them are
	// numbers, and by name otherwise. See globSortNumericLess.
	globSortNumeric
	// globSortNone is `nosort`: the order the directory itself gave, which
	// is neither the sorted list nor its reverse.
	globSortNone
)

// globSortOrder is a parsed value of the parameter.
type globSortOrder struct {
	key globSortKey
	// desc reverses the whole comparison, tie-break included: measured,
	// `-size` puts three files that tie at zero bytes in *descending* name
	// order, so the sign is applied to the answer rather than to the key.
	desc bool
}

// parseGlobSort reads the parameter's value, and reports false for a value
// this shell does not recognize — which the caller then answers with the
// default order, silently, exactly as bash does.
//
// The shape is measured rather than taken from a manual, and four of the rows
// are corrections to what the obvious reading would be:
//
//	 -name     descending          the sign is the direction, `+` the default
//	 -         descending by name  a sign with no key is the name key
//	 -NAME     the default         the key is case sensitive
//	 -nam      the default         and it is the whole name, not a prefix
//	 -name     the default         a *trailing* blank is not trimmed
//	" -name"   descending          a *leading* blank is
//	 -nosort   the directory order the sign is ignored where there is no sort
//	 bogus     the default         an unknown key says nothing, at status 0
func parseGlobSort(v string) (globSortOrder, bool) {
	// Leading blanks only. The trailing half is measured and is the
	// asymmetry worth keeping: `GLOBSORT='-name '` is the default order.
	v = strings.TrimLeft(v, " \t")
	var o globSortOrder
	if v != "" && (v[0] == '+' || v[0] == '-') {
		o.desc = v[0] == '-'
		v = v[1:]
	}
	switch v {
	case "", "name":
		o.key = globSortName
	case "size":
		o.key = globSortSize
	case "mtime":
		o.key = globSortMtime
	case "atime":
		o.key = globSortAtime
	case "ctime":
		o.key = globSortCtime
	case "blocks":
		o.key = globSortBlocks
	case "numeric":
		o.key = globSortNumeric
	case "nosort":
		// The one key the sign says nothing about: `nosort`, `-nosort` and
		// `+nosort` are one answer, measured.
		return globSortOrder{key: globSortNone}, true
	default:
		return globSortOrder{}, false
	}
	return o, true
}

// globSortOrderOf resolves the parameter for one expansion.
//
// The second result is false for the order this shell would give anyway —
// no parameter in the dialect, nothing set, a value it does not recognize, or
// a value that names the default. That keeps every column but one, and every
// script but the one that asked, on the path this walk has always taken.
func (r *Runner) globSortOrderOf() (globSortOrder, bool) {
	name := r.sem().SortOrderVariable
	if name == "" {
		return globSortOrder{}, false
	}
	v, ok := r.getVar(name)
	if !ok || v == "" {
		return globSortOrder{}, false
	}
	o, ok := parseGlobSort(v)
	if !ok || (o.key == globSortName && !o.desc) {
		return globSortOrder{}, false
	}
	return o, true
}

// sortMatchesBy puts the words an expansion produced in the order the
// parameter asked for.
//
// Two slices rather than one, and they are the two halves of what a match is:
// the *word* is what the order is about where the key is the name, and the
// *path* is the only thing that can be asked a size or a time. Sorting the
// paths alone answered `GLOBSORT=numeric` by the absolute path, which is
// never a number, so every set came back in name order.
//
// The stat is taken once per path rather than inside the comparison, which
// keeps a directory of a thousand names to a thousand stats instead of the
// several thousand a sort would ask for. A path that will not stat sorts as
// zero, the same bargain the rest of this file strikes for a name the policy
// hides.
func (r *Runner) sortMatchesBy(words, paths []string, o globSortOrder) {
	if len(words) != len(paths) {
		return
	}
	if o.key == globSortNone {
		// `nosort` is the listing's order **reversed**, which is measured
		// rather than reasoned: `ls -f` on the directory that produced
		// `tiny.o d2 big.o mid alpha.o d1 zed` writes those seven names the
		// other way round, and three directories built in three different
		// orders each gave the exact reverse. A walk that simply skipped its
		// sorts came back with the listing order and was wrong every time —
		// which is the useful half of the finding, because "no sort" reads
		// like "whatever was there" and is not that.
		//
		// The whole result rather than each level: `GLOBSORT=nosort; echo
		// */*` is `d2/a d1/b` where the top level lists d1 before d2, and
		// reversing the finished list is what gives that.
		slices.Reverse(words)
		return
	}
	value := make([]int64, len(paths))
	if o.key != globSortName && o.key != globSortNumeric {
		for i, p := range paths {
			value[i] = r.globSortValue(p, o.key)
		}
	}
	at := make([]int, len(words))
	for i := range at {
		at[i] = i
	}
	slices.SortFunc(at, func(a, b int) int {
		c := 0
		switch o.key {
		case globSortNumeric:
			c = globSortNumericLess(words[a], words[b])
		case globSortName:
		default:
			switch {
			case value[a] < value[b]:
				c = -1
			case value[a] > value[b]:
				c = 1
			}
		}
		if c == 0 {
			// The name is the tie-break for every key, measured with three
			// files that tie at zero bytes.
			c = shellOrder(words[a], words[b])
		}
		if o.desc {
			return -c
		}
		return c
	})
	sorted := make([]string, len(words))
	for i, j := range at {
		sorted[i] = words[j]
	}
	copy(words, sorted)
}

// globSortValue is the number one key takes from one path.
func (r *Runner) globSortValue(path string, key globSortKey) int64 {
	info, err := r.stat(path)
	if err != nil {
		return 0
	}
	switch key {
	case globSortSize:
		return info.Size()
	case globSortMtime:
		return info.ModTime().UnixNano()
	case globSortAtime:
		if t, ok := fileAccessTime(info); ok {
			return t.UnixNano()
		}
	case globSortCtime:
		if t, ok := fileChangeTime(info); ok {
			return t.UnixNano()
		}
	case globSortBlocks:
		if n, ok := fileBlocks(info); ok {
			return n
		}
	}
	return 0
}

// globSortNumericLess compares two paths as numbers where both of them are
// numbers, and says nothing otherwise so that the name decides.
//
// Measured, and the second row is why the rule is "both" rather than "either
// one, with the rest as zero":
//
//	9 10 100 a10 a2   numeric   the numbers in order, then the two names
//	10x 2x            numeric   by name, because neither is a number
//
// A set that mixes the two has no total order to be put in — `3` is before
// `2x` by this rule and `2x` is before `3` by name, and bash's own answer to
// such a set is whatever its sort made of a comparison that contradicts
// itself. So the sequence for a mixed set is not claimed here; what is
// claimed is each row above, which is what a script that sorts numbered files
// by number is asking for.
func globSortNumericLess(a, b string) int {
	x, okx := wholeNumber(a)
	y, oky := wholeNumber(b)
	if !okx || !oky {
		return 0
	}
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// wholeNumber reads a path as a number, and only where the whole of it is
// one: `007` is seven and `2x` is not a number at all.
func wholeNumber(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return 0, false
		}
		n = n*10 + int64(s[i]-'0')
		if n < 0 {
			// Longer than an int64 holds, which is a name rather than a
			// number for this purpose: nothing sensible is ranked by it.
			return 0, false
		}
	}
	return n, true
}
