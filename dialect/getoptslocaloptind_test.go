// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import "testing"

// What a function call does to the half of the `getopts` position that is not
// a parameter, across every preset a shipped binary runs under (#3293).
//
// The position has two halves: OPTIND, which counts words, and how far into a
// clustered word the letters have been read. Two answers govern the second
// half and they compose — Semantics.GetoptsFunctionPosition, which says what a
// *call* does to it, and Semantics.GetoptsLocalOptindRestoresTheCursor, which
// says what a `local OPTIND` *declaration* does to it — and the whole of this
// bug was that the first ran last and wrote over the second. dash and BusyBox
// ash answer the two differently from each other, so they are the only columns
// where the composition is visible at all, and the answer that was never
// measured was the one that won.
//
// Measured 2026-09-16 from a script file, one file per shape run in its own
// shell, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh` with stdin from
// /dev/null in a fresh directory. BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `--init`; `/bin/bash` is 3.2.57 and the
// panel bash is Homebrew's 5.3.20. ksh93u+ 2012-08-01 has no `local`, so it
// reports `local: not found` and runs the rest — which is an answer, and the
// same one bash gives by the other route.
//
//	shape                bash 5.3   bash 3.2   zsh 5.9.2   ksh93     dash / BusyBox ash
//	plain call           b ind=2    b ind=2    b ind=1     b ind=2   b ind=2
//	callee scans its own ? ind=2    ? ind=2    b ind=1     ? ind=2   b ind=2
//	local OPTIND         b ind=1    a ind=1    b ind=1     b ind=1   ? ind=2
//	local OPTIND, novalue b ind=2   a ind=1    b ind=1     b ind=2   ? ind=2
//	`-ab -c`, local      b then c   a then b   b then c    b then c  c then ?
//	`-a -b`, local       b ind=3    b ind=3    b ind=2     b ind=3   b ind=3
//
// **The `-ab -c` shape is the discriminator** and is why it is here. Every
// other shape has only one word to scan, so "the place inside the word was
// dropped" and "the scan ran off the end" print the same thing: `?` at status
// 1. With a second word behind it the two part company — dash resumes at the
// word OPTIND *names* and reads `c`, where a scan that had simply ended would
// print `?` twice. Ours read `a` again and went backwards, which is the shape
// that never finishes (#2226).
//
// bash 3.2 is a third answer and not either side of this one: it does not hand
// the place back and has no cursor of its own either, so it re-reads the same
// letter where dash moves on to the next word. It is not a preset here.
//
// The first two rows are the controls, and they are what say this is about the
// declaration rather than about calls: with nothing declared, dash and BusyBox
// ash hand the caller its place back — including across a callee that ran a
// whole scan of its own — and a fix that simply stopped restoring would pass
// every row below and fail these two.
func TestWhatAFunctionCallDoesToTheGetoptsCursor(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			// The control. No declaration anywhere, so the caller's place
			// inside `-ab` survives the call in every column.
			name: "a plain call hands the place back",
			src: "f() { :; }\n" +
				"set -- -ab\n" +
				"getopts ab o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"getopts ab o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\n2=[b] st=0 ind=2\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n",
				"ksh":   "1=[a] ind=1\n2=[b] st=0 ind=2\n",
				"dash":  "1=[a] ind=2\n2=[b] st=0 ind=2\n",
				"ash":   "1=[a] ind=2\n2=[b] st=0 ind=2\n",
				"posix": "1=[a] ind=1\n2=[b] st=2 ind=2\n",
			},
		},
		{
			// The second control, and the sharper one: the callee runs a
			// whole scan of its own words and the caller still comes back to
			// the middle of `-ab` in the two columns that give a call its own
			// cursor. bash and ksh93 share the cursor, so the callee's scan
			// spends it and the caller finds the options gone.
			name: "a callee that scans its own words hands it back too",
			src: "f() { while getopts cd x \"$@\"; do printf '%s ' \"$x\"; done; printf 'cend=%s\\n' \"$OPTIND\"; }\n" +
				"set -- -ab\n" +
				"getopts ab o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f -cd\n" +
				"getopts ab o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\nd cend=2\n2=[?] st=1 ind=2\n",
				"zsh":   "1=[a] ind=1\nc d cend=2\n2=[b] st=0 ind=1\n",
				"ksh":   "1=[a] ind=1\nd cend=2\n2=[?] st=1 ind=2\n",
				"dash":  "1=[a] ind=2\nc d cend=2\n2=[b] st=0 ind=2\n",
				"ash":   "1=[a] ind=2\nc d cend=2\n2=[b] st=0 ind=2\n",
				"posix": "1=[a] ind=1\ncend=2\n2=[?] st=2 ind=2\n",
			},
		},
		{
			// The shape the issue was filed from, with a word of three
			// letters so that the number and the place are further apart.
			name: "a local OPTIND takes the place inside the word away",
			src: "f() { local OPTIND=1; :; }\n" +
				"set -- -abc\n" +
				"getopts abc o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"getopts abc o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\n2=[b] st=0 ind=1\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n",
				"ksh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n",
				"dash":  "1=[a] ind=2\n2=[?] st=1 ind=2\n",
				"ash":   "1=[a] ind=2\n2=[?] st=1 ind=2\n",
				"posix": "1=[a] ind=1\n2=[a] st=2 ind=1\n",
			},
		},
		{
			// The declaration and not its value: with no `=1` behind it the
			// answer is the same, which is what says the number written is
			// beside the point and the scope is the whole of it.
			name: "a local OPTIND with no value takes it away just the same",
			src: "f() { local OPTIND; :; }\n" +
				"set -- -ab\n" +
				"getopts ab o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"getopts ab o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\n2=[b] st=0 ind=2\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n",
				"ksh":   "1=[a] ind=1\n2=[b] st=0 ind=2\n",
				"dash":  "1=[a] ind=2\n2=[?] st=1 ind=2\n",
				"ash":   "1=[a] ind=2\n2=[?] st=1 ind=2\n",
				"posix": "1=[a] ind=1\n2=[a] st=2 ind=1\n",
			},
		},
		{
			// The discriminator. A second word behind the clustered one, so
			// that "the place inside the word was dropped" and "the options
			// ran out" print different things: dash and BusyBox ash resume at
			// the word OPTIND names and read `c`, then run out.
			name: "the scan resumes at the word OPTIND names",
			src: "f() { local OPTIND=1; :; }\n" +
				"set -- -ab -c\n" +
				"getopts abc o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"getopts abc o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n" +
				"getopts abc o; printf '3=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\n2=[b] st=0 ind=2\n3=[c] st=0 ind=3\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n3=[c] st=0 ind=2\n",
				"ksh":   "1=[a] ind=1\n2=[b] st=0 ind=2\n3=[c] st=0 ind=3\n",
				"dash":  "1=[a] ind=2\n2=[c] st=0 ind=3\n3=[?] st=1 ind=3\n",
				"ash":   "1=[a] ind=2\n2=[c] st=0 ind=3\n3=[?] st=1 ind=3\n",
				"posix": "1=[a] ind=1\n2=[a] st=2 ind=1\n3=[b] st=2 ind=2\n",
			},
		},
		{
			// The third control: written as separate words there is no place
			// inside one to lose, so the declaration costs nothing anywhere
			// and every column reads the next option. A fix that dropped the
			// cursor unconditionally rather than only its intra-word half
			// would still pass this, which is what the row above is for.
			name: "separate words leave nothing to take away",
			src: "f() { local OPTIND=1; :; }\n" +
				"set -- -a -b\n" +
				"getopts ab o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"getopts ab o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=2\n2=[b] st=0 ind=3\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=2\n",
				"ksh":   "1=[a] ind=2\n2=[b] st=0 ind=3\n",
				"dash":  "1=[a] ind=2\n2=[b] st=0 ind=3\n",
				"ash":   "1=[a] ind=2\n2=[b] st=0 ind=3\n",
				"posix": "1=[a] ind=2\n2=[b] st=2 ind=3\n",
			},
		},
		{
			// The override a call leaves behind is not the last word: an
			// assignment the *script* makes outranks it, because writing
			// OPTIND is how a scan is restarted. Without this row a cursor
			// saved across a call would go on being read over the top of the
			// restart, and the caller would resume at the word *after* the
			// one it asked for — which is a scan that skips an option rather
			// than one that repeats it, and is the quieter of the two
			// failures.
			name: "an assignment after the call outranks what the call left",
			src: "f() { :; }\n" +
				"set -- -ab -c\n" +
				"getopts abc o; printf '1=[%s] ind=%s\\n' \"$o\" \"$OPTIND\"\n" +
				"f\n" +
				"OPTIND=1\n" +
				"getopts abc o; printf '2=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n" +
				"getopts abc o; printf '3=[%s] st=%s ind=%s\\n' \"$o\" \"$?\" \"$OPTIND\"\n",
			want: map[string]string{
				"bash":  "1=[a] ind=1\n2=[a] st=0 ind=1\n3=[b] st=0 ind=2\n",
				"zsh":   "1=[a] ind=1\n2=[b] st=0 ind=1\n3=[c] st=0 ind=2\n",
				"ksh":   "1=[a] ind=1\n2=[a] st=0 ind=1\n3=[b] st=0 ind=2\n",
				"dash":  "1=[a] ind=2\n2=[a] st=0 ind=2\n3=[b] st=0 ind=2\n",
				"ash":   "1=[a] ind=2\n2=[a] st=0 ind=2\n3=[b] st=0 ind=2\n",
				"posix": "1=[a] ind=1\n2=[b] st=2 ind=2\n3=[c] st=2 ind=3\n",
			},
		},
		{
			// The regression this must not buy: a helper that parses its own
			// options, called twice, reads them both times in the three
			// columns whose calls have a cursor of their own (#2944). It is
			// the reason those calls are handed a fresh cursor at all, and a
			// fix that stopped handing the caller's back by simply not
			// restoring would take this with it.
			name: "a helper called twice reads its options both times",
			src: "f() { while getopts ab o \"$@\"; do printf '%s ' \"$o\"; done; printf 'end=%s\\n' \"$OPTIND\"; }\n" +
				"OPTIND=1; f -a -b; f -a -b\n",
			want: map[string]string{
				"bash":  "a b end=3\nend=3\n",
				"zsh":   "a b end=3\na b end=3\n",
				"ksh":   "a b end=3\nend=3\n",
				"dash":  "a b end=3\na b end=3\n",
				"ash":   "a b end=3\na b end=3\n",
				"posix": "end=2\nend=3\n",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for preset, want := range c.want {
				t.Run(preset, func(t *testing.T) {
					out, _, _ := splitRun(t, presets[preset], c.src)
					if out != want {
						t.Errorf("wrote %q, want %q", out, want)
					}
				})
			}
		})
	}
}

// The POSIX preset's rows above carry statuses of 2 and letters a dialect
// would not print, and both come from *other* getopts axes it leaves
// unanswered — OPTARG's clearing, and a declaration without a value. They are
// recorded as measured rather than trimmed, because the preset is the one
// column whose GetoptsFunctionPosition is shared with the caller *and* whose
// GetoptsLocalOptindRestoresTheCursor is no: it is the row that says a shared
// cursor never takes the call's own restore path, and the letters it prints
// are what would move if it did. See presets in readname_test.go for why the
// substrate's own vector is graded beside the five dialects rather than left
// to inherit in silence.
