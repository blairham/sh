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
		used, code, stop := r.printfOnce(format, operands)
		if code != 0 {
			status = code
		}
		if stop {
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

// printfOnce runs the format through once, returning how many operands it
// consumed, the status of any complaint, and whether output stopped early.
func (r *Runner) printfOnce(format string, operands []string) (int, int, bool) {
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
			text, n, stop := r.expandPrintfEscape(format[i:])
			b.WriteString(text)
			i += n
			if stop {
				return used, status, true
			}
		case c != '%':
			b.writeByte(c)
			i++
		default:
			spec, verb, timeFmt, n, code := r.scanPrintfSpec(format[i:])
			if code != 0 {
				return used, code, true
			}
			i += n
			if verb == '%' {
				b.writeByte('%')
				continue
			}
			if verb == 0 {
				return used, r.printfBadVerb(format[:i], badVerbName(format, i), format[i:]), true
			}
			text, code, stop := r.printfVerb(spec, verb, timeFmt, next)
			if code != 0 {
				status = code
			}
			b.WriteString(text)
			if stop {
				return used, status, true
			}
		}
	}
	return used, status, false
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
		text, _, _ := r.expandPrintfEscapes(arg)
		return fmt.Sprintf(spec+"s", text), 0, false
	case 'c':
		if arg == "" {
			return "", 0, false
		}
		return fmt.Sprintf(spec+"c", rune(arg[0])), 0, false
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
// Three answers and one absence, which is why it is a policy: bash and zsh
// escape each character that needs it, ksh93 wraps the whole word in single
// quotes, and dash does not have the verb at all.
func (r *Runner) printfQuote(spec, arg string) (string, int, bool) {
	switch r.quoteStyle() {
	case PrintfQuoteBackslash:
		return fmt.Sprintf(spec+"s", backslashQuote(arg)), 0, false
	case PrintfQuoteSingle:
		return fmt.Sprintf(spec+"s", singleQuote(arg)), 0, false
	case PrintfQuoteAbsent:
		// A conversion the shell does not have stops the output where it is,
		// as any other unknown one does.
		return "", r.printfBadVerb("%q", "q", ""), true
	}
	return "", r.status, true
}

// backslashQuote escapes what the shell would otherwise read as syntax.
func backslashQuote(s string) string {
	if s == "" {
		return "''"
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '"', '\'', '\\', '$', '`', '&', '|', ';',
			'(', ')', '<', '>', '*', '?', '[', ']', '{', '}', '~', '!', '#', '^':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
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
	verb := s[i]
	if strings.IndexByte("sbcqdiouxXfeEgG%", verb) < 0 {
		return spec, 0, "", i + 1, 0
	}
	return spec, verb, "", i + 1, 0
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

// badVerbName is what a diagnostic calls a conversion it does not have.
//
// Two of the panel name the character *after* the one they could not read —
// `%z]` is reported as `]` — and two name the conversion as written. The
// wordings differ too, so this hands both spellings over and each dialect
// takes the one it uses.
func badVerbName(format string, after int) string {
	if after < len(format) {
		return format[after : after+1]
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
// Three verbs, because the panel does not agree on what to name: %[1]s is
// the character *after* the one it could not read, which bash reports;
// %[2]s is the conversion as written, which dash and zsh report; and %[3]s
// is the rest of the format after the conversion with its escapes already
// expanded, which is ksh93's answer — for `%z\n` it names a newline, so its
// complaint really does end in two colons on two lines.
func (r *Runner) printfBadVerb(conversion, next, rest string) int {
	d := r.diag()
	if i := strings.LastIndexByte(conversion, '%'); i >= 0 {
		conversion = conversion[i:]
	}
	rest, _, _ = r.expandPrintfEscapes(rest)
	r.diagf("%s\n", Wording(d.PrintfBadVerb, "printf: %[2]s: invalid directive", next, conversion, rest))
	return orDefault(d.PrintfBadVerbStatus, 1)
}

func (r *Runner) printfReport(kind printfErrorKind, operand string) int {
	d := r.diag()
	switch kind {
	case printfBadNumber:
		r.diagf("%s\n", Wording(d.PrintfBadNumber, "printf: %[1]s: invalid number", operand))
		return orDefault(d.PrintfBadNumberStatus, 1)
	case printfBadVerb:
		return r.printfBadVerb(operand, operand, "")
	}
	usage := Wording(d.PrintfUsage, "printf: usage: printf format [arguments]")
	if d.PrintfUsageUnprefixed {
		r.errf("%s\n", usage)
	} else {
		r.diagf("%s\n", usage)
	}
	return orDefault(d.PrintfUsageStatus, 2)
}

// expandPrintfEscapes expands the escapes `printf` understands, which is a
// longer list than `echo`'s and includes the one that stops output.
//
// It reports the text, how much of the input it read, and whether `\c` ended
// things — which is two ordinary characters in half the panel, so the caller
// asks the dialect before believing it.
func (r *Runner) expandPrintfEscapes(s string) (string, int, bool) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		text, n, stop := r.expandPrintfEscape(s[i:])
		b.WriteString(text)
		i += n
		if stop {
			return b.String(), i, true
		}
	}
	return b.String(), len(s), false
}

// expandPrintfEscape expands the one escape at the front of s.
func (r *Runner) expandPrintfEscape(s string) (string, int, bool) {
	if len(s) < 2 {
		return `\`, len(s), false
	}
	switch c := s[1]; c {
	case 'n':
		return "\n", 2, false
	case 't':
		return "\t", 2, false
	case 'r':
		return "\r", 2, false
	case 'a':
		return "\a", 2, false
	case 'b':
		return "\b", 2, false
	case 'f':
		return "\f", 2, false
	case 'v':
		return "\v", 2, false
	case '\\':
		return `\`, 2, false
	case 'c':
		// Three answers, and the middle one is why this is not a bool: ksh93
		// reads `\cX` as control-X, which *looks* like truncation next to
		// zsh's stopping until the bytes are read.
		switch r.backslashC() {
		case PrintfBackslashCStops:
			return "", 2, true
		case PrintfBackslashCControl:
			if len(s) > 2 {
				// Control-X is the letter with its top bits cleared, and a
				// `\c` with nothing after it is a NUL.
				return string(rune(s[2] & 0x1f)), 3, false
			}
			return "\x00", 2, false
		}
		return `\c`, 2, false
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
		return string(rune(n)), 1 + digits, false
	}
	return `\` + string(s[1]), 2, false
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
func (p printfWriter) writeByte(c byte) { p.WriteString(string(c)) }

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
