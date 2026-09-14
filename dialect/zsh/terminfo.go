// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"maps"
	"strconv"
	"sync"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The `zsh/terminfo` and `zsh/termcap` modules: the terminal's capabilities,
// presented as two associations a script can read.
//
// The names are all that is here. What a capability *is*, and where the
// answers come from, is repl/terminfo.go, for the reason every terminal fact
// in this tree sits there: it is a statement about the terminal, and a
// dialect that owned it would be a dialect the substrate had to know about.
// This file is the two spellings — `$terminfo` keyed by terminfo's long
// names, `$termcap` by termcap's two-letter codes — over one reading of the
// description `$TERM` names.
//
// # What #1388 fixed, and what #2076 fixed after it
//
// #1388 was the parameters' absence. A plugin manager builds its entire color
// table behind one test:
//
//	if [[ -z $SOURCED && ( ${+terminfo} -eq 1 && -n ${terminfo[colors]} ) \
//	   || ( ${+termcap} -eq 1 && -n ${termcap[Co]} ) ]] { … }
//
// With neither parameter registered the test was false, the table was never
// filled in, and every message the shell printed during startup came out as
// `{error}Error{ehi}:{rst} …` rather than as colored text.
//
// #1388 answered it with a fixed table of thirteen capabilities and a refusal
// — `terminfo[cnorm]: capability not implemented yet` — for every other name.
// #2076 is what that cost. powerlevel10k's `_p9k_init_prompt` guards its
// scroll-and-redraw block on `(( $+terminfo[cuu1] ))`, so a refused `cuu1`
// did not produce an error: it produced a *different prompt*, correct for a
// terminal that cannot move the cursor up, missing the newline and `\e[A`
// that begin zsh's own rendering, with nothing said. A capability test is how
// a theme decides what to build, so refusing one silently changes what gets
// built.
//
// Both parameters now read the terminfo database. `${terminfo[colors]}` is
// still non-empty under any `$TERM` with a color entry, which is #1388's
// test; `$+terminfo[cuu1]` is 1 with the terminal's own bytes behind it,
// which is #2076's.
//
// # Both are readonly, and hidden with it
//
// Measured against zsh 5.9.2: `terminfo[colors]=9` is `read-only variable:
// terminfo`, `unset terminfo` the same, and `typeset -p terminfo` writes
// `typeset -Ar terminfo` — the bare name, with no values. So it is the pair
// `builtins` needs in parameter.go, and for the same two reasons: readonly
// because zsh says so, and hidden because readonly is an attribute, an
// attribute puts the name in the tables a listing walks, and a listing would
// otherwise write out the whole capability table as an assignment somebody
// could source back.
//
// # `echoti` is here and `echotc` is not
//
// Each module names a builtin as well as a parameter — `zmodload -lF
// zsh/terminfo` is `+b:echoti` and `+p:terminfo`, and `zsh/termcap` is
// `+b:echotc` and `+p:termcap`. `echoti` arrived with #2142, because
// powerlevel10k's instant-prompt teardown calls it on every start; see
// echoti.go, which reads the same table this file builds so that a capability
// cannot be present to `${+terminfo[x]}` and absent to `echoti x`.
//
// `echotc` is still missing, and that is the module rule in zmodload.go
// rather than an omission: a missing builtin refuses at its own call site, by
// name, on the line that ran it, so it never holds a module shut. The modules
// load because their *parameters* are here, and a script that calls `echotc`
// finds out where it called it.

// capabilityTables is one reading of the terminal description, under both
// name systems, kept for as long as the environment it was read from says the
// same thing.
//
// A cache rather than a read per expansion, because a produced association is
// produced on every read and a prompt theme asks about capabilities in bulk:
// powerlevel10k's initialization alone tests dozens of names, and each test
// would otherwise be a directory search and a parse of a few kilobytes.
//
// The key is the environment the answer depends on, so a script that exports
// a different `$TERM` — or points `$TERMINFO` at its own database — is
// answered from the new one rather than from the old table. That is the same
// rule SetDynamicAssocWriter exists for one layer up: a view that stops
// tracking is worse than no view, because nothing about it says it stopped.
type capabilityTables struct {
	// A mutex rather than nothing, because a Runner's producers are shared
	// with the subshells it spawns and two of those can read a parameter at
	// once.
	mu       sync.Mutex
	from     string
	terminfo interp.AssocArray
	// listed is `$terminfo` again with the description's *extended* section
	// left out: every name that can be read, minus the ones that are not
	// enumerated. See registerCapabilityParameter for the measurement and for
	// why the two differ at all.
	listed  interp.AssocArray
	termcap interp.AssocArray
	// kinds is which section each terminfo name came from, which the two
	// parameters have no use for and `echoti` cannot do without: a string
	// capability is bytes for the terminal and a number or a boolean is a
	// word for a person. Keyed by the terminfo name alone, because that is
	// the only spelling the builtin takes.
	kinds map[string]repl.TerminalCapabilityKind
}

// terminfoEnvironment is every variable the answer depends on, in the order
// repl reads them.
var terminfoEnvironment = []string{"TERM", "TERMINFO", "TERMINFO_DIRS", "HOME"}

// terminfoTable is every capability by its terminfo name — the *readable*
// set, extended section included — and which section each came from, for
// `echoti`, whose answer has to be the parameter's.
func (c *capabilityTables) terminfoTable(r *interp.Runner) (
	interp.AssocArray, map[string]repl.TerminalCapabilityKind,
) {
	found, _, _, kinds := c.tables(r)
	return found, kinds
}

// listedTable is the enumerated subset: the same names without the
// description's extended section. See registerCapabilityParameter.
func (c *capabilityTables) listedTable(r *interp.Runner) interp.AssocArray {
	_, listed, _, _ := c.tables(r)
	return listed
}

// readTable is the readable set alone, which is what one key is looked up in.
func (c *capabilityTables) readTable(r *interp.Runner) interp.AssocArray {
	found, _, _, _ := c.tables(r)
	return found
}

// termcapTable is the `$termcap` half, read through the same cache.
func (c *capabilityTables) termcapTable(r *interp.Runner) interp.AssocArray {
	_, _, byTermcap, _ := c.tables(r)
	return byTermcap
}

// tables is the whole of one reading: every capability by terminfo name, the
// subset of those names that is *enumerated*, the termcap spelling, and which
// section each came from.
func (c *capabilityTables) tables(r *interp.Runner) (
	interp.AssocArray, interp.AssocArray, interp.AssocArray,
	map[string]repl.TerminalCapabilityKind,
) {
	env := func(name string) string {
		value, _ := r.GetVar(name)
		return value
	}
	key := ""
	for _, name := range terminfoEnvironment {
		// A separator that cannot appear in a variable's value, so that two
		// different environments cannot spell one key.
		key += env(name) + "\x00"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminfo != nil && c.from == key {
		return c.terminfo, c.listed, c.termcap, c.kinds
	}
	caps := repl.TerminalCapabilities(env)
	kinds := make(map[string]repl.TerminalCapabilityKind, len(caps))
	byTerminfo := make(interp.AssocArray, len(caps))
	listed := make(interp.AssocArray, len(caps))
	byTermcap := make(interp.AssocArray, len(caps))
	for _, entry := range caps {
		byTerminfo[entry.Terminfo] = interp.Scalar(entry.Value)
		kinds[entry.Terminfo] = entry.Kind
		if !entry.Extended {
			listed[entry.Terminfo] = interp.Scalar(entry.Value)
		}
		// Skipped rather than keyed by the empty string: an extended
		// capability is a name the description carries itself and predates no
		// termcap, so it has no two-letter code to be found under.
		//
		// First writer wins, which is measured rather than arbitrary. Three
		// codes are claimed twice by terminfo(5)'s own table — `MT` by the
		// boolean `OTMT` and the string `smgtb`, `ma` by the number and the
		// string of that name, `ML` by `smgl` and `smglr` — and zsh answers
		// `$termcap[MT]` with the boolean, which is the one its search
		// reaches first because booleans come before strings.
		if _, taken := byTermcap[entry.Termcap]; entry.Termcap != "" && !taken {
			byTermcap[entry.Termcap] = interp.Scalar(entry.Value)
		}
	}
	c.from, c.terminfo, c.listed, c.termcap, c.kinds = key, byTerminfo, listed, byTermcap, kinds
	return byTerminfo, listed, byTermcap, kinds
}

// registerTerminfoModules installs `$terminfo` and `$termcap`: two views over
// one reading of the terminal's description, each keyed by its own name
// system.
func registerTerminfoModules(r *interp.Runner) {
	tables := &capabilityTables{}
	registerCapabilityParameter(r, "terminfo", "cols", "lines",
		tables.listedTable, tables.readTable)
	// The same two names under termcap's spelling, and there the two readings
	// are one table: an extended capability has no two-letter code, so the
	// section that makes them differ is already absent from this half.
	registerCapabilityParameter(r, "termcap", "co", "li",
		tables.termcapTable, tables.termcapTable)
	registerEchoti(r, tables)
}

// registerCapabilityParameter installs one of them, with the two things a
// produced association over a terminal's description needs: the producer, and
// the readonly-and-hidden pair.
//
// One function for both because the two parameters differ in exactly one
// thing — which column of the capability table is the key — and writing them
// out twice is how the second one comes to be missing whatever the first one
// gains.
//
// There is no SetAbsentElements call here and there was, until #2076. A key
// the table has no answer for is now genuinely a capability this terminal
// does not have, which is what real zsh reports and what a script testing
// `$+terminfo[…]` is written against; refusing it was right only while the
// table was a stub.
//
// # Two tables, because the parameter reads more than it lists
//
// listed and read are the same map for `$termcap` and two maps for
// `$terminfo`, and the split is measured: zsh answers `${terminfo[Se]}` with
// the cursor sequence and `${+terminfo[Se]}` with 1 while leaving `Se` out of
// `${(k)terminfo}` — `${#terminfo}` is 220 for `xterm-256color` there against
// 281 for every name this reader finds, and the 61 are exactly the
// description's extended section.
//
// That is the escape hatch [interp.Runner.SetDynamicAssocElement]'s contract
// now names, and it is one-way: a produced association may **read more than
// it lists, never less**. The direction matters because what the contract
// protects is `${m[k]:-d}` and `${+m[k]}`, and both are answered by the
// element reading — so a key that reads and is not listed leaves every
// branch a script takes correct, where a key that lists and does not read
// would make `${m[k]:-d}` take the default for a name the shell had just
// enumerated (#2102).
//
// # The two size capabilities are the screen's, not the description's
//
// `cols` and `lines` — `co` and `li` in termcap's spelling — are answered
// from [interp.Runner.ScreenSize] in both readings and whatever the
// description holds. Measured through a pseudo-terminal opened 100x37: zsh
// answers `cols=100` under `TERM=xterm-256color`, whose description says 80,
// and answers 80 by 24 for `TERM=linux`, whose description carries neither
// (#2101). They are answered ahead of the table rather than folded into it so
// that a *one-key* read costs one ioctl instead of a copy of the whole map —
// the same reason the element reading exists at all.
func registerCapabilityParameter(
	r *interp.Runner, name, colsKey, linesKey string,
	listed, read func(*interp.Runner) interp.AssocArray,
) {
	r.SetDynamicAssoc(name, func(r *interp.Runner) interp.AssocArray {
		table := listed(r)
		if len(table) == 0 {
			// No description, so no capabilities — the size included.
			// Measured: under a `$TERM` the database has never heard of,
			// `${#terminfo}` is 0 and `${+terminfo[cols]}` is 0, where
			// `${+terminfo}` is still 1. So the two size keys belong to a
			// description that was found and not to the parameter.
			return nil
		}
		out := maps.Clone(table)
		rows, cols := r.ScreenSize()
		out[colsKey] = interp.Scalar(strconv.Itoa(cols))
		out[linesKey] = interp.Scalar(strconv.Itoa(rows))
		return out
	})
	// One key without building the map, which is the shape a capability test
	// has: a theme asks about a name at a time.
	r.SetDynamicAssocElement(name, func(r *interp.Runner, key string) (string, bool) {
		table := read(r)
		if len(table) == 0 {
			return "", false
		}
		switch key {
		case colsKey:
			_, cols := r.ScreenSize()
			return strconv.Itoa(cols), true
		case linesKey:
			rows, _ := r.ScreenSize()
			return strconv.Itoa(rows), true
		}
		value, ok := table[key]
		return value.Str, ok
	})
	// Readonly rather than given a writer, which is zsh's own answer and the
	// same call `builtins` makes in parameter.go. A produced association with
	// neither would take an assignment into a stored table, and a stored
	// table is what a later read finds first — so one `terminfo[colors]=9`
	// would turn the view into a snapshot that never says it stopped
	// tracking.
	r.MarkReadonly(name)
	hideModuleParameter(r, name)
}
