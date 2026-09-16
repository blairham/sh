// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `compvalues`: `comparguments` for `_values`, which is the same spec
// language over a list of values rather than a command line.
//
// # Measured, 2026-09-15 against zsh 5.9.2
//
// Through the pseudo-terminal instrument in computil.go. With `tar c` typed —
// so the word being completed is `c`:
//
//	compvalues -i -s '' 'tar function' '(c t)A[append]' 'c[create]' 'v[verbose]'
//	                            → 0
//	compvalues -V na ar op      → 0, na=(A:append v:verbose) ar=() op=()
//	compvalues -d d             → 0, d=`tar function`
//	compvalues -D d a           → 1
//	compvalues -S s             → 0, s=`=`
//
// Four things that says:
//
//   - `-i` is `[-s separator] [-S argument-separator] description spec …`,
//     and the specs are the `optspec` language with the leading `-` left off.
//   - **The value under the cursor counts as already given**, exactly as an
//     option does for `comparguments`: `c` is missing from `-V`'s answer and
//     nothing else is. A shell that read it as untyped would offer it back.
//   - `-V` sorts the values into those with no argument and those with one,
//     as `name:description` — the third array is for values whose argument
//     is a whole set of its own, which nothing in the shipped tree fills.
//   - **The argument separator defaults to `=`**, not to the empty string:
//     the probe above passed `-s` and no `-S`, and `-S` still answered `=`.
//
// `-D` answers the message and action of the value whose argument is being
// completed, and 1 where the cursor is not past a separator.

// valuesState is one `compvalues -i`.
type valuesState struct {
	description string
	separator   string
	argSep      string
	values      []optionSpec
	// name is the value the cursor is writing an argument for, empty where
	// it is writing a value name.
	name string

	// given is what the word under the cursor already holds, the piece being
	// written included — see the measurement above.
	given map[string]bool
}

// defaultValuesArgSep is what `-S` answers when `-i` named none. Measured.
const defaultValuesArgSep = "="

func compvaluesBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	cs, st, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if args[0] == "-i" {
		return compvaluesInit(r, cs, st, args[1:])
	}
	v := st.values
	if v == nil {
		r.Diagnosef("no parsed state\n")
		return 1
	}
	return v.query(r, args)
}

func (v *valuesState) query(r *interp.Runner, args []string) int {
	if len(args) < 2 && args[0] != "-V" {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	switch args[0] {
	case "-d":
		r.SetVar(args[1], v.description)
		return 0
	case "-s":
		r.SetVar(args[1], v.separator)
		return 0
	case "-S":
		r.SetVar(args[1], v.argSep)
		return 0
	case "-D":
		if len(args) < 3 {
			r.Diagnosef("not enough arguments\n")
			return 1
		}
		return v.describe(r, args[1], args[2])
	case "-V":
		if len(args) < 4 {
			r.Diagnosef("not enough arguments\n")
			return 1
		}
		return v.offer(r, args[1:4])
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}

// compvaluesInit reads the switches, the description and the value specs, and
// works out whether the cursor is on a value name or on a value's argument.
func compvaluesInit(r *interp.Runner, cs *completionState, st *computilState, args []string) int {
	v := &valuesState{argSep: defaultValuesArgSep}
	i := 0
	for ; i+1 < len(args) && (args[i] == "-s" || args[i] == "-S"); i += 2 {
		if args[i] == "-s" {
			v.separator = args[i+1]
		} else {
			v.argSep = args[i+1]
		}
	}
	if i >= len(args) {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	v.description = args[i]
	for _, spec := range args[i+1:] {
		excl, body, star, hidden := specPrefixes(spec)
		// The value language is the option language with no leading `-`, so
		// one is lent to the parser and taken off again.
		opt, ok := parseOptionSpec("-" + body)
		if !ok {
			r.Diagnosef("invalid argument: %s\n", spec)
			return 1
		}
		for j, name := range opt.names {
			opt.names[j] = strings.TrimPrefix(name, "-")
		}
		opt.excl, opt.repeat, opt.hidden = excl, star, hidden
		v.values = append(v.values, opt)
	}
	if v.argSep != "" {
		if name, _, found := strings.Cut(cs.prefix, v.argSep); found {
			v.name = name
		}
	}
	v.given = v.valuesWritten(cs.prefix)
	st.values = v
	return 0
}

// describe is `-D`: the message and action of the value whose argument the
// cursor is inside.
func (v *valuesState) describe(r *interp.Runner, descr, action string) int {
	for _, value := range v.values {
		if value.names[0] != v.name || len(value.optargs) == 0 {
			continue
		}
		r.SetVar(descr, value.optargs[0].message)
		r.SetVar(action, value.optargs[0].action)
		return 0
	}
	r.SetVar(descr, "")
	r.SetVar(action, "")
	return 1
}

// offer is `-V`: the values still available, split by whether they take an
// argument.
func (v *valuesState) offer(r *interp.Runner, names []string) int {
	var noargs, args []string
	// Answered whatever the cursor is on. Measured with `cmd bb=`, where the
	// cursor is inside `bb`'s argument and `-D` answers for it: `-V` still
	// lists `aa` and `bb`, so it is not a question about the cursor.
	for _, value := range v.values {
		if value.hidden || (v.given[value.names[0]] && !value.repeat) {
			continue
		}
		offer := optionOffer(value.names[0], value.descr)
		if len(value.optargs) > 0 {
			args = append(args, offer)
		} else {
			noargs = append(noargs, offer)
		}
	}
	r.SetArray(names[0], noargs)
	r.SetArray(names[1], args)
	r.SetArray(names[2], nil)
	return 0
}

// valuesWritten is which value names a word already holds.
//
// An empty separator does not mean "one value": it means the values are
// single letters written end to end, which is what `_values -s ” ` is for
// and what `tar cvf` is. So an empty separator splits per character, and any
// other separator splits on itself.
func (v *valuesState) valuesWritten(word string) map[string]bool {
	given := map[string]bool{}
	var pieces []string
	switch {
	case v.separator == "":
		for _, ch := range word {
			pieces = append(pieces, string(ch))
		}
	default:
		pieces = strings.Split(word, v.separator)
	}
	for _, piece := range pieces {
		if v.argSep != "" {
			piece, _, _ = strings.Cut(piece, v.argSep)
		}
		if piece != "" {
			given[piece] = true
		}
	}
	return given
}
