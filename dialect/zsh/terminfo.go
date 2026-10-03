// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"maps"
	"strconv"
	"strings"
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
// names, `$termcap` by termcap's two-letter codes — over the description
// `$TERM` names, in whichever of its two readings the shell is in; see
// terminalsetup.go.
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

// capabilityTables is the terminal description under both name systems, in
// both of its readings, kept for as long as the environment it was read from
// says the same thing.
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
//
// What it does **not** hold is which of the two readings a script sees. That
// is state of the shell's own, it differs between a shell and its subshells,
// and it is kept in terminalSetupStore; see terminalSetup.
type capabilityTables struct {
	// A mutex rather than nothing, because a Runner's producers are shared
	// with the subshells it spawns and two of those can read a parameter at
	// once.
	mu   sync.Mutex
	from string
	// stored is the description as the file holds it, and converted is the
	// reading repl.ConvertedCapabilities gives of it.
	stored, converted *capabilityReading
}

// capabilityReading is one reading of a description: every capability by
// terminfo name, the subset of those names that is *enumerated*, the termcap
// spelling, and which section each came from.
type capabilityReading struct {
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
	// termcapKinds is the same by termcap code, for `echotc`.
	termcapKinds map[string]repl.TerminalCapabilityKind
}

// terminfoEnvironment is every variable the answer depends on, in the order
// repl reads them.
var terminfoEnvironment = []string{"TERM", "TERMINFO", "TERMINFO_DIRS", "HOME"}

// terminalUse is what a caller is about to do with the description, which is
// what decides whether the shell sets its terminal up first. See
// terminalSetup.
type terminalUse uint8

const (
	// listingCapabilities enumerates a parameter, or matches a pattern
	// subscript against it. It reaches the module and sets nothing up.
	listingCapabilities terminalUse = iota
	// readingCapability looks one capability up — an element of either
	// parameter, `echoti` or `echotc`.
	readingCapability
	// drawingAttribute is the prompt writing one of its attribute codes.
	drawingAttribute
)

// terminfoTable is every capability by its terminfo name — the *readable*
// set, extended section included — and which section each came from, for
// `echoti`, whose answer has to be the parameter's.
func (c *capabilityTables) terminfoTable(r *interp.Runner) (
	interp.AssocArray, map[string]repl.TerminalCapabilityKind,
) {
	reading := c.reading(r, readingCapability)
	return reading.terminfo, reading.kinds
}

// listedTable is the enumerated subset: the same names without the
// description's extended section. See registerCapabilityParameter.
func (c *capabilityTables) listedTable(r *interp.Runner) interp.AssocArray {
	return c.reading(r, listingCapabilities).listed
}

// readTable is the readable set alone, which is what one key is looked up in.
func (c *capabilityTables) readTable(r *interp.Runner) interp.AssocArray {
	return c.reading(r, readingCapability).terminfo
}

// termcapEntry is one capability by termcap code, and which section it came
// from, for `echotc`.
func (c *capabilityTables) termcapEntry(r *interp.Runner, code string) (string, repl.TerminalCapabilityKind, bool) {
	code = termcapCode(code)
	reading := c.reading(r, readingCapability)
	v, ok := reading.termcap[code]
	return v.Str, reading.termcapKinds[code], ok
}

// termcapTable is the `$termcap` half for one key, read through the same
// cache.
func (c *capabilityTables) termcapTable(r *interp.Runner) interp.AssocArray {
	return c.reading(r, readingCapability).termcap
}

// termcapListed is the `$termcap` half for an enumeration.
func (c *capabilityTables) termcapListed(r *interp.Runner) interp.AssocArray {
	return c.reading(r, listingCapabilities).termcap
}

// promptTable is the `$termcap` half the prompt's attribute codes are read
// from.
func (c *capabilityTables) promptTable(r *interp.Runner) interp.AssocArray {
	return c.reading(r, drawingAttribute).termcap
}

// reading is the reading in force once use has had its effect on the shell.
func (c *capabilityTables) reading(r *interp.Runner, use terminalUse) *capabilityReading {
	stored, converted := c.readings(r)
	if advanceTerminalSetup(r, use) {
		return converted
	}
	return stored
}

// readings is both readings of the description the environment names.
func (c *capabilityTables) readings(r *interp.Runner) (stored, converted *capabilityReading) {
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
	if c.stored != nil && c.from == key {
		return c.stored, c.converted
	}
	caps := repl.TerminalCapabilities(env)
	if genericDescription(caps) {
		caps = nil
	}
	c.from = key
	c.stored = newCapabilityReading(caps)
	c.converted = newCapabilityReading(repl.ConvertedCapabilities(caps))
	return c.stored, c.converted
}

// genericDescription says a description is marked generic — `gn`, a
// description of a kind of terminal rather than of one — which answers
// exactly as no description at all does.
//
// Measured 2026-10-02 on zsh 5.9.2 over Homebrew's database: `unknown` and
// `ibm327x` are the two descriptions there carrying `gn`, and under either
// `${#terminfo}` is 0, `echoti bel` is silent at 1 and `print -P %B` writes
// no attribute, exactly as under a `$TERM` the database has never heard of;
// `infocmp` prints both descriptions whole. Every other description agrees
// with this reader's enumeration (#5315).
func genericDescription(caps []repl.TerminalCapability) bool {
	for _, c := range caps {
		if c.Terminfo == "gn" && !c.Extended {
			return c.Value == "yes"
		}
	}
	return false
}

// newCapabilityReading files one list of capabilities under both name
// systems.
func newCapabilityReading(caps []repl.TerminalCapability) *capabilityReading {
	kinds := make(map[string]repl.TerminalCapabilityKind, len(caps))
	byTerminfo := make(interp.AssocArray, len(caps))
	listed := make(interp.AssocArray, len(caps))
	byTermcap := make(interp.AssocArray, len(caps))
	termcapKinds := make(map[string]repl.TerminalCapabilityKind, len(caps))
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
			termcapKinds[entry.Termcap] = entry.Kind
		}
	}
	// The termcap `me` is not the terminfo `sgr0` it is filed under, where
	// the two differ; see termcapExitAttributes. In both readings, because a
	// lookup answers it so in both: measured 2026-10-02, `${termcap[me]}` in
	// an interactive shell, which never leaves the stored reading, is the
	// derived `\e[0m` for `xterm-256color` and not its `sgr0`.
	if sgr0, ok := byTerminfo["sgr0"]; ok {
		byTermcap["me"] = interp.Scalar(termcapExitAttributes(sgr0.Str, byTerminfo["sgr"].Str, byTerminfo["rmacs"].Str))
	}
	return &capabilityReading{
		terminfo: byTerminfo, listed: listed, termcap: byTermcap,
		kinds: kinds, termcapKinds: termcapKinds,
	}
}

// registerTerminfoModules installs `$terminfo` and `$termcap`: two views over
// one reading of the terminal's description, each keyed by its own name
// system.
func registerTerminfoModules(r *interp.Runner) {
	tables := &capabilityTables{}
	// And the prompt's attribute codes read the same description, through
	// the termcap names: see PromptStyle's SequenceCapabilities and
	// promptCapability.
	r.SetTerminalCapabilityReader(func(r *interp.Runner, code string) string {
		return promptCapability(tables.promptTable(r), code)
	})
	registerCapabilityParameter(r, "terminfo", "cols", "lines",
		tables.listedTable, tables.readTable)
	// The same two names under termcap's spelling, and there the two readings
	// are one table: an extended capability has no two-letter code, so the
	// section that makes them differ is already absent from this half.
	registerCapabilityParameter(r, "termcap", "co", "li",
		tables.termcapListed, tables.termcapTable)
	registerEchoti(r, tables)
	registerEchotc(r, tables)
	// And `$TERM` itself, whose assignment sets an interactive shell's
	// terminal up again: see terminalsetup.go.
	r.SetAssignmentAction("TERM", terminalAssigned)
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
		rows, cols := screenSizeFor(r, table, colsKey, linesKey)
		out[colsKey] = interp.Scalar(strconv.Itoa(cols))
		out[linesKey] = interp.Scalar(strconv.Itoa(rows))
		return out
	})
	// One key without building the map, which is the shape a capability test
	// has: a theme asks about a name at a time.
	r.SetDynamicAssocElement(name, func(r *interp.Runner, key string) (string, bool) {
		if name == "termcap" {
			key = termcapCode(key)
		}
		table := read(r)
		if len(table) == 0 {
			return "", false
		}
		switch key {
		case colsKey:
			_, cols := screenSizeFor(r, table, colsKey, linesKey)
			return strconv.Itoa(cols), true
		case linesKey:
			rows, _ := screenSizeFor(r, table, colsKey, linesKey)
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

// termcapExitAttributes is the termcap `me` a description's `sgr0` comes to.
//
// Where the description also says how to set every attribute (`sgr`) and how
// to leave the alternate character set (`rmacs`), `me` is `sgr` with every
// attribute off and the character-set half taken out — provided that comes to
// the same reset `sgr0` does once its own character-set half is out.
// Otherwise it is `sgr0` as written. Measured on zsh 5.9.2 by reading
// `${(V)termcap[me]}` beside the terminfo strings, entry by entry:
//
//	          sgr0           rmacs     me
//	xterm     \E(B\E[m       \E(B      \E[0m   sgr off is \E(B\E[0m
//	screen    \E[m^O         ^O        \E[0m   sgr off is \E[0m^O
//	vt100     \E[m^O$<2>     ^O        \E[0m
//	ansi      \E[0;10m       \E[10m    \E[0m   the 10 is the character set
//	linux     \E[m^O         ^O        \E[m^O  sgr off is \E[0;10m^O, which
//	                                            is not the same reset
//	vt220     \E[m\E(B       \E(B$<4>  \E[0m\E(B  the padded exit is in
//	                                            neither, so nothing comes out
//	no sgr    \E[m           (none)    \E[m
//
// The rows are each other's controls: screen and linux have the same `sgr0`
// and `rmacs` and part company on `sgr` alone.
func termcapExitAttributes(sgr0, sgr, rmacs string) string {
	if sgr == "" || rmacs == "" {
		return sgr0
	}
	off := withoutPadding(withoutCharset(tparm(sgr, make([]int, 9)), rmacs))
	plain := withoutPadding(withoutCharset(sgr0, rmacs))
	if sameReset(off, plain) {
		return off
	}
	return sgr0
}

// withoutCharset takes the alternate character set's exit out of a reset: the
// bytes themselves where they appear, or — where the exit is a one-parameter
// SGR, `\E[10m` — that parameter out of the reset's own list.
func withoutCharset(s, rmacs string) string {
	// As written, padding and all: vt220's `rmacs` is `\E(B$<4>`, which its
	// `sgr` does not contain, and its `me` keeps the `\E(B` — measured,
	// `\E[0m\E(B`.
	rm := rmacs
	if strings.Contains(s, rm) {
		return strings.Replace(s, rm, "", 1)
	}
	if p, ok := strings.CutPrefix(rm, "\x1b["); ok {
		if p, ok = strings.CutSuffix(p, "m"); ok && p != "" {
			if body, ok := strings.CutPrefix(s, "\x1b["); ok {
				if body, ok = strings.CutSuffix(body, "m"); ok {
					var keep []string
					for _, part := range strings.Split(body, ";") {
						if part != p {
							keep = append(keep, part)
						}
					}
					return "\x1b[" + strings.Join(keep, ";") + "m"
				}
			}
		}
	}
	return s
}

// sameReset reports whether two SGR strings reset alike: `\E[m` and `\E[0m`
// are one reset.
func sameReset(a, b string) bool {
	norm := func(s string) string { return strings.ReplaceAll(s, "\x1b[m", "\x1b[0m") }
	return norm(a) == norm(b)
}

// promptCapability is what one attribute code writes: the termcap string,
// with its padding taken off the way the terminal library's output routine
// takes it off — measured, vt100's `$termcap[md]` is `\E[1m$<2>` and `%B`
// under that TERM writes `\E[1m`. Empty where the description has no such
// capability, which is what every code writes with no description at all.
func promptCapability(table interp.AssocArray, code string) string {
	if _, up := table["up"]; !up {
		// A terminal the cursor cannot move up on is one zsh draws no
		// attribute on at all: measured, a description holding `bold`,
		// `sgr0` and `smso` writes nothing for `%B|%b|%S` until `cuu1` is
		// added to it, and then writes all three — `cols`, `lines`, `am`,
		// `cup` and `clear` each changed nothing. With no description at all
		// there is no `up` either, which is TERM unset and `dumb`.
		return ""
	}
	v, ok := table[code]
	if !ok {
		return ""
	}
	return withoutPadding(v.Str)
}

// screenSizeFor is the two size capabilities: the terminal's own size, the
// environment's, and — where neither answers — the description's, before the
// classic 80 by 24. Measured 2026-10-02 on zsh 5.9.2 with no terminal under
// `env -i`: `TERM=guru`, whose description says `lines#33`, answers 33 lines
// and 80 columns, `TERM=wy520-36w` answers 36 and 132, `LINES=50
// COLUMNS=99` in the environment wins over both, and `TERM=linux`, whose
// description says neither, answers 24 and 80 (#5315).
func screenSizeFor(r *interp.Runner, table interp.AssocArray, colsKey, linesKey string) (rows, cols int) {
	described := func(key string) int {
		n, err := strconv.Atoi(table[key].Str)
		if err != nil {
			return 0
		}
		return n
	}
	return r.ScreenSizeOr(described(linesKey), described(colsKey))
}

// termcapCode is the code a termcap lookup reads out of the name it is given:
// its first two characters, since a termcap code is two characters long and
// the lookup reads no further.
//
// Measured 2026-10-02 on zsh 5.9.2, `TERM=xterm-256color`: `$termcap[blx]`,
// `$termcap[blxyz]` and `$termcap[bl ]` are all `bl`'s bell, `$termcap[cols]`
// is `co`'s 80, `echotc bla` writes the bell and `echotc cols` writes 80 —
// while `$termcap[b]` and `echotc b` find nothing and `$termcap[ bl]` is not
// `bl` (#5431). Only the lookup reads this way; `${(k)termcap}` lists the
// codes as they are.
func termcapCode(name string) string {
	if len(name) > 2 {
		return name[:2]
	}
	return name
}
