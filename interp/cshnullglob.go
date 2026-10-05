// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// One shell's `cshnullglob`: a pattern matching nothing is deleted from its
// word list, and the list is an error only where none of its patterns
// matched. Measured 2026-10-02 on zsh 5.9.2 under `-fc`, in a directory
// holding `tmpa` and `tmpb`:
//
//	print tmp* nothing* blah                 tmpa tmpb blah
//	print nothing* blah; print after         no match, and the script ends
//	for x in nothing* tmp*; do …             tmpa, tmpb
//	a=(nothing*)                             no match
//	setopt nullglob; print nothing* blah     blah: the emptying option wins
//	print "nothing*" blah                    nothing* blah: no pattern
//
// The sentence is its own, `no match`, with no pattern in it (#5155).

// globUnit is one word list the option is judged over: whether any of its
// patterns matched, and whether any missed.
type globUnit struct {
	matched, missed bool
}

// SetCshNullGlob turns the reading on and off for what this runner expands
// from here on.
func (r *Runner) SetCshNullGlob(on bool) { r.cshNullGlob = on }

// CshNullGlob reports it.
func (r *Runner) CshNullGlob() bool { return r.cshNullGlob }

// beginGlobUnit opens a word list for the option and answers with what
// closes it, refusing the list where a pattern in it missed and none
// matched. Nested lists — a substitution's command inside an argument —
// open their own and put the outer one back.
func (r *Runner) beginGlobUnit() func() {
	if !r.cshNullGlob {
		return func() {}
	}
	outer := r.globUnit
	unit := &globUnit{}
	r.globUnit = unit
	return func() {
		r.globUnit = outer
		if unit.missed && !unit.matched && !r.givingUpAlready() {
			r.diagf("%s\n", Wording(r.diag().CshNullGlobNoMatch, "no match"))
			// Sets its status as an unmatched pattern does — see
			// Runner.refuseUnmatchedPattern.
			restore := r.globRefusalSetsItsStatus()
			r.failedExpansion()
			restore()
		}
	}
}

// SetHistSubstPattern turns one shell's `histsubstpattern` on and off: `:s`
// reads its left half as a pattern. See Runner.substitutePattern.
func (r *Runner) SetHistSubstPattern(on bool) { r.histSubstPattern = on }

// HistSubstPattern reports it.
func (r *Runner) HistSubstPattern() bool { return r.histSubstPattern }

// SetMarkDirs turns one shell's `markdirs` on and off: a directory a pattern
// produces is written with a slash after it.
func (r *Runner) SetMarkDirs(on bool) { r.markDirs = on }

// MarkDirs reports it.
func (r *Runner) MarkDirs() bool { return r.markDirs }

// SetPathDirs turns one shell's `pathdirs` on and off: a command word with a
// slash in it, other than one written from the root, `./` or `../`, is
// looked for down the path when it does not run from the working directory.
// See Runner.searchesTheSlashedName.
func (r *Runner) SetPathDirs(on bool) { r.pathDirs = on }

// PathDirs reports it.
func (r *Runner) PathDirs() bool { return r.pathDirs }

// SetPromptBang turns one shell's `promptbang` on and off. See
// promptBangText.
func (r *Runner) SetPromptBang(on bool) { r.promptBang = on }

// PromptBang reports it.
func (r *Runner) PromptBang() bool { return r.promptBang }

// promptBangText is a prompt under `promptbang`: a `!` is the history
// number, `%!`, and `!!` is a `!`. Measured 2026-10-02 on zsh 5.9.2 under
// `-f` (#5155), where the number is 0:
//
//	print -P '!'           0
//	print -P 'a!!b'        a!b
//	print -P '%%!'         %0
//	print -P '%(!.a.b)'    b — a condition's letter is not one
//	PS4='!> '; set -x      0> true
//	print ${(%):-!}        ! — one flag draws the escapes alone
//	print ${(%%):-!}       0 — two draw the value as a prompt
//
// and with `promptsubst` a `!` a substitution produced is converted too, so
// this runs between the substitution and the escapes.
func promptBangText(text string) string {
	if !strings.ContainsRune(text, '!') {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '%' && i+1 < len(text):
			// An escape is copied through whole as far as its letter, and a
			// conditional's letter with it.
			j := i + 1
			for j < len(text) && (text[j] == '-' || text[j] >= '0' && text[j] <= '9') {
				j++
			}
			if j < len(text) && text[j] == '(' {
				j++
				for j < len(text) && text[j] >= '0' && text[j] <= '9' {
					j++
				}
			}
			if j < len(text) {
				j++
			}
			b.WriteString(text[i:j])
			i = j - 1
		case c == '!' && i+1 < len(text) && text[i+1] == '!':
			b.WriteByte('!')
			i++
		case c == '!':
			b.WriteString("%!")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// SetPromptPercent turns one shell's `promptpercent` on and off: with it off
// a prompt's `%` escapes are text. Measured 2026-10-02 on zsh 5.9.2 under
// `-f` (#5155): `unsetopt promptpercent; print -P '%/ %%'` writes `%/ %%`,
// `${(%%):-%/}` and a `%N` in PS4 stay as written, and `${(%):-%/}` — one
// flag, the escapes alone — still draws the directory. `promptbang` goes on
// working: `print -P '!'` is still the history number.
func (r *Runner) SetPromptPercent(on bool) { r.promptPercentOff = !on }

// PromptPercent reports it.
func (r *Runner) PromptPercent() bool { return !r.promptPercentOff }

// SetShFileExpansion turns one shell's `shfileexpansion` on and off: a
// word's `=cmd` is expanded before its braces and its parameters rather than
// after them. Measured 2026-10-02 on zsh 5.9.2 under `-f`, with `nomatch`
// off so a failed lookup leaves the word (#5155):
//
//	                         off            on
//	print ={ls,}             /bin/ls =      =ls =
//	foo='=ls'; print ${~foo} /bin/ls        =ls
func (r *Runner) SetShFileExpansion(on bool) { r.shFileExpansion = on }

// ShFileExpansion reports it.
func (r *Runner) ShFileExpansion() bool { return r.shFileExpansion }

// SetLocalLoops turns one shell's `localloops` on and off. See
// Runner.endLocalLoops.
func (r *Runner) SetLocalLoops(on bool) { r.localLoops = on }

// LocalLoops reports it.
func (r *Runner) LocalLoops() bool { return r.localLoops }

// endLocalLoops ends a call under `localloops`: the option goes back to what
// the caller had, and a `break` or `continue` still pending when the body
// finished is reported and goes no further. Measured 2026-10-02 on zsh 5.9.2
// under `-f` (#5155):
//
//	setopt localloops; f(){ break }; for i in 1 2; do print $i; f; done
//	    1, `break' active at end of function scope, 2, and the same again
//	f(){ break } and the option off       1, and the loop ends
//	f(){ setopt localloops }; f            off again afterwards
//	setopt localloops; f(){ setopt nolocalloops; break }
//	    the caller's setting decides: both lines and both reports
//	f(){ continue } under the option       the `continue' report, then the
//	                                       `break' one, and the loop goes on
//
// So the option is local to every call as `localoptions` would make it, and
// the value that decides is the caller's.
func (r *Runner) endLocalLoops(caller bool) {
	r.localLoops = caller
	if !caller || (r.ctl != controlBreak && r.ctl != controlContinue) {
		return
	}
	if r.ctl == controlContinue {
		r.diagf("`continue' active at end of function scope\n")
	}
	r.diagf("`break' active at end of function scope\n")
	r.ctl, r.ctlDepth = controlNone, 0
}

// SetPrintExitValue turns one shell's `printexitvalue` on and off. See
// Runner.reportExitValue.
func (r *Runner) SetPrintExitValue(on bool) { r.printExitValue = on }

// PrintExitValue reports it.
func (r *Runner) PrintExitValue() bool { return r.printExitValue }

// reportExitValue writes `exit N` for a builtin or a function call that has
// just failed, under `printexitvalue`. Measured 2026-10-02 on zsh 5.9.2,
// `zsh -f` reading the script from standard input (#5155):
//
//	false                      zsh: exit 1
//	f(){ return 4 }; f         zsh: exit 4, and nothing for the body
//	eval false                 twice — the body's line is top level
//	builtin false              once
//	/usr/bin/false             nothing: an external command is not reported
//	( false ), x=$(false)      nothing: a subshell is not
//	[[ a = b ]], (( 0 ))       nothing: neither is a builtin
//	source ./f                 the file's own lines are not, the call is
//	false 2>/dev/null          nothing: written inside the redirections
//	exit 5                     nothing
//
// and nothing at all from `-c` or a script file.
func (r *Runner) reportExitValue(st int) {
	if !r.printExitValue || st == 0 || r.inSubshell || len(r.frames) != 0 || r.ctl == controlExit {
		return
	}
	if r.Route != RouteStandardInput && !r.Interactive {
		return
	}
	// The shell's name alone, wherever the command was: not the builtin
	// that failed, and not `(eval)` for a line inside an `eval`.
	r.errf("%s: exit %d\n", r.name(), st)
}

// searchesTheSlashedName says whether a command name with a slash in it is
// looked for down the path, under `pathdirs`. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5155), with `sub/x` in a path directory:
//
//	sub/x, no sub/x here               runs the path's copy
//	sub/x, a runnable sub/x here       runs this one; `whence` says sub/x
//	sub/x here and not executable      runs the path's copy
//	sub/x nowhere                      command not found: sub/x
//	./sub/x, ../sub/x, /abs/sub/x      never searched
//
// and `whence`, `type`, `command -v` and `which` name the copy found.
func (r *Runner) searchesTheSlashedName(name string) bool {
	return r.pathDirs && !strings.HasPrefix(name, "/") &&
		!strings.HasPrefix(name, "./") && !strings.HasPrefix(name, "../")
}
