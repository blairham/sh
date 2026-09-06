// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"io"
	"sort"
	"strconv"
	"strings"

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
//     number nothing writable is open at is `bad file number: 9`; both 1.
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
//   - `--` and a lone `-` both end the options.
//
// An unknown letter is `bad option: -q` at 1, with nothing printed.
//
// Two edges are recorded rather than claimed. `print -u0` is `bad mode on fd
// 3` in zsh — a number that names neither the descriptor asked about nor
// anything else in the command — and is `bad file number: 0` here, because
// repeating a wording that is wrong about its own subject would be the worse
// of the two. And `print -m '['` is `bad pattern: [` at 1 there; the core's
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
	_, _ = io.WriteString(out, text)
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
		if !strings.HasPrefix(word, "-") || word == "-" {
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

// expandPrintEscapes is the measured escape set, applied to one operand. The
// second result reports a `\c`, which ends the whole command's output where it
// stands.
func expandPrintEscapes(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
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
			return b.String(), true
		case 'e', 'E':
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
			n, j := 0, i
			for j < len(s) && j <= i+2 && s[j] >= '0' && s[j] <= '7' {
				n = n*8 + int(s[j]-'0')
				j++
			}
			b.WriteByte(byte(n))
			i = j - 1
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
			base, width := metaControlTarget(s[j:])
			if c == 'M' {
				b.WriteByte(base | 0x80)
			} else {
				b.WriteByte(controlByte(base))
			}
			i = j + width - 1
		default:
			// An escape this shell does not know loses its backslash, which
			// is where `print` and this shell's `echo` part company.
			b.WriteByte(c)
		}
	}
	return b.String(), false
}

// metaControlTarget reads the byte `\M-` or `\C-` applies to, which may itself
// be one of the two — `\M-\C-a` is 0x81, the pair applied in turn — and
// reports how much of the text it took.
func metaControlTarget(s string) (byte, int) {
	if len(s) >= 2 && s[0] == '\\' && (s[1] == 'M' || s[1] == 'C') {
		j := 2
		if j < len(s) && s[j] == '-' {
			j++
		}
		if j < len(s) {
			base, width := metaControlTarget(s[j:])
			if s[1] == 'M' {
				return base | 0x80, j + width
			}
			return controlByte(base), j + width
		}
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
