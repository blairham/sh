// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `-O shopt_option`, which is this shell's alone and which this binary
// advertised for a long time without having — #3264.
//
// The usage block printed under a refused option is bash's own line, `-ilrsD
// or -c command or -O shopt_option (invocation only)`, and the letter was not
// read: it fell through as a `set` letter, by which time the option's *name*
// had already been taken as the script operand. `bash -O checkhash -c 'shopt
// checkhash'` exited **127** with `checkhash: No such file or directory`
// about a word that was never a path.
//
// Measured against bash 5.3.20 (Homebrew) on 2026-09-16, each probe with
// standard input on /dev/null. The whole panel is on
// Semantics.ShellOptionInvocationLetter: bash 5.3, bash as `sh` and bash
// 3.2.57 all answer `checkhash on` at status 0, and zsh, ksh93u+, dash and
// BusyBox ash have no such letter.
//
// This is the end-to-end row. driver/shelloptionletter_test.go asks whether
// the front end reads the letter at all, with a namespace of its own; here
// the letter, the `shopt` table and the diagnostics are one binary, which is
// the only place the two can be wrong together.
//
// The ordering this file used to hold out is closed (#3284). Every shell in
// the panel judges its option words before it looks at the operand behind
// them, and this front end resolved the source first — so `bash -O nosuchopt
// /nope/x.sh` answered `/nope/x.sh: No such file or directory` at 127 where
// bash names the option at 2. The operand is resolved lazily now: a script
// that will not open is carried on the source and raised once the options
// have been applied. Two rows below moved with it and say so.

// invalidOptionZ is what this shell says about a letter nobody has, which is
// bash's own usage block byte-for-byte — measured on 5.3.20, where the only
// difference is the name the shell was invoked under. It is here because one
// row below is about *which* of two refusals is reported, so the other
// refusal has to be written out in full.
var invalidOptionZ = "bash: -Z: invalid option\n" +
	"Usage:\tbash [GNU long option] [option] ...\n" +
	"\tbash [GNU long option] [option] script-file ...\n" +
	"GNU long options:\n" +
	"\t--debug\n\t--debugger\n\t--dump-po-strings\n\t--dump-strings\n" +
	"\t--help\n\t--init-file\n\t--login\n\t--noediting\n\t--noprofile\n" +
	"\t--norc\n\t--posix\n\t--pretty-print\n\t--rcfile\n\t--restricted\n" +
	"\t--verbose\n\t--version\n" +
	"Shell options:\n" +
	"\t-ilrsD or -c command or -O shopt_option\t\t(invocation only)\n" +
	"\t-abefhkmnptuvxBCEHPT or -o option\n"

// captureArgs runs a whole argument vector through this binary's own shell,
// with the given text on standard input — empty for the rows that name a `-c`
// string, and a script for the rows where the letter is the last word there
// is.
func captureArgs(t *testing.T, stdin string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdin = strings.NewReader(stdin)
	sh.Stdout = &o
	sh.Stderr = &e
	code = driver.MainArgs(sh, append([]string{"bash"}, argv...))
	return o.String(), e.String(), code
}

func TestTheShoptOptionLetterIsReadAtInvocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		out  string
		errs string
		code int
	}{
		// The issue's two rows.
		{
			name: "the minus turns one on",
			argv: []string{"-O", "checkhash", "-c", "shopt checkhash"},
			out:  "checkhash           \ton\n",
		},
		{
			// Status 1 because `shopt name` reports whether the name is on,
			// which is the builtin's answer and not the letter's: measured,
			// `bash +O checkhash -c 'shopt checkhash'` is `off` at 1.
			name: "the plus turns one off",
			argv: []string{"+O", "checkhash", "-c", "shopt checkhash"},
			out:  "checkhash           \toff\n", code: 1,
		},
		// An option that is on with nothing said, so the plus is the half
		// that moves and a shell ignoring the sign would look right on the
		// two rows above.
		{
			name: "one that starts on",
			argv: []string{"+O", "sourcepath", "-c", "shopt sourcepath"},
			out:  "sourcepath          \toff\n", code: 1,
		},
		{
			name: "and back on again",
			argv: []string{"+O", "sourcepath", "-O", "sourcepath", "-c", "shopt sourcepath"},
			out:  "sourcepath          \ton\n",
		},
		// The word is taken wherever the letter sits in a bundle, and the
		// rest of the bundle is still option letters.
		{
			name: "in a bundle, letter first",
			argv: []string{"-Ou", "checkhash", "-c", "shopt checkhash; echo $-"},
			out:  "checkhash           \ton\nhuBc\n",
		},
		{
			name: "in a bundle, letter last",
			argv: []string{"-uO", "checkhash", "-c", "shopt checkhash; echo $-"},
			out:  "checkhash           \ton\nhuBc\n",
		},
		// A name this shell does not have. Measured: status 2, the sentence
		// names the option and **not** the builtin — there was no builtin —
		// and the command string never runs.
		{
			name: "a name nobody has",
			argv: []string{"-O", "nosuchopt_zz", "-c", "echo RAN"},
			errs: "bash: line 0: nosuchopt_zz: invalid shell option name\n", code: 2,
		},
		{
			// The empty word is a name, and a bad one — not "no word at all".
			name: "the empty name",
			argv: []string{"-O", "", "-c", "echo RAN"},
			errs: "bash: line 0: : invalid shell option name\n", code: 2,
		},
		{
			// Whatever the word looks like, including a word that is itself
			// an option: `bash -O -c 'echo hi'` takes `-c` as the name.
			// Measured on bash 5.3.20, which refuses it at status 2 — and
			// this shell now says the same sentence at the same status.
			// Until #3284 it reached a refusal by a longer road, answering
			// `echo RAN: No such file or directory` at 127 about a word
			// that was never a path, because the operand was opened before
			// any option was applied.
			name: "the word may itself look like an option",
			argv: []string{"-O", "-c", "echo RAN"},
			errs: "bash: line 0: -c: invalid shell option name\n", code: 2,
		},
		{
			// A letter written *ahead* of this one in the same word is
			// judged first, which is what keeps the bundle a left-to-right
			// sequence rather than two passes. Measured: `bash -ZO
			// nosuchopt_zz -c cmd` names `-Z` and not the option name.
			//
			// And in the other order too, since #3284: `bash -O
			// nosuchopt_zz -Z -c cmd` names `-Z` as well. bash judges every
			// `set` letter and every `set -o` name before it looks at a
			// `shopt` name, whichever order the words were written in, and
			// this front end applies the second namespace last for that
			// reason — see driver.Shell.applyOptions for the six rows it was
			// measured from. The `set -o` names are **not** reordered, which
			// was measured in the same run.
			name: "a letter ahead of it in the same word is judged first",
			argv: []string{"-ZO", "nosuchopt_zz", "-c", "echo RAN"},
			errs: invalidOptionZ, code: 2,
		},
		{
			// And a letter written *behind* it, which is the row that pins
			// the deferral: the shopt name is applied after every letter
			// and every `set -o` name however the words were ordered.
			name: "and a letter behind it is judged first too",
			argv: []string{"-O", "nosuchopt_zz", "-Z", "-c", "echo RAN"},
			errs: invalidOptionZ, code: 2,
		},
		{
			// The same the other way about: a `set -o` name this shell does
			// not have beats a shopt name it does not have, in either
			// order. Measured on bash 5.3.20, both are `nosuchname:
			// invalid option name`.
			name: "and so is a set -o name behind it",
			argv: []string{"-O", "nosuchopt_zz", "-o", "nosuchname", "-c", "echo RAN"},
			errs: "bash: line 0: bash: nosuchname: invalid option name\n", code: 2,
		},
		{
			// And a welded name is not a name: the next word is. Measured on
			// bash 5.3.20, `bash -Ocheckhash -c cmd` is `-c: invalid shell
			// option name` at 2 — the letter took the next word and read
			// `checkhash` as more letters, the `c` among them meaning the
			// operand is a command string rather than a file. Same answer
			// here, and the held-out ordering above never arises because
			// nothing is opened.
			name: "nothing attaches to the letter",
			argv: []string{"-Ocheckhash", "-c", "echo RAN"},
			errs: "bash: line 0: -c: invalid shell option name\n", code: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := captureArgs(t, "", tc.argv...)
			if code != tc.code {
				t.Fatalf("status %d, want %d — out %q, stderr %q", code, tc.code, out, errs)
			}
			if out != tc.out {
				t.Errorf("stdout %q, want %q", out, tc.out)
			}
			if errs != tc.errs {
				t.Errorf("stderr %q, want %q", errs, tc.errs)
			}
		})
	}
}

// TestTheShoptLetterWithNoNameListsTheTable is the other half of the letter,
// and it is not a refusal: measured on bash 5.3.20, `bash -O` with nothing
// after it writes the same listing `shopt` writes, `bash +O` writes the same
// one `shopt -p` writes — byte-for-byte, both compared by digest — and the
// shell then runs whatever it was given at status 0.
func TestTheShoptLetterWithNoNameListsTheTable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		letter  string
		builtin string
	}{
		{"the minus is the plain listing", "-O", "shopt"},
		{"the plus is the re-inputtable one", "+O", "shopt -p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The letter is the last word there is, so it has none to take
			// and the program arrives on standard input instead.
			out, errs, code := captureArgs(t, "echo RAN\n", tc.letter)
			if code != 0 {
				t.Fatalf("status %d — out %q, stderr %q", code, out, errs)
			}
			want, werrs, wcode := captureArgs(t, "", "-c", tc.builtin)
			if wcode != 0 {
				t.Fatalf("the builtin itself: status %d, stderr %q", wcode, werrs)
			}
			if out != want+"RAN\n" {
				t.Errorf("stdout %q, want the `%s` listing and then RAN, which is %q",
					out, tc.builtin, want+"RAN\n")
			}
			if !strings.Contains(want, "checkhash") {
				t.Fatalf("the listing being compared against names no options: %q", want)
			}
		})
	}
}
