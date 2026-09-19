// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"sort"
	"strings"
)

// helpBlock is what `--help` writes, built from this dialect's own three
// tables rather than committed as text.
//
// **Generated because a committed block is a claim about a roster it cannot
// see.** The names are `zshOptions`, the aliases are `zshOptionAliases` and
// the letters are `setLetterOptions`; a name added to any of them — which this
// campaign does most weeks — moves this block on the commit that adds it, and
// nothing has to remember. That is the same rule `docs/spec/` follows and the
// same one the Unicode tables are generated under.
//
// **And the alternative was measured, not argued.** The reference shell's own
// `--help` misstates four of its own rows, which is what a paste would have
// committed. Measured 2026-09-19 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with `-f`, each probe read back with `[[ -o name ]]` and against a
// control run with no option at all:
//
//	written           its own help says     what actually moved
//	--histappend      --appendcreate        appendhistory; appendcreate stays off
//	--no-histexpand   --badpattern          banghist; badpattern stays on
//	--physical        --cdsilent            chaselinks; cdsilent stays off
//	-C                --no-checkjobs        clobber off; checkjobs stays on
//	-K                --no-badpattern       banghist off; badpattern stays on
//
// Every one of those five agrees with the table in setopt.go, which was
// measured the same way when it was written. So the block a paste would have
// carried is wrong about the shell it came from, and this repository's whole
// method is that a measurement beats a document.
//
// **The three lines that are not generated are the front end's, not a
// roster.** `-c`, `-o` and `+o` are how an invocation is spelled rather than
// names in a table, and each of them is verified by
// TestEverySpellingTheHelpBlockAdvertisesWorks rather than asserted here —
// which is how `-b` stayed out of the list: the reference takes it as "end of
// option processing" and this front end refuses it outright (#3754), so
// advertising it would have been the same failure the paste was rejected for.
func helpBlock() string {
	var b strings.Builder
	// The shell's own name, which Wording fills in where the block is
	// written. The reference spells the whole word it was invoked by here.
	b.WriteString("Usage: %[1]s [<options>] [<argument> ...]\n")
	b.WriteString("\nSpecial options:\n")
	b.WriteString("  --help          show this message, then exit\n")
	b.WriteString("  --version       show the version, then exit\n")
	b.WriteString("  --emulate MODE  start under MODE's semantics, before anything is read\n")
	b.WriteString("  -c COMMAND      take the first argument as a command to run\n")
	b.WriteString("  -o OPTION       turn an option on by name (listed below)\n")
	b.WriteString("  +o OPTION       turn one off\n")
	b.WriteString("  -b              stop reading options: every later word is an operand\n")
	b.WriteString("\nEvery option below is a name, and each is written four ways:\n" +
		"`--NAME' and `-o NAME' turn it on, `--no-NAME' and `+o NAME' turn it\n" +
		"off. A letter's row may name a negative spelling — `--noglob' is `glob'\n" +
		"turned off, as `--no-glob' is — and the positive name turns it back on.\n")

	b.WriteString("\nNamed options:\n")
	names := make([]string, 0, len(zshOptions))
	for i := range zshOptions {
		names = append(names, zshOptions[i].base)
	}
	sort.Strings(names)
	for _, n := range names {
		b.WriteString("  --" + n + "\n")
	}

	b.WriteString("\nOption aliases:\n")
	aliases := make([]string, 0, len(zshOptionAliases))
	for a := range zshOptionAliases {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	for _, a := range aliases {
		b.WriteString(equivalence("  --"+a, aliasColumn, zshOptionAliases[a].base, zshOptionAliases[a].inv))
	}

	b.WriteString("\nOption letters:\n")
	letters := make([]rune, 0, len(setLetterOptions))
	for l := range setLetterOptions {
		letters = append(letters, l)
	}
	sort.Slice(letters, func(i, j int) bool { return letters[i] < letters[j] })
	for _, l := range letters {
		name := setLetterOptions[l]
		if name == "" {
			// The one letter that is taken and moves nothing — see
			// setLetterOptions, where the measurement is. Said rather than
			// dropped: a letter missing from the list reads as a letter the
			// shell refuses, and this one is accepted.
			b.WriteString(padTo("  -"+string(l), letterColumn) + "taken, and moves nothing\n")
			continue
		}
		b.WriteString(equivalence("  -"+string(l), letterColumn, name, false))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// The two columns the equivalence rows line up on. An alias is a whole name
// and a letter is one character, so one width for both would leave one of the
// sections looking like a gap.
const (
	aliasColumn  = 27
	letterColumn = 8
)

// equivalence is one `X   equivalent to --NAME' row, padded to the section's
// column, and inverted where the alias table says the name is reached by
// turning its target off.
func equivalence(written string, column int, target string, inv bool) string {
	name := "--" + target
	if inv {
		name = "--no-" + target
	}
	return padTo(written, column) + "equivalent to " + name + "\n"
}

// padTo brings a row out to a column, leaving a longer one alone with a single
// space after it — so a name past the width pushes its own column rather than
// running into the words.
func padTo(row string, column int) string {
	if len(row) >= column {
		return row + " "
	}
	return row + strings.Repeat(" ", column-len(row))
}
