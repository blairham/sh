// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
// what the rest of this file is about. All of them agree that the format is
// *reused* until the arguments run out, that `%b` expands escapes in its
// argument and `%s` does not, that escapes in the format itself are always
// expanded, and on `%c`, widths — including the `*` that takes one from the
// operand list (#2646) — precisions, `%%` and octal escapes.
//
// A missing argument is the empty string or zero rather than an error in four
// of the five, and that sentence used to say "all". ash is the fifth: it
// reads a numeric conversion with nothing left as a conversion of the empty
// string, complaint and all (#2648). The paragraph above was written against
// a four-shell panel and the fifth column is what found it, which is what
// that column is for.
//
// Five things they do not agree on, and each is an axis or a wording rather
// than a branch here:
//
//   - A `%d` given something that is not a number. bash and dash complain and
//     report failure; ksh93 and zsh print zero and say nothing. Both still
//     print the zero.
//   - `%q`. bash and zsh backslash-escape, ksh93 single-quotes, and dash does
//     not have it at all.
//   - `\c` in the format, which stops output there in ksh93 and zsh and is
//     two ordinary characters in bash and dash.
//   - The `'` flag, which asks for a number's digits to be grouped the way
//     the locale groups them. bash, zsh and ksh93 have it; dash and ash do
//     not, and for them the character is the conversion it is refused as.
//     ksh93 also reads it later in the prefix than the other two do (#2665).
//   - What an unknown verb is called, and what `printf` with no format says.

func init() {
	builtins["printf"] = biPrintf
}

func biPrintf(r *Runner, _ context.Context, args []string) (status int) {
	args, assign, code := r.printfOptions(args)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		return r.printfReport(printfUsage, "")
	}
	format, operands := args[0], args[1:]

	// An output parameter that is not a name is refused before anything is
	// formatted, and ahead of the freeze, which is the order the operand
	// forces: a word that cannot be a name is not a name anything could have
	// frozen. `read` has judged its operands since #1440 and this builtin
	// judged nothing at all — it stored under whatever word it was handed and
	// reported 0, so `printf -v '1x' %s Q` left a parameter no expansion can
	// read back and said nothing about it, where both columns that have `-v`
	// refuse (#3515). See Runner.isPrintfName and
	// Semantics.BadNameToPrintfFatal.
	if assign != "" && !r.isPrintfName(assign) {
		if r.unspecified {
			return 2
		}
		return r.badPrintfName(assign)
	}

	// A frozen output parameter is refused before anything is formatted, and
	// the freeze is asked of the *name a subscript belongs to* — the same
	// question `read` asks through the same function, because it is the same
	// question: a builtin writing to its own output parameter. Measured on
	// bash 5.3.20, which refuses `printf -v s`, `printf -v 'a[0]'` and
	// `printf -v 'm[k]'` alike with the base name in the sentence, at status
	// 1, and writes nothing (#3469).
	if assign != "" && !r.readMayWrite(assign) {
		return 1
	}

	// `-v name` collects the text instead of printing it. Swapped rather than
	// threaded through, because everything below writes to r.stdout() and the
	// format is reused in a loop.
	if assign != "" {
		var into strings.Builder
		saved := r.Stdout
		r.Stdout = &into
		defer func() {
			r.Stdout = saved
			// Through the operand store and not setVar, because the name may
			// carry a subscript: `printf -v 'q[1]'` fills an element and
			// `printf -v 'm[k]'` a keyed one. setVar made a *scalar* whose
			// name was the six characters `q[1]`, so the array a script then
			// read was untouched and the builtin reported 0 — measured
			// against bash 5.3.20, which fills the element in both
			// containers. It is the same route `read 'a[2]'` takes, for the
			// same reason (#2298).
			//
			// A subscript that will not evaluate is the builtin's status as
			// well as the store's complaint, which is why the return is named
			// — measured 2026-09-17, `r=(1 2 3); printf -v 'r[1/0]' %s Q` is
			// status 1 with the array untouched in bash 5.3.20, where this
			// reported the formatting's 0 over a store that never happened.
			if st, refused := r.storeThroughOperand(assign, into.String()); refused {
				status = st
			}
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
	// followed. The others hold to the end, so their complaint reaches the
	// reader first — their output is still in a buffer when it goes out.
	b := &printfWriter{w: r.stdout(), r: r, through: r.printfWritesThrough()}
	// On the runner for the length of the pass, because two things outside
	// this loop need it: every diagnostic reveals what has been produced
	// before it is written, and the one refusal that takes a pass back
	// rewinds from inside the star reader. Saved and put back rather than
	// cleared, so a pass is never left holding another's writer.
	saved := r.printfOut
	r.printfOut = b
	defer func() { r.printfOut = saved }()
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
			// The conversion is a window: what it produces can still be
			// taken back until it resolves, and what it *says* waits for it.
			// See printfWriter.
			mark := b.mark()
			b.begin()
			spec, verb, timeFmt, n, code := r.scanPrintfSpec(format[i:])
			if code != 0 {
				b.end(mark, false)
				return used, code, printfPassStopped
			}
			unfinished := format[i : i+n]
			i += n
			if verb == '%' {
				b.end(mark, false)
				b.writeByte('%')
				continue
			}
			if verb == 0 {
				if spec != "" {
					// The complaint before the close, because it is what
					// tells the writer to let out the text in front of this
					// conversion. See printfWriter.end.
					code := r.printfBadVerb(format[:i], badVerbName(format, i))
					b.end(mark, false)
					return used, code, printfPassStopped
				}
				// An empty prefix is a format that ran out before it
				// reached a conversion character — `%`, `%5`, `%ll` at the
				// end. There is no character to name, and one shell does not
				// treat it as an error at all: it writes a bare `%` for the
				// whole unfinished conversion, prefix and all, and succeeds.
				if r.ask(r.sem().PrintfUnfinishedConversionIsAPercent, "a format that ends inside a conversion") {
					// The one dialect that writes a bare `%` for it. A `*`
					// in the part that was written still takes its operand,
					// even though the conversion never completed — so the
					// format is reused once per star's worth of operands
					// where `%` and `%5` consume nothing and end the
					// builtin after one pass (#2667).
					code, stop := r.printfUnfinishedStars(unfinished, next)
					if stop {
						b.end(mark, false)
						return used, code, printfPassStopped
					}
					b.end(mark, false)
					b.writeByte('%')
					// Truncated and not stopped, so the loop over the
					// operands runs again. A format with no star reads
					// nothing here, and the loop's own guard against a pass
					// that consumed nothing is what ends the builtin then.
					return used, 0, printfPassTruncated
				}
				if r.unspecified {
					b.end(mark, false)
					return used, r.status, printfPassStopped
				}
				code := r.printfMissingVerb(format[:i])
				b.end(mark, false)
				return used, code, printfPassStopped
			}
			took := used
			text, code, stop := r.printfVerb(spec, verb, timeFmt, next)
			if code != 0 {
				status = code
			}
			b.WriteString(text)
			// Completed means it reached the operand list and finished, which
			// is what a rewind stops at. A conversion that stopped did not,
			// however many star operands it read on the way.
			b.end(mark, !stop && used > took)
			if stop {
				return used, status, printfPassStopped
			}
		}
	}
	return used, status, printfPassRan
}

// printfVerb formats one conversion, resolving the width and the precision a
// `*` stands for before the operand being converted is read.
//
// The order is the whole of it: `printf '%*.*f' 10 2 3.14159` takes the width,
// then the precision, then the value, and a fix that resolved one star would
// pass `%*d` and `%.*s` and still get that one wrong.
func (r *Runner) printfVerb(spec string, verb byte, timeFmt string, next func() (string, bool)) (string, int, bool) {
	starCode := 0
	lost := r.printfLostStars
	r.printfLostStars = printfLostStars{}
	if strings.IndexByte(spec, '*') >= 0 || lost.any() {
		// Guarded, so an ordinary `printf '%d' 5` never reaches the star
		// code and never consults the axis inside it.
		var stop bool
		if spec, starCode, stop = r.printfStars(spec, lost, next); stop {
			return "", starCode, true
		}
	}
	text, code, stop := r.printfConvert(spec, verb, timeFmt, next)
	if code == 0 {
		// The star's complaint still stands where the conversion itself had
		// nothing to say: `printf '%*s' abc hi` writes `hi` and reports.
		code = starCode
	}
	return text, code, stop
}

// printfStars replaces each `*` in a conversion's prefix with the operand it
// takes, returning the prefix as if those widths had been written out.
//
// Unanimous across the whole panel — bash 5.3, bash as sh, bash 3.2, zsh,
// ksh93, dash and BusyBox ash — in three respects, measured 2026-09-13:
// one operand per star and in written order, a negative width meaning the `-`
// flag and the width without its sign, and a negative precision meaning no
// precision at all rather than a zero one. `printf '%.*s' -3 hello` is
// `hello` in all seven, where a precision of zero would be the empty string.
func (r *Runner) printfStars(spec string, lost printfLostStars, next func() (string, bool)) (string, int, bool) {
	i := 1 // past the %
	for i < len(spec) && strings.IndexByte("-+ #0", spec[i]) >= 0 {
		i++
	}
	flags, status := spec[1:i], 0
	// The stars a later run in the same field replaced. Their operands are
	// taken here, in the place they were written, and then dropped: see
	// printfLostStars. Only the ksh93 reading produces any.
	code, stop := r.printfDropStars(lost.width, next)
	if stop {
		return "", code, true
	}
	if code != 0 {
		status = code
	}
	width := ""
	if i < len(spec) && spec[i] == '*' {
		i++
		n, absent, code, stop := r.printfStarOperand(next)
		if stop {
			return "", code, true
		}
		if code != 0 {
			status = code
		}
		if absent {
			// No width at all, which is what the out-of-range reading
			// leaves behind — and what the zero below writes as nothing.
			n = 0
		} else if n < 0 {
			// C's rule, and the panel's: a negative width is the `-` flag
			// and the magnitude. Added only where the flag is not already
			// there, because `%--6d` is not a spelling Go's fmt reads.
			if !strings.ContainsRune(flags, '-') {
				flags += "-"
			}
			if n = -n; n < 0 {
				// The one value whose sign cannot be dropped: negating it
				// wraps back to itself, which would put a `-` where the
				// width goes. Clamped, so it stays a width nobody meant
				// rather than a spec nothing can read.
				n = 1<<63 - 1
			}
		}
		if n != 0 {
			// A zero width is written as no width rather than as `0`: with
			// no flags before it that digit *is* the zero-padding flag, and
			// the two mean the same thing here only by accident.
			width = strconv.FormatInt(n, 10)
		}
	} else {
		start := i
		for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
			i++
		}
		width = spec[start:i]
	}
	prec := ""
	if code, stop := r.printfDropStars(lost.prec, next); stop {
		return "", code, true
	} else if code != 0 {
		status = code
	}
	if i < len(spec) && spec[i] == '.' {
		i++
		if i < len(spec) && spec[i] == '*' {
			i++
			n, absent, code, stop := r.printfStarOperand(next)
			if stop {
				return "", code, true
			}
			if code != 0 {
				status = code
			}
			if n >= 0 && !absent {
				prec = "." + strconv.FormatInt(n, 10)
			}
		} else {
			start := i
			for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
				i++
			}
			prec = "." + spec[start:i]
		}
	}
	return "%" + flags + width + prec + spec[i:], status, false
}

// printfDropStars reads and throws away the operands of n stars a later run
// in the same field replaced, which only the ksh93 prefix grammar can write.
//
// Read exactly as a surviving star's operand is, complaints and refusal and
// all, because that is what the reference does: `printf "[%*'5d]" abc 42`
// earns the same arithmetic complaint a `%*d` would, and a list that has run
// out is the same refusal.
func (r *Runner) printfDropStars(n int, next func() (string, bool)) (int, bool) {
	status := 0
	for range n {
		_, _, code, stop := r.printfStarOperand(next)
		if stop {
			return code, true
		}
		if code != 0 {
			status = code
		}
	}
	return status, false
}

// printfStarOperand reads the operand a `*` takes, which is the same number
// the conversion itself would read and so carries the same complaints.
//
// The operand list running out is where this parts from the conversion. A
// star with nothing left is a silent zero in six of the seven, ash included —
// and ash is the column that *does* complain about an absent operand at the
// conversion, so the two cases are not one question. ksh93 is the seventh and
// refuses the directive outright.
func (r *Runner) printfStarOperand(next func() (string, bool)) (n int64, absent bool, code int, stop bool) {
	// ksh93 names `.` for a `*` operand in the second complaint line as well
	// as in the refusal below, which is the same constant seen twice.
	saved := r.printfConversionName
	r.printfConversionName = "."
	defer func() { r.printfConversionName = saved }()
	arg, present := next()
	if !present {
		if r.ask(r.sem().PrintfStarWithoutOperandIsRefused, "`printf '%*d'` refusing a `*` the operands ran out before") {
			// The refusal takes the pass back with it, which is the half
			// #2664 was filed for: ksh93's stdout for `printf 'AB%sCD%*dEF' q`
			// is `AB` and not `ABqCD`. The rewind reaches the start of the
			// last conversion that completed, and printfOnce has been
			// committing everything before that as it went — so there is
			// nothing to measure here, only what is still pending to drop.
			//
			// Before the complaint and not after, because the complaint
			// reveals what is pending on its way out.
			if r.printfOut != nil {
				r.printfOut.rewind()
			}
			// ksh93 names `.` whatever the conversion was — `%*s` and
			// `%*.*f` both report `.` — so the name is the constant it
			// measured as rather than anything read out of the format.
			return 0, false, r.printfBadVerb(".", "."), true
		}
		if r.unspecified {
			return 0, false, r.status, true
		}
		return 0, false, 0, false
	}
	n, code, stop = r.printfNumber(arg, true)
	if stop {
		return n, false, code, true
	}
	if code == 0 && (n > printfFieldMax || n < math.MinInt32) {
		// A number the shell read perfectly well and the width cannot hold.
		// Settled here rather than where the format's own digits are
		// settled, because the panel answers the two routes differently —
		// see Semantics.PrintfStarBeyondAnInt.
		switch r.starReading() {
		case PrintfStarWrapsToAnInt:
			w, left := printfFieldWrap(n)
			n = int64(w)
			if left {
				n = -n
			}
		case PrintfStarIsOutOfRange:
			// The field goes *absent* rather than to zero, and the pass
			// carries on: `printf 'A[%*s]B' 21474836470 x` is `A[x]B` and
			// `printf 'A[%.*f]B' 21474836470 1` is `A[1.000000]B` — the
			// default six places, which is what an omitted precision means
			// and a precision of nought does not.
			code, n, absent = r.printfOutOfRange(arg), 0, true
		case PrintfStarIsNotANumber:
			// The same complaint, and the zero an unreadable number leaves
			// rather than an absence: `printf '[%.*f]' 21474836470 1` is
			// `[1]` here, which is this column's answer for `abc` as well.
			// printfOutOfRange words it from the bad-number sentence, since
			// Diagnostics.PrintfNumberOutOfRange is empty in this column.
			code, n = r.printfOutOfRange(arg), 0
		default:
			return 0, false, r.status, true
		}
	}
	if code != 0 && !r.ask(r.sem().PrintfStarComplaintCostsTheStatus, "a `printf` complaint about a `*` operand reporting failure") {
		// ash alone writes the complaint and reports success anyway:
		// `printf '%*s' abc hi` is `hi` on stderr's evidence and 0 on the
		// status's. Asked only where there is a complaint to cost anything,
		// so the two dialects that never complain are never questioned.
		if r.unspecified {
			return n, absent, r.status, true
		}
		code = 0
	}
	return n, absent, code, false
}

// printfUnfinishedStars reads the operands the stars of an *unfinished*
// conversion take — a format that ended before its conversion character, with
// a `*` somewhere in the part that was written.
//
// It is reached only in the dialect whose answer to
// PrintfUnfinishedConversionIsAPercent is yes, because the other four refuse
// the unfinished conversion outright and the pass is over before any operand
// is looked at.
//
// The star takes its operand even though the conversion never completes,
// which is the whole of #2667: `printf 'a%*' 5 9` is `a%a%` in ksh93 and
// `printf 'a%' 5 9` and `printf 'a%5' 5 9` are `a%`. One operand per star and
// in written order, exactly as a finished conversion reads them —
// `printf 'a%*.*' 5 9 7 3` is `a%a%` — and the list running out is the same
// refusal, rewind and all: `printf 'a%*'` with no operands writes nothing at
// all and reports 1.
//
// So this is printfStarOperand in a loop rather than a reader of its own. A
// second one beside it is how the refusal, its status and its rewind would
// reach a finished conversion and not this one.
func (r *Runner) printfUnfinishedStars(unfinished string, next func() (string, bool)) (int, bool) {
	status := 0
	for _, c := range []byte(unfinished) {
		if c != '*' {
			continue
		}
		_, _, code, stop := r.printfStarOperand(next)
		if stop {
			return code, true
		}
		if code != 0 {
			status = code
		}
	}
	return status, false
}

// printfFmtFieldCeiling is the widest field Go's `fmt` will render. Past it
// the package writes its own error text — `%!(NOVERB)%!(EXTRA …)` — into the
// output instead of the field, which is a diagnostic aimed at a Go programmer
// arriving in a shell script's stdout.
//
// Measured by bisection 2026-09-14: `fmt.Sprintf("%10000009d", 1)` is ten
// million characters and `%10000010d` is 25 characters of complaint.
//
// It bounds a width and a precision alike, because `fmt` reads both with one
// scanner — `parsenum` gives up once the number it has accumulated is past
// 1e6, which is why the ceiling is a seven-digit prefix and a spare digit
// rather than a round number. So the name says field and not width: the
// precision side is #3017 and the width side was #2663, and one constant is
// how a measurement of the boundary reaches both.
const printfFmtFieldCeiling = 10000009

// printfSpecParts reads a settled spec into the flags, the width and the
// precision it carries. A precision of -1 is one the spec does not write,
// which is C's "omitted" and not a zero — `%.f` writes a zero and is 0 here.
//
// Every `*` has been replaced by the operand it took before this is reached,
// so the runs are digits, and anything past a C int has already been settled
// by printfFieldBeyondAnInt — so the numbers fit.
func printfSpecParts(spec string) (flags string, width, prec int) {
	i := 1 // past the %
	i += runOfBytes(spec, i, "-+ #0'")
	flags = spec[1:i]
	j := i + printfFieldRun(spec, i)
	width, _ = strconv.Atoi(spec[i:j])
	prec = -1
	if j < len(spec) && spec[j] == '.' {
		j++
		// Atoi of the empty run answers zero, which is exactly what a `.`
		// with no digits after it means.
		prec, _ = strconv.Atoi(spec[j : j+printfFieldRun(spec, j)])
	}
	return flags, width, prec
}

// printfWidePrecision reports a precision this shell has to honor itself
// because `fmt` will not — the band between printfFmtFieldCeiling and the C
// int printfFieldBeyondAnInt settles, which used to fall straight through and
// put Go's `%!(NOVERB)` on the shell's stdout at status 0 (#3017).
//
// printfWideField is the width's half of this and takes the spec apart
// itself, because what it does about a wide width is rewrite the spec. This
// one cannot: a precision means something different for every family of verb
// — truncation for a string, minimum digits for an integer, digits after the
// point for a float — so the callers are where the value is, and each one
// lays its own field out.
func printfWidePrecision(spec string) bool {
	_, _, prec := printfSpecParts(spec)
	return prec > printfFmtFieldCeiling
}

// printfByteField lays a conversion whose field is a string out through
// `fmt`, counting the width and the precision in **bytes**, and honoring a
// precision past what `fmt` renders here instead.
//
// Bytes, because that is C's `printf` and every column but one: `printf
// '[%.2s]' αβγ` is `[α]` — two bytes, one character — and `printf '[%7s]'
// αβγ` pads the six bytes with one space, in bash 5.3.20, bash 3.2.57,
// ksh93u+, dash 0.5.12 and BusyBox ash 1.37.0, under `LC_ALL=C` and under a
// UTF-8 locale alike (measured 2026-09-16). Go's `fmt` counts runes, which is
// the other column's answer — see Semantics.PrintfFieldCountsCharacters —
// and it was the answer this shell gave everywhere, so a UTF-8 operand came
// out longer and padded shorter than any of the five.
//
// A string conversion's precision only truncates, so one past the operand's
// length is a no-op — which is why the wide band is exact and costs nothing:
// the text is cut at the precision and the precision then leaves the spec,
// so `fmt` is left with a width it can render. `printf '[%.10000010s]' xyz`
// is `[xyz]` in bash 5.3 and is `[xyz]` here.
//
// The width stays with `fmt`, because printfWideField has already taken out
// any width `fmt` would refuse. It is handed an ASCII stand-in of the text's
// length in bytes, so `fmt`'s own flags — `-`, and `0`, which it honors for a
// string — lay the field out exactly as they always have.
func printfByteField(spec, text string) string {
	_, _, prec := printfSpecParts(spec)
	if isASCII(text) && prec <= printfFmtFieldCeiling {
		return fmt.Sprintf(spec+"s", text)
	}
	if prec >= 0 && prec < len(text) {
		text = text[:prec]
	}
	return printfFieldAround(printfWithoutPrecision(spec), text, len(text))
}

// printfCharacterField is printfByteField counting **characters**: the
// precision cuts after that many characters and the width pads to that many.
//
// Only reached where a dialect says so and the locale decodes characters at
// all — see printfFieldUnit. A byte that does not begin a valid sequence is a
// character of its own here, which is what `fmt` does with one; what the
// column that reaches this through the `l` modifier does with such a byte is
// something else again and is not modeled.
func printfCharacterField(spec, text string) string {
	_, _, prec := printfSpecParts(spec)
	if prec >= 0 && prec < utf8.RuneCountInString(text) {
		cut, n := 0, 0
		for n < prec {
			_, size := utf8.DecodeRuneInString(text[cut:])
			cut += size
			n++
		}
		text = text[:cut]
	}
	return printfFieldAround(printfWithoutPrecision(spec), text, utf8.RuneCountInString(text))
}

// printfFieldAround lays text out in the field spec describes as though it
// were units long, by letting `fmt` lay out a stand-in of that length and
// putting the text where the stand-in landed. spec carries no precision.
func printfFieldAround(spec, text string, units int) string {
	stand := strings.Repeat("x", units)
	field := fmt.Sprintf(spec+"s", stand)
	at := strings.Index(field, stand)
	return field[:at] + text + field[at+len(stand):]
}

// printfStringField lays out the field of a conversion whose operand is text
// — `%s`, `%b` and a date — in the unit the dialect and the locale name.
//
// long says the conversion carried the `l` length modifier, which is a
// question of its own in one column: see printfFieldUnit. unanswered reports
// an axis nothing answered, and the conversion then stops.
func (r *Runner) printfStringField(spec, text string, long bool) (field string, unanswered bool) {
	chars, unanswered := r.printfFieldUnit(spec, text, long)
	if unanswered {
		return "", true
	}
	if chars {
		return printfCharacterField(spec, text), false
	}
	return printfByteField(spec, text), false
}

// printfFieldUnit reports whether a string conversion's width and precision
// count characters rather than bytes.
//
// Two axes, and the `l` modifier's is asked first because it is the more
// specific: where it answers No the conversion falls back to the plain one's
// reading, which is exactly what a column that ignores the letter does.
//
// Asked only where the answer changes what is written: an operand that is all
// ASCII is the same length either way, and a conversion with neither a width
// nor a precision writes the operand whole. Then the locale, last, so a
// dialect that counts bytes is never asked what the locale decodes.
func (r *Runner) printfFieldUnit(spec, text string, long bool) (chars, unanswered bool) {
	if isASCII(text) {
		return false, false
	}
	if _, width, prec := printfSpecParts(spec); width == 0 && prec < 0 {
		return false, false
	}
	if long {
		chars = r.ask(r.sem().PrintfLongModifierCountsCharacters,
			"`printf '%ls'` counting its field in characters")
		if r.unspecified {
			return false, true
		}
	}
	if !chars {
		chars = r.ask(r.sem().PrintfFieldCountsCharacters,
			"`printf '%s'` counting its field in characters")
		if r.unspecified {
			return false, true
		}
	}
	if !chars {
		return false, false
	}
	chars = r.countsTheLocalesCharacters()
	return chars, r.unspecified
}

// printfWideCharacter is `%lc` in the column where the `l` modifier makes it
// a wide character: the operand's first **character** rather than its first
// byte, laid out as a one-character string — precision and all.
//
// The precision is the tell that it is a string and not C's `%c`. Measured
// 2026-09-16 on bash 5.3.20 under a UTF-8 locale:
//
//	printf '[%lc]'    αβγ   [α]         `%c` writes the byte 0xce
//	printf '[%3lc]'   αβγ   [  α]       two spaces: the width is characters
//	printf '[%.0lc]'  abc   []          `%.0c` writes [a] in every column
//	printf '[%5.0lc]' ''    [     ]     five spaces and no NUL
//	printf '[%-3lc]'  ''    [\0  ]      the NUL `%c` writes, then the pad
//
// and all five are the byte reading under `LC_ALL=C`, where the locale has no
// characters to take. handled is false wherever this reading and `%c`'s
// cannot differ — an ASCII first byte with no precision — and wherever the
// axis or the locale says bytes, so the caller writes `%c` as it always has.
//
// A first byte that does not begin a valid sequence is not modeled: bash
// writes nothing at all for it, and this falls back to the byte.
func (r *Runner) printfWideCharacter(spec, arg string) (field string, handled, unanswered bool) {
	_, _, prec := printfSpecParts(spec)
	if prec < 0 && (arg == "" || arg[0] < utf8.RuneSelf) {
		return "", false, false
	}
	if !r.ask(r.sem().PrintfLongModifierCountsCharacters, "`printf '%lc'` taking a character") {
		return "", false, r.unspecified
	}
	if !r.countsTheLocalesCharacters() {
		return "", false, r.unspecified
	}
	char := "\x00"
	if arg != "" {
		c, size := utf8.DecodeRuneInString(arg)
		if c == utf8.RuneError && size <= 1 {
			return "", false, false
		}
		char = arg[:size]
	}
	return printfCharacterField(spec, char), true, false
}

// printfWideInteger lays an integer conversion out here because its precision
// — C's *minimum number of digits* — is past what `fmt` renders.
//
// The digits are really produced, because the references produce them:
// `printf '%.10000010d' 1` is ten million zeros and then a 1 in bash 5.3,
// measured 2026-09-15, and the same for `%x`, `%o` and `%u`. Clamping would
// be a different answer and not a cheaper one.
//
// signed says whether the value is read as a signed number. The unsigned
// conversions take the operand's 64-bit pattern rather than its magnitude —
// see the `%x` note in printfConvert — so the same int64 is a bare pattern
// there and a sign and a magnitude at `%d`.
//
// The `0` flag is deliberately not honored: C says it is ignored where a
// precision is written for an integer conversion, and one is written here by
// definition.
func printfWideInteger(spec string, verb byte, n int64, signed bool) string {
	flags, width, prec := printfSpecParts(spec)
	base := 10
	switch verb {
	case 'o':
		base = 8
	case 'x', 'X':
		base = 16
	}
	v, neg := uint64(n), false
	if signed && n < 0 {
		// Unary minus on an unsigned is 0 minus it, so the magnitude of
		// INT64_MIN comes out right rather than overflowing a negation.
		v, neg = -v, true
	}
	digits := strconv.FormatUint(v, base)
	if verb == 'X' {
		digits = strings.ToUpper(digits)
	}
	if prec == 0 && v == 0 {
		// C's one erasure: a precision of nought and a value of nought
		// write no characters at all. Unreachable from the wide band, where
		// the precision is ten million, and here because this is a renderer
		// and TestTheWidePrecisionRenderersAgreeWithFmt grades it as one.
		digits = ""
	}
	if prec > len(digits) {
		digits = strings.Repeat("0", prec-len(digits)) + digits
	}
	prefix := ""
	if strings.ContainsRune(flags, '#') {
		switch verb {
		case 'o':
			// C's `#` on an octal raises the precision until there is a
			// leading zero, which a precision this wide has already done.
			if !strings.HasPrefix(digits, "0") {
				prefix = "0"
			}
		case 'x':
			if v != 0 {
				prefix = "0x"
			}
		case 'X':
			if v != 0 {
				prefix = "0X"
			}
		}
	}
	sign := ""
	switch {
	case neg:
		sign = "-"
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	return printfPadToWidth(sign+prefix+digits, width, strings.Contains(flags, "-"), false)
}

// printfWideFloat lays a floating conversion out here because its precision —
// digits that have to be *produced* — is past what `fmt` renders.
//
// strconv is what produces them, and it is the reason this half is tractable
// at all: `strconv.FormatFloat(1, 'f', 10000010, 64)` is ten million decimal
// places in six milliseconds, where `fmt` writes 25 characters of complaint.
// A float64's exact decimal expansion is finite, so everything past it is
// zeros and strconv writes those too — which is what bash writes.
//
// An infinity and a not-a-number never reach here: printfNonFinite answers
// them first, and its field has no precision in it.
func printfWideFloat(spec string, verb byte, f float64) string {
	flags, width, prec := printfSpecParts(spec)
	if verb == 'F' {
		// Go has no `%F`, and strconv has no `'F'`. The capital only ever
		// changed the spelling of a non-finite value, which is written
		// elsewhere.
		verb = 'f'
	}
	alternate := strings.ContainsRune(flags, '#')
	body := ""
	if alternate && (verb == 'g' || verb == 'G') {
		// strconv's `'g'` strips the trailing zeros, which is `%g` and is
		// not `%#g`. The alternate form is the one place the two differ
		// enough to need a renderer of its own, and it is chosen before the
		// digits are produced rather than after: at ten million places the
		// rendering nobody keeps is ten megabytes nobody keeps.
		body = printfAlternateG(math.Abs(f), verb, prec)
	} else {
		body = strconv.FormatFloat(math.Abs(f), verb, prec, 64)
	}
	if alternate {
		body = printfForcePoint(body)
	}
	sign := ""
	switch {
	case math.Signbit(f):
		// The value's own sign, so a negative zero keeps its minus.
		sign = "-"
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	return printfPadToWidth(sign+body, width,
		strings.Contains(flags, "-"), strings.Contains(flags, "0"))
}

// printfAlternateG is C's `%#g`: the significant digits `%g` chooses, with
// the trailing zeros `%g` strips kept and the point always written.
//
// `%g` picks its style from the exponent — the scientific one below -4 or at
// the precision, the plain one between — and strconv's `'g'` picks it the
// same way, so the only thing rebuilt here is the digits. Go's `fmt` is
// right about every row of this and strconv is not, which is why the narrow
// path needs none of it: measured against bash 5.3 2026-09-15, `%#.1g` of 1
// is `1.`, of 0.00001 is `1.e-05`, `%#.3g` of 0.0001 is `0.000100` and
// `%#.2g` of 9.99 is `10.` — and `fmt` writes those four.
//
// The exponent is read back out of the scientific rendering rather than
// computed, so a carry out of the rounding is already in it: 9.99 at one
// digit is `1.e+01` and not `9.e+00`.
func printfAlternateG(f float64, verb byte, prec int) string {
	if prec == 0 {
		// C reads a precision of nought at `%g` as one, and so does strconv.
		prec = 1
	}
	e := byte('e')
	if verb == 'G' {
		e = 'E'
	}
	sci := strconv.FormatFloat(f, e, prec-1, 64)
	exp := 0
	if i := strings.IndexAny(sci, "eE"); i >= 0 {
		exp, _ = strconv.Atoi(sci[i+1:])
	}
	body := sci
	if exp >= -4 && exp < prec {
		body = strconv.FormatFloat(f, 'f', prec-1-exp, 64)
	}
	return body
}

// printfForcePoint is the decimal point C's `#` flag forces onto a floating
// conversion, which is visible only where the digits left none to separate:
// `%#.0f` of 1 is `1.` and `%#.0e` of it is `1.e+00`, in bash 5.3 and in Go's
// `fmt` alike. In the scientific style the point belongs in front of the
// exponent and not at the end of the field.
func printfForcePoint(body string) string {
	if strings.ContainsRune(body, '.') {
		return body
	}
	if i := strings.IndexAny(body, "eE"); i >= 0 {
		return body[:i] + "." + body[i:]
	}
	return body + "."
}

// printfWideField reports a width this shell has to lay out itself because
// `fmt` will not, answering the spec with that width taken out.
//
// The width may arrive as digits in the format or through a `*` operand, and
// both are settled into the spec before they get here — so one test at the
// one place every conversion passes covers both routes (#2663).
func printfWideField(spec string) (narrow, flags string, width int, wide bool) {
	i := 1 // past the '%'
	for i < len(spec) && strings.ContainsRune("-+ #0'", rune(spec[i])) {
		i++
	}
	j := i
	for j < len(spec) && spec[j] >= '0' && spec[j] <= '9' {
		j++
	}
	if j == i {
		return spec, "", 0, false
	}
	n, err := strconv.Atoi(spec[i:j])
	if err != nil || n <= printfFmtFieldCeiling {
		return spec, "", 0, false
	}
	// The flags are returned rather than looked for in the whole spec,
	// because a width is made of digits: `%10000010d` holds a `0` that is not
	// the zero flag, and asking `strings.Contains(spec, "0")` zero-pads a
	// field the panel pads with spaces.
	return spec[:i] + spec[j:], spec[1:i], n, true
}

// printfWithoutPrecision answers the spec with its precision taken out, the
// flags and the width left where they were.
//
// It exists for `%c`, the one conversion C gives no precision at all and Go
// does: the character is written through `%s`, which truncates. Five of the
// six references ignore a precision there outright — `printf '[%.0c]' abc` is
// `[a]` in bash 5.3, zsh, ksh93, dash and BusyBox ash — and the sixth is bash
// 3.2, which writes `[]`. That is the same company #2647 kept and for the
// same reason: an old bash rather than a language, and a dialect is not an
// age.
//
// ksh93 is the one column that does something *with* a precision here rather
// than ignoring it — `printf '[%.3c]' abc` is `[aaa]` there, the character
// repeated — and that is a reading of its own, measured but not reproduced.
// It is recorded in docs/spec/semantics.md beside this, so an axis for it
// starts from the evidence rather than from the four easy cases.
//
// Every `*` has been replaced by the operand it took before this is reached,
// so printfFieldRun sees only digits — but it is the one that knows what a
// field is, and a second scanner beside it is how a fix reaches one and not
// the other.
func printfWithoutPrecision(spec string) string {
	i := 1 // past the %
	i += runOfBytes(spec, i, "-+ #0'")
	i += printfFieldRun(spec, i)
	if i >= len(spec) || spec[i] != '.' {
		return spec
	}
	j := i + 1
	j += printfFieldRun(spec, j)
	return spec[:i] + spec[j:]
}

// printfWithoutSignFlags answers the spec with `+` and a space taken out of
// its flags, the rest of the flags and the field left where they were.
//
// It exists for the unsigned conversions, where C says a sign flag applies to
// a signed conversion and says nothing about these — and every reference
// ignores it: `printf '[%+x]' 255` is `[ff]` and `[% x]` of it is `[ff]` in
// bash 5.3, zsh, ksh93 and dash alike, where Go writes `+ff` and ` ff`. The
// flag was already reaching the output before the sign was fixed; what the
// fix changed is that it became visible, since a negative operand used to
// carry a `-` of its own and Go writes only one sign.
//
// The `#` and `0` flags are untouched — both mean something here, and
// `%#x` of 255 is `0xff` in every column.
//
// Read with the same flag run printfWithoutPrecision reads, for the same
// reason: a second scanner beside it is how a fix reaches one and not the
// other.
func printfWithoutSignFlags(spec string) string {
	i := 1 // past the %
	n := runOfBytes(spec, i, "-+ #0'")
	flags := spec[i : i+n]
	if !strings.ContainsAny(flags, "+ ") {
		return spec
	}
	return "%" + strings.NewReplacer("+", "", " ", "").Replace(flags) + spec[i+n:]
}

// printfFieldMax is the C `int` every reference on the panel stores a width
// and a precision in. Nothing there lays a field out past it — it refuses, or
// writes an empty field, or wraps the number into the int and uses what is
// left — and this shell laying one out is the hang #3008 reports: a width ten
// times this is 21 GB of padding, which is not a slow answer.
const printfFieldMax = math.MaxInt32

// printfFieldNumber reads the run of digits at i as a C `strtol` would: the
// value, saturated at the top of a 64-bit signed integer rather than wrapped,
// because that is where the reading stops rather than where the storing does.
//
// The saturation is load-bearing and is measured, not assumed:
// `printf '[%99999999999999999999s]' x` is `[x]` in zsh — one character, no
// padding — which is LONG_MAX truncated to an int32 giving -1, a negative
// width being a left-justified one, and a width of one holding a single
// character. A value wrapped modulo 2^64 instead would land somewhere else
// and pad.
func printfFieldNumber(s string, i, n int) int64 {
	var v int64
	for ; n > 0; i, n = i+1, n-1 {
		d := int64(s[i] - '0')
		if v > (math.MaxInt64-d)/10 {
			return math.MaxInt64
		}
		v = v*10 + d
	}
	return v
}

// printfFieldWrap stores a field number in a C `int` and answers what is left,
// as the two wrapping columns do.
//
// A negative width is a left-justified one, which is C's own rule, so the
// answer is a magnitude and a flag rather than a signed number. The one value
// with no magnitude is INT_MIN, whose negation is itself: measured as a width
// of nothing at all — `printf '[%2147483648s]' x` is `[x]` in zsh — so it
// answers zero rather than two billion.
func printfFieldWrap(v int64) (width int, left bool) {
	w := int32(uint32(v))
	if w >= 0 {
		return int(w), false
	}
	if n := -w; n > 0 {
		return int(n), true
	}
	return 0, true
}

// printfFieldBeyondAnInt reports whether the width or the precision written in
// spec is past the int a reference stores it in, and answers the spec those
// numbers wrapped into one — which is what the wrapping columns use and what
// the other two readings never look at.
//
// Every `*` has been replaced by the operand it took before this is reached,
// so the runs here are digits.
//
// The boundary is not the same for every reading and the caller is what knows
// which: this answers `beyond` at the *widest* of them, INT_MAX itself, and
// PrintfFieldEmpty's columns turn one above that. See
// Semantics.PrintfFieldBeyondAnInt.
func printfFieldBeyondAnInt(spec string) (beyond, atTheEdge bool, wrapped string) {
	i := 1 // past the %
	flagEnd := i + runOfBytes(spec, i, "-+ #0'")

	widthAt := flagEnd
	widthRun := printfFieldRun(spec, widthAt)
	width := printfFieldNumber(spec, widthAt, widthRun)

	precAt, precRun := -1, 0
	prec := int64(-1)
	if j := widthAt + widthRun; j < len(spec) && spec[j] == '.' {
		precAt = j + 1
		precRun = printfFieldRun(spec, precAt)
		prec = printfFieldNumber(spec, precAt, precRun)
	}
	// Strictly below, because the widest boundary any reading uses is
	// INT_MAX *itself*: bash and dash refuse there. atTheEdge is what lets
	// the readings that turn one past it say so.
	if width < printfFieldMax && prec < printfFieldMax {
		return false, false, spec
	}
	atTheEdge = width == printfFieldMax || prec == printfFieldMax

	// Rebuilt rather than patched: a width and a precision can both be past
	// the edge, and each wraps on its own.
	flags := spec[i:flagEnd]
	var b strings.Builder
	b.WriteByte('%')
	if widthRun > 0 {
		n, left := printfFieldWrap(width)
		if left && !strings.Contains(flags, "-") {
			// The flag rather than a sign, so the scanners that read flags
			// out of a spec see a left-justified field where C would.
			flags += "-"
		}
		b.WriteString(flags)
		b.WriteString(strconv.Itoa(n))
	} else {
		b.WriteString(flags)
	}
	if precAt >= 0 {
		// A negative precision is C's "as if it were omitted", and the
		// omission takes the dot with it.
		if n, left := printfFieldWrap(prec); !left {
			b.WriteString("." + strconv.Itoa(n))
		}
	}
	// Whatever followed the field: the length modifiers, and nothing else,
	// since a spec does not carry its conversion character.
	tail := widthAt + widthRun
	if precAt >= 0 {
		tail = precAt + precRun
	}
	b.WriteString(spec[tail:])
	return true, atTheEdge, b.String()
}

// printfFieldRefused is the complaint the two refusing columns write, and the
// status they report with it.
func (r *Runner) printfFieldRefused() int {
	d := r.diag()
	r.diagf("%s\n", Wording(d.PrintfFieldBeyondAnInt,
		"printf: Value too large to be stored in data type"))
	return orDefault(d.PrintfFieldBeyondAnIntStatus, 1)
}

// starReading resolves Semantics.PrintfStarBeyondAnInt, refusing an
// unanswered axis the way every other unanswered axis here is refused.
func (r *Runner) starReading() PrintfStarReading {
	p := r.sem().PrintfStarBeyondAnInt
	if p == PrintfStarUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("printf: a `*` operand past a C int")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// fieldReading resolves Semantics.PrintfFieldBeyondAnInt, refusing an
// unanswered axis the way every other unanswered axis here is refused.
func (r *Runner) fieldReading() PrintfFieldReading {
	p := r.sem().PrintfFieldBeyondAnInt
	if p == PrintfFieldUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("printf: a width or a precision past a C int")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// printfPadToWidth lays a rendered field out to a width `fmt` refused.
//
// Only the space and zero paddings are modeled, which is the whole of what a
// width means once the field is rendered. A `0` that would have to fall
// *inside* the text — after a sign, or after a `0x` — is deliberately not
// reconstructed here: the narrowed spec keeps the `0` flag, so `fmt` has
// already placed any zeros it owes against the precision, and what is left is
// the outer field.
func printfPadToWidth(field string, width int, left, zero bool) string {
	if len(field) >= width {
		return field
	}
	pad := byte(' ')
	if zero && !left {
		pad = '0'
	}
	fill := strings.Repeat(string(pad), width-len(field))
	if left {
		return field + fill
	}
	if pad == '0' && field != "" && (field[0] == '-' || field[0] == '+' || field[0] == ' ') {
		// The sign stays in front of the padding it earned.
		return field[:1] + fill + field[1:]
	}
	return fill + field
}

// printfConvert formats one conversion whose width and precision are settled.
func (r *Runner) printfConvert(spec string, verb byte, timeFmt string, next func() (string, bool)) (string, int, bool) {
	// A field past the C int a reference stores it in is settled before
	// anything else, because the layout below would honor it: 21 GB of
	// padding is a hang rather than a slow answer, and no shell on the panel
	// lays one out (#3008).
	if beyond, atTheEdge, wrapped := printfFieldBeyondAnInt(spec); beyond {
		switch reading := r.fieldReading(); reading {
		case PrintfFieldRefused:
			return "", r.printfFieldRefused(), true
		case PrintfFieldEmpty:
			// Only above the edge: this reading's columns lay INT_MAX out
			// in full and give up one past it.
			if !atTheEdge {
				// The operand is taken and then dropped, which is what
				// makes `printf '[%21474836470s]' a b` two empty fields
				// rather than one pass.
				next()
				return "", orDefault(r.diag().PrintfFieldBeyondAnIntStatus, 1), false
			}
		case PrintfFieldWrapsToAnInt:
			spec = wrapped
		default:
			// Unanswered. fieldReading has already said so and set the
			// status; the pass stops rather than guessing a layout.
			return "", r.status, true
		}
	}

	// A width past what `fmt` renders is laid out here instead. Done before
	// the argument is taken so the conversion below sees exactly the spec it
	// would have, minus a width it could not have honored.
	// One conversion lays out its own width however wide it is, so the
	// ceiling below does not apply to it: `%a` builds its field by hand
	// down to the fill, and taking the width away from it put the fill
	// outside the `0x` on one side of the ceiling and inside it on the
	// other. The two sides have to agree — that is what a ceiling is —
	// which is what #3089 was.
	if verb != 'a' && verb != 'A' {
		if narrow, flags, width, wide := printfWideField(spec); wide {
			_, _, prec := printfSpecParts(spec)
			zero := strings.Contains(flags, "0") &&
				!r.printfZeroFlagIsIgnored(flags, prec, verb)
			field, code, stop := r.printfConvert(narrow, verb, timeFmt, next)
			return r.printfPadWideField(field, verb, width,
				strings.Contains(flags, "-"), zero), code, stop
		}
	}
	// And the column that keeps the `0` flag against a precision lays the
	// field out here for the same reason `fmt` cannot: the flag it would
	// need is one `fmt` obeys C about. See
	// Semantics.PrintfZeroFlagSurvivesAPrecision.
	if narrow, width, ok := printfZeroFillShape(spec, verb); ok {
		field, code, stop := r.printfConvert(narrow, verb, timeFmt, next)
		if stop || len(field) >= width {
			// A field the value already fills has no fill to place, and the
			// two readings coincide there — so nothing is asked, the way
			// printfAlternatePrefixField asks nothing of `%#3x`.
			return field, code, stop
		}
		return printfPadToWidth(field, width, false,
			r.printfZeroFlagSurvivesAPrecision()), code, stop
	}
	arg, present := next()
	// The conversion character, for the one column that names it in a
	// second complaint about an operand its arithmetic could not read. Set
	// here rather than threaded through the number reader, which is several
	// calls below and takes the same operand from four different verbs.
	// See Diagnostics.PrintfArithArgumentType.
	saved := r.printfConversionName
	r.printfConversionName = string(verb)
	defer func() { r.printfConversionName = saved }()
	switch verb {
	case 'T':
		return r.printfTime(spec, timeFmt, arg, present)
	case 's':
		field, unanswered := r.printfStringField(spec, arg, r.printfLongModifier)
		if unanswered {
			return "", r.status, true
		}
		return field, 0, false
	case 'b':
		// The one verb whose *argument* is escaped, where `%s` leaves it
		// alone. Unanimous, and the difference people reach for `%b` to get.
		//
		// A `\c` in the argument ends the whole `printf` and not only this
		// conversion — `printf '[%b][%s]' 'a\cb' x` is `[a` in all six — so
		// the flag is returned rather than dropped.
		text, stop := r.expandBEscapes(arg)
		// Never the `l` modifier's reading: `printf '[%.1lb]' αβ` cuts at a
		// byte in bash 5.3, where `%.1ls` keeps the character.
		field, unanswered := r.printfStringField(spec, text, false)
		if unanswered {
			return "", r.status, true
		}
		if stop && field != text &&
			!r.ask(r.sem().PrintfBStopIsPadded, "a `%b` a `\\c` cut short still going through its field") {
			// ksh93 alone: what the stop left is written as it stands, width
			// and precision and all. Asked only where the field would change
			// the text, so a bare `printf '%b' 'a\cb'` needs no dialect.
			return text, 0, true
		}
		return field, 0, stop
	case 'c':
		if r.printfLongModifier {
			field, handled, unanswered := r.printfWideCharacter(spec, arg)
			if unanswered {
				return "", r.status, true
			}
			if handled {
				return field, 0, false
			}
		}
		if arg == "" {
			// No character to write — an empty operand, or none left at all
			// — and the answer is one NUL byte rather than nothing (#2647).
			// The two cases are one case: `printf '[%c]' ''` and `printf
			// '[%c]'` are the same bytes in every reference, so this reads
			// `arg` and never `present`.
			//
			// Every dialect's reference agrees — bash 5.3, zsh, ksh93, dash
			// and BusyBox ash — so there is no axis. The one panel column
			// that writes nothing is bash 3.2, which is a bash predating the
			// agreement rather than a language of its own: the record splits
			// on the age of a binary, and a dialect is not an age.
			arg = "\x00"
		}
		// The first *byte*, padded as a string. `%c` of a rune would encode
		// it: `printf '%c' $'\xc0'` is the one byte 0xc0 in every shell in
		// the panel, and Go's `%c` on `rune(0xc0)` writes two.
		//
		// The NUL above goes through the same field, because that is what
		// the references do with it: `printf '[%3c]' ''` is two spaces and
		// then the NUL in all five, and `%-3c` the NUL and then two spaces.
		//
		// The *precision* is taken back out, because C's `%c` has none and
		// Go's `%s` does. `printf '[%.0c]' abc` is `[a]` in bash 5.3, zsh,
		// ksh93, dash and BusyBox ash alike, and handing the one-byte string
		// to `%.0s` wrote nothing at all (#2714). The width is untouched: it
		// is the half of the field a `%c` really has.
		return fmt.Sprintf(printfWithoutPrecision(spec)+"s", arg[:1]), 0, false
	case 'q':
		return r.printfQuote(spec, arg)
	case 'd', 'i':
		n, code, stop := r.printfNumber(arg, present)
		if stop {
			return "", code, true
		}
		if printfWidePrecision(spec) {
			return printfWideInteger(spec, 'd', n, true), code, false
		}
		if field, ok := r.printfNoughtField(spec, 'd', n); ok {
			return field, code, false
		}
		return fmt.Sprintf(spec+"d", n), code, false
	case 'o', 'u', 'x', 'X':
		n, code, stop := r.printfNumber(arg, present)
		if stop {
			return "", code, true
		}
		if verb == 'u' {
			// Go has no `%u`. Unsigned decimal is `%d` of an unsigned
			// value, which is what the conversion below hands it.
			verb = 'd'
		}
		// The operand's bit pattern, 64 bits wide. An unsigned conversion
		// takes the value's representation and not its magnitude, so
		// `printf '%x' -1` is `ffffffffffffffff` and `%u` of it is
		// `18446744073709551615` — unanimous across bash 5.3, zsh, ksh93,
		// dash and BusyBox ash, which is why no dialect is consulted
		// (#2902). The width is the shell's `intmax_t` and not C's `int`:
		// C prints `ffffffff` for the same call.
		//
		// A signed reading is what wrote `-ff` for `printf '%x' -255`, which
		// is not a numeral any of those shells would read back, and `%u` with
		// a minus sign in front of it.
		unsigned := printfWithoutSignFlags(spec)
		if printfWidePrecision(unsigned) {
			return printfWideInteger(unsigned, verb, n, false), code, false
		}
		if field, ok := r.printfNoughtField(unsigned, verb, n); ok {
			return field, code, false
		}
		if field, ok := r.printfAlternatePrefixField(unsigned, verb, n); ok {
			return field, code, false
		}
		return fmt.Sprintf(unsigned+string(verb), uint64(n)), code, false
	case 'f', 'e', 'E', 'g', 'G', 'F', 'a', 'A':
		f, code, stopped := r.printfFloat(arg, present)
		if stopped {
			return "", code, true
		}
		if text, ok, stop := r.printfNonFinite(spec, verb, f); ok {
			return text, code, stop
		}
		if verb == 'a' || verb == 'A' {
			text, stop := r.printfHexFloat(spec, verb, f)
			return text, code, stop
		}
		if verb == 'g' || verb == 'G' {
			spec = printfSignificantDigits(spec)
		}
		if verb == 'F' {
			// Go has no `%F`. The only thing C's capital changes is the
			// spelling of a non-finite value, and printfNonFinite above has
			// already written that one.
			verb = 'f'
		}
		if printfWidePrecision(spec) {
			return printfWideFloat(spec, verb, f), code, false
		}
		return fmt.Sprintf(spec+string(verb), f), code, false
	}
	return "", 0, false
}

// printfHexFloat is C's `%a`: the value in hexadecimal, with a binary
// exponent, laid out here rather than handed to Go.
//
// Go's `%x` on a float is close and is not the same. It writes a *two-digit*
// exponent where C writes the shortest one — `fmt.Sprintf("%x", 1.5)` is
// `0x1.8p+00` against C's `0x1.8p+0` — so handing the verb through would
// write a digit no reference writes, which is the family of bug `%g`'s
// default precision was (#2687). Its `#` flag pads the significand rather
// than forcing the point, and its zero padding lands *inside* the `0x`:
// `fmt.Sprintf("%020x", 1.5)` is `000000000000x1.8p+00`. So the digits come
// from strconv and everything around them is built here.
//
// The layout is measured, bash 5.3.15 and dash agreeing on every row
// 2026-09-14 under `LC_ALL=C`:
//
//	%a 1.5      0x1.8p+0     the shortest run of digits that names the value
//	%.2a 1.5    0x1.80p+0    a stated precision is digits after the point
//	%#.0a 1.5   0x1.p+0      `#` forces the point and nothing else
//	%+a 1.5     +0x1.8p+0    the sign flags are C's
//	% a 0       ` 0x0p+0`
//	%a -0.0     -0x0p+0      the sign of a negative zero is the value's
//	%-14a 1.5   `0x1.8p+0   ` the `-` flag pads on the right with blanks
//	%014a 1.5   0x0000001.8p+0 and the `0` flag pads *after* the `0x`
//	%A 1.5      0X1.8P+0     the capital reaches the prefix and the `p`
//
// The digits are not strconv's either, and that is the second half of why
// this is written out. `strconv.FormatFloat(f, 'x', p, 64)` rounds a tie away
// from zero and *renormalizes* when the rounding carries, so `%.0a 1.5` comes
// out `0x1p+1` and `%.1a 255` comes out `0x1.0p+8`. bash and dash write
// `0x1p+0` and `0x2.0p+7`: the tie goes toward zero, and a carry out of the
// leading digit makes that digit a 2 and leaves the exponent where it was.
// Both are measured — `%.1a 1.09375` is `0x1.1p+0` and `%.1a 1.15625` is
// `0x1.2p+0`, which is a tie rounded down twice and not to-even — and both
// are invisible until a precision is written small enough to round, which is
// exactly how a renderer that was wrong on them would have looked right.
// ksh93u+ rounds as strconv does on both rows and is the recorded divergence
// here; see docs/spec/semantics.md.
func (r *Runner) printfHexFloat(spec string, verb byte, f float64) (string, bool) {
	i := 1 // past the %
	i += runOfBytes(spec, i, "-+ #0'")
	flags := spec[1:i]
	i += printfFieldRun(spec, i)
	width, _ := strconv.Atoi(spec[1+len(flags) : i])
	prec := -1
	if i < len(spec) && spec[i] == '.' {
		i++
		prec, _ = strconv.Atoi(spec[i : i+printfFieldRun(spec, i)])
	} else if r.ask(r.sem().PrintfHexFloatDefaultIsTwelveDigits,
		"`printf '%a'` with no precision writing twelve digits of significand rather than the shortest run that names the value") {
		// One column's default, and a precision rather than a minimum: the
		// thirteenth digit of `0.1` is rounded away there.
		prec = 12
	} else if r.unspecified {
		return "", true
	}

	body := printfHexFloatDigits(math.Abs(f), prec)
	if strings.ContainsRune(flags, '#') && !strings.ContainsRune(body, '.') {
		// C's `#` forces the point and adds no digits. It is visible only
		// where there are none to separate — `%#a 1.5` is `0x1.8p+0`, the
		// same as `%a`, and `%#.0a 1.5` is `0x1.p+0`.
		if p := strings.IndexByte(body, 'p'); p >= 0 {
			body = body[:p] + "." + body[p:]
		}
	}
	if verb == 'A' {
		body = strings.ToUpper(body)
	}

	sign := ""
	switch {
	case math.Signbit(f):
		// The value's own, and a negative zero has one: `printf '%a' -0.0`
		// is `-0x0p+0` in bash and dash alike.
		sign = "-"
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	field := sign + body
	if len(field) >= width {
		return field, false
	}
	fill := strings.Repeat(" ", width-len(field))
	switch {
	case strings.ContainsRune(flags, '-'):
		return field + fill, false
	case !strings.ContainsRune(flags, '0'):
		return fill + field, false
	}
	zeros := strings.Repeat("0", width-len(field))
	if r.ask(r.sem().PrintfHexFloatZeroFillPrecedesThePrefix,
		"`printf '%a'` putting its zero fill in front of the `0x` rather than inside it") {
		// One column's placement, at the same total width — see the axis.
		return sign + zeros + body, false
	}
	// The zero flag pads between the `0x` and the digits, which is where C
	// puts it and is the one place Go's own `%x` gets the position wrong.
	return sign + body[:2] + zeros + body[2:], false
}

// printfHexFloatDigits is the unsigned significand and exponent of a `%a`,
// written C's way: `0x`, the leading digit, the fraction a precision of -1
// makes as short as names the value exactly, and `p` with the shortest run of
// exponent digits.
//
// f is finite and not negative — printfNonFinite has already taken the
// infinities and the not-a-numbers, and the sign is the caller's, because a
// negative zero has one and `math.Abs` has thrown it away by here.
//
// The significand is normalized to a leading 1, which is what every column
// measured writes and is a choice C leaves open: `printf '%a' 5e-324` is
// `0x1p-1074` in bash and dash, so a subnormal is shifted up rather than
// written with the leading 0 its bits hold.
//
// Rounding is bash's and dash's, and is the reason strconv is not used: a tie
// goes toward zero, and a carry out of the leading digit makes it a 2 rather
// than renormalizing. See printfHexFloat for the measurements.
func printfHexFloatDigits(f float64, prec int) string {
	const fracDigits = 13 // 52 bits of significand, four bits to the digit
	bits := math.Float64bits(f)
	exp := int(bits>>52) & 0x7FF
	mant := bits & (1<<52 - 1)
	lead := byte('1')
	switch {
	case exp == 0 && mant == 0:
		lead, exp = '0', 0
	case exp == 0:
		// A subnormal, shifted up until the leading bit is where a normal
		// number keeps it. The exponent of the smallest normal is -1022, and
		// each shift takes one off it.
		exp = -1022
		for mant&(1<<52) == 0 {
			mant <<= 1
			exp--
		}
		mant &= 1<<52 - 1
	default:
		exp -= 1023
	}
	frac := []byte(fmt.Sprintf("%0*x", fracDigits, mant))

	if prec < 0 {
		// The shortest run that names the value exactly, which is the digits
		// with their trailing zeros taken off.
		n := len(frac)
		for n > 0 && frac[n-1] == '0' {
			n--
		}
		frac = frac[:n]
	} else if prec < len(frac) {
		if printfHexRoundsUp(frac, prec) {
			lead = printfHexCarry(frac[:prec], lead)
		}
		frac = frac[:prec]
	} else {
		frac = append(frac, bytes.Repeat([]byte("0"), prec-len(frac))...)
	}

	var b strings.Builder
	b.WriteString("0x")
	b.WriteByte(lead)
	if len(frac) > 0 {
		b.WriteByte('.')
		b.Write(frac)
	}
	b.WriteByte('p')
	if exp < 0 {
		b.WriteByte('-')
		exp = -exp
	} else {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(exp))
	return b.String()
}

// printfHexRoundsUp reports whether dropping everything past prec digits
// rounds the digit before them up.
//
// Half goes *down*, which is the rule the panel's two columns share and the
// one nothing but an exact tie can show: `%.1a 1.09375` is `0x1.1p+0` and
// `%.1a 1.15625` is `0x1.2p+0` in bash 5.3.15 and dash, so it is toward zero
// and not to-even.
func printfHexRoundsUp(frac []byte, prec int) bool {
	first := printfHexValue(frac[prec])
	if first != 8 {
		return first > 8
	}
	for _, c := range frac[prec+1:] {
		if c != '0' {
			return true
		}
	}
	return false
}

// printfHexCarry adds one to the last of the kept digits, carrying leftwards
// and finally into the leading digit — which becomes a 2 and leaves the
// exponent alone, where renormalizing would have moved it.
func printfHexCarry(kept []byte, lead byte) byte {
	for i := len(kept) - 1; i >= 0; i-- {
		if v := printfHexValue(kept[i]) + 1; v < 16 {
			kept[i] = "0123456789abcdef"[v]
			return lead
		}
		kept[i] = '0'
	}
	if lead == '0' {
		return '1'
	}
	return lead + 1
}

// printfHexValue is one lower-case hexadecimal digit's value.
func printfHexValue(c byte) int {
	if c >= 'a' {
		return int(c-'a') + 10
	}
	return int(c - '0')
}

// printfNonFinite writes an infinity or a not-a-number, and reports whether
// this was one — a finite float is left to the conversion, which is right
// about every one of them.
//
// C spells these `inf` and `nan`, capitalized under an upper-case conversion;
// Go's `strconv` spells them `+Inf`, `-Inf` and `NaN`, and handing the float
// to `fmt.Sprintf` wrote Go's spelling in every float conversion at once
// (#2707). The sign is the smaller half of that and the same bug: `+Inf` is
// wrong twice, because C writes a sign for an infinity only when the value is
// negative or a flag asked for one.
//
// The field is C's `%s` field and not the float conversion's. A width pads,
// `-` pads on the right, and the `0` flag and the precision are both ignored:
// `printf '[%010f][%.2f][%08.2f]' inf inf inf` is `[       inf][inf][     inf]`
// in bash, dash and BusyBox ash. So the text is put through the spec with
// everything but `-` and the width stripped out, which is what leaves the
// padding in place — a special case that returns the bare word drops it, and
// that is the shape zsh actually has and the other five do not.
//
// A not-a-number never carries a sign here. See
// Semantics.PrintfNonFiniteIsConverted for the axis, for why the sign of a
// not-a-number is not one, and for the measurement behind both.
//
// The three results are the text, whether this was a non-finite value at all,
// and whether an unanswered axis stopped the format — the same three the rest
// of this file's conversions return.
func (r *Runner) printfNonFinite(spec string, verb byte, f float64) (string, bool, bool) {
	if !math.IsInf(f, 0) && !math.IsNaN(f) {
		return "", false, false
	}
	word := "nan"
	if math.IsInf(f, 0) {
		word = "inf"
	}
	bare := word
	if math.IsInf(f, -1) {
		// The sign the *value* carries, which both readings write. It is
		// not a flag, which is why it survives zsh's bare word.
		bare = "-" + word
	}
	full := printfNonFiniteField(spec, verb, f, word)
	if full == bare {
		// The two readings agree, so there is nothing to ask. That covers
		// the whole of `printf '%f' inf` — the common case, and one no
		// dialect should have to be chosen for.
		return bare, true, false
	}
	if !r.ask(r.sem().PrintfNonFiniteIsConverted, "`printf` putting an infinity or a not-a-number through the conversion rather than writing the bare word") {
		if r.unspecified {
			return "", true, true
		}
		return bare, true, false
	}
	return full, true, false
}

// printfNonFiniteField is the converted reading: the word capitalized to
// match the verb, the sign a flag or the value asked for, and the whole put
// through the width.
//
// The prefix is read with the same two helpers printfSpecPrefixAt reads it
// with, rather than a third copy of the grammar. Every `*` has been replaced
// with the operand it took by the time this is reached, so printfFieldRun
// sees only digits here — but it is the one that knows what a field is, and a
// second scanner beside it is how a fix reaches one and not the other.
func printfNonFiniteField(spec string, verb byte, f float64, word string) string {
	if verb == 'E' || verb == 'G' || verb == 'F' || verb == 'A' {
		word = strings.ToUpper(word)
	}
	i := 1 // past the %
	i += runOfBytes(spec, i, "-+ #0")
	flags := spec[1:i]
	width := spec[i : i+printfFieldRun(spec, i)]

	sign := ""
	switch {
	case math.IsNaN(f):
		// No sign, whatever the flags and whatever the value's own sign.
	case math.IsInf(f, -1):
		sign = "-"
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	left := ""
	if strings.ContainsRune(flags, '-') {
		left = "-"
	}
	return fmt.Sprintf("%"+left+width+"s", sign+word)
}

// printfSignificantDigits writes `%g`'s default precision out, because C's
// default and Go's are different numbers and neither of them is "none".
//
// C gives `%g` six significant digits where no precision is written, and Go
// gives it "the shortest representation that round-trips" — so handing the
// verb through unchanged wrote every digit the float had: `1.234567e+06`
// against the `1.23457e+06` that bash 5.3, bash as sh, bash 3.2, ksh93, zsh,
// dash and BusyBox ash all answer (#2687). Unanimous across the panel, so
// this is the core's number rather than an axis.
//
// Only `%g` and `%G`. Go's default precision for `%e`, `%E` and `%f` is
// already six, which is why the gap survived beside verbs that look like it.
//
// It is the *default* and nothing else: a spec that already carries a `.` is
// returned untouched, so `%.10g`, `%.0g` — which C takes as 1, and so does
// Go's strconv — and the `%.` that means a precision of zero all keep the
// precision they were written with. A `%.*g` whose operand was negative has
// no precision by then, which is C's rule for a negative one, and so takes
// this default: `printf '%.*g' -1 123456789` is `1.23457e+08` in all seven.
//
// Six digits is also all it does. The trailing zeros `%g` strips, and the
// exponent threshold that strips them from `999999.5` all the way to `1e+06`,
// are already Go's `%g` behaving as C's — the conversion was never wrong
// about those, only about how many digits to start from.
func printfSignificantDigits(spec string) string {
	if strings.IndexByte(spec, '.') >= 0 {
		return spec
	}
	return spec + ".6"
}

func (r *Runner) printfEmptyNumberIsAnError(present bool) bool {
	if !present {
		// Read rather than asked, and that is the point: every dialect that
		// lets a present-and-empty operand through lets an absent one
		// through too. The implication holds in all seven columns, so a
		// dialect that says no here has nothing left to decide and must not
		// be questioned about it — which is also what keeps a core with
		// neither axis answered writing the silent zero the panel agrees on.
		if r.sem().PrintfEmptyIsNotANumber != Yes {
			return false
		}
		return r.ask(r.sem().PrintfAbsentNumberIsAnEmptyOne, "`printf` reading a numeric conversion with no operand left as an empty one")
	}
	return r.ask(r.sem().PrintfEmptyIsNotANumber, "`printf` complaining about an empty operand where a number belongs")
}

// cNotANumber reads the not-a-number operands C's `strtod` takes and Go's
// `strconv.ParseFloat` does not: one written with a sign, and the
// `nan(n-char-sequence)` form.
//
// Go takes `nan` and `NaN` and it takes a sign before an infinity, but a sign
// before a not-a-number is a syntax error there and the parenthesized form is
// one too. Six columns read both — bash 5.3, bash as sh, bash 3.2, zsh, dash
// and BusyBox ash all answer `nan` for `-nan`, `+nan`, `nan(1)`, `nan()` and
// `nan(abc)`, measured 2026-09-13 — so this is a correction and not an axis.
// The seventh is ksh93, which reads neither because it reads no operand: see
// Semantics.PrintfNonFiniteIsConverted.
//
// The sign is read and dropped. Every column writes `nan` for `-nan`, and
// nothing downstream can see the bit anyway — printfNonFinite writes a
// not-a-number unsigned whatever its sign.
//
// Only the spellings Go refuses. The plain ones still go through ParseFloat
// above, so there is one reader for `nan` and not two.
func cNotANumber(s string) (float64, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if len(s) < 3 || !strings.EqualFold(s[:3], "nan") {
		return 0, false
	}
	rest := s[3:]
	if rest == "" {
		// `-nan` or `+nan`: the sign is the only thing Go objected to.
		return math.NaN(), true
	}
	// `nan(…)`, whose characters C leaves to the implementation. The closing
	// parenthesis has to be the last byte, so `nan(1)x` stays the bad
	// operand it is in bash 3.2, zsh and ash — and what is between the
	// parentheses is not read at all.
	//
	// Not read, rather than read and checked against C's n-char-sequence,
	// and that is measured. bash 5.3, bash as sh, bash 3.2, zsh and dash
	// take `nan(a-b)`, `nan(a b)` and `nan(*)` and answer `nan` at 0;
	// BusyBox ash refuses all three and takes `nan(1)`, `nan()`, `nan(abc)`
	// and `nan(_1)`, which is exactly C's letters, digits and underscores.
	// That is BSD's strtod against musl's — the same C library split as the
	// sign a not-a-number gets under `%+f` — so this takes the five, and the
	// characters cannot change the value either way.
	if rest[0] != '(' || rest[len(rest)-1] != ')' {
		return 0, false
	}
	return math.NaN(), true
}

// printfQuote is `%q`, which quotes so the shell can read it back.
//
// Three answers and one absence, and the three are in interp/printfquote.go
// with the measurement that separates them. The one thing they agree on is
// the contract: what comes out has to read back as what went in, which is why
// a backslash before a newline is not one of the answers (#1707).
func (r *Runner) printfQuote(spec, arg string) (string, int, bool) {
	switch r.quoteStyle() {
	// A quoted operand's field is counted in bytes in every column that has
	// the conversion, zsh included — which counts characters for `%s` —
	// so no dialect is asked: `printf '[%.2q]' αβγ` is `[α]` in bash 5.3,
	// zsh 5.9.2 and ksh93u+ under a UTF-8 locale.
	case PrintfQuoteAnsiCWord:
		return printfByteField(spec, ansiCWordQuote(arg)), 0, false
	case PrintfQuoteAnsiCCharacter:
		// The same function `${(q)…}` uses, which is the same job: this
		// shell's `%q` and its `q` flag were measured against each other over
		// every printable byte at three positions and every control byte, and
		// they agree everywhere.
		return printfByteField(spec, quoteWithBackslashes(arg, false)), 0, false
	case PrintfQuoteSingle:
		return printfByteField(spec, kshSingleQuote(arg)), 0, false
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
	r.printfLongModifier = false
	i, spec, code := r.printfSpecPrefix(s)
	if code != 0 {
		return "", 0, "", i, code
	}
	if i >= len(s) {
		return "", 0, "", len(s), 0
	}
	// The prefix arrives already rebuilt without its `'`s: they are this
	// dialect's grouping flag, and it is dropped rather than honored. It asks
	// for the digits to
	// be parted by `LC_NUMERIC`'s thousands separator, and
	// interp/localenumeric.go is the one home for what this shell knows about
	// that category: the separator is empty under every locale it has numeric
	// data for, so the grouped conversion and the plain one are the same
	// string and Go — which has no such flag — can be handed the plain one.
	//
	// That the two halves agree is not left to this comment.
	// TestPrintfDropsTheGroupingFlagOnlyBecauseTheSeparatorIsEmpty asserts
	// LocaleNumericFor's separator beside this output, so teaching the shell
	// a locale that groups fails there rather than silently writing a number
	// with the flag thrown away. #2675 records why there is no such locale.
	//
	// A dialect that does *not* have the flag never gets this far, because
	// the `'` is the conversion character it was refused as. See
	// Semantics.PrintfGroupingFlag — and printfKshPrefix, where dropping it
	// is not enough, because in that dialect the quote *ends the digit run
	// it is in* and the digits around it are not the number they look like.
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
	n := r.lengthModifierRun(s, i)
	r.printfLongModifier = strings.IndexByte(s[i:i+n], 'l') >= 0
	i += n
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
	if strings.IndexByte("FaA", verb) >= 0 {
		// The three C99 added. Asked here rather than at the top, so a
		// dialect is questioned only where one of the letters is actually
		// written — `%f` never raises it. A dialect without them wants the
		// letter *not* taken, so that it arrives below as the conversion
		// character it is and is refused the way any unknown one is; that is
		// the shape #2646 had, and the reason both halves live here.
		if r.ask(r.sem().PrintfC99FloatConversions, "`printf` having C99's `%F`, `%a` and `%A` float conversions") {
			return spec, verb, "", i + 1, 0
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
	case PrintfLengthModifiersC99ExceptJAndT:
		// The same run, over four of the six letters. `j` and `t` are not
		// modifiers in this answer, so a conversion carrying one arrives
		// below as the character it is and is refused there.
		n := 0
		for i+n < len(s) && strings.IndexByte("hlzL", s[i+n]) >= 0 {
			n++
		}
		return n
	}
	return 0
}

// quotedDirective is the directive a refusal names, which one dialect writes
// without the length modifiers it just read past.
//
// See Diagnostics.PrintfDirectiveDropsLengthModifiers. The letters are the
// ones lengthModifierRun above takes, and the drop is applied to the text
// rather than to the scan, because the scan already accepted them: `%zd`
// writes its operand in that shell and only a directive that *fails* is ever
// quoted back.
func (r *Runner) quotedDirective(conversion string) string {
	if !r.diag().PrintfDirectiveDropsLengthModifiers {
		return conversion
	}
	var b strings.Builder
	b.Grow(len(conversion))
	for i := 0; i < len(conversion); i++ {
		if strings.IndexByte("hlzLjt", conversion[i]) >= 0 {
			continue
		}
		b.WriteByte(conversion[i])
	}
	return b.String()
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
// flags, the width and the precision — and a non-zero code is an axis
// nothing answered.
//
// The `'` in a prefix is the only part of this that is a question, and it is
// asked here rather than at the top of the builtin so that `printf '%d' 5`
// never reaches it. Two questions, because the panel parts three ways and not
// two: bash and zsh take `'` among the flags, ksh93 takes it anywhere in the
// prefix, dash and BusyBox ash do not take it at all. See
// Semantics.PrintfGroupingFlag and PrintfGroupingFlagAfterTheWidth.
//
// A dialect without the flag needs the `'` **not consumed here**, so that it
// arrives at the scan as the conversion character and is refused the way any
// other unknown one is. That is the same shape #2646 had, and the reason
// both halves live in this function.
func (r *Runner) printfSpecPrefix(s string) (int, string, int) {
	group, after := false, false
	if inFlags, pastFlags := printfGroupingFlagPositions(s); inFlags || pastFlags {
		group = r.ask(r.sem().PrintfGroupingFlag, "`printf` taking `'` as the flag that groups a number's digits")
		if r.unspecified {
			end, spec, _ := printfSpecPrefixAt(s, true, true, true, true)
			return end, spec, r.status
		}
		if group && pastFlags {
			after = r.ask(r.sem().PrintfGroupingFlagAfterTheWidth, "`printf` taking the `'` flag written past the flags")
			if r.unspecified {
				end, spec, _ := printfSpecPrefixAt(s, true, true, true, true)
				return end, spec, r.status
			}
		}
	}
	restart := false
	if printfFlagAfterTheField(s, group) {
		restart = r.ask(r.sem().PrintfFlagAfterTheField,
			"`printf` taking a flag written past a field")
		if r.unspecified {
			end, spec, _ := printfSpecPrefixAt(s, true, true, true, true)
			return end, spec, r.status
		}
	}
	mixed := false
	if printfStarBesideDigits(s, group, after, restart) {
		mixed = r.ask(r.sem().PrintfStarBesideTheFieldDigits,
			"`printf` taking a `*` written beside a width's own digits")
		if r.unspecified {
			end, spec, _ := printfSpecPrefixAt(s, true, true, true, true)
			return end, spec, r.status
		}
	}
	end, spec, lost := printfSpecPrefixAt(s, group, after, mixed, restart)
	// Carried on the runner rather than out of the scan: the operands a lost
	// star takes are read by printfStars, several returns below, and adding a
	// sixth result to scanPrintfSpec for a shape one dialect can write would
	// put it in every other caller's signature too.
	r.printfLostStars = lost
	return end, spec, 0
}

// printfStarBesideDigits reports whether the prefix at s holds a width or a
// precision in which a `*` and digits stand **side by side** — `%5*d`, `%*5d`,
// `%.2*d` — which is the one place the two readings of a field run disagree
// and so the one place Semantics.PrintfStarBesideTheFieldDigits is asked.
//
// Asked of the reading the dialect already has rather than of the text: where
// that reading stops is the verb, so a byte there that could have gone on
// being part of the field is exactly the disagreement. `%*.*d` and `%50d`
// stop at their conversion character and are never questioned.
func printfStarBesideDigits(s string, group, after, restart bool) bool {
	end, _, _ := printfSpecPrefixAt(s, group, after, false, restart)
	if end >= len(s) {
		return false
	}
	c := s[end]
	return c == '*' || (c >= '0' && c <= '9')
}

// printfFlagAfterTheField reports whether the prefix at s holds a flag
// written **past** a field, which is the one place the restart above can be
// seen and so the one place Semantics.PrintfFlagAfterTheField is asked.
//
// Read off C's own grammar — flags, a field, a `.` and a field — because that
// is the reading the question is about: a flag byte standing where that scan
// has already finished with the flags is exactly the disagreement. `%-5d` and
// `%-5.3d` stop at their conversion character and are never questioned.
//
// The `0` is not in either set. After a width it has already been taken as a
// digit, so it can never stand here; after a precision the same. And `#` is
// asked about only past a *width*: past a precision that shell reads it as
// something else entirely — `%.3#.4d` of 42 is `4#222`, a base rather than a
// field — which is a feature of its own and not this one.
func printfFlagAfterTheField(s string, group bool) bool {
	flags := "-+ #0"
	if group {
		flags += "'"
	}
	i := 1 // past the %
	i += runOfBytes(s, i, flags)
	if n := printfFieldRun(s, i); n > 0 {
		i += n
		if i < len(s) && strings.IndexByte("-+ #", s[i]) >= 0 {
			return true
		}
	}
	if i < len(s) && s[i] == '.' {
		i++
		i += printfFieldRun(s, i)
		if i < len(s) && strings.IndexByte("-+ ", s[i]) >= 0 {
			return true
		}
	}
	return false
}

// printfSpecPrefixAt is printfSpecPrefix once the two questions are settled:
// group says `'` is one of this dialect's flags, and after says it may also
// be written past the flag run, where the width and the precision go.
//
// One grammar and not two. printfGroupingFlagPositions calls this with both
// readings open to find out what a conversion is even asking, so the widest
// reading and the dialect's reading can never drift apart.
func printfSpecPrefixAt(s string, group, after, mixed, restart bool) (int, string, printfLostStars) {
	if after || mixed || restart {
		return printfKshPrefix(s, after, mixed, restart)
	}
	flags := "-+ #0"
	if group {
		flags += "'"
	}
	i := 1 // past the %
	i += runOfBytes(s, i, flags)
	i += printfFieldRun(s, i)
	if i < len(s) && s[i] == '.' {
		i++
		i += printfFieldRun(s, i)
	}
	// The `'` that survived is the grouping flag, and it is dropped rather
	// than honored — see scanPrintfSpec, where the reason is written down.
	// Taking it out of a run of flags leaves a spec `fmt` reads, which is
	// the whole of what this reading needs. No star is ever lost here: this
	// grammar has one field run apiece.
	return i, "%" + strings.ReplaceAll(s[1:i], "'", ""), printfLostStars{}
}

// printfKshPrefix is the one dialect that takes a `'` anywhere in the prefix,
// and the grammar is not "the same prefix with quotes allowed in more places"
// — the quote **ends the digit run it is in**, and before the `.` the scan
// starts over at the flags (#2688).
//
//	%1'0d    width 1, then `0` read as the zero-padding *flag*   [42]
//	%1'2'3d  each run after a quote replaces the width           [ 42]
//	%5'0d    an empty run leaves the width already read          [00042]
//	%*'5d    and a digit run after one replaces a star's width   [   42]
//	%.'5d    after a `.` the scan does not restart: precision 5  [00042]
//	%.5'3d   and there too the last run wins                     [042]
//
// Measured 2026-09-13 and 2026-09-14 against ksh93u+ over twenty-nine
// conversions of 42 under `LC_ALL=C`, which is what says it is a grammar
// rather than "skip the quote": deleting the quote makes `%1'0d` a width of
// ten and pads to ten, where ksh93 writes `42`.
//
// The flags accumulate across the restarts and the last *non-empty* run of
// each field wins, so the answer is rebuilt from the runs rather than edited
// out of the text.
//
// **A star that loses is still read.** `printf "[%*'5d]" 3 42` is `[   42]`
// in ksh93: the star took the 3 for a width the `5` then replaced, and the 42
// reached the conversion. Rebuilding the prefix alone would have handed `%5d`
// on and made the 3 the value — and, because the pass would then have
// consumed one operand instead of two, reused the format and written a second
// field nobody asked for. So the losing stars are counted out of here as
// printfLostStars and read by printfStars in the place they were written.
func printfKshPrefix(s string, quote, mixed, restart bool) (int, string, printfLostStars) {
	i := 1 // past the %
	flags, width, prec := "", "", ""
	widthStars, precStars := 0, 0
	for {
		for {
			if n := runOfBytes(s, i, "-+ #0"); n > 0 {
				flags += s[i : i+n]
				i += n
			}
			if n, stars, text := printfKshFieldRun(s, i, mixed); n > 0 {
				widthStars += stars
				width = text
				i += n
			}
			if quote && i < len(s) && s[i] == '\'' {
				i++
				continue
			}
			if restart && i < len(s) && strings.IndexByte("-+ #", s[i]) >= 0 {
				// A flag past the field restarts the scan the same way the
				// quote does, and the run above takes the flag on the next
				// turn — so the loop always moves on. `0` is not in this set
				// and cannot be: the field run has already taken it, which
				// is what makes `%50d` a width of fifty.
				continue
			}
			break
		}
		if i == len(s) || s[i] != '.' {
			break
		}
		i++
		// Precision 0 until a run says otherwise, which is what a bare `.`
		// means and what an empty run after a quote leaves behind.
		prec = "."
		back := false
		for {
			if n, stars, text := printfKshFieldRun(s, i, mixed); n > 0 {
				precStars += stars
				prec = "." + text
				i += n
			}
			if quote && i < len(s) && s[i] == '\'' {
				i++
				continue
			}
			if restart && i < len(s) && (s[i] == '+' || s[i] == ' ') {
				// Taken as a flag, and the precision goes on being read: the
				// run after it replaces the one before, exactly as a quote's
				// does. `%.3+5d` is `+00042`.
				flags += string(s[i])
				i++
				continue
			}
			if restart && i < len(s) && s[i] == '-' {
				// And this one is the rule that disagrees with the width's.
				// A `-` past a precision **throws the precision away** and
				// is not itself a flag: `%.3-5d` is `   42`, width five and
				// right-justified, where a `-` past a *width* left-justifies
				// (#2910). What follows it is read as a width again, which
				// is why the scan goes back around rather than ending here.
				i++
				prec, back = "", true
			}
			break
		}
		if !back {
			break
		}
	}
	// The last run of each field is the one the conversion uses. Every star
	// before it took an operand nothing then used, and if the last run is not
	// a star at all then every star in that field did.
	lost := printfLostStars{width: widthStars, prec: precStars}
	if width == "*" {
		lost.width--
	}
	if prec == ".*" {
		lost.prec--
	}
	return i, "%" + dedupFlags(flags) + width + prec, lost
}

// printfKshFieldRun reads one width or precision for the reading above, and
// reports how far it reached, how many `*` it held and what the field comes to.
//
// mixed is the whole of the difference, and it is
// Semantics.PrintfStarBesideTheFieldDigits: with it off a run is a run of
// digits *or* a single star, which is C's grammar and every other column's;
// with it on the two may stand side by side and the **star wins**, whichever
// side of the digits it was written on.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01 under `LC_ALL=C`, with the value
// 42 and the width operands in front of it:
//
//	%5*d     width from the operand, the 5 dropped        [  42] for 4
//	%*5d     the same the other way round                 [  42] for 4
//	%*8*d    two stars, both read, and the last wins      [    42] for 4 6
//	%8*9d    digits on both sides change nothing          [  42] for 4
//	%.2*d    and the precision reads the same way         [0042] for 4
//	%*0d     a `0` after a star is a digit, not the flag  [  42] for 4
//
// A losing star has still taken its operand, exactly as one a quote's restart
// replaced has — see printfLostStars — which is why the count comes back out
// rather than the star simply being dropped.
func printfKshFieldRun(s string, i int, mixed bool) (n, stars int, text string) {
	if !mixed {
		n = printfFieldRun(s, i)
		if n == 0 {
			return 0, 0, ""
		}
		if s[i] == '*' {
			return n, 1, s[i : i+n]
		}
		return n, 0, s[i : i+n]
	}
	start := i
	for i < len(s) && (s[i] == '*' || (s[i] >= '0' && s[i] <= '9')) {
		if s[i] == '*' {
			stars++
		}
		i++
	}
	if i == start {
		return 0, 0, ""
	}
	if stars > 0 {
		return i - start, stars, "*"
	}
	return i - start, 0, s[start:i]
}

// printfLostStars is how many `*` in a conversion's prefix took an operand
// that nothing then used, counted on each side of the surviving one.
//
// Only the ksh93 reading produces any: there a `'` restarts the field, so a
// run written after a star *replaces* it and the star has already taken its
// operand. `printf "[%*'5d]" 3 42` is `[   42]` — the 3 went to a width the
// 5 replaced, and the 42 reached the conversion.
//
// Counted on each side because the order the operands are read in is the
// order the stars are written in, and a width star always precedes a
// precision star: `%*.*'5d` takes its width from the first and then loses the
// second.
type printfLostStars struct{ width, prec int }

func (l printfLostStars) any() bool { return l.width > 0 || l.prec > 0 }

// dedupFlags keeps one of each flag, in the order they were written.
//
// The restarts above can write the same one twice — `%0'0'5d` holds two
// zeros — and a spec `fmt` is handed has to be one it reads.
func dedupFlags(flags string) string {
	var b strings.Builder
	for i := 0; i < len(flags); i++ {
		if strings.IndexByte(b.String(), flags[i]) < 0 {
			b.WriteByte(flags[i])
		}
	}
	return b.String()
}

// printfGroupingFlagPositions says where a `'` is written in a conversion's
// prefix: among the flags, past them, or — the common answer, and the one
// that asks nothing — neither.
//
// The prefix is walked under the *widest* reading, ksh93's, because the
// question is what the conversion is asking for and not yet what this shell
// answers. Narrowing it to the dialect first would ask the flag axis only
// where the dialect already had the flag, which is the wrong way round.
func printfGroupingFlagPositions(s string) (inFlags, pastFlags bool) {
	end, _, _ := printfSpecPrefixAt(s, true, true, true, true)
	flagEnd := 1 + runOfBytes(s, 1, "-+ #0'")
	for i := 1; i < end && i < len(s); i++ {
		switch {
		case s[i] != '\'':
		case i < flagEnd:
			inFlags = true
		default:
			pastFlags = true
		}
	}
	return inFlags, pastFlags
}

// runOfBytes is how many bytes at i are drawn from set.
func runOfBytes(s string, i int, set string) int {
	n := 0
	for i+n < len(s) && strings.IndexByte(set, s[i+n]) >= 0 {
		n++
	}
	return n
}

// printfFieldRun is how much of s at i is a width or a precision: a run of
// digits, or the single `*` that takes one from the operand list instead.
//
// The star belongs here and not at the verb. Accepting only digits is what
// made `printf '%*d' 6 42` refuse: the `*` fell out of the prefix and
// arrived at the scan as the conversion character, so every dialect reported
// a conversion it did not have — each in its own correct wording, which is
// why the diagnostics looked right and the answer was wrong (#2646).
func printfFieldRun(s string, i int) int {
	if i < len(s) && s[i] == '*' {
		return 1
	}
	n := 0
	for i+n < len(s) && s[i+n] >= '0' && s[i+n] <= '9' {
		n++
	}
	return n
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
		var stop bool
		n, code, stop = r.printfNumber(arg, present)
		if stop {
			return "", code, true
		}
		t = time.Unix(n, 0)
	}
	t = t.In(r.timeZone())
	if format == "" {
		format = "%X"
	}
	// The width and the flags belong to the *result*, not to the date: a
	// `%10(%Y)T` pads the four digits out to ten.
	field, unanswered := r.printfStringField(spec, strftime(format, t), false)
	if unanswered {
		return "", r.status, true
	}
	return field, code, false
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
	field, unanswered := r.printfStringField(spec, strftime(format, t), false)
	if unanswered {
		return "", r.status, true
	}
	return field, code, false
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
	r.diagf("%s\n", Wording(d.PrintfBadVerb, "printf: %[2]s: invalid directive", verb, r.quotedDirective(conversion)))
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
	r.diagf("%s\n", Wording(d.PrintfMissingVerb, "printf: %[1]s: missing format character", r.quotedDirective(conversion)))
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
	case 'e', 'E':
		// The colour idiom's own escape, and one shape asked twice.
		//
		// Not the `%b` site's pair under another name: ksh93 takes both
		// letters here and only `\E` there, so a format that borrowed
		// PrintfBEscEscape would write `a\eZ` for the one column whose
		// two sites disagree. zsh and BusyBox ash take `\e` at both sites
		// and `\E` at neither, which is what keeps the two letters two
		// questions here as they are there (#3225).
		axis := r.sem().PrintfEscEscape
		if c == 'E' {
			axis = r.sem().PrintfCapitalEscEscape
		}
		if r.ask(axis, `printf: \`+string(c)+` in a format`) {
			return "\x1b", 2, printfPassRan
		}
		return `\` + string(c), 2, printfPassRan
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
			x, next, ok := r.controlArgument(DollarSingleControlToggled, s, 2)
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
		// and a zero. One of the two shells that get here says nothing at
		// all, which is an answer rather than an empty wording — see
		// Semantics.PrintfReportsAMissingHexDigit.
		if r.ask(r.sem().PrintfReportsAMissingHexDigit, "`printf '\\x'` reporting a missing hex digit") {
			d := r.diag()
			r.diagf("%s\n", Wording(d.PrintfMissingHexDigit, `printf: missing hex digit for \x`))
		}
		if r.unspecified {
			return "", 0
		}
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
// depending on which the dialect does — and, in the one dialect that takes a
// pass back, the buffer that rewind comes out of.
//
// Everything is buffered now, which used to be the hold-it-all dialects'
// arrangement alone. The dialect that writes through releases each
// conversion's worth as the *next* conversion begins, so what is still held
// is exactly what a rewind could reach — the pass back to the start of the
// last conversion that completed (#2664).
//
// The diagnostics a conversion produces are held with it, and that is what
// lets both halves be true at once. ksh93 writes `[` before it complains
// about the `%z` that followed, so the complaint cannot simply wait for the
// end of the pass; and it answers `X` for `printf 'X%dY%*.*dZ' 42abc 7abc`,
// where the complaint about `42abc` goes out and the rewind still reaches
// back past `42Y`. Holding the complaint until the conversion it belongs to
// has resolved settles both: the output before that conversion is released
// first, or the pass is taken back, and only then does the complaint go out.
type printfWriter struct {
	w io.Writer
	r *Runner
	// pending is what has been produced and not yet written. In a
	// write-through dialect it reaches back only as far as a rewind could;
	// in the others it is the whole pass.
	pending []byte
	// written is how much of the pass has already gone out, so that a mark
	// can name a place in the pass rather than a place in pending.
	written int
	// floor is the mark a rewind stops at: the start of the last conversion
	// that completed. Kept in both kinds of dialect, because the refusal
	// that rewinds is an axis of its own and not the write-through one.
	floor int
	// inConv says a conversion is being formatted, so a diagnostic belongs
	// to it and waits for it. diags is what has been said meanwhile.
	inConv bool
	diags  []string
	// rewound records that this conversion refused and took the pass back,
	// so the end of it releases nothing.
	rewound bool
	// through says the dialect writes as it produces. The others hold the
	// whole pass on purpose — their complaint reaches the reader first.
	through bool
}

func (p *printfWriter) WriteString(s string) { p.pending = append(p.pending, s...) }

// writeByte is not WriteByte: that name carries an error return by
// convention, and this writer reports one the way every builtin's output
// does — on the runner, for the dispatcher to fold in.
//
// The byte is appended rather than converted through `string(c)`, which is a
// *rune* conversion: it spells 0xc0 as the two bytes UTF-8 gives U+00C0, so a
// format holding a byte no encoding claims came out as two.
func (p *printfWriter) writeByte(c byte) { p.pending = append(p.pending, c) }

// mark names where the pass stands now, counted from its start rather than
// from the front of pending — which moves as text is released.
func (p *printfWriter) mark() int { return p.written + len(p.pending) }

// begin opens a conversion: what it says is held until end says how it went.
func (p *printfWriter) begin() { p.inConv = true }

// end closes the conversion that began at mark, releases or rewinds, and only
// then lets out what the conversion had to say.
//
// completed is whether the conversion reached the operand list and finished,
// which is what moves the floor. Reaching the list is the test and not
// finding anything in it: `printf 'AB%sCD%*dEF'` with no operands at all is
// `AB` in ksh93, so a `%s` that read a missing operand marks exactly as one
// that read a present operand does.
func (p *printfWriter) end(mark int, completed bool) {
	p.inConv = false
	switch {
	case p.rewound:
		// Taken back, so nothing before this conversion may go out either:
		// the rewind reaches past it to the floor.
		p.rewound = false
	case completed:
		p.release(mark)
		if mark > p.floor {
			p.floor = mark
		}
	case len(p.diags) > 0:
		// Something is about to be said about a conversion that did not
		// complete. In the dialect that writes through, what stands in front
		// of it goes out first — ksh93's `[` arrives before its complaint
		// about the `%z` that followed. A conversion with nothing to say and
		// nothing consumed releases nothing, which is what keeps a `%%` from
		// putting the text before it beyond a later rewind's reach.
		p.release(mark)
	}
	diags := p.diags
	p.diags = nil
	for _, d := range diags {
		p.r.errf("%s", d)
	}
}

// hold takes a diagnostic written while a conversion is being formatted, and
// reports whether it was taken. Outside a conversion nothing is held.
func (p *printfWriter) hold(msg string) bool {
	if !p.inConv {
		return false
	}
	p.diags = append(p.diags, msg)
	return true
}

// release lets out everything produced before to, in the dialect that writes
// through. In the others it does nothing: they hold the pass to the end.
func (p *printfWriter) release(to int) {
	if !p.through {
		return
	}
	n := to - p.written
	if n <= 0 {
		return
	}
	p.write(string(p.pending[:n]))
	p.pending = append(p.pending[:0], p.pending[n:]...)
	p.written = to
}

// rewind takes the pass back to the floor, which is the refusal in #2664.
// Text already released cannot be taken back, and nothing puts the floor
// behind what was released.
func (p *printfWriter) rewind() {
	if n := p.floor - p.written; n >= 0 && n <= len(p.pending) {
		p.pending = p.pending[:n]
	}
	p.rewound = true
}

func (p *printfWriter) flush() {
	p.written += len(p.pending)
	text := string(p.pending)
	p.pending = p.pending[:0]
	p.write(text)
}

func (p *printfWriter) write(s string) {
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

// charConstant reads an operand written `'c` or `"c`, where a numeric
// conversion takes the *character's* value rather than reading digits.
//
// POSIX XCU gives `printf` this in so many words — "if the leading character
// is a single-quote or double-quote, the value shall be the numeric value in
// the underlying codeset of the character following" — and all seven columns
// have it: bash 5.3, bash-as-`sh`, bash 3.2, zsh 5.9.2, ksh93, dash and
// BusyBox ash. So it is the core's. Without it `printf '0x%x' "'a"` was
// `printf: 'a: invalid number` and a zero, which is what the idiom for "the
// code point of this character" — the one way a shell has of asking — came
// to here.
//
// Three details, each measured:
//
//   - The quote has to be the operand's first byte. A blank in front of it
//     makes the word an ordinary operand again and a bad number — six of the
//     seven, ksh93 alone reading through the blank — so the text is not
//     trimmed before this, where a plain numeral is.
//   - Anything after the first character is ignored rather than refused:
//     `'AB` is 65 in every column, ksh93 printing a warning beside the same
//     answer.
//   - A quote with nothing after it is zero, not an error.
//
// The character is the locale's, through the reader every other length and
// position in this package goes through: a UTF-8 locale gives the code point
// and a single-byte one gives the first byte, which is what the panel does —
// `printf '%d' "'é"` is 233 under a UTF-8 locale and 195 under LC_ALL=C.
func (r *Runner) charConstant(arg string) (int64, bool) {
	if arg == "" || (arg[0] != '\'' && arg[0] != '"') {
		return 0, false
	}
	rest := arg[1:]
	if rest == "" {
		return 0, true
	}
	if r.countsCharacters(rest) {
		c, _ := utf8.DecodeRuneInString(rest)
		return int64(c), true
	}
	return int64(rest[0]), true
}
