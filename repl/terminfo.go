// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// What this shell can say about the terminal's capabilities, under the
// terminfo and termcap names a script asks for them by.
//
// A **capability** is one thing a terminal can be told to do or one key it
// sends, named by a short word and answered with the bytes that do it: `ed`
// erases to the end of the screen, `kcuu1` is what the Up arrow sends, `cuu1`
// moves the cursor up one line. Two name systems are in use for the same set
// of facts — terminfo's long names and termcap's two-letter codes — so every
// capability here carries both.
//
// It is here rather than in a dialect package for the reason cellwidth.go is:
// this is a fact about the terminal, not about which shell is speaking. A
// dialect owns the *name* a script reads it through — one shell calls it
// `$terminfo` — and nothing else about it.
//
// # The answers come from the database, which is what #2076 changed
//
// This used to answer a fixed table of thirteen capabilities and refuse every
// other name, `$TERM` never consulted. The argument for it was that the table
// described *this shell* — the line editor writes `\e[J` whatever `$TERM`
// says — and each of the thirteen was a sequence this program already speaks
// and that every description measured spelled identically.
//
// It was wrong about how the names get used, and powerlevel10k is the case
// that shows why. A theme does not merely read a capability; it **tests
// whether the capability is there** and builds something different when it is
// not. `_p9k_init_prompt` guards its scroll-and-redraw block on `(( $+terminfo
// [cuu1] ))`, so with `cuu1` refused the guard was false, the block never
// entered the prompt, and what got installed was a prompt correct for a
// terminal that cannot move the cursor up. Nothing failed. The first two
// characters of zsh's rendering — a newline and `\e[A` — were simply not
// there, and the shell had reported no error to say so.
//
// That is the shape of the whole problem, not one missing key: a refusal is a
// silent substitution at the place a theme decides what to draw. Answering
// only `cuu1` would move the same failure to whichever capability the next
// theme asks about, so the fix is to answer from the description `$TERM`
// names — every capability in it, under both name systems, with the bytes the
// file holds. terminfodb.go reads it and terminfonames.go says which slot is
// which.
//
// # A `$TERM` with no description answers nothing, and says so by absence
//
// Measured against zsh 5.9.2 on 2026-09-11: `${+terminfo}` is 1 under every
// `$TERM` including one the database has never heard of, and `${#terminfo}`
// is 220 for `xterm-256color`, 136 for `screen`, 50 for `dumb` and **0** for
// a name with no entry. So the parameter always exists and the table is what
// varies, which is exactly what a script testing `$+terminfo[cuu1]` is
// written against.
//
// This is why nothing here substitutes a plausible value for a capability it
// cannot find. A shell that answered `cuu1` with the xterm spelling under
// `TERM=screen` would be handing a theme the wrong bytes with the theme's own
// have-I-got-it test satisfied, which is worse than the absence: the absence
// at least takes the branch written for it.

// # What still differs from real zsh, measured
//
// Swept over 250 of the 2,684 descriptions in /usr/share/terminfo, comparing
// this reader's answers against zsh 5.9.2's `$terminfo` key by key and byte
// by byte, 2026-09-11. Every value agreed except for three things, and all
// three are recorded rather than emulated:
//
//   - **`rs2` and `is3`.** zsh answers absent for these two on descriptions
//     that carry them — `screen`, `vt100`, `sun`, `aixterm`, `putty` and
//     about a fifth of the database — while `infocmp` prints the value and
//     this reader returns it. Reproduced with descriptions compiled for the
//     purpose: `rs2` is answered when `rs1` or `rs3` is also present and not
//     otherwise, which is a rule with no explanation behind it. Emulating it
//     would be encoding an unexplained artifact of one machine's curses
//     library; reporting what the description holds is the answer `infocmp`
//     gives and the one a script asking for a reset string wants.
//   - **`cols` and `lines`.** Where a description carries neither — `dumb`,
//     `linux`, `cygwin`, `putty` — zsh answers 80 and 24 anyway, and answers
//     `$COLUMNS` and `$LINES` when those are set. That is the *screen size*
//     rather than the description, a separate thing this shell tracks
//     elsewhere, and wiring the two together is its own change.
//   - **Which capabilities are enumerated.** `${#terminfo}` is 220 for
//     `xterm-256color` in zsh and 281 here, and the difference is entirely
//     the extended section: zsh answers `${terminfo[Se]}` with the cursor
//     sequence and `${+terminfo[Se]}` with 1 while leaving `Se` out of
//     `${(k)terminfo}`. Listing what can be read is the consistency
//     interp.Runner.SetDynamicAssocElement requires of a produced
//     association, so the extended names are in both readings here.
//
// TerminalCapabilityKind is which of the description's three sections a
// capability came from.
//
// Carried because the sections are not interchangeable to a caller that has to
// *write* a capability out: a string capability is bytes meant for the
// terminal and goes out as they stand, where a number and a boolean are
// answers meant for a person and are written as a word with a newline after
// it. Both readings of the parameter answer with a string, so a caller given
// only the value would have to guess from its shape — and `yes` is a plausible
// value for either.
type TerminalCapabilityKind uint8

const (
	// BooleanCapability is a flag: `yes` or `no`, and every boolean name
	// answers whether the description stores it or not.
	BooleanCapability TerminalCapabilityKind = iota
	// NumericCapability is a count, written in decimal.
	NumericCapability
	// StringCapability is the bytes the terminal is sent.
	StringCapability
)

// TerminalCapability is one capability, under both name systems.
type TerminalCapability struct {
	// Terminfo is the terminfo capability name — the long one.
	Terminfo string

	// Termcap is the termcap capability name — the two-letter code. Empty for
	// an extended capability, which is a name the description carries itself
	// and predates no termcap.
	Termcap string

	// Value is the bytes for a string capability, a decimal count for a
	// numeric one, and `yes` or `no` for a boolean.
	//
	// A string capability whose sequence takes an argument keeps terminfo's
	// own parameter language — `\e[%p1%dA` and not `\e[A` — because that is
	// what the file holds and what the parameter hands a script in the shell
	// being modeled.
	Value string

	// Kind is which section it came from, which is what says whether Value is
	// bytes for the terminal or a word for a person.
	Kind TerminalCapabilityKind
}

// TerminalCapabilities is every capability in the description `$TERM` names,
// or nothing when there is no such description.
//
// env reads one variable of the shell's environment and is how `$TERM`,
// `$TERMINFO`, `$TERMINFO_DIRS` and `$HOME` are reached. A parameter rather
// than this package reading the process's environment, for the reason
// AGENTS.md gives about os.Getenv in library code and for one more: a test
// that read the developer's real `$TERM` would pass on the machine that wrote
// it and answer differently on a runner.
//
// The order is the description's own: every boolean name, then the numeric
// capabilities it carries, then the string ones, then whatever its extended
// section adds. A caller wanting them by name builds the map, which is what
// both of this shell's two spellings of the parameter do.
func TerminalCapabilities(env func(string) string) []TerminalCapability {
	if env == nil {
		return nil
	}
	term := env("TERM")
	if !terminalNameIsOneComponent(term) {
		return nil
	}
	for _, dir := range terminfoDirectories(env) {
		for _, path := range terminfoEntryPaths(dir, term) {
			data, err := readTerminalDescription(path)
			if err != nil {
				continue
			}
			caps, err := parseTerminalDescription(data)
			if err != nil {
				continue
			}
			return caps
		}
	}
	return nil
}
