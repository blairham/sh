// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/interp"
)

// The `zsh/datetime` module: the clock, as a script reads it.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files. `zmodload -lF zsh/datetime` there names four features and this file
// has all four:
//
//	+b:strftime  +p:EPOCHSECONDS  +p:EPOCHREALTIME  +p:epochtime
//
// All four are implemented rather than refused, which is the unusual half of
// this module and worth saying why: **none of them needs a seam that does not
// exist**. The three parameters are one clock read each, through
// [interp.Runner.Now] — the hook `printf '%(fmt)T'` already reads, so a shell
// whose embedder pins the clock pins these too. `strftime` is a formatter over
// the same format language `printf '%(fmt)T'` writes, and it calls
// [interp.Strftime] rather than carrying a second copy of it.
//
// So the module loads because everything it names is here, not because the
// rule in zmodload.go forgave anything. That rule is what made it worth
// asking *which* of the four to implement instead of whether the module could
// load at all (#1146, #1154).
//
// A real plugin manager reads three of them: `$EPOCHREALTIME` to stamp when a
// plugin was loaded, `$EPOCHSECONDS` to schedule deferred work, and
// `strftime` both ways round — forward to name a file after a timestamp, and
// `-r` to read an HTTP `Last-Modified` header back into seconds. `$epochtime`
// is the one it never touches, and it is here anyway because it is the same
// clock read with the nanoseconds beside the seconds instead of inside them.

// registerDatetimeModule installs the module's four features.
//
// The parameters are produced rather than stored, for the reason `$SECONDS`
// is: a clock that was read once is wrong from the instant afterwards, and
// silently — the caller still gets a number.
func registerDatetimeModule(r *interp.Runner) {
	r.SetDynamic("EPOCHSECONDS", func(rr *interp.Runner) string {
		return strconv.FormatInt(rr.Now().Unix(), 10)
	})
	r.SetDynamic("EPOCHREALTIME", func(rr *interp.Runner) string {
		return epochRealtime(rr.Now())
	})
	r.SetDynamicArray("epochtime", func(rr *interp.Runner) []string {
		t := rr.Now()
		return []string{strconv.FormatInt(t.Unix(), 10), strconv.Itoa(t.Nanosecond())}
	})
	// Readonly, which is what zsh's own `${(t)EPOCHSECONDS}` says —
	// `integer-readonly-hide-hideval-special`, and the same for the other
	// two — and it is not decoration: a produced parameter a script can
	// assign to is shadowed by the assignment from then on, so it would stop
	// tracking the clock and never say so.
	//
	// And hidden, which zsh's are — `${(t)EPOCHSECONDS}` says `hide` and
	// `hideval` both — and which this file said for a while was
	// unobservable here. It was, and it is not any more, so the claim is
	// replaced rather than left standing.
	//
	// What made it observable is two listings learning to write a produced
	// value they had been dropping by accident. `typeset -p epochtime` now
	// says `typeset -ar epochtime`, the array letter included, where it used
	// to say `typeset -r epochtime`; and the bare `typeset` listing now
	// leaves a hidden name's value out instead of writing `=''` after it.
	// Without this call the same two listings would have written the clock
	// *into* their output — and one asked twice would differ from itself,
	// since the two reads are nanoseconds apart. Both forms match zsh
	// exactly with it: `array readonly epochtime` in the bare listing and
	// `typeset -ar epochtime` from `-p`.
	for _, name := range []string{"EPOCHSECONDS", "EPOCHREALTIME", "epochtime"} {
		r.MarkReadonly(name)
		r.MarkHidden(name)
	}
	// The letters the two scalars list with, which the readonly mark could
	// not supply: measured 2026-09-12 after `zmodload zsh/datetime`, real zsh
	// writes `typeset -ir EPOCHSECONDS` and `typeset -Fr EPOCHREALTIME` and
	// this wrote `typeset -r` for both. The mark put the names into a
	// listing — which is how the array row above came to be right — and a
	// mark that means "cannot be assigned to" was never going to say which
	// type the name has (#2451).
	r.SetDynamicDeclaration("EPOCHSECONDS", interp.ProducedDeclaration{Integer: true})
	r.SetDynamicDeclaration("EPOCHREALTIME", interp.ProducedDeclaration{Float: true})
	r.Register("strftime", strftimeBuiltin)
}

// epochRealtimeDigits is how many places `$EPOCHREALTIME` carries.
//
// Ten, which is `typeset -F`'s default precision and what zsh prints — far
// past what a float64 of a nanosecond epoch can distinguish, so the last
// places are the same rounding noise a real zsh shows (`…785.5219841003`).
// Kept at ten anyway: a script comparing two of them wants the same shape it
// gets from the shell this dialect imitates.
const epochRealtimeDigits = 10

func epochRealtime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', epochRealtimeDigits, 64)
}

// `strftime` writes a time through a format, and with `-r` reads one back.
//
//	strftime [-n] [-r] [-s scalar] format [seconds [nanoseconds]]
//
// Measured: the letters are not stackable in the sense that matters here —
// `-rs v` is read as `-r -s v`, so they are — and every complaint carries the
// builtin's own name in the location (`<file>:strftime:2: too many
// arguments`), which is [interp.Runner.Diagnosef] rather than the shell's
// voice. None of them is fatal: status 1 and the script carries on.

// strftimeOpts is what the letters asked for.
type strftimeOpts struct {
	noNewline bool   // -n
	reverse   bool   // -r
	scalar    string // -s
	assign    bool   // whether -s was given, since the empty name is not one
}

func strftimeBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	opts, rest, code := strftimeOptions(r, args)
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	// Three operands at most: the format, the seconds and the nanoseconds.
	if len(rest) > 3 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	if opts.assign && !isIdentifier(opts.scalar) {
		r.Diagnosef("not an identifier: %s\n", opts.scalar)
		return 1
	}
	if opts.reverse {
		return strftimeReverse(r, opts, rest)
	}
	return strftimeForward(r, opts, rest)
}

// strftimeOptions reads the leading option words.
//
// `--` ends them, and a word that is not an option ends them too — the
// format may begin with a `-` only behind `--`, which is the same rule every
// other builtin here follows.
func strftimeOptions(r *interp.Runner, args []string) (strftimeOpts, []string, int) {
	var opts strftimeOpts
	for len(args) > 0 {
		word := args[0]
		if word == "--" {
			return opts, args[1:], 0
		}
		if len(word) < 2 || word[0] != '-' {
			return opts, args, 0
		}
		args = args[1:]
		for i := 1; i < len(word); i++ {
			switch word[i] {
			case 'n':
				opts.noNewline = true
			case 'r':
				opts.reverse = true
			case 's':
				// The name is the rest of the word where there is one, and
				// the next word otherwise — so `-s v` and `-rs v` both name
				// `v`, which is the spelling a real script uses.
				opts.assign = true
				if i+1 < len(word) {
					opts.scalar = word[i+1:]
					i = len(word)
					break
				}
				if len(args) == 0 {
					r.Diagnosef("argument expected: -s\n")
					return opts, nil, 1
				}
				opts.scalar, args = args[0], args[1:]
			default:
				r.Diagnosef("bad option: -%c\n", word[i])
				return opts, nil, 1
			}
		}
	}
	return opts, nil, 0
}

// strftimeWhen is the moment the operands name, which is now when they name
// none. bad is the operand that would not read, when one would not.
//
// An epoch operand is that second **exactly**: the fraction is the third
// operand or nothing, and never the clock's — measured, `strftime "%N"
// 1788698096` is nine zeros where the same format with no operand at all is
// the clock's own nanoseconds. That falls out of the epoch being rebuilt at
// second precision rather than needing a branch of its own; a branch was
// there and a mutant proved it could not be told from its absence.
func strftimeWhen(r *interp.Runner, rest []string) (t time.Time, bad string, ok bool) {
	t = r.Now()
	if len(rest) > 1 {
		secs, err := strconv.ParseInt(strings.TrimSpace(rest[1]), 10, 64)
		if err != nil {
			return t, rest[1], false
		}
		t = time.Unix(secs, 0)
	}
	nsec := int64(t.Nanosecond())
	if len(rest) > 2 {
		n, err := strconv.ParseInt(strings.TrimSpace(rest[2]), 10, 64)
		if err != nil {
			return t, rest[2], false
		}
		nsec = n
	}
	return time.Unix(t.Unix(), nsec), "", true
}

func strftimeForward(r *interp.Runner, opts strftimeOpts, rest []string) int {
	t, bad, ok := strftimeWhen(r, rest)
	if !ok {
		r.Diagnosef("%s: invalid argument\n", bad)
		return 1
	}
	out := zshStrftime(rest[0], t)
	if opts.assign {
		r.SetVar(opts.scalar, out)
		return 0
	}
	if !opts.noNewline {
		out += "\n"
	}
	_, _ = fmt.Fprint(r.Out(), out)
	return 0
}

func strftimeReverse(r *interp.Runner, opts strftimeOpts, rest []string) int {
	if len(rest) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	t, left, unknown, ok := zshStrptime(rest[0], rest[1])
	if unknown != "" {
		// Not "format not matched": the input may match perfectly well and
		// the shortfall is this shell's. Saying otherwise would send a
		// script looking at its data for a bug that is here.
		r.Diagnosef("-r: %s is not implemented yet\n", unknown)
		return 1
	}
	if !ok {
		r.Diagnosef("format not matched\n")
		return 1
	}
	if left != "" {
		// A warning and not a refusal: measured, zsh writes this line and
		// still answers with the seconds it did read, at status 0.
		r.Diagnosef("warning: input string not completely matched\n")
	}
	out := strconv.FormatInt(t.Unix(), 10)
	if opts.assign {
		r.SetVar(opts.scalar, out)
		return 0
	}
	if !opts.noNewline {
		out += "\n"
	}
	_, _ = fmt.Fprint(r.Out(), out)
	return 0
}

// zshStrftime is the format language `printf '%(fmt)T'` writes, plus the five
// conversions this shell's builtin has beyond it.
//
// The POSIX ones are not repeated here: each is handed to [interp.Strftime]
// one conversion at a time, so a fix to `%V` reaches both spellings and
// neither can drift from the other. Only the extras are answered here, and
// they are the ones measured against zsh 5.9.2:
//
//	%N   the nanoseconds, nine digits
//	%.   the fraction, three digits, or `%<n>.` for n of them, rounded
//	%f   the day of the month, unpadded — `6`, where `%d` is `06`
//	%K   the hour of a 24-hour clock, unpadded
//	%L   the hour of a 12-hour clock, unpadded
//
// A `%` at the very end of the format is written as itself, and a conversion
// neither this nor [interp.Strftime] knows keeps its letter and loses the
// `%` — measured, `%Q` is `Q` — which is the C library's answer rather than
// this shell's.
func zshStrftime(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		// `%<digits>.` is the fraction with a width, and the digits belong to
		// the conversion rather than being text before it.
		if width, end, ok := strftimeFractionWidth(format, i+1); ok {
			b.WriteString(fraction(t, width))
			i = end
			continue
		}
		i++
		switch format[i] {
		case 'N':
			b.WriteString(pad9(t.Nanosecond()))
		case '.':
			b.WriteString(fraction(t, 3))
		case 'f':
			b.WriteString(strconv.Itoa(t.Day()))
		case 'K':
			b.WriteString(strconv.Itoa(t.Hour()))
		case 'L':
			b.WriteString(strconv.Itoa(twelveHour(t.Hour())))
		default:
			b.WriteString(interp.Strftime("%"+string(format[i]), t))
		}
	}
	return b.String()
}

// strftimeFractionWidth reads the digits of a `%<n>.` conversion, and reports
// false for anything else — including `%3d`, whose digits are not a width in
// this language and whose `d` is a conversion of its own.
func strftimeFractionWidth(format string, i int) (width, end int, ok bool) {
	start := i
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		i++
	}
	if i == start || i >= len(format) || format[i] != '.' {
		return 0, 0, false
	}
	n, err := strconv.Atoi(format[start:i])
	if err != nil {
		return 0, 0, false
	}
	return n, i, true
}

// fraction is the nanoseconds rounded to width digits — `%6.` of 123456789ns
// is 123457 and not 123456, measured.
func fraction(t time.Time, width int) string {
	if width <= 0 {
		return ""
	}
	if width > 9 {
		width = 9
	}
	div := 1
	for i := width; i < 9; i++ {
		div *= 10
	}
	n := (t.Nanosecond() + div/2) / div
	// Rounding can carry past the width — 999999999ns to three places is a
	// whole second — and the digits it would need are not there to take, so
	// the largest value the width holds is the answer.
	if limit := pow10(width); n >= limit {
		n = limit - 1
	}
	return pad(n, width)
}

func pow10(n int) int {
	out := 1
	for i := 0; i < n; i++ {
		out *= 10
	}
	return out
}

func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func pad9(n int) string { return pad(n, 9) }

func twelveHour(h int) int {
	h %= 12
	if h == 0 {
		return 12
	}
	return h
}

// strptimeFields is the broken-down time a format fills in, starting from
// the one an all-blank format gives.
//
// Measured: `strftime -r %H:%M 1:2` is -2208985080, which is 1900-01-01
// 01:02:00 local — so an unnamed field is the start of 1900 rather than
// today, and a script reading a header with no year in it gets a date in
// 1900 rather than a plausible one this year. That is the shell's answer and
// it is the one to keep, because the alternative silently invents a year.
type strptimeFields struct {
	year, month, day    int
	hour, minute, sec   int
	pm, pmSeen, epoch   bool
	epochSecs           int64
	hourIsTwelveHourish bool
}

// zshStrptime reads a time out of a string, `strftime -r`.
//
// Three answers rather than two, because "this format has a conversion I do
// not implement" is not the same as "this input does not match": the first
// has to be refused by name, or a format nobody supports would be reported as
// a mismatched input and a script would go looking at its data. unknown names
// the conversion when that is what happened.
//
// A run of whitespace in the format matches any run of whitespace in the
// input, including none, which is what C's strptime does and what makes
// `"%d %b %Y"` read a header with two spaces in it.
func zshStrptime(format, input string) (t time.Time, left, unknown string, ok bool) {
	f := strptimeFields{year: 1900, month: 1, day: 1}
	i, j := 0, 0
	for i < len(format) {
		c := format[i]
		switch {
		case isSpaceByte(c):
			i++
			for j < len(input) && isSpaceByte(input[j]) {
				j++
			}
			continue
		case c != '%':
			if j >= len(input) || input[j] != c {
				return t, "", "", false
			}
			i++
			j++
			continue
		}
		if i+1 >= len(format) {
			return t, "", "", false
		}
		i++
		conv := format[i]
		i++
		if expanded, isCompound := strptimeCompound[conv]; isCompound {
			// A compound conversion is its own expansion, spliced in where it
			// stood — one table rather than a second parser for each.
			format = format[:i-2] + expanded + format[i:]
			i -= 2
			continue
		}
		n, adv, why := strptimeConversion(&f, conv, input[j:])
		if why != "" {
			return t, "", why, false
		}
		if !adv {
			return t, "", "", false
		}
		j += n
	}
	if f.epoch {
		return time.Unix(f.epochSecs, 0), input[j:], "", true
	}
	hour := f.hour
	if f.pmSeen && f.hourIsTwelveHourish {
		hour = twelveHour(hour) % 12
		if f.pm {
			hour += 12
		}
	}
	t = time.Date(f.year, time.Month(f.month), f.day, hour, f.minute, f.sec, 0, time.Local)
	return t, input[j:], "", true
}

// strptimeCompound are the conversions that stand for a format of their own,
// exactly as they do on the way out.
var strptimeCompound = map[byte]string{
	'D': "%m/%d/%y",
	'F': "%Y-%m-%d",
	'T': "%H:%M:%S",
	'R': "%H:%M",
	'r': "%I:%M:%S %p",
	'c': "%a %b %e %H:%M:%S %Y",
	'x': "%m/%d/%y",
	'X': "%H:%M:%S",
}

// strptimeConversion reads one conversion out of the input, reporting how
// many bytes it took. An empty why means it was a conversion this
// understands, whether or not the input matched.
func strptimeConversion(f *strptimeFields, conv byte, in string) (n int, ok bool, why string) {
	switch conv {
	case '%':
		if len(in) > 0 && in[0] == '%' {
			return 1, true, ""
		}
		return 0, false, ""
	case 'n', 't':
		n := 0
		for n < len(in) && isSpaceByte(in[n]) {
			n++
		}
		return n, true, ""
	case 'Y':
		return strptimeNumber(in, 4, func(v int) { f.year = v })
	case 'y':
		return strptimeNumber(in, 2, func(v int) {
			// The window POSIX names: 69–99 is the last century and 00–68
			// this one.
			if v >= 69 {
				f.year = 1900 + v
				return
			}
			f.year = 2000 + v
		})
	case 'C':
		return strptimeNumber(in, 2, func(v int) { f.year = v*100 + f.year%100 })
	case 'm':
		return strptimeNumber(in, 2, func(v int) { f.month = v })
	case 'd', 'e':
		return strptimeNumber(in, 2, func(v int) { f.day = v })
	case 'j':
		return strptimeNumber(in, 3, func(v int) { f.month, f.day = 1, v })
	case 'H', 'k':
		return strptimeNumber(in, 2, func(v int) { f.hour, f.hourIsTwelveHourish = v, false })
	case 'I', 'l':
		return strptimeNumber(in, 2, func(v int) { f.hour, f.hourIsTwelveHourish = v, true })
	case 'M':
		return strptimeNumber(in, 2, func(v int) { f.minute = v })
	case 'S':
		return strptimeNumber(in, 2, func(v int) { f.sec = v })
	case 's':
		return strptimeNumber(in, 0, func(v int) { f.epoch, f.epochSecs = true, int64(v) })
	case 'b', 'B', 'h':
		return strptimeName(in, monthNames, func(v int) { f.month = v + 1 })
	case 'a', 'A':
		// Read and discarded: the weekday is derived from the date, and a
		// header naming one that disagrees is still the date it names.
		return strptimeName(in, dayNames, func(int) {})
	case 'p':
		return strptimeName(in, []string{"AM", "PM"}, func(v int) { f.pm, f.pmSeen = v == 1, true })
	case 'Z':
		// A zone *name* carries no offset this can apply, so it is matched
		// and dropped — the same thing C's strptime does with one.
		n := 0
		for n < len(in) && (in[n] == '+' || in[n] == '-' || isAlnumByte(in[n])) {
			n++
		}
		return n, true, ""
	}
	return 0, false, "%" + string(conv)
}

// strptimeNumber reads up to width digits, or as many as there are when width
// is zero, allowing a leading `-` for the epoch form.
//
// Fewer digits than the width is a match, not a failure: measured, `%Y-%m-%d`
// reads `2026-9-6`.
func strptimeNumber(in string, width int, set func(int)) (int, bool, string) {
	n := 0
	if width == 0 && n < len(in) && (in[n] == '-' || in[n] == '+') {
		n++
	}
	for n < len(in) && in[n] >= '0' && in[n] <= '9' {
		n++
		if width > 0 && n >= width {
			break
		}
	}
	if n == 0 || (n == 1 && (in[0] == '-' || in[0] == '+')) {
		return 0, false, ""
	}
	v, err := strconv.Atoi(in[:n])
	if err != nil {
		return 0, false, ""
	}
	set(v)
	return n, true, ""
}

// strptimeName matches one of a set of names, longest spelling first and
// without regard to case — `Sep`, `September` and `SEPTEMBER` are the same
// month.
func strptimeName(in string, names []string, set func(int)) (int, bool, string) {
	best, bestIdx := 0, -1
	for idx, full := range names {
		spellings := []string{full}
		if len(full) > 3 {
			// The abbreviation a month or a weekday also answers to. `AM`
			// and `PM` are two letters and have no second spelling, which is
			// why this is a length test rather than an unconditional slice.
			spellings = append(spellings, full[:3])
		}
		for _, name := range spellings {
			if len(name) > best && len(in) >= len(name) &&
				strings.EqualFold(in[:len(name)], name) {
				best, bestIdx = len(name), idx
			}
		}
	}
	if bestIdx < 0 {
		return 0, false, ""
	}
	set(bestIdx)
	return best, true, ""
}

var monthNames = []string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var dayNames = []string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func isAlnumByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
