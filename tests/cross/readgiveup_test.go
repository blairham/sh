// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"fmt"
	"strings"
	"testing"
)

// When a trapped signal gives up a `read`, in the three dialects that give one
// up at all (#7027). Every row is a script run from -c on dash 0.5.12, ksh93u+
// 2012-08-01 and BusyBox ash 1.37.0 in the pinned alpine image on 2026-10-29,
// and the wanted output is what that shell printed. See
// interp.Semantics.ReadAbandonedWhen.
//
// The streams are fifos the script makes. `exec 3<>p` opens one both ways;
// `exec 4<>p 3<p` hands the read a descriptor open for reading alone, with a
// writer held on 4 so the open does not wait.

// aLineStillArriving writes two 64 KiB blocks of `a` — the second cannot be
// written until the read has taken the first, so the read is under way — then
// sends INT with bytes still waiting, then writes `then`, pauses, and ends the
// line. The pause is a wait that begins after the signal.
func aLineStillArriving(open, then string) string {
	return `mkfifo p; c=a; i=0; while [ $i -lt 16 ]; do c=$c$c; i=$((i+1)); done; ` + open + `; ` +
		`trap "echo T" INT; trap "echo U" USR1; ` +
		`{ printf %s "$c"; printf %s "$c"; kill -INT $$; ` + then + ` echo; echo tail; } >p & ` +
		`read -r l <&3; s=$?; read -r m <&3; echo "st=$s len=${#l} next=${#m}"; wait`
}

// partOfALine has `ab` waiting before the read, sends INT 0.3 s in while the
// read waits for the rest, and writes `cd` and `end` half a second after that:
// a read given up has `ab`, and the next one `cd`; a read that goes on has
// `abcd`, and the next one `end`.
func partOfALine(open, first, read string) string {
	return `mkfifo p; ` + open + `; trap "echo T" INT; printf '` + first + `' >p; ` +
		`(sleep 0.3; kill -INT $$; sleep 0.5; printf 'cd\nend\n' >p) & ` +
		read + `; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`
}

const (
	bothWays    = `exec 3<>p`
	readingOnly = `exec 4<>p 3<p`
	moreBlocks  = `printf %s "$c"; printf %s "$c"; sleep 0.2;`
	thenUSR1    = `sleep 3; kill -USR1 $$; sleep 0.3;`
)

func TestATrappedSignalGivesUpAReadWhereTheShellsDo(t *testing.T) {
	t.Parallel()
	whole := "T\nst=0 len=262144 next=4\n"
	type row struct {
		name, dialect, src string
		// want is the output; given, where it is set instead, is the length
		// the two reads share, for a give-up whose point is a matter of
		// timing — the status is checked and the first read must have part.
		want  string
		given int
	}
	rows := []row{
		// A line still arriving. dash and ash finish it, on either kind of
		// descriptor: the signal lands with bytes there, and the wait the
		// pause makes began after it.
		{"arriving both ways", "dash", aLineStillArriving(bothWays, moreBlocks), whole, 0},
		{"arriving reading only", "dash", aLineStillArriving(readingOnly, moreBlocks), whole, 0},
		{"arriving both ways", "ash", aLineStillArriving(bothWays, moreBlocks), whole, 0},
		{"arriving reading only", "ash", aLineStillArriving(readingOnly, moreBlocks), whole, 0},
		// ksh93 finishes it only from a descriptor open for reading alone,
		// and gives it up partway from one open both ways — measured at
		// `st=258 len=131072 next=131072`, ksh93 having taken what was
		// written before the signal; this shell stops at the next byte.
		{"arriving reading only", "ksh", aLineStillArriving(readingOnly, moreBlocks), whole, 0},
		{"arriving both ways", "ksh", aLineStillArriving(bothWays, moreBlocks), "", 262144},
		// The same, with no more bytes after INT and a USR1 once the read is
		// waiting: the second signal lands in a wait and gives the read up in
		// dash, and ksh93 reading a line it has begun reads on. Three seconds
		// rather than the half second the real shells need, because a Linux
		// pipe holds 64 KiB and this shell under the race detector took 0.8 s
		// to drain it: a USR1 landing with bytes still there is, rightly, not
		// a give-up.
		{"second signal both ways", "dash", aLineStillArriving(bothWays, thenUSR1), "T\nU\nst=1 len=131072 next=0\n", 0},
		{
			"second signal reading only", "dash", aLineStillArriving(readingOnly, thenUSR1),
			"T\nU\nst=1 len=131072 next=0\n", 0,
		},
		{
			"second signal reading only", "ksh", aLineStillArriving(readingOnly, thenUSR1),
			"T\nU\nst=0 len=131072 next=4\n", 0,
		},
		{"second signal both ways", "ksh", aLineStillArriving(bothWays, thenUSR1), "", 131072},
		// Part of a line in, and a wait the signal lands in. dash gives it
		// up whatever the descriptor; ksh93 reads on only on its standard
		// input from a descriptor open for reading alone.
		{"part, reading only", "dash", partOfALine(readingOnly, "ab", `read -r l <&3`), "T\nst=1 l=[ab]\nnext [cd]\n", 0},
		{"part, both ways", "dash", partOfALine(bothWays, "ab", `read -r l <&3`), "T\nst=1 l=[ab]\nnext [cd]\n", 0},
		{"part, reading only", "ksh", partOfALine(readingOnly, "ab", `read -r l <&3`), "T\nst=0 l=[abcd]\nnext [end]\n", 0},
		{"part, both ways", "ksh", partOfALine(bothWays, "ab", `read -r l <&3`), "T\nst=258 l=[ab]\nnext [cd]\n", 0},
		{"part, -u", "ksh", partOfALine(readingOnly, "ab", `read -r -u3 l`), "T\nst=258 l=[ab]\nnext [cd]\n", 0},
		{"part, -n", "ksh", partOfALine(readingOnly, "ab", `read -r -n 4 l <&3`), "T\nst=0 l=[ab]\nnext [cd]\n", 0},
		// A newline read starts the next line, so a continued line is given
		// up after its backslash-newline.
		{"continued", "ksh", partOfALine(readingOnly, `a\\\n`, `read l <&3`), "T\nst=258 l=[a]\nnext [cd]\n", 0},
		{"continued", "dash", partOfALine(readingOnly, `a\\\n`, `read l <&3`), "T\nst=1 l=[a]\nnext [cd]\n", 0},
		// `-t` reads are given up by the same readings, and stopped by their
		// deadline as well.
		{"timed, nothing in", "ksh", partOfALine(readingOnly, "", `read -r -t 5 l <&3`), "T\nst=258 l=[]\nnext [cd]\n", 0},
		{"timed, nothing in", "ash", partOfALine(readingOnly, "", `read -r -t 5 l <&3`), "T\nst=1 l=[]\nnext [cd]\n", 0},
		{"timed, part", "ksh", partOfALine(readingOnly, "ab", `read -r -t 5 l <&3`), "T\nst=0 l=[abcd]\nnext [end]\n", 0},
		{
			"timed, part, both ways", "ksh", partOfALine(bothWays, "ab", `read -r -t 5 l <&3`),
			"T\nst=258 l=[ab]\nnext [cd]\n", 0,
		},
		// A pipe the shell made, read in the current shell: open for reading
		// alone, so ksh93 reads on. The rest is written only once the signal
		// has been sent.
		{
			"a pipe", "ksh",
			`mkfifo q; trap "echo T" INT; (sleep 0.3; kill -INT $$; echo go >q) & ` +
				`{ printf ab; read x <q; echo cd; } | read -r l; echo "st=$? l=[$l]"; wait`,
			"T\nst=0 l=[abcd]\n", 0,
		},
	}
	for _, r := range rows {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			t.Parallel()
			got := runReleasingFifos(t, presets[r.dialect], r.src)
			if r.given == 0 {
				if got != r.want {
					t.Errorf("got %q, want %q", got, r.want)
				}
				return
			}
			var l, m int
			tail := got[strings.Index(got, "st=")+len("st=258 "):]
			_, err := fmt.Sscanf(tail, "len=%d next=%d", &l, &m)
			if err != nil || !strings.Contains(got, "st=258 ") || l <= 0 || l >= r.given || l+m != r.given {
				t.Errorf("got %q, want 258 with the %d bytes split between the two reads", got, r.given)
			}
		})
	}
}
