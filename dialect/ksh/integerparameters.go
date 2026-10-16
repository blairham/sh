// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// The shell's own integer parameters beside OPTIND, which ksh93 starts with
// the integer attribute and, for two of them, a value (#6633).
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c`, `env -i PATH=/bin`:
//
//	typeset -p PPID TMOUT MAILCHECK     typeset -i PPID=…
//	                                    typeset -i TMOUT=0
//	                                    typeset -i MAILCHECK=600
//	typeset PPID=7+1; echo $PPID        8
//	TMOUT=2+3; MAILCHECK=1+1            5 and 2
//	TMOUT=x                             0, the value of the unset name x
//	unset TMOUT; TMOUT=1+1              1+1: the attribute goes with the name
//
// and with each name handed in the environment instead, the value it was
// handed, exported:
//
//	TMOUT=7                             typeset -x -i TMOUT=7
//	MAILCHECK=1+1                       typeset -x -i MAILCHECK=0
//
// So an inherited value is not evaluated but read as one integer constant —
// see importedInteger — and anything else is 0.
var startupIntegers = []struct {
	name  string
	value string
}{
	{"TMOUT", "0"},
	{"MAILCHECK", "600"},
}

func registerTheIntegerParameters(r *interp.Runner) {
	for _, p := range startupIntegers {
		value := p.value
		if inherited, ok := r.GetVar(p.name); ok {
			value = strconv.Itoa(importedInteger(inherited))
		}
		// The value first and the attribute after it, so that laying the
		// parameter down is not an evaluation.
		r.SetSpecial(p.name, value)
		r.MarkInteger(p.name)
		r.KeepThroughDeclarations(p.name)
	}
	// PPID's value is the shell's own, and only the attribute is added.
	r.MarkInteger("PPID")
	r.KeepThroughDeclarations("PPID")
}

// importedInteger reads a value the environment handed one of these names.
// The whole text must be one integer constant — decimal, `0x` hex or `base#`
// digits — or it is 0. Measured over TMOUT and MAILCHECK alike:
//
//	7 → 7     -1 → -1    +5 → 5     08 → 8     010 → 10    0x10 → 16
//	16#10 → 16           -0x10 → -16           --1 → 0     16#-1 → 0
//	2x → 0    1+1 → 0    " 2 " → 0  3.9 → 0    abc → 0     "" → 0
func importedInteger(s string) int {
	neg := false
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		neg, s = true, rest
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	base := 10
	switch {
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0X"):
		base, s = 16, s[2:]
	case strings.Contains(s, "#"):
		b, digits, _ := strings.Cut(s, "#")
		n, err := strconv.Atoi(b)
		if err != nil || n < 2 || n > 36 {
			return 0
		}
		base, s = n, digits
	}
	// One sign, in front: ParseInt would take a second one after it.
	if s == "" || strings.ContainsAny(s, "+-") {
		return 0
	}
	v, err := strconv.ParseInt(s, base, 0)
	if err != nil {
		return 0
	}
	if neg {
		v = -v
	}
	return int(v)
}
