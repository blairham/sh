// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `compdescribe -g`: the arrangement the `list-grouped` style asks for, where
// options that share a description are listed on one row —
//
//	--all                       -a  -- list entries starting with .
//	--almost-all                -A  -- list all except . and ..
//
// — rather than a row each (#6178).
//
// # How it is asked for, and what comes back
//
// The `-g` stands after the explanation array and before the first
// definition: `_arguments` writes `compdescribe -I '' 60 '-- ' _expl -g
// _a_11 -M … -- _a_15 …`. zsh refuses it anywhere later — `… -- -g H` is
// `invalid argument: -g`.
//
// What it answers is not a group of rows but a **grid**, handed back one
// cell at a time for the caller to `compadd` in order: each name its own
// `-g` answer, in a block named as the explanation named it but unsorted
// and keeping duplicates (`-J ej` becomes `-2V ej`), the empty cells as
// `-E n` runs, and the descriptions as a final column of empty matches whose
// display strings are the descriptions. The listing, told `packed` through
// the first array `-g` fills, lays those cells out column after column, and
// the rows come out as above. Measured on zsh 5.9.2, 2026-10-05, from inside
// a `zle -C` widget, `G=(-a:same -b:same -c:same -d:other)` and
// `compdescribe -I '' 60 '-- ' E -g G` with `E=(-J ej -X ex)`, `$COLUMNS`
// 120, reading `-g` back until it answers 1:
//
//	packed  (-2V ej -X ex)        -c
//	packed  (-2V ej -X ex)        -d
//	packed  (-2V ej -X ex)        -b
//	packed  (-E1 -J ej -X ex)
//	packed  (-2V ej -X ex)        -a
//	packed  (-E1 -J ej -X ex)
//	packed  (-E2 -J ej -X ex)     displays: `-- same` and `-- other`, each
//	                              padded to 106 columns
//
// which is the grid
//
//	-c  -b  -a  -- same
//	-d          -- other
//
// # The rules, each measured on the same widget
//
//   - **A row is one description**, and its names are ordered shortest first
//     and then reversed, ties kept in the order they were defined:
//     `-b:s -a:s -c:s` is `-c -a -b`, `-a:s --bb:s -c:s --dd:s` is
//     `--dd --bb -c -a`, and `--all:x -a:x` is `--all -a`. Names from
//     different definitions share a row.
//   - **A row wider than the second argument wraps**, and its description
//     goes on its last line. A name costs its width and two, and a line
//     takes another name only while the total stays *below* the width: with
//     seven nine-character names, 22 holds one a line and 23 holds two, 33
//     two and 34 three. Each line is reversed on its own.
//   - **Rows are sorted by their first name**, and the lines of a wrapped
//     row stay together.
//   - **Packing is done only where it saves a row.** Where every description
//     is its own — or the width is so narrow that every name is a line — the
//     answer is the ordinary one (described names under `-l`, then the bare
//     ones), exactly as without `-g`.
//   - **The description column is as wide as what the names leave**:
//     `$COLUMNS` less each name column's widest name and two, less two more,
//     and a description longer than that is cut to it — `-- ` and a 150-
//     character description come back 110 columns wide beside two
//     two-character name columns.
//   - **An empty cell is a run**: consecutive empty cells in one column are
//     one `-E n`, and a run never crosses into the next column.
//   - **Each name carries its own definition's options**, so a row may hold
//     names that `compadd` adds with different suffixes. The empty cells and
//     the descriptions carry the options of the definition that owns the
//     **first row's first name**: with `G` given `-Q` and `H` given `-S=`,
//     `G=(--aa:s -z:u) H=(-a:s -b:t)` answers its runs with `-Q` and
//     `G=(-z:u -a:s) H=(--aa:s -b:t)` with `-S=` — `--aa` heads the first
//     row both times, and it is the last definition only the second time.
//     The `ls -<TAB>` trace agrees: its runs carry `--all`'s definition's
//     options and not the last definition's `-qS=`.
//   - **The undescribed names come last**, one answer per definition, not
//     packed, with the definition's options and the explanation *without*
//     its `-X` heading: `-z` above comes back `(-Q -J ej)`.
//
// The row order is the one place this is knowingly not zsh's: zsh sorts the
// rows by the locale's collation (`-g -l -m -o -p -S -t` under
// `en_US.UTF-8`), and this shell orders words by byte everywhere — see
// interp/order.go for why, and #6168.

// packedCell is one answer of a packed `-g`.
type packedCell struct {
	options  []string
	words    []string
	displays []string
	packed   bool
}

// packedName is one described name on its way into the grid.
type packedName struct {
	name, match string
	def         int
}

// pack builds the grid, or answers nil where packing would save nothing and
// the ordinary answer is the right one.
func (d *describeState) pack(r *interp.Runner) []packedCell {
	type described struct {
		descr string
		names []packedName
	}
	var rows []*described
	byDescr := map[string]*described{}
	bare := make([][2][]string, len(d.groups))
	total := 0
	for gi, g := range d.groups {
		pairs, _ := r.GetArray(g.pairs)
		var matches []string
		if g.matches != "" {
			matches, _ = r.GetArray(g.matches)
		}
		for i, pair := range pairs {
			name, descr, has := strings.Cut(pair, ":")
			match := name
			if i < len(matches) {
				match = matches[i]
			}
			if !has {
				bare[gi][0] = append(bare[gi][0], match)
				bare[gi][1] = append(bare[gi][1], name)
				continue
			}
			row := byDescr[descr]
			if row == nil {
				row = &described{descr: descr}
				byDescr[descr] = row
				rows = append(rows, row)
			}
			row.names = append(row.names, packedName{name: name, match: match, def: gi})
			total++
		}
	}
	// Each description's names, shortest first and then reversed, filled
	// into lines no wider than the width argument.
	type line struct {
		names []packedName
		descr string
		last  bool
	}
	type block struct{ lines []line }
	var blocks []block
	count := 0
	for _, row := range rows {
		names := row.names
		sort.SliceStable(names, func(i, j int) bool {
			return repl.DisplayWidth(names[i].name) < repl.DisplayWidth(names[j].name)
		})
		var b block
		var cur []packedName
		used := 0
		for _, n := range names {
			cost := repl.DisplayWidth(n.name) + 2
			if len(cur) > 0 && used+cost >= d.width {
				b.lines = append(b.lines, line{names: cur})
				cur, used = nil, 0
			}
			cur = append(cur, n)
			used += cost
		}
		b.lines = append(b.lines, line{names: cur})
		for i := range b.lines {
			reverseNames(b.lines[i].names)
		}
		last := &b.lines[len(b.lines)-1]
		last.descr, last.last = row.descr, true
		count += len(b.lines)
		blocks = append(blocks, b)
	}
	if count >= total {
		return nil
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		return shellListingOrder(blocks[i].lines[0].names[0].name, blocks[j].lines[0].names[0].name) < 0
	})
	var lines []line
	for _, b := range blocks {
		lines = append(lines, b.lines...)
	}
	cols := 0
	for _, l := range lines {
		cols = max(cols, len(l.names))
	}
	widths := make([]int, cols)
	for _, l := range lines {
		for c, n := range l.names {
			widths[c] = max(widths[c], repl.DisplayWidth(n.name))
		}
	}
	room := columnsOf(r) - 2
	for _, w := range widths {
		room -= w + 2
	}
	room = max(room, 0)

	fillOptions := d.groups[lines[0].names[0].def].options
	var out []packedCell
	run := func(n int, displays []string) {
		if n == 0 {
			return
		}
		opts := append([]string{"-E" + strconv.Itoa(n)}, fillOptions...)
		opts = append(opts, d.explanation...)
		out = append(out, packedCell{options: opts, displays: displays, packed: true})
	}
	for c := range cols {
		empty := 0
		for _, l := range lines {
			if c >= len(l.names) {
				empty++
				continue
			}
			run(empty, nil)
			empty = 0
			n := l.names[c]
			opts := append(append([]string{}, d.groups[n.def].options...), unsortedKeepingAll(d.explanation)...)
			out = append(out, packedCell{options: opts, words: []string{n.match}, displays: []string{n.name}, packed: true})
		}
		run(empty, nil)
	}
	descrs := make([]string, len(lines))
	for i, l := range lines {
		if l.last {
			descrs[i] = fitToWidth(d.separator+l.descr, room)
		}
	}
	run(len(lines), descrs)
	for gi, g := range d.groups {
		if len(bare[gi][0]) == 0 {
			continue
		}
		opts := append(append([]string{}, g.options...), withoutHeading(d.explanation)...)
		out = append(out, packedCell{options: opts, words: bare[gi][0], displays: bare[gi][1]})
	}
	return out
}

// reverseNames reverses one line of a row in place.
func reverseNames(names []packedName) {
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
}

// unsortedKeepingAll is the explanation as a grid's names are added under
// it: the block it names, unsorted and keeping every match — `-J ej`
// becomes `-2V ej`, wherever in the array the `-J` stands. Measured: the
// `ls -<TAB>` explanation `-M m:{…} -J options -X …` comes back
// `-M m:{…} -2V options -X …`.
func unsortedKeepingAll(explanation []string) []string {
	out := append([]string{}, explanation...)
	for i, word := range out {
		if word == "-J" || word == "-V" {
			out[i] = "-2V"
			break
		}
	}
	return out
}

// withoutHeading is the explanation with its `-X` heading taken out, which
// is how the undescribed names after a grid are added: the heading is
// already over the grid.
func withoutHeading(explanation []string) []string {
	var out []string
	for i := 0; i < len(explanation); i++ {
		if explanation[i] == "-X" {
			i++
			continue
		}
		out = append(out, explanation[i])
	}
	return out
}

// fitToWidth pads a description out to the width the grid leaves it, or
// cuts it there.
func fitToWidth(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// columnsOf is the terminal width a grid is laid out against: `$COLUMNS`,
// or 80 where it says nothing usable.
func columnsOf(r *interp.Runner) int {
	if v, ok := r.GetVar("COLUMNS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 80
}

// shellListingOrder is the order the rows of a grid are put in: by byte,
// the order this shell sorts every listing in. See the file comment for
// where that parts from zsh's.
func shellListingOrder(a, b string) int { return strings.Compare(a, b) }
