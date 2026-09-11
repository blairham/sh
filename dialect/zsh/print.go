// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/blairham/sh/interp"
)

// `print` is this shell's richer `echo`, and it is *not* ksh93's builtin under
// the same spelling. Measured 2026-09-05 against zsh 5.9.2 and ksh93u+ side by
// side, under LC_ALL=C, and the two disagree in every part that could differ:
//
//   - The letters. `-l`, `-N`, `-m`, `-o`, `-O`, `-i`, `-z`, `-S`, `-b`, `-c`,
//     `-C`, `-D`, `-a`, `-P`, `-x` and `-X` are this shell's and ksh93 has
//     none of them; `-e` is a *bad option* here, where ksh93 uses it to put
//     escape expansion back — this shell spells that only inside `-R`.
//   - The escape set. `\1` is a byte here and two characters in ksh93; `\e`,
//     `\x41`, `A`, `\U00000041`, `\M-a` and `\C-a` are escapes here and
//     literal text there; and an escape this shell does not know loses its
//     backslash — `\z` prints `z` here and `\z` in ksh93. It is a superset of
//     what this shell's own `echo` expands, which stops at `\e` and `\x`: the
//     bare-octal, `\M-` and `\C-` forms and the backslash-dropping default are
//     `print`'s alone.
//   - Every wording, and the status. A usage complaint is 1 here with no usage
//     line after it, where ksh93 prints one and exits 2.
//
// The separator and the terminator are two settings rather than one, which is
// what makes `-l` and `-N` compose the way they were measured to:
//
//	          separator   terminator
//	default   " "         "\n"
//	-l        "\n"        "\n"
//	-N        "\0"        "\0"
//	-lN       "\n"        "\0"
//	-n        (unchanged) ""
//
// Measured option by option, and the ones this shell has that are not
// implemented here are refused by name rather than accepted and ignored:
//
//   - `-r` prints the operands raw. `-R` is raw *and* switches the parsing of
//     every **later** word to echo's: only a word made of `e` and `n` is an
//     option there, `-e` puts the expansion back, and anything else — `--`
//     included — is an operand. Letters after `R` in the *same* bundle are
//     still read as print's own, which is why `-Rl` lists one per line and
//     `-R -l` prints `-l` as a word.
//   - `-n` withholds the terminator, `-l` separates with newlines, `-N`
//     separates and terminates with NULs.
//   - `-u fd` aims the output at a descriptor, the number attached or the next
//     word. A word that is no number is `number expected after -u: q`; a
//     number nothing is open at is `bad file number: 9`; a number open for
//     reading is `bad mode on fd 3`, which is the refusal the write itself
//     reports — see printWriteFailed. All three are 1.
//   - `-m` takes the first operand as a pattern and prints only the operands
//     matching it; with no operand at all it is `no pattern specified`, 1.
//   - `-o` sorts the operands, `-O` sorts them in reverse and `-i` folds case
//     while doing it. `-O` is not the later-wins opposite of `-o` but a
//     reversing bit on top of it: `print -O -o` and `print -Oo` both sort
//     descending, measured. The order recorded is
//     the byte order this repository's oracle measures under LC_ALL=C; in a
//     collating locale the shell answers the locale rather than itself.
//   - `-s` and `-S` write to a history this shell does not keep here, and `-z`
//     to a line editor that is not running: the operands are consumed and the
//     answer is 0, which is what non-interactive zsh answers too. `-S` takes
//     at most one operand — `option -S takes a single argument`, 1.
//   - `-p` writes to the coprocess `coproc` started, and is the measured
//     `-p: no coprocess` at 1 when none is running — the same shape `read -p`
//     answers, in the same words.
//   - `-f format` hands the whole command to printf — format reused over the
//     operands, no terminator added.
//   - `--` and a lone `-` both end the options, and so does a dash with a
//     digit straight after it — `print -1` prints `-1`. See
//     printNumberOperand.
//
// An unknown letter is `bad option: -q` at 1, with nothing printed.
//
// Two edges are recorded rather than claimed. `print -u0` is `bad mode on fd
// 3` in zsh — a number that names neither the descriptor asked about nor
// anything else in the command — and is `bad file number: 0` here, because
// repeating a wording that is wrong about its own subject would be the worse
// of the two. The *sentence* is this shell's own and is written where it is
// true: a descriptor really open for reading gets it, with its own number in
// it. And `print -m '['` is `bad pattern: [` at 1 there; the core's
// matcher treats an unterminated bracket as this dialect's fatal pattern,
// which abandons the script, so the pattern is checked here first and refused
// with the measured wording and status.

// printLetters are the option letters implemented here.
const printLetters = "rRnlNmoOiszSpufP"

// printUnimplemented are the letters zsh's print has that this one does not:
// the column layouts (`-a`, `-c`, `-C`), the bindkey-style escapes (`-b`), the
// `~`-abbreviating one (`-D`), prompt expansion (`-P`), assignment to a
// parameter (`-v`) and the tab-expanding pair (`-x`, `-X`).
const printUnimplemented = "acCbDvxX"

// registerPrint installs the builtin.
func registerPrint(r *interp.Runner) {
	r.Register("print", printBuiltin)
}

// printOptions is what the option words asked for.
type printOptions struct {
	raw       bool
	echoMode  bool
	noTerm    bool
	lineSep   bool
	nulSep    bool
	match     bool
	sortDesc  bool
	sorted    bool
	fold      bool
	history   bool
	single    bool
	editor    bool
	prompt    bool
	format    string
	hasFormat bool
	fd        int
}

// separator and terminator are the two settings the table above records.
func (o printOptions) separator() string {
	switch {
	case o.lineSep:
		return "\n"
	case o.nulSep:
		return "\x00"
	default:
		return " "
	}
}

func (o printOptions) terminator() string {
	switch {
	case o.noTerm:
		return ""
	case o.nulSep:
		return "\x00"
	default:
		return "\n"
	}
}

func printBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts := printOptions{fd: 1}
	rest, code := readPrintOptions(r, args, &opts)
	if code >= 0 {
		return code
	}
	if opts.match {
		var ok bool
		if rest, ok = printMatching(r, rest); !ok {
			return 1
		}
	}
	if opts.sorted {
		printSort(rest, opts)
	}
	switch {
	case opts.single && len(rest) > 1:
		r.Diagnosef("option -S takes a single argument\n")
		return 1
	case opts.history, opts.editor:
		// A history this shell does not keep and a line editor that is not
		// running: the operands are consumed and nothing is written.
		return 0
	}
	if opts.hasFormat {
		return printFormatted(r, ctx, opts, rest)
	}
	out, ok := r.WriterForFd(opts.fd)
	if !ok {
		r.Diagnosef("bad file number: %d\n", opts.fd)
		return 1
	}
	text, ok := printText(r, opts, rest)
	if !ok {
		return 1
	}
	if _, err := io.WriteString(out, text); err != nil {
		return printWriteFailed(r, opts.fd, err)
	}
	return 0
}

// printWriteFailed says what a write that did not happen was refused for.
//
// The one refusal measured, and the one this exists for, is a descriptor open
// for reading: zsh 5.9.2, 2026-09-10, with `zmodload zsh/system`, a file of
// text, `sysopen -u ro f` — which opens read-only with no direction letter —
// and `print -u $ro -- nope`:
//
//	zsh:print:1: bad mode on fd 3    st=1
//
// Before this the write went to a descriptor that could not take it, the error
// was discarded, and the answer was 0 with nothing written (#1751). A script
// writing to the wrong one of two descriptors it holds was told nothing at
// all, and the bytes went nowhere.
//
// The mode is read from the refusal rather than asked for in advance, because
// a descriptor's mode is not a thing this shell's table records: what it holds
// is a stream, and only the write can say whether the file behind it will take
// one. EBADF on a descriptor the table *does* hold is that answer — the number
// is open here and the file will not be written — and it is what this shell's
// sentence is about. `bad file number` is the other complaint and is already
// answered above, where the number names nothing at all.
//
// Any other error is left as it was: what this shell says for a write that
// fails for some other reason is unmeasured, and inventing a second sentence
// under the first one's wording would be a guess wearing a measurement.
func printWriteFailed(r *interp.Runner, fd int, err error) int {
	if errors.Is(err, syscall.EBADF) {
		r.Diagnosef("bad mode on fd %d\n", fd)
		return 1
	}
	return 0
}

// printText is the whole of what one `print` writes.
//
// Escapes are expanded per operand rather than over the joined text, which is
// what `print 'a\' t` measures: the trailing backslash stays a backslash
// instead of joining the next operand's letter into an escape. A `\c` ends the
// command's output where it stands — the rest of that operand, every operand
// after it, and the terminator.
//
// `-P` runs the prompt escapes over each operand **after** the backslash
// escapes and never over their own result, which is measured both ways round:
// `print -P '\045n'` is the user's name — the `\045` became a `%` and the
// prompt pass then read `%n` — while `print -P '%%n'` is the two characters
// `%n`, because the `%` that `%%` produced is not looked at again. `-r`
// suppresses the backslash pass and leaves this one: `print -rP 'a\tb %n'`
// keeps the backslash and expands the name.
//
// The second result is false where an escape was refused, and the refusal has
// already been written. Nothing is printed in that case — a `print` that
// wrote the operands it managed and then complained would leave a script
// holding a line it could not tell apart from a whole one.
func printText(r *interp.Runner, opts printOptions, words []string) (string, bool) {
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteString(opts.separator())
		}
		expanded, stopped := w, false
		if !opts.raw {
			expanded, stopped = expandPrintEscapes(w)
		}
		if opts.prompt {
			var ok bool
			if expanded, ok = r.PromptExpand(expanded); !ok {
				return "", false
			}
		}
		b.WriteString(expanded)
		if stopped {
			return b.String(), true
		}
	}
	b.WriteString(opts.terminator())
	return b.String(), true
}

// printMatching is `-m`: the first operand is a pattern and the rest are kept
// only where they match it.
func printMatching(r *interp.Runner, rest []string) ([]string, bool) {
	if len(rest) == 0 {
		r.Diagnosef("no pattern specified\n")
		return nil, false
	}
	pattern, operands := rest[0], rest[1:]
	if unterminatedBracket(pattern) {
		// The core's matcher makes this dialect's fatal pattern out of an
		// unterminated bracket, which abandons the script; the builtin's own
		// answer is a refusal it returns from.
		r.Diagnosef("bad pattern: %s\n", pattern)
		return nil, false
	}
	kept := make([]string, 0, len(operands))
	for _, w := range operands {
		if r.MatchPattern(pattern, w) {
			kept = append(kept, w)
		}
	}
	return kept, true
}

// unterminatedBracket reports a `[` that never closes, which is the one
// pattern shape this shell rejects outright. A `!` or `^` directly after the
// bracket negates and a `]` directly after that is a member rather than the
// terminator, so `[]]` closes and `[]` does not.
func unterminatedBracket(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			j := i + 1
			if j < len(s) && (s[j] == '!' || s[j] == '^') {
				j++
			}
			if j < len(s) && s[j] == ']' {
				j++
			}
			for ; j < len(s); j++ {
				if s[j] == ']' {
					break
				}
			}
			if j >= len(s) {
				return true
			}
			i = j
		}
	}
	return false
}

// printSort is `-o`, `-O` and `-i`. Byte order, which is the order the panel
// gives under the LC_ALL=C the oracle runs in.
func printSort(words []string, opts printOptions) {
	key := func(s string) string {
		if opts.fold {
			return strings.ToLower(s)
		}
		return s
	}
	sort.SliceStable(words, func(i, j int) bool {
		a, b := key(words[i]), key(words[j])
		if opts.sortDesc {
			return a > b
		}
		return a < b
	})
}

// readPrintOptions reads the leading option words. A code of -1 means run;
// anything else is the answer, already reported.
func readPrintOptions(r *interp.Runner, args []string, opts *printOptions) (rest []string, code int) {
	rest = args
	for len(rest) > 0 {
		word := rest[0]
		if !strings.HasPrefix(word, "-") || word == "-" || printNumberOperand(word) {
			break
		}
		if opts.echoMode {
			// After `-R`, the parsing is echo's: only a word made of `e` and
			// `n` is an option, and `--` is an operand like anything else.
			if strings.Trim(word[1:], "en") != "" {
				break
			}
		} else if word == "--" {
			rest = rest[1:]
			return rest, -1
		}
		// `-e` is echo's letter and is reachable only in a word that was
		// already in echo mode when it started: `print -R -e` puts the
		// expansion back, and `print -Re` is `bad option: -e`.
		echoWord := opts.echoMode
		rest = rest[1:]
		letters := word[1:]
		for i := 0; i < len(letters); i++ {
			letter := letters[i]
			switch {
			case letter == 'e' && echoWord:
				opts.raw = false
			case letter == 'f' || letter == 'u':
				arg, more, ok := printOptionArgument(r, letter, letters[i+1:], rest)
				if !ok {
					return nil, 1
				}
				rest = more
				if code := applyPrintArgument(r, letter, arg, opts); code >= 0 {
					return nil, code
				}
				i = len(letters)
			case strings.IndexByte(printLetters, letter) >= 0:
				if code := setPrintLetter(r, letter, opts); code >= 0 {
					return nil, code
				}
			case strings.IndexByte(printUnimplemented, letter) >= 0:
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return nil, 1
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return nil, 1
			}
		}
	}
	if len(rest) > 0 && rest[0] == "-" {
		rest = rest[1:]
	}
	return rest, -1
}

// printNumberOperand reports a word that looks like an option and is not one:
// a dash with a **digit** straight after it, which this shell's `print` reads
// as the first operand and not as a bundle of letters.
//
// It is the first character after the dash that decides, and nothing else.
// Measured 2026-09-10 on zsh 5.9.2 with no startup files, where `print -1`,
// `print -12`, `print -1x`, `print -0` and `print -1.5` each print themselves
// and `print -r -1` prints `-1` — so the word is not a *number*, it merely
// starts with a digit, and a letter word before it is still an option word.
// The rule is one-sided: a digit that is not first is an option letter in that
// shell as it is here, and `print -n1` is `bad option: -1` in both.
//
// A `+` leads an operand too, and needs no test — the loop only ever looks at
// a word beginning with a dash.
//
// The word *ends* the options rather than being passed over, which is what
// `print -1 -r` measures: both words print, so the second is an operand and
// was never read as a letter.
//
// Without this, anything printing a negative number without `--` in front of
// it drew a complaint about an option nobody wrote (#1652).
func printNumberOperand(word string) bool {
	return len(word) > 1 && word[1] >= '0' && word[1] <= '9'
}

// printOptionArgument reads the argument of `-f` or `-u`: attached to the
// letter, or the next word.
func printOptionArgument(r *interp.Runner, letter byte, attached string, rest []string) (arg string, more []string, ok bool) {
	if attached != "" {
		return attached, rest, true
	}
	if len(rest) == 0 {
		r.Diagnosef("argument expected: -%c\n", letter)
		return "", nil, false
	}
	return rest[0], rest[1:], true
}

// applyPrintArgument stores what `-f` or `-u` was given. A code of -1 means
// carry on.
func applyPrintArgument(r *interp.Runner, letter byte, arg string, opts *printOptions) int {
	if letter == 'f' {
		opts.format, opts.hasFormat = arg, true
		return -1
	}
	fd, err := strconv.Atoi(arg)
	if err != nil {
		r.Diagnosef("number expected after -u: %s\n", arg)
		return 1
	}
	if fd < 0 {
		r.Diagnosef("bad file number: %d\n", fd)
		return 1
	}
	opts.fd = fd
	return -1
}

// setPrintLetter applies one flag letter. A code of -1 means carry on; `-p` is
// the one letter whose whole answer is a refusal.
func setPrintLetter(r *interp.Runner, letter byte, opts *printOptions) int {
	switch letter {
	case 'r':
		opts.raw = true
	case 'R':
		opts.raw, opts.echoMode = true, true
	case 'n':
		opts.noTerm = true
	case 'l':
		opts.lineSep = true
	case 'N':
		opts.nulSep = true
	case 'm':
		opts.match = true
	case 'o':
		opts.sorted = true
	case 'O':
		opts.sorted, opts.sortDesc = true, true
	case 'i':
		opts.fold = true
	case 's':
		opts.history = true
	case 'S':
		opts.history, opts.single = true, true
	case 'z':
		opts.editor = true
	case 'P':
		opts.prompt = true
	case 'p':
		fd, running := r.CoprocWrite()
		if !running {
			r.Diagnosef("-p: no coprocess\n")
			return 1
		}
		opts.fd = fd
	}
	return -1
}

// printFormatted is `-f`: printf with this command's operands. The core printf
// writes to the runner's standard output, so a descriptor `-u` moved off 1
// would need a seam printf does not have — refused out loud rather than sent
// to the wrong stream.
func printFormatted(r *interp.Runner, ctx context.Context, opts printOptions, rest []string) int {
	if opts.fd != 1 {
		r.Diagnosef("-f with -u is not implemented yet\n")
		return 1
	}
	printf, ok := r.Builtin("printf")
	if !ok {
		return 1
	}
	return printf(r, ctx, append([]string{opts.format}, rest...))
}

// expandFlagArgumentEscapes is the same set read for an expansion flag's
// argument — the `(p)` flag's whole job — and it differs from `print`'s in
// exactly one place.
//
// `\c` ends `print`'s output where it stands, and in a flag argument it is an
// escape this shell does not know. Measured 2026-09-07 on zsh 5.9.2 with
// `a=(x y)`: `${(pj:A\cB:)a}` is `xAcBy`, so the backslash is dropped and the
// `c` kept, and `${(pj:\\c:)a}` is `x\cy`, where the doubled backslash is a
// backslash and the `c` after it is not an escape at all.
//
// Everything else is the same measurement, so this is the one decoder told
// which of the two it is rather than a second copy of it: `\101` is `A` in
// both, `\q` is `q` in both, and a trailing backslash is a backslash in both.
func expandFlagArgumentEscapes(s string) string {
	out, _ := expandEscapes(s, escapeReading{bareOctal: true, printEscapes: true})
	return out
}

// expandPrintEscapes is the measured escape set, applied to one operand. The
// second result reports a `\c`, which ends the whole command's output where it
// stands.
func expandPrintEscapes(s string) (string, bool) {
	return expandEscapes(s, escapeReading{
		cTruncates: true, bareOctal: true, printEscapes: true,
	})
}

// expandExpansionFlagEscapes is the third reading of the same set: the `(g)`
// expansion flag, whose delimited argument names which parts of it are live.
//
// The vendor manual gives the option letters as three additions to a base —
// `o` for octal escapes that need no leading zero, `e` for the `\M-t` family,
// `c` for `^X` — and says that in none of the readings is `\c` interpreted.
// Measured 2026-09-09 on zsh 5.9.2, which is where the base itself comes
// from, since "like the echo builtin" is a claim about *that* shell's echo:
//
//	value      ${(g::)v}  ${(g:o:)v}  ${(g:e:)v}  ${(g:c:)v}
//	X\tY       tab        tab         tab         tab
//	X\eY       escape     escape      escape      escape
//	X\EY       X\EY       X\EY        escape      X\EY
//	X\x41Y     XAY        XAY         XAY         XAY
//	X\101Y     X\101Y     XAY         X\101Y      X\101Y
//	X\0101Y    XAY        backspace1  XAY         XAY
//	X\u0041Y   XAY        XAY         XAY         XAY
//	X\cY       X\cY       X\cY        XcY         X\cY
//	X\M-AY     X\M-AY     X\M-AY      0xc1        X\M-AY
//	X\C-AY     X\C-AY     X\C-AY      0x01        X\C-AY
//	X\qY       X\qY       X\qY        XqY         X\qY
//	X^XY       X^XY       X^XY        X^XY        0x18
//
// Two rows are the discriminating ones and both are `o`. `\0101` is `A` in
// the base and a backspace followed by `1` under `o`, because `o` does not
// mean "octal as well" — it means the leading zero is not part of the escape,
// so the same three digits are read from a different place. And `\101` is
// text in every reading but that one. A `g` implemented as "process the
// escapes" answers both rows the way the base does and looks right on the
// other ten.
//
// `\c` is the row the manual is explicit about and the row a reader would
// otherwise get from `print`: it never truncates here, so under `e` it is an
// escape this shell does not know and loses its backslash, exactly as `\q`
// does, and in the other three readings it is two characters of text.
func expandExpansionFlagEscapes(s, opts string) string {
	out, _ := expandEscapes(s, escapeReading{
		bareOctal:    strings.ContainsRune(opts, 'o'),
		printEscapes: strings.ContainsRune(opts, 'e'),
		caret:        strings.ContainsRune(opts, 'c'),
	})
	return out
}

// escapeReading is which of the set's parts one reader has live.
//
// Four booleans rather than four decoders, because the set is one
// measurement: `\101` is `A` wherever octal is read bare, `\M-\C-a` is 0x81
// wherever the family is read at all, and a second copy of either is a second
// place for them to stop agreeing. The three callers above are the readings
// this shell actually has.
type escapeReading struct {
	// cTruncates says whether `\c` ends the output, which it does for an
	// operand of `print` and does not for the argument of an expansion flag
	// or for any reading of the `(g)` flag.
	cTruncates bool
	// bareOctal says whether `\NNN` is octal without a leading zero. When it
	// is not, only `\0NNN` is octal and the zero is not one of the digits.
	bareOctal bool
	// printEscapes says whether the `\M-x` and `\C-x` family is read, `\E` is
	// the escape character, and an escape this shell does not know loses its
	// backslash rather than keeping it. The four move together: they are what
	// `print` reads and this shell's `echo` does not.
	printEscapes bool
	// caret says whether `^X` is a control character, which only the `(g)`
	// flag's `c` option turns on.
	caret bool
}

// expandEscapes is the escape set with the places it is read more than one way
// made parameters. See escapeReading, and expandExpansionFlagEscapes for the
// measurement behind each.
func expandEscapes(s string, how escapeReading) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '^' && how.caret && i+1 < len(s) {
			// `^X` is the control character, and it is read where the
			// character is rather than behind a backslash. Measured:
			// `X^^^AY` under `c` is 0x1e then 0x01, so a `^` is as good a
			// target as a letter, and a trailing `^` with nothing after it
			// is a `^`.
			b.WriteByte(controlByte(s[i+1]))
			i++
			continue
		}
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 == len(s) {
			// A trailing backslash is a backslash.
			b.WriteByte('\\')
			continue
		}
		i++
		switch c := s[i]; c {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'c':
			if how.cTruncates {
				return b.String(), true
			}
			// Not an escape here, so it falls to whichever rule the reading
			// has for one it does not know.
			writeUnknownEscape(&b, c, how)
		case 'e':
			b.WriteByte(0x1b)
		case 'E':
			// The capitalized spelling is `print`'s and not `echo`'s, so it
			// is text in a reading without the family. Measured: `X\EY`
			// under `${(g::)v}` is `X\EY` and under `${(g:e:)v}` is the
			// escape character.
			if !how.printEscapes {
				writeUnknownEscape(&b, c, how)
				break
			}
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\':
			b.WriteByte('\\')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			if !how.bareOctal {
				if c != '0' {
					// Without the zero it is not an escape at all.
					writeUnknownEscape(&b, c, how)
					break
				}
				// The zero introduces the escape and is not one of the three
				// digits, which is what makes `\0101` an `A` here and a
				// backspace followed by `1` under `o`.
				i = writeOctal(&b, s, i+1)
				break
			}
			i = writeOctal(&b, s, i)
		case 'x':
			n, j := 0, i+1
			for j < len(s) && j <= i+2 && isPrintHexDigit(s[j]) {
				n = n*16 + printHexValue(s[j])
				j++
			}
			// `\x` with no digit after it is a NUL, measured.
			b.WriteByte(byte(n))
			i = j - 1
		case 'u', 'U':
			width := 4
			if c == 'U' {
				width = 8
			}
			n, j := 0, i+1
			for j < len(s) && j <= i+width && isPrintHexDigit(s[j]) {
				n = n*16 + printHexValue(s[j])
				j++
			}
			b.WriteRune(rune(n))
			i = j - 1
		case 'M', 'C':
			if !how.printEscapes {
				writeUnknownEscape(&b, c, how)
				break
			}
			// `\M-x` sets the high bit and `\C-x` takes the control
			// character; the dash is optional, so `\MY` is `\M-Y`.
			j := i + 1
			if j < len(s) && s[j] == '-' {
				j++
			}
			if j >= len(s) {
				b.WriteByte('\\')
				b.WriteByte(c)
				break
			}
			base, width := metaControlTarget(s[j:], how, c == 'M')
			if c == 'M' {
				b.WriteByte(base | 0x80)
			} else {
				b.WriteByte(controlByte(base))
			}
			i = j + width - 1
		default:
			writeUnknownEscape(&b, c, how)
		}
	}
	return b.String(), false
}

// writeUnknownEscape is what becomes of a backslash this reading cannot use:
// the letter alone where the `\M-x` family is read, and both characters where
// it is not. Measured on `X\qY`, which is `XqY` under `${(g:e:)v}` and `X\qY`
// under the other three readings — the same split `print` and this shell's
// `echo` have, which is why the two travel with the family rather than being
// a fifth switch.
func writeUnknownEscape(b *strings.Builder, c byte, how escapeReading) {
	if !how.printEscapes {
		b.WriteByte('\\')
	}
	b.WriteByte(c)
}

// writeOctal reads up to three octal digits from at and writes the byte they
// come to, returning the index of the last one consumed. A value above 255 is
// truncated to a byte, measured: `\400` under `o` is a NUL and `\777` is 0xff.
func writeOctal(b *strings.Builder, s string, at int) int {
	n, j := 0, at
	for j < len(s) && j <= at+2 && s[j] >= '0' && s[j] <= '7' {
		n = n*8 + int(s[j]-'0')
		j++
	}
	b.WriteByte(byte(n))
	return j - 1
}

// metaControlTarget reads the byte `\M-` or `\C-` applies to, which may itself
// be one of the two — `\M-\C-a` is 0x81, the pair applied in turn — and
// reports how much of the text it took.
//
// A `^X` is a target too, but only where the reading has the caret and only
// behind `\M-`: measured under `${(g:ec:)v}`, `X\M-^AY` is 0x81 while
// `X\C-^AY` is 0x1e followed by an `A`, so `\C-` takes the `^` itself as its
// character. Reading the caret on both sides would answer the second row 0x01
// and look right on the first.
func metaControlTarget(s string, how escapeReading, meta bool) (byte, int) {
	if len(s) >= 2 && s[0] == '\\' && (s[1] == 'M' || s[1] == 'C') {
		j := 2
		if j < len(s) && s[j] == '-' {
			j++
		}
		if j < len(s) {
			base, width := metaControlTarget(s[j:], how, s[1] == 'M')
			if s[1] == 'M' {
				return base | 0x80, j + width
			}
			return controlByte(base), j + width
		}
	}
	if meta && how.caret && len(s) >= 2 && s[0] == '^' {
		return controlByte(s[1]), 2
	}
	return s[0], 1
}

// controlByte is `\C-x`: `?` is delete and everything else keeps its low five
// bits, so `\C-@` is NUL and `\C-a` and `\C-A` are both 1.
func controlByte(c byte) byte {
	if c == '?' {
		return 0x7f
	}
	return c & 0x1f
}

func isPrintHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func printHexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}
