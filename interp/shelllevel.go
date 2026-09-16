// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// ShellLevelPolicy is whether this shell counts how deep it is, and what it
// does with a count that has run away.
//
// `$SHLVL` is the depth of shells a process is standing in, and it is the one
// parameter here whose value is only half about this shell: it is read out of
// the environment, incremented, and **exported**, so the number every later
// child reads is the one this shell wrote. A shell that does not keep it does
// not merely lack a name — every shell started underneath it counts from one
// again, and a prompt that marks a nested shell, a `.profile` guard that only
// runs at the top level, and a script that refuses to recurse all take the
// wrong branch at status 0 (#3097).
//
// Measured 2026-09-16, `env -i PATH=… LC_ALL=C <shell> -c 'echo "[${SHLVL-NONE}]"'`
// and `env | grep '^SHLVL'` in the same shell:
//
//	bash 5.3.20, bash-as-sh, bash 3.2.57   1, exported
//	zsh 5.9.2                              1, exported
//	ksh93u+ 2012-08-01                     1, exported
//	BusyBox ash 1.37.0 (alpine)            1, exported
//	dash 0.5.12                            unset, and nothing exported
//
// Six columns of seven, and dash is the one that has no such parameter at
// all. BusyBox ash is the column the issue's own table did not have, and it
// is on the side that counts — so `cmd/ash` owes the parameter and only
// `cmd/dash` does not.
//
// **The increment is per shell process and not per subshell.** Measured on
// bash, zsh, ksh93 and ash in the same run: a `( )` subshell, a command
// substitution, a brace group and a pipeline element all read the level the
// shell itself has. Only starting another shell adds one, which is what makes
// the number the depth of *processes that are shells* rather than a nesting
// count of anything else.
//
// **What a value the shell cannot use does.** The whole panel treats an
// absent, empty or plainly non-numeric value as zero and starts at 1. Past
// that they part company, and the split is what this axis carries:
//
//	inherited    bash 5.3   bash 3.2   zsh    ksh93   ash
//	(unset)      1          1          1      1       1
//	""           1          1          1      1       1
//	abc          1          1          1      1       1
//	0            1          1          1      1       1
//	3            4          4          4      4       4
//	-1           0          0          0      0       0
//	-5           0          0          -4     -4      4294967292
//	998          999        999        999    999     999
//	999          warning, 1 (empty)     1000   1000    1000
//	1000         warning, 1 warning, 1  1001   1001    1001
//
// bash alone has a ceiling: a new level of 1000 or more is refused with
// `warning: shell level (N) too high, resetting to 1` on standard error and
// the count starts again at 1, and a level that would go negative is floored
// at 0 first. The other three keep counting whatever they read. (bash 3.2
// warns one step later and writes an *empty* `SHLVL` at exactly 1000, which
// is a defect of its own and not a rule; the bash dialect here answers as
// bash 5.3, which is the version it claims to be.)
//
// **Deliberately not modeled**, because each of the four columns reads
// rubbish through a different C library call rather than through a rule:
// `2x` is 3 in zsh and BusyBox ash — as much of the front as is a number —
// and 1 in bash and ksh93; `0x10` is 17 in zsh and ksh93, 1 in bash and
// BusyBox ash; a leading blank is skipped by bash, zsh and ash and refuses
// the whole value in ksh93; and an enormous value wraps at 64 bits in zsh, at
// 32 in BusyBox ash, and is refused by bash and ksh93. bash's reading is the
// one this package takes for every dialect that counts — the entire value,
// blanks allowed on either end, sign and decimal digits and nothing else —
// because it is the only one of the four that is a rule, and no shell's own
// startup ever writes a value the four disagree about.
type ShellLevelPolicy int

const (
	// ShellLevelUnspecified is no answer, and counts nothing — which is also
	// what a Runner with no vector at all does, so an embedder gets a shell
	// that neither invents the parameter nor touches one it was handed.
	ShellLevelUnspecified ShellLevelPolicy = iota
	// ShellLevelNotCounted is dash: no such parameter, nothing exported, and
	// an inherited `SHLVL` left exactly as it arrived.
	ShellLevelNotCounted
	// ShellLevelCounted is zsh, ksh93 and BusyBox ash: one more than whatever
	// was inherited, with no floor and no ceiling.
	ShellLevelCounted
	// ShellLevelCountedToACeiling is bash: the same count, floored at 0, and
	// refused with a warning at ShellLevelCeiling — see ShellLevelPolicy for
	// the measurement.
	ShellLevelCountedToACeiling
)

func (p ShellLevelPolicy) String() string {
	switch p {
	case ShellLevelNotCounted:
		return "ShellLevelNotCounted"
	case ShellLevelCounted:
		return "ShellLevelCounted"
	case ShellLevelCountedToACeiling:
		return "ShellLevelCountedToACeiling"
	}
	return "ShellLevelUnspecified"
}

// ShellLevelName is the parameter the count is kept in. One name across the
// whole panel, so it is a constant rather than a field a dialect fills in.
const ShellLevelName = "SHLVL"

// ShellLevelCeiling is the level bash refuses. Measured 2026-09-16 on bash
// 5.3.20: an inherited 998 gives 999 and is quiet, and an inherited 999 gives
// `warning: shell level (1000) too high, resetting to 1`. So the test is on
// the *new* level and it is "at or above", not "above".
const ShellLevelCeiling = 1000

// settleShellLevel reads the inherited depth, adds this shell to it, and
// exports the result.
//
// Once per session, like settleInheritedOldpwd and for the same reason: a
// front end reading a person's input hands over a chunk a line, and a count
// that were re-decided per chunk would climb a level for every command typed.
// The flag is separate from the presence of the name because one policy
// writes no name at all, and "nothing there" would ask again forever.
func (r *Runner) settleShellLevel() {
	if r.shellLevelSettled {
		return
	}
	r.shellLevelSettled = true
	policy := r.sem().ShellLevel
	if policy == ShellLevelUnspecified || policy == ShellLevelNotCounted {
		return
	}
	if _, ok := r.Vars[ShellLevelName]; ok || r.removed[ShellLevelName] {
		// A caller set one, or a script already has. Either is this shell's
		// own answer and the environment has nothing to add to it.
		return
	}
	inherited, _ := r.inheritedValue(ShellLevelName)
	level := readShellLevel(inherited) + 1
	if policy == ShellLevelCountedToACeiling {
		if level < 0 {
			level = 0
		}
		if level >= ShellLevelCeiling {
			// bash's wording, and the one diagnostic in this package that
			// carries no location. Measured 2026-09-16 both ways round: a
			// `-c` and a script file write the same line, and it names the
			// *shell* as it was invoked rather than the script — the count
			// is settled before the first line of either is read, so there
			// is no location to write. Through errf rather than diagf for
			// exactly that reason: diagf would put `line 1:` in it.
			r.errf("%s: warning: shell level (%d) too high, resetting to 1\n",
				r.invokedAs(), level)
			level = 1
		}
	}
	r.setVarQuietly(ShellLevelName, strconv.Itoa(level))
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	// The half that is not about this shell at all. Without it the count is a
	// number this shell can print and no child is ever told, which is the
	// state every shell started underneath it reads as depth one.
	r.exported[ShellLevelName] = true
}

// readShellLevel is the depth an inherited value names, and 0 for one that
// names none.
//
// bash's reading, taken for every dialect that counts — see ShellLevelPolicy
// for the three others and why they are not modeled. Blanks on either end
// are allowed and nothing else is: the rest of the value must be an optional
// sign and decimal digits, and a value that overflows an int names no depth.
func readShellLevel(value string) int {
	trimmed := strings.Trim(value, " \t\n\v\f\r")
	if trimmed == "" {
		return 0
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return n
}
