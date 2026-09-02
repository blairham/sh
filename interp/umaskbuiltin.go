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
			r.diagf("%s\n", Wording(d.UmaskBadOption,
				"umask: %[1]s: invalid option", args[0]))
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
	mask, ok := parseUmask(args[0])
	if !ok {
		r.diagf("%s\n", Wording(r.diag().UmaskBadMask,
			"umask: %[1]s: octal number out of range", args[0]))
		return orDefault(r.diag().UmaskBadMaskStatus, 1)
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
	old, err := r.SetUmask(0)
	if err != nil {
		r.diagf("umask: %v\n", err)
		return 1
	}
	if _, err := r.SetUmask(old); err != nil {
		r.diagf("umask: %v\n", err)
		return 1
	}
	if symbolic {
		r.printf("%s\n", symbolicUmask(old))
		return 0
	}
	// Four digits in three of the four; zsh drops the leading zero and
	// prints three. Measured: `022` against `0022`.
	if r.ask(r.sem().UmaskPrintsFourDigits, "`umask` printing a leading zero") {
		r.printf("%04o\n", old)
	} else {
		r.printf("%03o\n", old)
	}
	return 0
}

// parseUmask reads an octal mask.
//
// Symbolic input — `u=rwx,g=,o=` — is accepted by all four and is not here
// yet; it is a separate piece of parsing rather than a different spelling of
// this one.
func parseUmask(s string) (int, bool) {
	n, err := strconv.ParseInt(s, 8, 32)
	if err != nil || n < 0 || n > 0o777 {
		return 0, false
	}
	return int(n), true
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
