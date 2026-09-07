// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `print` is ksh93's `echo`, and the spelling real ksh scripts write. It is a
// printf wrapper at heart: operands joined with single spaces, a newline
// after, and echo's escapes expanded on the way — which is exactly what the
// options steer.
//
// Measured, option by option:
//
//   - default: `\t`, `\n`, `\a`, `\b`, `\f`, `\v`, `\r`, `\E` to the escape
//     character, `\0` with up to three octal digits, `\\` — and `\x41` left
//     alone, which is the same set this dialect's echo expands. `\c` stops
//     the whole command's output, the newline included.
//   - `-r` prints the operands raw and `-e` puts the expansion back; in one
//     bundle the later letter wins, which is what reading them in order does.
//   - `-n` withholds the newline.
//   - `-u fd` aims the output at a descriptor, the number attached or the
//     next word; a number nothing writable is open at — or a word that is no
//     number at all — is `bad file unit number [Bad file descriptor]`, 1.
//   - `--` ends the options, and so does a lone `-`: `print - -n` prints the
//     word. Neither of them does after `-R`, where both print as words.
//   - `-R` is `-r` plus the end of this shell's option parsing: the rest of
//     its own bundle goes unread, and of the words after it only a bare `-n`
//     is still an option — `print -R -e a` writes `-e a`, where zsh's `-R`
//     would read the `-e` and put escape expansion back.
//   - `-s` sends the operands to the history file. Non-interactive ksh93
//     answers 0 and shows nothing, and with no history here that whole
//     behavior is the reachable one: the operands are consumed, nothing is
//     written, 0.
//   - `-p` writes to the coprocess, and this grammar has no `|&` to start
//     one: the only reachable answer is the measured refusal, `no query
//     process [Bad file descriptor]` and 1 — read's letter says the same.
//   - `-f format` hands everything to printf, the format reused over the
//     operands and no newline added.
//   - `-v` and `-C` are ksh93's value-quoting forms and are not implemented;
//     the letters are refused the way the substrate refuses an option a
//     dialect has and this shell does not. docs/spec/semantics.md records it.
//
// An unknown letter is `unknown option` with the usage line after it, 2.
const printUsage = "Usage: print [-enprsvC] [-f format] [-u fd] [string ...]"

// registerPrint installs the builtin.
func registerPrint(r *interp.Runner) {
	r.Register("print", printBuiltin)
}

// printOptions is what the option words asked for.
type printOptions struct {
	raw     bool
	newline bool
	history bool
	format  string
	fd      int
}

func printBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts := printOptions{newline: true, fd: 1}
	rest, code := readPrintOptions(r, args, &opts)
	if code >= 0 {
		return code
	}
	if opts.history {
		// The operands went to a history this shell does not keep.
		return 0
	}
	if opts.format != "" {
		return printFormatted(r, ctx, opts, rest)
	}
	out, ok := r.WriterForFd(opts.fd)
	if !ok {
		r.Diagnosef("print: bad file unit number [Bad file descriptor]\n")
		return 1
	}
	text := strings.Join(rest, " ")
	if !opts.raw {
		expanded, stopped := expandPrintEscapes(text)
		if stopped {
			_, _ = io.WriteString(out, expanded)
			return 0
		}
		text = expanded
	}
	if opts.newline {
		text += "\n"
	}
	_, _ = io.WriteString(out, text)
	return 0
}

// readPrintOptions reads the leading option words. A code of -1 means run;
// anything else is the answer, already reported.
func readPrintOptions(r *interp.Runner, args []string, opts *printOptions) (rest []string, code int) {
	rest = args
	echoMode := false
	for !echoMode && len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" && rest[0] != "--" {
		word := rest[0][1:]
		rest = rest[1:]
		for i := 0; i < len(word); i++ {
			switch word[i] {
			case 'e':
				opts.raw = false
			case 'r':
				opts.raw = true
			case 'R':
				// `-R` is raw *and* the end of this shell's own option
				// parsing: the rest of the bundle is not read at all —
				// `print -Rf %s a` prints `%s a` — and of the words after
				// it only a bare `-n` is an option. Measured; the letter is
				// not zsh's `-R`, which keeps reading `-e` and `-n` from
				// every later word and ends on a lone `-`.
				opts.raw, echoMode = true, true
				// Of the rest of this bundle only `n` still counts:
				// `print -Rn a` withholds the newline and `print -Rf %s a`
				// writes `%s a` rather than reading a format.
				if strings.ContainsRune(word[i+1:], 'n') {
					opts.newline = false
				}
				i = len(word)
			case 'n':
				opts.newline = false
			case 's':
				opts.history = true
			case 'p':
				// The coprocess `cmd |&` started. The wording when none is
				// running is the shell's own, and it names the descriptor
				// the letter would have written to.
				fd, running := r.CoprocWrite()
				if !running {
					r.Diagnosef("print: no query process [Bad file descriptor]\n")
					return nil, 1
				}
				opts.fd = fd
			case 'v', 'C':
				r.Diagnosef("print: -%c is not implemented yet\n", word[i])
				return nil, 2
			case 'f', 'u':
				arg := word[i+1:]
				if arg == "" {
					if len(rest) == 0 {
						r.Diagnosef("print: -%c: argument expected\n", word[i])
						_, _ = fmt.Fprintf(r.Err(), "%s\n", printUsage)
						return nil, 2
					}
					arg, rest = rest[0], rest[1:]
				}
				if word[i] == 'f' {
					opts.format = arg
				} else {
					fd, err := strconv.Atoi(arg)
					if err != nil || fd < 0 {
						r.Diagnosef("print: bad file unit number [Bad file descriptor]\n")
						return nil, 1
					}
					opts.fd = fd
				}
				i = len(word)
			default:
				r.Diagnosef("print: -%c: unknown option\n", word[i])
				_, _ = fmt.Fprintf(r.Err(), "%s\n", printUsage)
				return nil, 2
			}
		}
	}
	if echoMode {
		// Neither `-` nor `--` ends the options here — both print as words —
		// and one `-n` is read, so `print -R -n -n a` writes `-n a`.
		if len(rest) > 0 && rest[0] == "-n" {
			opts.newline = false
			rest = rest[1:]
		}
		return rest, -1
	}
	if len(rest) > 0 && (rest[0] == "-" || rest[0] == "--") {
		rest = rest[1:]
	}
	return rest, -1
}

// printFormatted is `-f`: printf with this command's operands. The core
// printf writes to the runner's standard output, so a descriptor `-u` moved
// off 1 would need a seam printf does not have — refused out loud rather than
// sent to the wrong stream.
func printFormatted(r *interp.Runner, ctx context.Context, opts printOptions, rest []string) int {
	if opts.fd != 1 {
		r.Diagnosef("print: -f with -u is not implemented yet\n")
		return 2
	}
	printf, ok := r.Builtin("printf")
	if !ok {
		return 1
	}
	return printf(r, ctx, append([]string{opts.format}, rest...))
}

// expandPrintEscapes is the measured escape set, applied to the joined
// operands. The second result reports a `\c`, which ends the output where it
// stands — the rest of the text and the newline both unwritten.
func expandPrintEscapes(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'c':
			return b.String(), true
		case 'E':
			b.WriteByte(27)
		case 'f':
			b.WriteByte(12)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte(11)
		case '\\':
			b.WriteByte('\\')
		case '0':
			// Up to three octal digits, `\0` alone being NUL.
			n, digits := 0, 0
			for digits < 3 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' {
				i++
				digits++
				n = n*8 + int(s[i]-'0')
			}
			b.WriteByte(byte(n))
		default:
			// Not an escape here — `\x41` stays as written.
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String(), false
}
