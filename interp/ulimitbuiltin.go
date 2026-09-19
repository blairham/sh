// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `ulimit` reads and changes the limits this shell and its children run under.
//
// The other half of #117. Before the names were reserved, `ulimit -n 256` found
// /usr/bin/ulimit on PATH, set the limit in a child, and exited 0 — so a script
// that raised its file-descriptor limit before opening a thousand files got no
// error and no headroom.
//
// The limits live in the process, so the work is done by hooks the caller
// supplies. See Runner.GetRlimit and Runner.SetRlimit.

func init() {
	builtins["ulimit"] = biUlimit
}

// UlimitListingRow is one line of `ulimit -a`, whose table no two shells lay
// out alike: the labels, the order, which rows exist at all and what unit
// each is counted in are all the dialect's. The prefix is everything before
// the value, spacing included, so a row is the prefix and then the number.
type UlimitListingRow struct {
	// Prefix is the label column exactly as the engine writes it.
	Prefix string
	// Letter is the option letter this row is read with, and it is where a
	// shell's letters come from: `ulimit -X` is the row spelled X, and a
	// letter no row spells is a bad option.
	//
	// Measured 2026-09-18 over seven columns on two kernels, and the letters
	// are not shared enough to live anywhere else: `-p` is the pipe buffer
	// in bash and ksh93 and the process count in dash, `-w` is file locks in
	// dash and swap in ksh93, `-x` is file locks in everyone but ksh93 —
	// where it is a row that says it has none — and `-u` is a letter dash
	// does not have at all. Zero means a row this shell prints and has no
	// letter for, which is the one zsh writes as `-N 15`.
	Letter byte
	// Fixed, when non-empty, is the whole value: the rows about socket
	// buffers, and the ones an engine lists as unsupported, are not resource
	// limits and never move. Reading one prints the text; setting one is
	// refused, since there is nothing behind it to set.
	Fixed string
	// Name is what a refusal calls this row where the label is not it. One
	// shell writes `ulimit: msgqueue: is read only` for a row its table
	// labels `message queue size (Kibytes)`, and the short name is nowhere
	// in the long one. Empty means the label, which is what every other
	// refusal already uses — see Diagnostics.ulimitResourceName.
	Name string
	// Res and Scale are the live rows' inputs — the resource, and what one
	// printed unit is worth, zero meaning the dialect's block unit.
	Res   Resource
	Scale int64
}

// name is what a refusal about this row calls it.
func (row UlimitListingRow) name() string {
	if row.Name != "" {
		return row.Name
	}
	label := row.Prefix
	if i := strings.IndexByte(label, '('); i >= 0 {
		label = label[:i]
	}
	return strings.TrimSpace(label)
}

// ulimitRows is the table this shell prints here and now: the dialect's rows,
// less the ones this kernel has no limit behind, and in the kernel's own
// order where the dialect lists them that way.
//
// It answers `ulimit -a` and `ulimit -X` alike, which is the measurement:
// every column accepts exactly the letters its own table prints. bash refuses
// `-e` on macOS and reads it on Linux from one binary, and zsh has no `-m` on
// macOS because the row is not there — while ksh93 reads `-e` on both,
// because its row is a sentence saying the limit is not supported rather than
// a limit (#2805).
func (r *Runner) ulimitRows() []UlimitListingRow {
	rows := r.diag().UlimitListing
	out := make([]UlimitListingRow, 0, len(rows))
	if r.diag().UlimitListingInKernelOrder && len(r.RlimitOrder) > 0 {
		byRes := make(map[Resource]UlimitListingRow, len(rows))
		for _, row := range rows {
			if row.Fixed == "" {
				byRes[row.Res] = row
			}
		}
		for _, res := range r.RlimitOrder {
			if row, ok := byRes[res]; ok {
				out = append(out, row)
			}
		}
		return out
	}
	for _, row := range rows {
		// A limit this kernel does not have is not a row that reads zero —
		// it is a row that is not there. See Runner.HasRlimit for the
		// three shells that were measured saying so.
		if row.Fixed == "" && r.HasRlimit != nil && !r.HasRlimit(row.Res) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// ulimitRow is the row a letter reads, and false where this shell has no such
// letter.
//
// A dialect that has not said what its table looks like falls back to the
// substrate's own letters — see resourceLetters, which is POSIX's set and not
// any shell's.
func (r *Runner) ulimitRow(c byte) (UlimitListingRow, bool) {
	if len(r.diag().UlimitListing) == 0 {
		res, scale, ok := lookupResource(c)
		return UlimitListingRow{Letter: c, Res: res, Scale: scale}, ok
	}
	for _, row := range r.ulimitRows() {
		if row.Letter == c {
			return row, true
		}
	}
	return UlimitListingRow{}, false
}

// ulimitListing is `ulimit -a`: every row the dialect lists, soft limits
// unless -H asked for the hard ones — the same choice a single report makes.
func (r *Runner) ulimitListing(hard bool) int {
	rows := r.diag().UlimitListing
	if len(rows) == 0 {
		r.diagf("%s\n", r.unanswered("how `ulimit -a` is laid out"))
		r.status = 2
		r.unspecified = true
		return 2
	}
	blockUnit := int64(512)
	if r.ask(r.sem().UlimitBlockIsKilobyte, "`ulimit -f` counting in 1024-byte blocks") {
		blockUnit = kilobyte
	}
	if r.unspecified {
		return r.status
	}
	for _, row := range r.ulimitRows() {
		if row.Fixed != "" {
			r.printf("%s%s\n", row.Prefix, row.Fixed)
			continue
		}
		unit := row.Scale
		if unit == 0 {
			unit = blockUnit
		}
		soft, max, err := r.GetRlimit(row.Res)
		if err != nil {
			r.diagf("ulimit: %v\n", err)
			return 1
		}
		v := soft
		if hard {
			v = max
		}
		if v == RlimitInfinity {
			r.printf("%sunlimited\n", row.Prefix)
			continue
		}
		r.printf("%s%d\n", row.Prefix, v/unit)
	}
	return 0
}

func biUlimit(r *Runner, _ context.Context, args []string) int {
	// `-H` and `-S` choose which limit is read or written; without either,
	// reading gives the soft one and writing sets both. Unanimous.
	var hard, soft, all bool
	// The default resource is `-f`, which is why bare `ulimit` reports the
	// file-size limit rather than a summary. Read from the dialect's own row
	// where it has one, so the unit is not written down twice.
	row, ok := r.ulimitRow('f')
	if !ok {
		row = UlimitListingRow{Letter: 'f', Res: ResourceFileSize}
	}
	for len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		letters := args[0][1:]
		for i := 0; i < len(letters); i++ {
			switch c := letters[i]; c {
			case 'H':
				hard = true
			case 'S':
				soft = true
			case 'a':
				// The whole table, in the dialect's own layout — see
				// UlimitListingRow. All four shells have the letter.
				all = true
			default:
				var found bool
				row, found = r.ulimitRow(c)
				if !found {
					line := Wording(r.diag().UlimitBadOption,
						"ulimit: -%[1]s: invalid option", string(c))
					// One column writes this one bare, where every other
					// bad-option refusal of the same shell carries the
					// script, the builtin and the line. See
					// Diagnostics.UlimitBadOptionUnprefixed.
					if r.diag().UlimitBadOptionUnprefixed {
						r.errf("%s\n", line)
					} else {
						r.diagf("%s\n", line)
					}
					// The usage line every other builtin's refusal is
					// followed by, in the dialects that print one. This
					// builtin has a refusal of its own — the letters are
					// resources rather than a fixed set — and had been
					// left out of the line that comes after it (#825).
					r.builtinUsageLine("ulimit")
					return orDefault(r.diag().UlimitBadOptionStatus, 2)
				}
			}
		}
		args = args[1:]
	}
	if r.GetRlimit == nil || r.SetRlimit == nil {
		r.diagf("ulimit: this shell was not given any limits to read or change\n")
		return 2
	}
	if all {
		return r.ulimitListing(hard)
	}
	if row.Fixed != "" {
		// A row that is a sentence rather than a limit. Reading it prints
		// the sentence; setting it is refused, and one shell's wording for
		// that refusal is its own — `ulimit: locks: is read only` at 1,
		// measured 2026-09-18 on ksh93u+ at every fixed row it has.
		if len(args) == 0 {
			r.printf("%s\n", row.Fixed)
			return 0
		}
		r.diagf("%s\n", Wording(r.diag().UlimitReadOnly,
			"ulimit: %[1]s: is read only", row.name()))
		return orDefault(r.diag().UlimitReadOnlyStatus, 1)
	}
	res := row.Res
	unit := row.Scale
	if unit == 0 {
		// The block scale, which is the one dialect question in the numbers:
		// 1024 bytes in bash, 512 in the other three.
		unit = 512
		if r.ask(r.sem().UlimitBlockIsKilobyte, "`ulimit -f` counting in 1024-byte blocks") {
			unit = kilobyte
		}
	}
	if len(args) == 0 {
		return r.reportLimit(res, hard, unit)
	}
	cur, curHard, err := r.GetRlimit(res)
	if err != nil {
		r.diagf("ulimit: %v\n", err)
		return 1
	}
	want, ok := r.ulimitOperand(args[0], unit, cur, curHard)
	if r.unspecified {
		return r.status
	}
	if !ok {
		r.diagf("%s\n", Wording(r.diag().UlimitBadNumber,
			"ulimit: %[1]s: invalid number", args[0]))
		return orDefault(r.diag().UlimitBadNumberStatus, 1)
	}
	// With neither flag, three of the four lower the ceiling as well as the
	// floor — which is what makes `ulimit -t 3600` irreversible. zsh moves
	// only the floor, so the same line there can be undone.
	newSoft, newHard := want, want
	switch {
	case hard && !soft:
		newSoft = cur
	case soft && !hard:
		newHard = curHard
	case !hard && !soft:
		if !r.ask(r.sem().UlimitSetsBothLimits, "`ulimit -t N` lowering the hard limit too") {
			newHard = curHard
		}
	}
	if err := r.SetRlimit(res, newSoft, newHard); err != nil {
		d := r.diag()
		r.diagf("%s\n", Wording(d.UlimitCannotChange,
			"ulimit: cannot change limit: %[3]s",
			d.ulimitResourceName(res), args[0], d.reasonText(reason(err))))
		return orDefault(d.UlimitCannotChangeStatus, 1)
	}
	return 0
}

// reportLimit prints one limit, or the word every shell uses for no limit.
func (r *Runner) reportLimit(res Resource, hard bool, unit int64) int {
	soft, max, err := r.GetRlimit(res)
	if err != nil {
		r.diagf("ulimit: %v\n", err)
		return 1
	}
	v := soft
	if hard {
		v = max
	}
	if v == RlimitInfinity {
		r.printf("unlimited\n")
		return 0
	}
	r.printf("%d\n", v/unit)
	return 0
}

// lookupResource maps an option letter to what it addresses.
func lookupResource(c byte) (Resource, int64, bool) {
	for _, e := range resourceLetters {
		if e.letter == c {
			return e.res, e.scale, true
		}
	}
	return 0, 0, false
}

// ulimitOperand reads the word a limit is being set from, in the three
// shapes the panel has: a plain number or `unlimited`, which every shell
// takes; the words `hard` and `soft`, which name the limits this resource
// already has; and an arithmetic expression, which is ksh93's reading of
// anything that is not one of the first two.
//
// The order is what makes `hard=333; ulimit -n hard` two different answers
// on one line: bash and zsh spend the keyword before a name can be looked
// up and raise the soft limit to the ceiling, where ksh93 has no keyword to
// spend and evaluates the name, giving 333. Both are reproduced here by
// checking the keyword first and falling through to the expression.
//
// cur and curHard are this resource's limits now, which is what the two
// keywords name. They are raw rather than scaled: `hard` is the ceiling
// itself and not a count of blocks, so the unit multiply belongs only to
// the numbers.
func (r *Runner) ulimitOperand(word string, unit, cur, curHard int64) (int64, bool) {
	if n, ok := parseLimit(word, unit); ok {
		return n, true
	}
	if word == "" {
		// An empty operand, which four of the seven columns read as a limit
		// of **nought** rather than refusing — and nought is what it sets,
		// so neither answer is a line that does nothing. See
		// Semantics.UlimitEmptyOperandIsZero, where the panel is.
		if r.ask(r.sem().UlimitEmptyOperandIsZero, "`ulimit -n \"\"` reading an empty operand as nought") {
			return 0, true
		}
		return 0, false
	}
	switch word {
	case "hard":
		if r.ask(r.sem().UlimitTakesHardKeyword, "`ulimit -n hard`") {
			return curHard, true
		}
	case "soft":
		if r.ask(r.sem().UlimitTakesSoftKeyword, "`ulimit -n soft`") {
			return cur, true
		}
	}
	if r.unspecified {
		return 0, false
	}
	if !r.ask(r.sem().UlimitOperandIsArithmetic, "a `ulimit` operand read as arithmetic") {
		return 0, false
	}
	return r.ulimitArithmetic(word, unit)
}

// ulimitArithmetic is the ksh93 reading of a limit that is not a plain
// number: the word is an expression, so `1000+999` is 1999 and `0x10` is 16.
//
// A bare name must be *set* here, which is the one place this position is
// stricter than `$(( ))`: `echo $((hard))` is 0 in that shell and `ulimit -n
// hard` is `hard: parameter not set`. So the names the expression mentions
// are checked before it is evaluated, and the first missing one is the
// complaint — which is why the two keyword words come out of this dialect
// with the wording the reference gives them without either being named here.
func (r *Runner) ulimitArithmetic(word string, unit int64) (int64, bool) {
	tree, err := r.arithTree(nil, word)
	if err != nil {
		return 0, false
	}
	if _, missing := r.arithUnsetName(tree); missing {
		return 0, false
	}
	n, err := r.evalArith(tree)
	if err != nil || n < 0 {
		return 0, false
	}
	return int64(n) * unit, true
}

// arithUnsetName finds the first name an expression reads that is not set,
// which is the check ulimitArithmetic needs and nothing else does.
//
// A walk of its own rather than a general one, because the question is
// narrow: the nodes that name a parameter are a bare name and a subscripted
// one, and everything else is reached only to get past it.
func (r *Runner) arithUnsetName(e syntax.ArithExpr) (string, bool) {
	switch x := e.(type) {
	case nil:
		return "", false
	case *syntax.ArithVar:
		if !r.arithNameIsSet(x.Name) {
			return x.Name, true
		}
	case *syntax.ArithIndex:
		if !r.arithNameIsSet(x.Name) {
			return x.Name, true
		}
		return r.arithUnsetName(x.Index)
	case *syntax.ArithUnary:
		return r.arithUnsetName(x.X)
	case *syntax.ArithBinary:
		if name, missing := r.arithUnsetName(x.X); missing {
			return name, true
		}
		return r.arithUnsetName(x.Y)
	case *syntax.ArithCond:
		if name, missing := r.arithUnsetName(x.Cond); missing {
			return name, true
		}
		if name, missing := r.arithUnsetName(x.Then); missing {
			return name, true
		}
		return r.arithUnsetName(x.Else)
	}
	return "", false
}

// parseLimit reads a limit written as every shell on the panel writes one:
// the word `unlimited`, or a run of decimal digits in the units the shell
// prints the resource in.
//
// Deliberately not strconv's idea of a number. ParseInt takes a leading sign,
// so `ulimit -n +1999` set the limit from a word bash, zsh, dash and BusyBox
// ash all refuse — silently, at status 0, which is the shape #2298 is about.
// Digits only, so the sign, the leading blank and the `0x` prefix all fall
// through to the dialect that actually reads them.
func parseLimit(s string, unit int64) (int64, bool) {
	if s == "unlimited" {
		return RlimitInfinity, true
	}
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n * unit, true
}
