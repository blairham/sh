// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"slices"
	"strings"
)

// The `o` and `O` glob qualifiers: the order a pattern's matches come back
// in, chosen by the pattern itself. See globSortSpec for the measurement.
//
// Where this runs against the rest of the qualifier list is measured too, on
// zsh 5.9.2 in a directory holding `a.zz` (newest), `b.aa` and `c.mm`:
//
//	*(:e)          aa mm zz   the name order is the order of the *modified*
//	*([1]:e)       aa         words, and the pick comes after that
//	*(om:e)        zz aa mm   while a file's own key orders the files
//	*(om[1]:e)     zz         and the pick is the newest one's
//
// So a match is made into its word first, the words are sorted — by name, or
// by what each one's file says — and the `[n,m]` picks then count into the
// sorted list. That is one pipeline for every qualifier list, sorting or not:
// `*([1]:e)` picked before the modifiers re-sorted until this was written,
// and gave `zz`.

// sortQualified puts the words in the order the list's specifiers ask for.
// paths are the files the words came from, one each, which is what a size or
// a time is asked of.
func (r *Runner) sortQualified(words, paths []string, specs []globSortSpec) {
	if len(words) != len(paths) {
		return
	}
	byName := r.globMatchOrder()
	type keyed struct {
		word  string
		path  string
		value []int64
		parts []string
	}
	items := make([]keyed, len(words))
	for i := range words {
		it := keyed{word: words[i], path: strings.TrimSuffix(paths[i], "/"), value: make([]int64, len(specs))}
		f := globFile{r: r, path: it.path}
		for k, spec := range specs {
			switch spec.key {
			case 'n':
			case 'd':
				if it.parts == nil {
					it.parts = strings.Split(it.path, "/")
				}
			default:
				it.value[k] = globSortValueOf(f.info(spec.follow), spec.key)
			}
		}
		items[i] = it
	}
	slices.SortStableFunc(items, func(a, b keyed) int {
		for k, spec := range specs {
			c := 0
			switch spec.key {
			case 'n':
				c = byName(a.word, b.word)
			case 'd':
				c = deeperFirst(a.parts, b.parts)
			default:
				switch {
				case a.value[k] < b.value[k]:
					c = -1
				case a.value[k] > b.value[k]:
					c = 1
				}
				if spec.key == 'a' || spec.key == 'm' || spec.key == 'c' {
					// A time sorts the youngest first, which is the larger
					// number: the manual's own wording, and measured.
					c = -c
				}
			}
			if spec.desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return byName(a.word, b.word)
	})
	for i, it := range items {
		words[i] = it.word
	}
}

// globSortValueOf is the number one key letter reads off a file. A file that
// would not stat sorts as zero, the bargain sortMatchesBy strikes for the
// same case.
func globSortValueOf(fi fs.FileInfo, key byte) int64 {
	if fi == nil {
		return 0
	}
	switch key {
	case 'L':
		return fi.Size()
	case 'l':
		if n, ok := fileLinks(fi); ok {
			return int64(n)
		}
	case 'm':
		return fi.ModTime().UnixNano()
	case 'a', 'c':
		if t, ok := fileTime(fi, key); ok {
			return t.UnixNano()
		}
	}
	return 0
}

// deeperFirst is the `d` key: of two paths, the one that goes on into a
// subdirectory where the other stops comes first, decided at the first
// component where they part. Two paths that both go on, or both stop, there
// are a tie for this key — `a/y.lis` and `d/e/b.lis` part at `a` and `d`,
// and neither is deeper *at that level* (see globSortSpec).
func deeperFirst(a, b []string) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	// A path that has run out stops where the other goes on into it:
	// `a/b/x.lis` is ahead of the directory `a/b` that holds it.
	aOn, bOn := len(a) > i+1, len(b) > i+1
	switch {
	case i == len(a) && i < len(b):
		bOn = true
	case i == len(b) && i < len(a):
		aOn = true
	}
	switch {
	case aOn && !bOn:
		return -1
	case bOn && !aOn:
		return 1
	}
	return 0
}
