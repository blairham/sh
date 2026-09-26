// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// The line one column writes **inside** its sentence for a substitution
// refused in a here-document body.
//
// That column is the only one in the panel that carries a line in the
// wording as well as in the location — `s.sh: line 1: syntax error at line
// 1: `)' unexpected` — and the two numbers are the same number. For a
// here-document body this shell counted the second from the body, so the
// sentence disagreed with the prefix in front of it.
//
// Measured 2026-09-26 over a script file under `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C` with stdin from /dev/null, against `/bin/ksh`
// `Version AJM 93u+ 2012-08-01` (AT&T's own build, not ksh93u+m and not
// ksh2020, `not a Go executable` by `go version -m`), each program holding
// `$(echo hi; for)` as its here-document body:
//
//	command, and the line it is on     reference                    before
//	cat <<END        line 1            s.sh: line 1: … at line 1    at line 2
//	cat <<END        line 2            s.sh: line 2: … at line 2    at line 3
//	cat <<END        line 3            s.sh: line 3: … at line 3    at line 4
//	cat <<END | cat  line 1            s.sh: line 1: … at line 1    at line 2
//	{ cat; } <<END   line 1            s.sh: … at line 0            at line 2
//	{ cat; } <<END   line 2            s.sh: line 1: … at line 1    at line 3
//	{ cat; } <<END   line 3            s.sh: line 2: … at line 2    at line 4
//	: <<END          line 1            s.sh[1]: … at line 0         at line 2
//	: <<END          line 2            s.sh[2]: … at line 0         at line 3
//	: <<END          line 3            s.sh[3]: … at line 0         at line 4
//
// Two rules come off those ten rows and each has rows that fail without it:
//
//   - **The sentence names the line the message is located at.** The `cat`
//     rows move with the command, the group rows move with it one line
//     below — that construct's failed redirection is reported a line below
//     the redirect's there — and the group written on line 1 has no line in
//     its prefix at all and writes `line 0`.
//   - **And nought where the location is the *builtin's* bracket.** The `:`
//     rows are `s.sh[1]:`, `s.sh[2]:` and `s.sh[3]:` and all three say `at
//     line 0`, so the bracketed counter is not the one the sentence reads.
//
// The body's own line is what this shell wrote, and the last column above is
// what that came to: a number one below the body's first line in every row,
// which agrees with nothing.

// heredocBodyRefusalLine is that number, and whether this dialect writes one.
//
// Read only for a here-document body: the same refusal in an ordinary word
// already agrees with the reference, because there the body's line and the
// command's are the same line. The older substitution spelling is excluded
// for the reason it is excluded everywhere Runner.expansionBodyLine is read
// — its body is read with the script's line.
func (r *Runner) heredocBodyRefusalLine(span syntax.Span) (int, bool) {
	if span.Backquoted || r.expansionBodyLine == 0 || !r.inHeredocBody {
		return 0, false
	}
	if !r.diag().HeredocBodyRefusalNamesTheLineItIsLocatedAt {
		return 0, false
	}
	if r.builtinIsSpeaking() {
		// The bracketed location is a counter of its own, and the sentence
		// does not read it.
		return 0, true
	}
	return r.line, true
}
