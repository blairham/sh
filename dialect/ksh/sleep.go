// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/interp"
)

// `sleep` is a builtin in ksh93 — `type sleep` there is `sleep is a shell
// builtin` — and the difference is not cosmetic: a builtin starts no child, so
// the shell reaps nothing while it sleeps, and whether a background job that
// ended meanwhile is still in the table depends on exactly that. Measured
// 2026-10-01 on ksh93u+ 2012-08-01: `(exit 4) & sleep 0.3; wait %%` is 4 and
// the same line with `/bin/sleep` is 0. See
// interp.Semantics.FinishedJobLeavesTheTable (#5302).
//
// What it reads, measured the same day:
//
//	sleep 0.3, .3, 3e-1, 0x0.8, +0.3, 1., 1e   the C library's numbers: a
//	                                            hex fraction, an exponent with
//	                                            no digits after the `e`, a sign
//	sleep 0.5s, 0.5S                            seconds, said so
//	sleep 0.05m, 0.2M                           minutes: 3 and 12 seconds
//	sleep nan                                   no time at all
//	sleep abc, 1x, 0,5, 2e-1s                   `sleep: X: bad number`, 1
//	sleep, sleep 1 2, sleep --                  `sleep: one operand expected`, 1
//	sleep -x 1                                  `sleep: -x: unknown option`,
//	                                            and then it sleeps: status 0
//	sleep -1                                    `-1: unknown option`, then no
//	                                            operand is left: one operand
//	                                            expected, 1
//	sleep -s                                    until a signal arrives
//
// **Not modeled**, and measured: ksh93 reads a duration through its date
// parser, which takes far more than numbers — `00:00:00.01` is a time of day
// and waits for it, and `0.01m` is a thousandth of a second where `0.05m` is
// three. Those spellings are refused here as bad numbers rather than guessed
// at.
func sleepBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	forever := false
	rest := args
	for len(rest) > 0 && len(rest[0]) > 1 && rest[0][0] == '-' {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for _, c := range word[1:] {
			if c == 's' {
				forever = true
				continue
			}
			say(r, "sleep: -%c: unknown option\n", c)
		}
	}
	if forever && len(rest) == 0 {
		st, _ := r.SleepFor(ctx, -1)
		return st
	}
	if len(rest) != 1 {
		say(r, "sleep: one operand expected\n")
		return 1
	}
	d, ok := sleepDuration(rest[0])
	if !ok {
		say(r, "sleep: %s: bad number\n", rest[0])
		return 1
	}
	st, _ := r.SleepFor(ctx, d)
	return st
}

// say writes one of the builtin's complaints, which carry no location: ksh93's
// `sleep` is one of the builtins it takes from its command library, and those
// write their own name and nothing in front of it — measured, `sleep abc` is
// `sleep: abc: bad number` from `-c`, from a script file and from inside a
// function, where `print -u9 x` is located as the shell's own.
func say(r *interp.Runner, format string, args ...any) {
	_, _ = fmt.Fprintf(r.Err(), format, args...)
}

// sleepDuration reads one operand as a duration.
func sleepDuration(word string) (time.Duration, bool) {
	s := strings.TrimSpace(word)
	unit := 1.0
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 's', 'S':
			s = s[:n-1]
		case 'm', 'M':
			s, unit = s[:n-1], 60
		}
	}
	if len(s) < len(strings.TrimSpace(word)) && strings.ContainsAny(s, "eE") {
		// A unit after an exponent is not read: measured, `2e-1s` is a bad
		// number where `0.2s` and `2e-1` are both a fifth of a second.
		return 0, false
	}
	v, ok := cNumber(s)
	if !ok {
		return 0, false
	}
	v *= unit
	switch {
	case math.IsNaN(v) || v <= 0:
		return 0, true
	case math.IsInf(v, 1) || v > float64(math.MaxInt64)/float64(time.Second):
		return -1, true
	}
	return time.Duration(v * float64(time.Second)), true
}

// cNumber reads the whole of s as the C library's strtod would, or reports
// false where any of it is left over: a sign, `inf` and `nan`, a hex number
// with an optional fraction and `p` exponent, or a decimal one whose `e` with
// no digits after it is left out of the number — which here means the `e` is
// simply the end, as ksh93 reads `1e` as one second.
func cNumber(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	sign := 1.0
	body := s
	if body[0] == '+' || body[0] == '-' {
		if body[0] == '-' {
			sign = -1
		}
		body = body[1:]
	}
	switch strings.ToLower(body) {
	case "inf", "infinity":
		return sign * math.Inf(1), true
	case "nan":
		return math.NaN(), true
	}
	if len(body) > 2 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') {
		hex := body[2:]
		if !strings.ContainsAny(hex, "pP") {
			hex += "p0"
		}
		v, err := strconv.ParseFloat("0x"+hex, 64)
		if err != nil {
			return 0, false
		}
		return sign * v, true
	}
	if strings.HasSuffix(body, "e") || strings.HasSuffix(body, "E") {
		body = body[:len(body)-1]
	}
	if body == "." {
		return 0, true
	}
	v, err := strconv.ParseFloat(body, 64)
	if err != nil || strings.ContainsAny(body, "xXpP_") {
		return 0, false
	}
	return sign * v, true
}

// registerSleep installs the builtin.
func registerSleep(r *interp.Runner) {
	r.Register("sleep", sleepBuiltin)
}
