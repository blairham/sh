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
// The mask is this Runner's own, so that a body of this shell has one of its
// own the way a fork would; it reaches the kernel at an open and at a spawn,
// through a hook the caller supplies. See Runner.SetUmask and umaskscope.go.

func init() {
	builtins["umask"] = biUmask
}

func biUmask(r *Runner, _ context.Context, args []string) int {
	// The letters this shell's `umask` has. `-S` is unanimous; `-p` is one
	// dialect's, so the set the reader accepts and the set a bad option is
	// named against are both built from the answer — a letter refused here
	// has to be refused as an option and not silently taken.
	letters := "S"
	reusable := r.sem().UmaskHasTheReusableLetter == Yes
	if reusable {
		letters = "Sp"
	}
	symbolic, prefixed := false, false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		// A bundle, because bash reads one: `umask -pS` and `umask -Sp` both
		// print the prefixed symbolic form. Every letter of the word has to
		// be one this shell has, or the word is refused whole — the same
		// bundle rule the shared option reader follows.
		bundle := args[0][1:]
		if strings.Trim(bundle, letters) == "" && bundle != "" {
			symbolic = symbolic || strings.ContainsRune(bundle, 'S')
			prefixed = prefixed || (reusable && strings.ContainsRune(bundle, 'p'))
			args = args[1:]
			continue
		}
		d := r.diag()
		// Named the way every other builtin's bad option is named, which is
		// the dialect's rule and not this builtin's: `umask --version` is
		// `--` in bash and `-v` in zsh, exactly as `export --version` is.
		// Spelling the whole word here made this the one builtin that
		// answered `umask: --version: invalid option`.
		_, name := r.badOption(args[0], letters)
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
	r.ensureUmask()
	if !r.maskKnown {
		// Refused rather than answered from somewhere else. The whole point
		// of #117 is that a mask this shell cannot change must not look as
		// though it changed.
		r.diagf("umask: this shell was not given a umask to read or change\n")
		return 2
	}
	if len(args) == 0 {
		return r.reportUmask(symbolic, prefixed)
	}
	mask, code := r.readMask(args[0])
	if code != 0 {
		return code
	}
	// Into this shell's own field and nowhere else, so that a body's mask is
	// the body's: nothing beside it is looking at the same place, and it goes
	// out with the body. See umaskscope.go.
	r.umask = mask
	// bash alone echoes the new mask when it was asked for the symbolic form
	// and given one to set. Setting without `-S` is silent in all four.
	if symbolic && r.ask(r.sem().UmaskSetWithSPrints, "`umask -S mask` echoing the mask it set") {
		r.printf("%s\n", symbolicUmask(mask))
	}
	return 0
}

// reportUmask prints the mask this shell holds.
//
// `prefixed` is `-p`: the same figure written as the command that would set
// it again, which is what makes `eval "$(umask -p)"` a way to put a saved
// mask back. Only the report takes it — `umask -p 077` is silent, and
// `umask -p -S 077` prints the bare symbolic form the `-S` echo prints.
func (r *Runner) reportUmask(symbolic, prefixed bool) int {
	old := r.umask
	if symbolic {
		r.printf("%s%s\n", umaskPrefix(prefixed, "umask -S "), symbolicUmask(old))
		return 0
	}
	// Four digits in three of the four. zsh writes a C octal literal with a
	// minimum of three digits instead, which is not "three digits": it is
	// `022` against bash's `0022`, and `0333` against bash's `0333`. The
	// leading zero comes back the moment the owner group denies anything,
	// and printing a flat three gave `333` there.
	if r.ask(r.sem().UmaskPrintsFourDigits, "`umask` printing a leading zero") {
		r.printf("%s%04o\n", umaskPrefix(prefixed, "umask "), old)
	} else {
		r.printf("%s%#03o\n", umaskPrefix(prefixed, "umask "), old)
	}
	return 0
}

// umaskPrefix is the word `-p` puts in front of the figure, or nothing.
func umaskPrefix(prefixed bool, prefix string) string {
	if prefixed {
		return prefix
	}
	return ""
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
	n, fail, ok := r.parseSymbolicUmask(arg, r.umask)
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
// `X` is worth execute or nothing depending on the mask the operand started
// from, so its entry here is a placeholder the reader replaces; see
// Semantics.SymbolicMaskTakesTheConditionalExecuteLetter.
var umaskPermissionBits = map[byte]int{'r': 4, 'w': 2, 'x': 1, 'X': 1, 's': 0, 't': 0}

// maskCopySource reads POSIX's `permcopy`: a `u`, `g` or `o` standing in a
// permission list for whatever that group is allowed now.
//
// The bits come back spread across all three positions, which is the shape
// the caller then narrows with the clause's `who` — the same shape a
// permission character produces.
func maskCopySource(c byte, allowed int) (int, bool) {
	shift := 0
	switch c {
	case 'u':
		shift = 6
	case 'g':
		shift = 3
	case 'o':
	default:
		return 0, false
	}
	bits := (allowed >> shift) & 7
	return bits<<6 | bits<<3 | bits, true
}

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
	// What `X` is worth, decided once from the mask the operand started
	// from rather than per clause. Measured: `umask 133; umask -S u+x,g+X`
	// leaves the group without it in every shell that has the letter, even
	// though the owner gained it a clause earlier.
	conditionalExecute := 0
	if allowed&0o111 != 0 {
		conditionalExecute = 1
	}
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
			sawCopy, sawLetter := false, false
			for ; i < len(clause); i++ {
				if copied, isCopy := maskCopySource(clause[i], allowed); isCopy {
					if !r.ask(r.sem().SymbolicMaskTakesAPermissionCopy,
						"a `u`, `g` or `o` after a `umask` operator") {
						if r.unspecified {
							return 0, maskFailure{}, false
						}
						return 0, maskFailure{bad: clause[i]}, false
					}
					sawCopy = true
					// POSIX makes a copy an *alternative* to a list of
					// permission characters rather than one of them, so
					// nothing portable writes the two together — and the
					// three shells that read it anyway read it three
					// different ways. The axis is asked only for the clause
					// that really holds both, which is what keeps the
					// portable spelling out of the question. See
					// Semantics.UmaskPermissionCopyBesideLetters.
					if !sawLetter {
						perms = copied
						continue
					}
					switch r.permissionCopy() {
					case UmaskPermissionCopyReplaces:
						perms = copied
					case UmaskPermissionCopyContributes:
						perms |= copied
					case UmaskPermissionCopyRefusesTheMixture:
						return 0, maskFailure{bad: clause[i]}, false
					default:
						return 0, maskFailure{}, false
					}
					continue
				}
				bit, isPerm := umaskPermissionBits[clause[i]]
				if !isPerm {
					if isMaskOperator(clause[i]) {
						break
					}
					return 0, maskFailure{bad: clause[i]}, false
				}
				if clause[i] == 'X' {
					bit = conditionalExecute
				}
				if refused, stop := r.maskLetterRefused(clause[i]); stop {
					return 0, refused, false
				}
				if sawCopy {
					// A letter *after* a copy, which is the other order of
					// the same mixture — and one column refuses both.
					switch r.permissionCopy() {
					case UmaskPermissionCopyReplaces, UmaskPermissionCopyContributes:
					case UmaskPermissionCopyRefusesTheMixture:
						return 0, maskFailure{bad: clause[i]}, false
					default:
						return 0, maskFailure{}, false
					}
				}
				sawLetter = true
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

// maskLetterRefused answers the three permission characters that are not
// taken everywhere, and is asked only when one of them appears: `s` and `t`
// change no bits — bash and ksh93 take both, dash takes `s` and refuses `t`,
// zsh refuses both — and `X` is chmod's conditional execute, which bash 5.3,
// ksh93, dash and BusyBox ash read and bash 3.2 and zsh refuse.
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
	case 'X':
		if r.ask(r.sem().SymbolicMaskTakesTheConditionalExecuteLetter, "`X` in a `umask` clause") {
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
