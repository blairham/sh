// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"
)

// `umask` reads and sets the mask taken away from the permissions of every
// file this shell and its children create.
//
// It was one of the nine names reserved in #117, and the reason that change
// was worth making on its own: macOS ships /usr/bin/umask, so before it
//
//	umask 077; touch t
//
// exited 0, printed a plausible 0022 from the next `umask`, and left the file
// world-readable — the mask having been set in a child that then exited.
//
// The mask itself lives in the process, not in this package, so the work is
// done by a hook the caller supplies. See Runner.SetUmask.

func init() {
	builtins["umask"] = biUmask
}

func biUmask(r *Runner, _ context.Context, args []string) int {
	symbolic := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		switch args[0] {
		case "--":
			args = args[1:]
		case "-S":
			symbolic, args = true, args[1:]
			continue
		default:
			d := r.diag()
			// Named the way every other builtin's bad option is named, which
			// is the dialect's rule and not this builtin's: `umask --version`
			// is `--` in bash and `-v` in zsh, exactly as `export --version`
			// is. Spelling the whole word here made this the one builtin
			// that answered `umask: --version: invalid option`.
			_, name := r.badOption(args[0], "S")
			r.diagf("%s\n", Wording(d.UmaskBadOption,
				"umask: %[1]s: invalid option", name))
			if d.UmaskUsage != "" {
				if d.UmaskUsageUnprefixed {
					r.errf("%s\n", d.UmaskUsage)
				} else {
					r.diagf("%s\n", d.UmaskUsage)
				}
			}
			return orDefault(d.UmaskBadOptionStatus, 2)
		}
		break
	}
	if r.SetUmask == nil {
		// Refused rather than answered from somewhere else. The whole point
		// of #117 is that a mask this shell cannot change must not look as
		// though it changed.
		r.diagf("umask: this shell was not given a umask to read or change\n")
		return 2
	}
	if len(args) == 0 {
		return r.reportUmask(symbolic)
	}
	mask, code := r.readMask(args[0])
	if code != 0 {
		return code
	}
	if _, err := r.SetUmask(mask); err != nil {
		r.diagf("umask: %v\n", err)
		return 1
	}
	// bash alone echoes the new mask when it was asked for the symbolic form
	// and given one to set. Setting without `-S` is silent in all four.
	if symbolic && r.ask(r.sem().UmaskSetWithSPrints, "`umask -S mask` echoing the mask it set") {
		r.printf("%s\n", symbolicUmask(mask))
	}
	return 0
}

// reportUmask prints the mask without changing it — which takes a set and a
// set-back, the system call offering no way to ask.
func (r *Runner) reportUmask(symbolic bool) int {
	old, err := r.currentUmask()
	if err != nil {
		r.diagf("umask: %v\n", err)
		return 1
	}
	if symbolic {
		r.printf("%s\n", symbolicUmask(old))
		return 0
	}
	// Four digits in three of the four. zsh writes a C octal literal with a
	// minimum of three digits instead, which is not "three digits": it is
	// `022` against bash's `0022`, and `0333` against bash's `0333`. The
	// leading zero comes back the moment the owner group denies anything,
	// and printing a flat three gave `333` there.
	if r.ask(r.sem().UmaskPrintsFourDigits, "`umask` printing a leading zero") {
		r.printf("%04o\n", old)
	} else {
		r.printf("%#03o\n", old)
	}
	return 0
}

// readMask reads either spelling of a mask, and reports the dialect's
// complaint about the one it could not.
//
// Which spelling is decided by the first character, not by trying one and
// falling back to the other: a leading digit is octal and anything else is
// symbolic. Measured, and the two are told apart by the *complaint* — bash
// answers `umask 1x` with "octal number out of range" and `umask -1` with
// "invalid symbolic mode character", so `1x` was never a candidate for the
// symbolic reading and `-1` was never a candidate for the octal one.
func (r *Runner) readMask(arg string) (mask, code int) {
	d := r.diag()
	if arg != "" && arg[0] >= '0' && arg[0] <= '9' {
		n, ok := parseUmask(arg)
		if !ok {
			r.diagf("%s\n", Wording(d.UmaskBadMask,
				"umask: %[1]s: octal number out of range", arg))
			return 0, orDefault(d.UmaskBadMaskStatus, 1)
		}
		return n, 0
	}
	current, err := r.currentUmask()
	if err != nil {
		r.diagf("umask: %v\n", err)
		return 0, 1
	}
	n, fail, ok := r.parseSymbolicUmask(arg, current)
	if !ok {
		if r.unspecified {
			return 0, r.status
		}
		wording := d.UmaskBadSymbolicMode
		// Two of the four say which kind of character it was — an operator
		// where `+-=` was wanted, a permission where `rwx` was. The other two
		// name the whole argument and do not distinguish, which is why an
		// empty entry here falls back rather than printing nothing.
		if fail.operator && d.UmaskBadSymbolicOperator != "" {
			wording = d.UmaskBadSymbolicOperator
		}
		if fail.numeric {
			// One dialect answers this corner with the complaint it gives a
			// number it could not read, so the symbolic wording is not used
			// at all.
			wording = d.UmaskBadMask
		}
		r.diagf("%s\n", Wording(wording,
			"umask: %[1]s: invalid symbolic mode", arg, string(fail.bad)))
		return 0, orDefault(d.UmaskBadMaskStatus, 1)
	}
	return n, 0
}

// currentUmask reads the mask without changing it, which takes a set and a
// set-back because the system call offers no way to ask.
func (r *Runner) currentUmask() (int, error) {
	old, err := r.SetUmask(0)
	if err != nil {
		return 0, err
	}
	if _, err := r.SetUmask(old); err != nil {
		return 0, err
	}
	return old, nil
}

// parseUmask reads an octal mask.
func parseUmask(s string) (int, bool) {
	n, err := strconv.ParseInt(s, 8, 32)
	if err != nil || n < 0 || n > 0o777 {
		return 0, false
	}
	return int(n), true
}

// umaskPermissionBits are the characters a clause may name, and what each is
// worth in one group.
//
// `s` and `t` are worth nothing — a umask has no setuid or sticky bit to deny
// — and whether they are *accepted* is a dialect's answer: bash and ksh93
// take both, dash takes `s` and refuses `t`, and zsh refuses both.
var umaskPermissionBits = map[byte]int{'r': 4, 'w': 2, 'x': 1, 's': 0, 't': 0}

// maskFailure is what went wrong reading a symbolic mask, in the terms the
// dialects word it with.
type maskFailure struct {
	// bad is the character to name. Zero is a character that is not there,
	// which one dialect names anyway — `umask g` is `umask: `\x00': invalid
	// symbolic mode operator` in bash, with a null byte between the quotes.
	bad byte
	// operator says the character was where an operator was wanted rather
	// than where a permission was, which two of the four tell apart.
	operator bool
	// numeric asks for the octal complaint instead. One dialect answers a
	// who with no operator with `bad umask` — the message it gives a number
	// it could not read — rather than with a symbolic complaint.
	numeric bool
}

// parseSymbolicUmask reads `u=rwx,g=,o=` and applies it to the mask in force.
//
// The mask records what is *taken away*, and the symbolic form names what is
// *allowed*, so the work is done on the complement and turned back at the end
// — which is why `u=` denies everything to the owner rather than allowing it.
//
// `+` allows, `-` denies, `=` says exactly what a group gets. An omitted who
// means all three: `umask 022; umask -- -w` gives 222 in all four, which is
// the whole `a` set and not just the owner.
//
// The corners are asked about only when the input reaches them, so an
// ordinary `u=rw` needs no answer from anybody.
func (r *Runner) parseSymbolicUmask(s string, current int) (mask int, fail maskFailure, ok bool) {
	allowed := ^current & 0o777
	for _, clause := range strings.Split(s, ",") {
		i := 0
		who := 0
		for ; i < len(clause); i++ {
			switch clause[i] {
			case 'u':
				who |= 0o700
			case 'g':
				who |= 0o070
			case 'o':
				who |= 0o007
			case 'a':
				who |= 0o777
			default:
				goto ops
			}
		}
	ops:
		named := who != 0
		if !named {
			who = 0o777
		}
		if i == len(clause) {
			// A who and nothing to do with it. ksh93 reads it as `=`, which
			// makes `umask g` deny the group everything; bash and dash
			// refuse it, and zsh answers with its complaint about a number.
			if r.ask(r.sem().SymbolicMaskWhoAloneSetsIt, "`umask g` reading as `umask g=`") {
				allowed = allowed &^ who
				continue
			}
			if r.unspecified {
				return 0, maskFailure{}, false
			}
			return 0, maskFailure{operator: true, numeric: r.diag().UmaskWhoAloneIsANumericComplaint}, false
		}
		for round := 0; i < len(clause); round++ {
			op := clause[i]
			if !isMaskOperator(op) {
				return 0, maskFailure{bad: op, operator: true}, false
			}
			if round == 1 && !r.ask(r.sem().SymbolicMaskTakesMoreThanOneOperator, "a second operator in one `umask` clause") {
				if r.unspecified {
					return 0, maskFailure{}, false
				}
				return 0, maskFailure{bad: op}, false
			}
			if r.unspecified {
				return 0, maskFailure{}, false
			}
			// An omitted who before `=` means all three groups, and that
			// is core rather than an axis. It was one — zsh was recorded
			// wanting a who and naming a `/` that is not in the input —
			// and the measurement behind that was `umask -- =w` written
			// unquoted, which in zsh is not the operand it looks like:
			// `=w` is that shell's `=cmd` expansion and reaches `umask` as
			// `/usr/bin/w`. Quoted, or under `setopt noequals`, zsh gives
			// `umask "=w"` the 0555 every other column gives it. See #2057.
			i++
			perms := 0
			for ; i < len(clause); i++ {
				bit, isPerm := umaskPermissionBits[clause[i]]
				if !isPerm {
					if isMaskOperator(clause[i]) {
						break
					}
					return 0, maskFailure{bad: clause[i]}, false
				}
				if refused, stop := r.maskLetterRefused(clause[i]); stop {
					return 0, refused, false
				}
				perms |= bit<<6 | bit<<3 | bit
			}
			switch op {
			case '=':
				allowed = allowed&^who | perms&who
			case '+':
				allowed |= perms & who
			case '-':
				allowed &^= perms & who
			}
		}
	}
	return ^allowed & 0o777, maskFailure{}, true
}

// isMaskOperator reports whether a character is one of the three a clause
// turns on.
func isMaskOperator(c byte) bool { return c == '+' || c == '-' || c == '=' }

// maskLetterRefused answers `s` and `t`, which change no bits and are not
// taken everywhere: bash and ksh93 take both, dash takes `s` and refuses `t`,
// zsh refuses both. Asked only when one of the two appears.
func (r *Runner) maskLetterRefused(c byte) (maskFailure, bool) {
	switch c {
	case 's':
		if r.ask(r.sem().SymbolicMaskTakesTheSetuidLetter, "`s` in a `umask` clause") {
			return maskFailure{}, r.unspecified
		}
	case 't':
		if r.ask(r.sem().SymbolicMaskTakesTheStickyLetter, "`t` in a `umask` clause") {
			return maskFailure{}, r.unspecified
		}
	default:
		return maskFailure{}, false
	}
	if r.unspecified {
		return maskFailure{}, true
	}
	return maskFailure{bad: c}, true
}

// symbolicUmask spells a mask the way `umask -S` does: the permissions it
// *allows*, not the bits it takes away.
//
// Unanimous across the panel, which is why it needs no dialect.
func symbolicUmask(mask int) string {
	var b strings.Builder
	for i, who := range []byte{'u', 'g', 'o'} {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte(who)
		b.WriteByte('=')
		shift := 6 - 3*i
		for j, bit := range []int{4, 2, 1} {
			if mask&(bit<<shift) == 0 {
				b.WriteByte("rwx"[j])
			}
		}
	}
	return b.String()
}
