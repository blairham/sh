// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// `printf`, the last builtin that was not one.
//
// It appeared to work because the operating system ships /usr/bin/printf, so
// `PATH= printf x` was 127 and everything else was a separate program the
// shell had no say over — the same way `test`, `[` and `kill` were before
// them. POSIX lists it as a builtin and every shell in the panel has it, for
// the reason a borrowed one cannot serve: `printf '%q'` has to quote the way
// *this* shell quotes, and the diagnostics have to be the shell's own.
//
// Most of it is unanimous, which is worth saying because the divergences are
// what the rest of this file is about. All four agree that the format is
// *reused* until the arguments run out, that a missing argument is the empty
// string or zero rather than an error, that `%b` expands escapes in its
// argument and `%s` does not, that escapes in the format itself are always
// expanded, and on `%c`, widths, precisions, `%%` and octal escapes.
//
// Four things they do not agree on, and each is an axis or a wording rather
// than a branch here:
//
//   - A `%d` given something that is not a number. bash and dash complain and
//     report failure; ksh93 and zsh print zero and say nothing. Both still
//     print the zero.
//   - `%q`. bash and zsh backslash-escape, ksh93 single-quotes, and dash does
//     not have it at all.
//   - `\c` in the format, which stops output there in ksh93 and zsh and is
//     two ordinary characters in bash and dash.
//   - What an unknown verb is called, and what `printf` with no format says.

func init() {
	builtins["printf"] = biPrintf
}

func biPrintf(r *Runner, _ context.Context, args []string) int {
	args, assign, code := r.printfOptions(args)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		return r.printfReport(printfUsage, "")
	}
	format, operands := args[0], args[1:]
	status := 0

	// `-v name` collects the text instead of printing it. Swapped rather than
	// threaded through, because everything below writes to r.stdout() and the
	// format is reused in a loop.
	if assign != "" {
		var into strings.Builder
		saved := r.Stdout
		r.Stdout = &into
		defer func() {
			r.Stdout = saved
			r.setVar(assign, into.String())
		}()
	}

	// The format is reused until the arguments run out, and once with none at
	// all. Unanimous, and the reason this is a loop rather than one pass.
	for pass := 0; ; pass++ {
		used, code, end := r.printfOnce(format, operands)
		if code != 0 {
			status = code
		}
		if end == printfPassStopped {
			break
		}
		if used >= len(operands) {
			break
		}
		operands = operands[used:]
		if used == 0 {
			// A format with no verbs consumes nothing, so reusing it would
			// never end.
			break
		}
	}
	return status
}

// printfOptions reads the leading `-` words, returning what is left, the name
// `-v` named, and a non-zero status if one was refused.
//
// There were no options at all before this: `printf -v out "%05d" 42` printed
// `-v` — the option itself, as the format — and left `out` empty, which is two
// wrongs at once and both silent. `printf -- "x\n"` printed `--` for the same
// reason.
//
// Ending them at `--` is unanimous. The rest is two axes: whether `-v` assigns,
// and what an unrecognized one means.
func (r *Runner) printfOptions(args []string) (rest []string, assign string, code int) {
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			// A lone `-` is an operand, not an option, in all four.
			break
		}
		if a == "--" {
			return args[1:], assign, 0
		}
		if a == "-v" && r.ask(r.sem().PrintfAssignsWithV, "`printf -v name`") {
			if len(args) < 2 {
				return nil, "", r.printfReport(printfUsage, "")
			}
			assign, args = args[1], args[2:]
			continue
		}
		if r.unspecified {
			return nil, "", 2
		}
		// Not an option this dialect knows. Three of the four refuse it —
		// even `printf "-%s\n" x`, whose *format* begins with a dash — and
		// zsh takes it as the format instead.
		if !r.ask(r.sem().PrintfRejectsUnknownOption, "printf refusing a leading `-` word it does not know") {
			break
		}
		if r.unspecified {
			return nil, "", 2
		}
		// The *first letter*, not the whole word: a leading `-` word is a
		// bundle of single-letter options, so `printf "-%s\n" x` is refused
		// as `-%` and not as `-%s\n`. Measured against bash and dash, which
		// stop there. ksh93 goes on through the bundle and complains about
		// each letter in turn, which this does not follow.
		return nil, "", r.printfBadOption("-" + string([]rune(a[1:])[0]))
	}
	return args, assign, 0
}

// printfBadOption is the complaint about a leading `-` word this dialect does
// not know, with the usage line after it where the dialect prints one.
func (r *Runner) printfBadOption(opt string) int {
	d := r.diag()
	r.diagf("%s\n", Wording(d.PrintfBadOption, "printf: %[1]s: invalid option", opt))
	if d.PrintfBadOptionShowsUsage {
		usage := Wording(d.PrintfUsage, "printf: usage: printf format [arguments]")
		if d.PrintfUsageUnprefixed {
			r.errf("%s\n", usage)
		} else {
			r.diagf("%s\n", usage)
		}
	}
	return orDefault(d.PrintfUsageStatus, 2)
}

// printfPassEnd says how one pass over the format ended, which is three
// things and not two.
//
// A `\c` that stops ends the *builtin* — zsh's `printf '[%s]\cZ' x y` is
// `[x]` — where ksh93's digitless `\u` ends only the pass, and the loop over
// the operands runs again: `printf '[%s]\uZ' x y` is `[x][y]`. Collapsing the
// two into one bool would have made the second of those `[x]`, which is no
// shell's answer.
type printfPassEnd int

const (
	// printfPassRan read the format through to its end.
	printfPassRan printfPassEnd = iota
	// printfPassTruncated dropped the rest of this pass. The operands that
	// are left are formatted by another one.
	printfPassTruncated
	// printfPassStopped ends the builtin, whether because a `\c` said so or
	// because something was refused.
	printfPassStopped
)

// printfOnce runs the format through once, returning how many operands it
// consumed, the status of any complaint, and how the pass ended — which is
// printfPassEnd's three and not a bool, because one shell drops the rest of a
// pass without ending the builtin.
func (r *Runner) printfOnce(format string, operands []string) (int, int, printfPassEnd) {
	used, status := 0, 0
	// Written as it is produced rather than collected and written at the end:
	// a shell that complains half way through has already printed the half
	// before it, and ksh93's `[` arrives before its complaint about what
	// followed.
	b := printfWriter{w: r.stdout(), r: r}
	if !r.printfWritesThrough() {
		// Held until the end, so a complaint reaches the reader first — which
		// is what three of the four do, their output still being in a buffer
		// when the complaint goes out.
		b.hold = new(strings.Builder)
	}
	defer b.flush()
	next := func() (string, bool) {
		if used < len(operands) {
			s := operands[used]
			used++
			return s, true
		}
		// A *missing* argument is the empty string, and zero where a number
		// was wanted — not an error in any of the panel. An argument that is
		// present and empty is a different thing, and one shell says so.
		used++
		return "", false
	}

	for i := 0; i < len(format); {
		c := format[i]
		switch {
		case c == '\\':
			text, n, end := r.expandPrintfEscape(format[i:])
			b.WriteString(text)
			i += n
			if end != printfPassRan {
				return used, status, end
			}
		case c != '%':
			b.writeByte(c)
			i++
		default:
			spec, verb, timeFmt, n, code := r.scanPrintfSpec(format[i:])
			if code != 0 {
				return used, code, printfPassStopped
			}
			i += n
			if verb == '%' {
				b.writeByte('%')
				continue
			}
			if verb == 0 {
				if spec != "" {
					return used, r.printfBadVerb(format[:i], badVerbName(format, i)), printfPassStopped
				}
				// An empty prefix is a format that ran out before it
				// reached a conversion character — `%`, `%5`, `%ll` at the
				// end. There is no character to name, and one shell does not
				// treat it as an error at all: it writes a bare `%` for the
				// whole unfinished conversion, prefix and all, and succeeds.
				if r.ask(r.sem().PrintfUnfinishedConversionIsAPercent, "a format that ends inside a conversion") {
					b.writeByte('%')
					return used, 0, printfPassStopped
				}
				if r.unspecified {
					return used, r.status, printfPassStopped
				}
				return used, r.printfMissingVerb(format[:i]), printfPassStopped
			}
			text, code, stop := r.printfVerb(spec, verb, timeFmt, next)
			if code != 0 {
				status = code
			}
			b.WriteString(text)
			if stop {
				return used, status, printfPassStopped
			}
		}
	}
	return used, status, printfPassRan
}

// printfVerb formats one conversion.
func (r *Runner) printfVerb(spec string, verb byte, timeFmt string, next func() (string, bool)) (string, int, bool) {
	arg, present := next()
	switch verb {
	case 'T':
		return r.printfTime(spec, timeFmt, arg, present)
	case 's':
		return fmt.Sprintf(spec+"s", arg), 0, false
	case 'b':
		// The one verb whose *argument* is escaped, where `%s` leaves it
		// alone. Unanimous, and the difference people reach for `%b` to get.
		//
		// A `\c` in the argument ends the whole `printf` and not only this
		// conversion — `printf '[%b][%s]' 'a\cb' x` is `[a` in all six — so
		// the flag is returned rather than dropped.
		text, stop := r.expandBEscapes(arg)
		field := fmt.Sprintf(spec+"s", text)
		if stop && field != text &&
			!r.ask(r.sem().PrintfBStopIsPadded, "a `%b` a `\\c` cut short still going through its field") {
			// ksh93 alone: what the stop left is written as it stands, width
			// and precision and all. Asked only where the field would change
			// the text, so a bare `printf '%b' 'a\cb'` needs no dialect.
			return text, 0, true
		}
		return field, 0, stop
	case 'c':
		if arg == "" {
			return "", 0, false
		}
		// The first *byte*, padded as a string. `%c` of a rune would encode
		// it: `printf '%c' $'\xc0'` is the one byte 0xc0 in every shell in
		// the panel, and Go's `%c` on `rune(0xc0)` writes two.
		return fmt.Sprintf(spec+"s", arg[:1]), 0, false
	case 'q':
		return r.printfQuote(spec, arg)
	case 'd', 'i':
		n, code := r.printfNumber(arg, present)
		return fmt.Sprintf(spec+"d", n), code, false
	case 'o', 'u', 'x', 'X':
		n, code := r.printfNumber(arg, present)
		if verb == 'u' {
			verb = 'd'
		}
		return fmt.Sprintf(spec+string(verb), n), code, false
	case 'f', 'e', 'E', 'g', 'G':
		f, code := r.printfFloat(arg, present)
		return fmt.Sprintf(spec+string(verb), f), code, false
	}
	return "", 0, false
}

// printfNumber reads an integer operand, complaining where the dialect does.
//
// The zero is printed either way: the two shells that report this still write
// the zero the conversion would have produced, so the complaint is beside the
// output rather than instead of it.
func (r *Runner) printfNumber(arg string, present bool) (int64, int) {
	if arg == "" && (!present || !r.ask(r.sem().PrintfEmptyIsNotANumber, "`printf` complaining about an empty operand where a number belongs")) {
		return 0, 0
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(arg), 0, 64); err == nil {
		return n, 0
	}
	if !r.ask(r.sem().PrintfReportsBadNumber, "`printf` complaining about an operand that is not a number") {
		return 0, 0
	}
	return 0, r.printfReport(printfBadNumber, arg)
}

func (r *Runner) printfFloat(arg string, present bool) (float64, int) {
	if arg == "" && (!present || !r.ask(r.sem().PrintfEmptyIsNotANumber, "`printf` complaining about an empty operand where a number belongs")) {
		return 0, 0
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil {
		return f, 0
	}
	if !r.ask(r.sem().PrintfReportsBadNumber, "`printf` complaining about an operand that is not a number") {
		return 0, 0
	}
	return 0, r.printfReport(printfBadNumber, arg)
}

// printfQuote is `%q`, which quotes so the shell can read it back.
//
// Three answers and one absence, and the three are in interp/printfquote.go
// with the measurement that separates them. The one thing they agree on is
// the contract: what comes out has to read back as what went in, which is why
// a backslash before a newline is not one of the answers (#1707).
func (r *Runner) printfQuote(spec, arg string) (string, int, bool) {
	switch r.quoteStyle() {
	case PrintfQuoteAnsiCWord:
		return fmt.Sprintf(spec+"s", ansiCWordQuote(arg)), 0, false
	case PrintfQuoteAnsiCCharacter:
		// The same function `${(q)…}` uses, which is the same job: this
		// shell's `%q` and its `q` flag were measured against each other over
		// every printable byte at three positions and every control byte, and
		// they agree everywhere.
		return fmt.Sprintf(spec+"s", quoteWithBackslashes(arg, false)), 0, false
	case PrintfQuoteSingle:
		return fmt.Sprintf(spec+"s", kshSingleQuote(arg)), 0, false
	case PrintfQuoteAbsent:
		// A conversion the shell does not have stops the output where it is,
		// as any other unknown one does.
		return "", r.printfBadVerb("%q", "q"), true
	}
	return "", r.status, true
}

// scanPrintfSpec reads one conversion, returning the flags-width-precision
// prefix, the verb, the date format where the conversion is a `%(…)T`, and
// how much of the format it took.
//
// A verb of 0 means the conversion is not one this shell has. A non-zero code
// is an axis nothing answered, which stops the format rather than printing
// half of it.
func (r *Runner) scanPrintfSpec(s string) (string, byte, string, int, int) {
	i := printfSpecPrefix(s)
	if i >= len(s) {
		return "", 0, "", len(s), 0
	}
	spec := "%" + strings.ReplaceAll(s[1:i], "'", "")
	if s[i] == '(' {
		// `%(fmt)T`, the one conversion whose format is inside the
		// conversion. One shell in the panel has it; asked here rather than
		// at the top, so a dialect without it is never questioned about a
		// format that has no `%(` in it.
		if r.ask(r.sem().PrintfTimeConversion, "`printf '%(…)T'` writing a date") {
			if end := strings.Index(s[i:], ")T"); end >= 0 {
				return spec, 'T', s[i+1 : i+end], i + end + 2, 0
			}
			// No `)T` to close it. bash meets this with two complaints and
			// a partial line; ours is the ordinary refusal of a conversion
			// it cannot read, which the next lines produce.
		}
		if r.unspecified {
			return "", 0, "", i, r.status
		}
	}
	i += r.lengthModifierRun(s, i)
	if r.unspecified {
		return "", 0, "", i, r.status
	}
	if i >= len(s) {
		// A format that ends inside a conversion. The modifier is not what
		// went wrong, so this is the same nothing `%` at the end of a format
		// already is.
		return "", 0, "", len(s), 0
	}
	verb := s[i]
	if verb == 'T' {
		// A `%T` with no parentheses, which is the date-string form's own
		// spelling and belongs to no other dialect: bash calls `T` an
		// invalid format character, dash an invalid directive. Asked here
		// rather than beside the `(` above, so a dialect is questioned only
		// when the letter is actually written.
		if r.ask(r.sem().PrintfTimeConversion, "`printf '%(…)T'` writing a date") &&
			r.ask(r.sem().PrintfTimeOperandIsADateString, "`printf '%T'` taking a date string") {
			return spec, 'T', "", i + 1, 0
		}
		if r.unspecified {
			return "", 0, "", i + 1, r.status
		}
		return spec, 0, "", i + 1, 0
	}
	if strings.IndexByte("sbcqdiouxXfeEgG%", verb) < 0 {
		return spec, 0, "", i + 1, 0
	}
	return spec, verb, "", i + 1, 0
}

// lengthModifierRun is how many bytes at i are a C length modifier this
// dialect takes.
//
// The letters are read and thrown away. Every shell that accepts one ignores
// it — `%hhd` with 300 is 300 rather than 44 — so this exists to stop a
// format bash and ksh93 accept from being reported as a conversion nobody
// has, which is the shape that sends someone debugging their format string.
//
// The axis is asked only when a letter one of the answers would take is
// actually there, so `%d` never raises the question.
func (r *Runner) lengthModifierRun(s string, i int) int {
	const letters = "hljztL"
	if i >= len(s) || strings.IndexByte(letters, s[i]) < 0 {
		return 0
	}
	switch r.lengthModifiers() {
	case PrintfLengthModifiersC89:
		// One letter, and only the three C89 had. The C99 additions are
		// exactly the ones this answer refuses.
		if strings.IndexByte("hlL", s[i]) >= 0 {
			return 1
		}
	case PrintfLengthModifiersC99:
		n := 0
		for i+n < len(s) && strings.IndexByte(letters, s[i+n]) >= 0 {
			n++
		}
		return n
	}
	return 0
}

// timeZone is the zone a date is written in, which is `$TZ` — the Runner's,
// not the process's.
//
// The PATH rule again: `os/time`'s Local reads the *process's* environment,
// and a Runner holds its own variables. `TZ=UTC` without an export changes
// the answer in the shell that has this conversion, so an exported-only
// lookup would be wrong as well as ambient.
//
// An unset TZ is the machine's zone, which is the honest answer to "nobody
// said". An empty one is UTC, and so is a name no zone database has —
// measured, and the same answer for both.
func (r *Runner) timeZone() *time.Location {
	tz, ok := r.getVar("TZ")
	if !ok {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil && tz != "" {
		return loc
	}
	return time.UTC
}

// printfSpecPrefix is where a conversion's verb starts: past the `%`, the
// flags, the width and the precision.
func printfSpecPrefix(s string) int {
	i := 1 // past the %
	for i < len(s) && strings.IndexByte("-+ #0'", s[i]) >= 0 {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i
}

// printfTime is `%(fmt)T`: an epoch through a date format.
//
// The operand is seconds since the epoch, with two numbers that are not
// times: -1 is now and -2 is when this shell started. Both are measured, and
// both are why the corpus pins a case with a *fixed* epoch — a case that
// asked for the current year would record the year it was recorded in.
//
// An empty format is the C locale's time of day, which is what the shell with
// this conversion writes for `%()T`.
func (r *Runner) printfTime(spec, format, arg string, present bool) (string, int, bool) {
	if r.ask(r.sem().PrintfTimeOperandIsADateString, "`printf '%T'` taking a date string") {
		return r.printfDate(spec, format, arg)
	}
	if r.unspecified {
		return "", r.status, true
	}
	var t time.Time
	code := 0
	switch {
	case !present, arg == "-1":
		t = r.Now()
	case arg == "-2":
		t = r.StartedAt()
	default:
		var n int64
		n, code = r.printfNumber(arg, present)
		t = time.Unix(n, 0)
	}
	t = t.In(r.timeZone())
	if format == "" {
		format = "%X"
	}
	// The width and the flags belong to the *result*, not to the date: a
	// `%10(%Y)T` pads the four digits out to ten.
	return fmt.Sprintf(spec+"s", strftime(format, t)), code, false
}

// printfDate is the other reading of `%T`: the operand is a date string, and
// an operand it cannot read is a warning plus the current time rather than a
// refusal — measured, and the status is still 1, so a script can tell.
//
// The default format is the full `date` line rather than the time of day,
// and it is the default for `%()T` as well as for a bare `%T`: both write
// `Sun Sep  9 01:46:40 GMT 2001` for the same instant where the epoch form
// writes `01:46:40`.
func (r *Runner) printfDate(spec, format, arg string) (string, int, bool) {
	t, ok := kshDate(arg, r.Now().In(r.timeZone()))
	code := 0
	if !ok {
		r.diagf("%s\n", Wording(r.diag().PrintfBadDateOperand,
			"printf: warning: invalid argument of type T"))
		code = 1
	}
	if format == "" {
		format = "%a %b %e %H:%M:%S %Z %Y"
	}
	return fmt.Sprintf(spec+"s", strftime(format, t)), code, false
}

// badVerbName is the conversion character a diagnostic names, given where the
// conversion ended.
//
// Two of the panel name that one character — `%v]xY` is reported as `v` in
// both — and two name the whole directive as written, `%v`. The wordings
// differ too, so this hands both spellings over and each dialect takes the
// one it uses.
//
// It reads backwards from the end of the conversion rather than forwards from
// its start, because what is in front of the verb is flags, a width, a
// precision and length modifiers, and the caller has already walked past all
// of them.
func badVerbName(format string, end int) string {
	if end > 0 && end <= len(format) {
		return format[end-1 : end]
	}
	return ""
}

// printfErrorKind is what went wrong, which the panel words four ways each.
type printfErrorKind int

const (
	// printfUsage is `printf` with no format at all.
	printfUsage printfErrorKind = iota
	// printfBadNumber is a numeric conversion given something that is not a
	// number.
	printfBadNumber
	// printfBadVerb is a conversion this shell does not have.
	printfBadVerb
)

// printfBadVerb reports a conversion this shell does not have.
//
// Two verbs, because the panel does not agree on what to name: %[1]s is the
// conversion character alone, which bash and ksh93 report — `%v]xY` is `v`
// in both — and %[2]s is the whole directive as written, `%v`, which dash
// and zsh report. Where a dialect takes length modifiers the two diverge
// further, since the modifier belongs to the directive and is not the
// character: zsh calls `%lQ` exactly that and bash calls it `Q`.
func (r *Runner) printfBadVerb(conversion, verb string) int {
	d := r.diag()
	if i := strings.LastIndexByte(conversion, '%'); i >= 0 {
		conversion = conversion[i:]
	}
	r.diagf("%s\n", Wording(d.PrintfBadVerb, "printf: %[2]s: invalid directive", verb, conversion))
	return orDefault(d.PrintfBadVerbStatus, 1)
}

// printfMissingVerb reports a format that ended before its conversion
// character, which is a different complaint from a conversion nobody has —
// and in one shell a different *wording* of it as well.
//
// One verb: the whole directive as written, `%5` and not `5`, because there
// is no conversion character in it to name. bash uses it, zsh spells its
// ordinary bad-conversion complaint with it, and dash names nothing at all
// and so takes the verb and drops it.
func (r *Runner) printfMissingVerb(conversion string) int {
	d := r.diag()
	if i := strings.LastIndexByte(conversion, '%'); i >= 0 {
		conversion = conversion[i:]
	}
	r.diagf("%s\n", Wording(d.PrintfMissingVerb, "printf: %[1]s: missing format character", conversion))
	return orDefault(d.PrintfMissingVerbStatus, 1)
}

func (r *Runner) printfReport(kind printfErrorKind, operand string) int {
	d := r.diag()
	switch kind {
	case printfBadNumber:
		r.diagf("%s\n", Wording(d.PrintfBadNumber, "printf: %[1]s: invalid number", operand))
		return orDefault(d.PrintfBadNumberStatus, 1)
	case printfBadVerb:
		return r.printfBadVerb(operand, operand)
	}
	usage := Wording(d.PrintfUsage, "printf: usage: printf format [arguments]")
	if d.PrintfUsageUnprefixed {
		r.errf("%s\n", usage)
	} else {
		r.diagf("%s\n", usage)
	}
	return orDefault(d.PrintfUsageStatus, 2)
}

// expandBEscapes expands the escapes a `%b` argument carries, which is a
// different table from the one a format carries and not a subset of it.
//
// The panel is unanimous that the two are separate, even where it disagrees
// about the entries. `\0101` is an `A` in a `%b` argument and a backspace
// followed by a `1` in a format, in all six shells measured — a `%b` reads
// `\0` and up to three octal digits after it, which is the XSI escape `echo`
// expands, where a format reads up to three digits with the zero optional.
// Reading a `%b` with the format's reader produced neither answer (#798).
//
// The table, and where each entry is decided:
//
//	\a \b \f \n \r \t \v \\   the XSI set — unanimous, xsiEscape
//	\0nnn                     one byte, unanimous
//	\c                        ends the output, unanimous — where a format
//	                          makes three different things of the same two
//	                          characters (PrintfBackslashC)
//	\nnn                      PrintfBOctalWithoutZero
//	\e                        PrintfBEscEscape
//	\E                        PrintfBCapitalEscEscape
//	\xHH                      PrintfBHexEscape
//	\uHHHH \UHHHHHHHH       PrintfBUnicodeEscape
//	anything else             the backslash and the character, unanimous
//
// It reports the text and whether a `\c` ended things.
func (r *Runner) expandBEscapes(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			// A backslash with nothing after it is a backslash: unanimous.
			b.WriteByte(s[i])
			i++
			continue
		}
		c := s[i+1]
		if e, ok := xsiEscape(c); ok {
			b.WriteByte(e)
			i += 2
			continue
		}
		switch c {
		case 'c':
			// Not PrintfBackslashC. That axis is the *format*'s question,
			// where the same two characters are literal in bash and dash,
			// control-X in ksh93 and a full stop in zsh; in a `%b` all six
			// stop, so there is nothing to ask.
			return b.String(), true
		case 'e', 'E':
			// One shape, asked twice, because ksh93 has `\E` and not `\e`
			// and zsh has `\e` and not `\E`.
			axis := r.sem().PrintfBEscEscape
			if c == 'E' {
				axis = r.sem().PrintfBCapitalEscEscape
			}
			if r.ask(axis, `printf: \`+string(c)+` in a %b argument`) {
				b.WriteByte(0x1b)
			} else {
				b.WriteByte('\\')
				b.WriteByte(c)
			}
			i += 2
		case 'x':
			text, n := r.hexEscapeText(r.bHexEscape(), s[i:])
			b.WriteString(text)
			i += n
		case 'u', 'U':
			// The format's reader, at the other site and with the other
			// axis. A truncating answer ends the argument's text here
			// rather than the builtin, which is the same "this pass and no
			// further" the format site gets; no dialect in the panel asks
			// for it at this site.
			//
			// A locale with no room for the code point is the same reader's
			// answer at both sites, and it ends the builtin rather than the
			// argument: measured, `printf '%b' 'a\u00e9Z'` under `LC_ALL=C`
			// writes `61` and abandons the script.
			text, n, end := r.unicodeEscapeText(r.bUnicodeEscape(), s[i:])
			b.WriteString(text)
			i += n
			if end == printfPassTruncated {
				return b.String(), false
			}
			if end == printfPassStopped {
				return b.String(), true
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// `\0` introduces up to three octal digits rather than being the
			// first of them, which is what makes `\0101` an `A` and `\01011`
			// an `A` and a `1`. Without the zero it is the same escape only
			// where the dialect says so.
			start := i + 2
			if c != '0' {
				if !r.ask(r.sem().PrintfBOctalWithoutZero, `printf: \nnn in a %b argument`) {
					b.WriteByte('\\')
					b.WriteByte(c)
					i += 2
					break
				}
				start = i + 1
			}
			n, used := scanBase(s[start:], 8, 3)
			b.WriteByte(byte(n))
			i = start + used
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
			i += 2
		}
	}
	return b.String(), false
}

// expandPrintfEscape expands the one escape at the front of a printf *format*.
//
// A `%b` argument is a different table and has expandBEscapes, which is the
// shape #765 measured and #798 finished: the two share the XSI eight and
// nothing else, and reading either one with the other's reader produces
// answers no shell in the panel gives.
func (r *Runner) expandPrintfEscape(s string) (string, int, printfPassEnd) {
	if len(s) < 2 {
		return `\`, len(s), printfPassRan
	}
	if e, ok := xsiEscape(s[1]); ok {
		return string([]byte{e}), 2, printfPassRan
	}
	switch c := s[1]; c {
	case 'x':
		text, n := r.hexEscapeText(r.hexEscape(), s)
		return text, n, printfPassRan
	case 'u', 'U':
		// One reader for both spellings and for both sites, because there is
		// one Unicode escape and not four: the letter says how many digits
		// it may take and nothing else.
		text, n, end := r.unicodeEscapeText(r.unicodeEscape(), s)
		return text, n, end
	case 'c':
		// Three answers, and the middle one is why this is not a bool: ksh93
		// reads `\cX` as control-X, which *looks* like truncation next to
		// zsh's stopping until the bytes are read.
		switch r.backslashC() {
		case PrintfBackslashCStops:
			return "", 2, printfPassStopped
		case PrintfBackslashCControl:
			// The same escape `$'…'` decodes, read the same way: the
			// dialect whose printf reads `\cX` as a control character is
			// the one that toggles bit 6 there, and it decodes the
			// argument before controlling it. Writing that arithmetic a
			// second time here is what let the two drift — `\c1` was
			// `0x11` in a format and `q` inside the quotes (#556).
			x, next, ok := controlArgument(DollarSingleControlToggled, s, 2)
			if !ok {
				// `\c` with nothing after it is a NUL. printf writes it
				// rather than ending there, because a format is a counted
				// string and not a C one.
				return "\x00", 2, printfPassRan
			}
			return string([]byte{controlByte(DollarSingleControlToggled, x)}), next, printfPassRan
		}
		return `\c`, 2, printfPassRan
	case '0', '1', '2', '3', '4', '5', '6', '7':
		// An octal escape, up to three digits after an optional leading zero.
		digits := 0
		n := 0
		for i := 1; i < len(s) && digits < 3; i++ {
			if s[i] < '0' || s[i] > '7' {
				break
			}
			n = n*8 + int(s[i]-'0')
			digits++
		}
		// A byte, not a code point. `string(rune(0300))` is the two bytes
		// UTF-8 spells U+00C0 with, and a format is a byte string: `\300`
		// is 0xc0 alone in every shell in the panel.
		return string([]byte{byte(n)}), 1 + digits, printfPassRan
	}
	return `\` + string(s[1]), 2, printfPassRan
}

// hexEscapeText decodes the `\x` at the front of s under one of the four
// readings, and is where both sites that have the escape meet: a format asks
// PrintfHexEscape for its policy and a `%b` argument asks PrintfBHexEscape,
// and ksh93 answers the two differently — but a shell that has the escape at
// a site reads its digits there the way it reads a format's, so there is one
// reader and two answers rather than two readers.
//
// The digits are scanned with the same reader `$'…'` uses, because there is
// one hexadecimal escape and not two — the lesson #556 left, one escape
// further along.
func (r *Runner) hexEscapeText(p PrintfHexEscapePolicy, s string) (string, int) {
	if p == PrintfHexEscapeAbsent || r.unspecified {
		return `\x`, 2
	}
	n, used := hexEscapeRun(s[2:], p == PrintfHexEscapeCodePoint)
	switch {
	case used == 0 && p == PrintfHexEscapeByte:
		// The escape stands, with a warning that does not change the status:
		// `printf 'a\x'; echo $?` writes the complaint, the two characters,
		// and a zero.
		d := r.diag()
		r.diagf("%s\n", Wording(d.PrintfMissingHexDigit, `printf: missing hex digit for \x`))
		return `\x`, 2
	case used == 0:
		// An empty digit run is a zero, and neither site ends at a NUL, so
		// the byte is written rather than stopping anything.
		return "\x00", 2
	case used <= 2:
		return string([]byte{byte(n)}), 2 + used
	}
	// A run past the last code point is *encoded* rather than refused, in
	// the extended form UTF-8 has room for. This used to answer nothing for
	// one, on the strength of a measurement that was not taken: re-measured
	// 2026-09-12 under `LC_ALL=C`, `printf '\x41414141'` on ksh93u+ writes
	// the six bytes fd 81 90 94 85 81, and `printf '\x110000b'` the five
	// f9 84 80 80 8b. A run too long for the value to hold keeps the low
	// bits, so `\x41414141414141414141` is the same six bytes as
	// `\x41414141` — which is what the scan's own overflow already does.
	return EncodeCodePoint(n), 2 + used
}

// unicodeEscapeText decodes the `\u` or `\U` at the front of s under one of the
// four readings, and is where both sites that have the escape meet: a format
// asks PrintfUnicodeEscape for its policy and a `%b` argument asks
// PrintfBUnicodeEscape, and ksh93 answers the two differently.
//
// One reader and one encoder, deliberately. The digits are scanned with the
// reader `$'…'` and the hexadecimal escape already use, and the value is
// written by EncodeCodePoint, which `echo` uses for the same escape — a
// second copy of either is how `\c1` came to mean two things (#556) and how
// `print` came to write a replacement character where the shell it follows
// writes the encoding (#1840).
//
// The letter decides only how many digits may follow: four after `\u` and
// eight after `\U`, with a shorter run accepted and ended by the first
// character that is not a digit.
func (r *Runner) unicodeEscapeText(p PrintfUnicodeEscapePolicy, s string) (string, int, printfPassEnd) {
	escape := s[:2]
	if p == PrintfUnicodeEscapeAbsent || r.unspecified {
		return escape, 2, printfPassRan
	}
	width := 4
	if s[1] == 'U' {
		width = 8
	}
	n, used := scanBase(s[2:], 16, width)
	if used == 0 {
		switch p {
		case PrintfUnicodeEscapeCodePointOrNul:
			// An empty digit run is a zero, and neither site ends at a NUL,
			// so the byte is written rather than stopping anything.
			return "\x00", 2, printfPassRan
		case PrintfUnicodeEscapeCodePointOrTruncate:
			// The rest of this pass over the format goes unwritten — and the
			// pass only, so the operands that are left get another one.
			return "", 2, printfPassTruncated
		}
		// The escape stands, with a warning that does not change the status:
		// `printf 'a\u'; echo $?` writes the complaint, the two characters,
		// and a zero. The letter is a verb so that one wording covers both.
		d := r.diag()
		r.diagf("%s\n", Wording(d.PrintfMissingUnicodeDigit,
			`printf: missing unicode digit for \%s`, string(s[1])))
		return escape, 2, printfPassRan
	}
	// The locale is consulted before anything is written, which is the same
	// question `echo` asks and the same reader: the character, the escape
	// written back, or a refusal — see [Runner.CodePointEscapeText]. A code
	// point the encoding has room for is written in UTF-8, and the original
	// UTF-8 at that: a surrogate and a value past the last code point are
	// encoded rather than refused, which is measured and not assumed.
	text, refused := r.CodePointEscapeText(n)
	if refused {
		// The complaint, what came before the escape, and nothing after it —
		// in this pass or in any later one, since the format is reused until
		// the operands run out and a refusal ends the builtin rather than
		// the pass. Measured 2026-09-11 under `LC_ALL=C`: `printf 'a\u00e9Z\n'`
		// writes `61` alone, without the newline the format ends with, and
		// the next command does not run.
		r.RefuseCodePoint()
		return "", 2 + used, printfPassStopped
	}
	return text, 2 + used, printfPassRan
}

// printfWriter is the shell's output stream, held back or written through
// depending on which the dialect does.
type printfWriter struct {
	w    io.Writer
	r    *Runner
	hold *strings.Builder
}

func (p printfWriter) WriteString(s string) {
	if s == "" {
		return
	}
	if p.hold != nil {
		p.hold.WriteString(s)
		return
	}
	p.write(s)
}

// writeByte is not WriteByte: that name carries an error return by
// convention, and this writer reports one the way every builtin's output
// does — on the runner, for the dispatcher to fold in.
//
// The conversion is through a one-byte slice and not through `string(c)`,
// which is a *rune* conversion: it spells 0xc0 as the two bytes UTF-8 gives
// U+00C0, so a format holding a byte no encoding claims came out as two.
func (p printfWriter) writeByte(c byte) { p.WriteString(string([]byte{c})) }

func (p printfWriter) flush() {
	if p.hold != nil {
		p.write(p.hold.String())
	}
}

func (p printfWriter) write(s string) {
	if s == "" {
		// Nothing to write cannot fail to be written: `printf '' >&-`
		// succeeds in every shell measured.
		return
	}
	if _, err := io.WriteString(p.w, s); err != nil {
		p.r.writeFailed = err
	}
}

// printfWritesThrough reports whether output goes out as it is produced.
//
// Asked without a description because it decides nothing a script can see
// except the order two streams arrive in, and refusing a whole `printf` over
// it would be worse than picking the majority.
func (r *Runner) printfWritesThrough() bool {
	return r.sem().PrintfOutputPrecedesComplaint == Yes
}
