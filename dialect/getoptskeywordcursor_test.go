// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import "testing"

// What a `function`-word call does to the `getopts` cursor, beside what the
// same body written `name() { … }` does (#3321).
//
// The shell that keys its scoping on the definition form hands a keyword-form
// call a fresh cursor — OPTIND at 1 and the place inside a word at its start —
// and puts the caller's back at the return, while a POSIX-form call shares the
// shell's. zsh scopes both forms and bash neither, so the **pairing** is the
// claim: a row with only the keyword form in it passes under zsh's answer too.
//
// Measured 2026-09-16 from a script file per shape, `env -i PATH=/usr/bin:/bin
// LC_ALL=C <shell> case.sh` with stdin from /dev/null in a fresh directory:
// AT&T ksh93u+ 2012-08-01, Homebrew bash 5.3.20 and zsh 5.9.2. dash has no
// `function` word and refuses every keyword row at parse time, so it and
// BusyBox ash are not columns here.
func TestAKeywordFunctionCallHasAGetoptsCursorOfItsOwn(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			// A body that reads the cursor and writes it: a probe that only
			// writes cannot tell a fresh cursor from a shared one whose write
			// was undone.
			name: "the keyword form is handed a fresh cursor and gives the caller's back",
			src: "set -- -a -b x\n" +
				"getopts ab o\n" +
				"function k { echo \"body=$OPTIND\"; OPTIND=9; }\n" +
				"k\n" +
				"echo \"after=$OPTIND\"\n",
			want: map[string]string{
				"bash": "body=2\nafter=9\n",
				"zsh":  "body=1\nafter=1\n",
				"ksh":  "body=1\nafter=2\n",
			},
		},
		{
			// The same body written the other way, which is the pairing the answer is.
			name: "the POSIX form shares the caller's",
			src: "set -- -a -b x\n" +
				"getopts ab o\n" +
				"k() { echo \"body=$OPTIND\"; OPTIND=9; }\n" +
				"k\n" +
				"echo \"after=$OPTIND\"\n",
			want: map[string]string{
				"bash": "body=2\nafter=9\n",
				"zsh":  "body=1\nafter=1\n",
				"ksh":  "body=2\nafter=9\n",
			},
		},
		{
			// The intra-word half, with the caller part-way through `-ab`.
			name: "a keyword call starts a clustered word over and hands the caller its letter back",
			src: "set -- -ab -c\n" +
				"getopts abc o; echo \"caller1=$o OPTIND=$OPTIND\"\n" +
				"function k { getopts abc p \"$@\"; echo \"body=$p OPTIND=$OPTIND\"; getopts abc p \"$@\"; echo \"body2=$p OPTIND=$OPTIND\"; }\n" +
				"k \"$@\"\n" +
				"getopts abc o; echo \"caller2=$o OPTIND=$OPTIND\"\n",
			want: map[string]string{
				"bash": "caller1=a OPTIND=1\nbody=b OPTIND=2\nbody2=c OPTIND=3\ncaller2=? OPTIND=3\n",
				"zsh":  "caller1=a OPTIND=1\nbody=a OPTIND=1\nbody2=b OPTIND=1\ncaller2=b OPTIND=1\n",
				"ksh":  "caller1=a OPTIND=1\nbody=a OPTIND=1\nbody2=b OPTIND=2\ncaller2=b OPTIND=2\n",
			},
		},
		{
			// And the same half beside it in the other form.
			name: "a POSIX call carries the caller's place inside the word in",
			src: "set -- -ab -c\n" +
				"getopts abc o; echo \"caller1=$o OPTIND=$OPTIND\"\n" +
				"k() { getopts abc p \"$@\"; echo \"body=$p OPTIND=$OPTIND\"; getopts abc p \"$@\"; echo \"body2=$p OPTIND=$OPTIND\"; }\n" +
				"k \"$@\"\n" +
				"getopts abc o; echo \"caller2=$o OPTIND=$OPTIND\"\n",
			want: map[string]string{
				"bash": "caller1=a OPTIND=1\nbody=b OPTIND=2\nbody2=c OPTIND=3\ncaller2=? OPTIND=3\n",
				"zsh":  "caller1=a OPTIND=1\nbody=a OPTIND=1\nbody2=b OPTIND=1\ncaller2=b OPTIND=1\n",
				"ksh":  "caller1=a OPTIND=1\nbody=b OPTIND=2\nbody2=c OPTIND=3\ncaller2=? OPTIND=3\n",
			},
		},
		{
			// A POSIX-form call inside a keyword one moves the keyword call's
			// cursor, and the keyword call's return puts the caller's back
			// over it.
			name: "the boundary is the word, not the call",
			src: "set -- -a -b; getopts ab o\n" +
				"k2() { echo \"k2=$OPTIND\"; OPTIND=7; }\n" +
				"function k { OPTIND=4; k2; echo \"k=$OPTIND\"; }\n" +
				"k; echo \"after=$OPTIND\"\n",
			want: map[string]string{
				"bash": "k2=4\nk=7\nafter=7\n",
				"zsh":  "k2=1\nk=4\nafter=1\n",
				"ksh":  "k2=4\nk=7\nafter=2\n",
			},
		},
		{
			// The script the answer is worth fixing for.
			name: "a keyword helper called twice reads its options both times",
			src: "set -- -x; getopts x o\n" +
				"function t { while getopts ab o \"$@\"; do printf '%s ' $o; done; echo \"end=$OPTIND\"; }\n" +
				"t -a -b; t -a -b; echo \"after=$OPTIND\"\n",
			want: map[string]string{
				"bash": "b end=3\nend=3\nafter=3\n",
				"zsh":  "a b end=3\na b end=3\nafter=1\n",
				"ksh":  "a b end=3\na b end=3\nafter=2\n",
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
