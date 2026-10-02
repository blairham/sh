// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// refuseExecWithoutACommand refuses an `exec` whose option words cannot be
// read, in the dialect that reads them ahead of the line — see
// Semantics.ExecOptionsRequireACommand — and reports whether it did.
//
// Asked before the line's redirections are made, because the refusal comes
// first there: `exec -c >f` and `exec -z ls >f` make no `f`. A line with no
// words after `exec` is the redirection form and is not this.
func (r *Runner) refuseExecWithoutACommand(argv []string) bool {
	if len(argv) < 2 || argv[0] != "exec" || r.dashPrecommand ||
		r.sem().ExecOptionsRequireACommand != Yes {
		return false
	}
	if _, ok := r.lookupBuiltin(argv[0]); !ok || !r.commandRunsInThisShell(argv) {
		return false
	}
	var sentence string
	switch refusal, letter := r.execOptionRefusal(argv[1:]); refusal {
	case execOptionsRead:
		return false
	case execNoCommand:
		sentence = Wording(r.diag().ExecRequiresACommand, "exec requires a command to execute")
	case execNoName:
		sentence = Wording(r.diag().ExecFlagRequiresAParameter,
			"exec flag -%c requires a parameter", letter)
	case execUnknownFlag:
		sentence = Wording(r.diag().ExecUnknownFlag, "unknown exec flag -%c", letter)
	}
	r.diagf("%s\n", sentence)
	r.status = 1
	r.fatalQuiet()
	return true
}

// execOptionReading is what the scan of `exec`'s option words found.
type execOptionReading uint8

const (
	// execOptionsRead is a command behind the options, or no options at all.
	execOptionsRead execOptionReading = iota
	// execNoCommand is an option word with nothing after it.
	execNoCommand
	// execNoName is an `-a` whose name is the last word.
	execNoName
	// execUnknownFlag is a letter this `exec` has not got.
	execUnknownFlag
)

// execOptionRefusal reads words — what follows `exec` — a word at a time, the
// way the reference does. Measured 2026-10-02 on zsh 5.9.2 under `-f`:
//
//	exec -c -z        exec requires a command to execute
//	exec -z -c        unknown exec flag -z
//	exec -a x -z      exec requires a command to execute
//	exec -a x -z ls   unknown exec flag -z
//	exec -a x         exec flag -a requires a parameter
//	exec -az x        command not found: x
//
// So a word that is the last one is refused for having no command before its
// letters are read, and a letter is refused before any later word is looked
// at. `a` takes the rest of its own word, or else the next word, which must
// not be the last.
func (r *Runner) execOptionRefusal(words []string) (execOptionReading, byte) {
	for i := 0; i < len(words); {
		w := globUnescape(words[i])
		if !strings.HasPrefix(w, "-") || w == "-" {
			return execOptionsRead, 0
		}
		if i == len(words)-1 {
			return execNoCommand, 0
		}
		if w == "--" {
			return execOptionsRead, 0
		}
		next := i + 1
		for j := 1; j < len(w); j++ {
			switch c := w[j]; {
			case c == 'a' && j+1 < len(w):
				j = len(w)
			case c == 'a':
				if next+1 >= len(words) {
					return execNoName, 'a'
				}
				next++
			case c == 'l' && r.sem().ExecTakesTheLoginLetter == Yes,
				c == 'c' && r.sem().ExecTakesTheEmptyEnvironmentLetter == Yes:
			default:
				return execUnknownFlag, c
			}
		}
		i = next
	}
	return execOptionsRead, 0
}
