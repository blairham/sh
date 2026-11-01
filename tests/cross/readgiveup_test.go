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

// pendingAsItBegins raises INT from a command substitution in the read's own
// redirection — after the shell has run the handlers waiting at the top of
// the command, and before the builtin starts — so the signal is already
// waiting as the read begins, with `first` there to be read. `cd` and `end`
// come half a second later. Nothing about it is a race, which is why it is
// the shape that pins "a signal that came before the wait": the flowing line
// below is the issue's own shape and races in dash and ash, which give a read
// up exactly when the signal finds the reader waiting — on a pipe fed by
// `head | tr`, three runs of four for real dash.
func pendingAsItBegins(open, first, read, next string) string {
	return `mkfifo p; ` + open + `; trap "echo T" INT; printf '` + first + `' >p; ` +
		`(sleep 0.5; printf 'cd\nend\n' >p) & ` +
		read + ` 2>"$(kill -INT $$)/dev/null"; echo "st=$? l=[$l]"; ` + next + `; echo "next [$m]"; wait`
}

// aLineStillArriving writes two 64 KiB blocks of `a` — the second cannot be
// written until the read has taken the first, so the read is under way — then
// sends INT, then writes `then`, pauses, and ends the line. Only the rows
// whose answer does not hang on whether the reader was waiting at that
// instant use it.
func aLineStillArriving(open, then, read, next string) string {
	return `mkfifo p; c=a; i=0; while [ $i -lt 16 ]; do c=$c$c; i=$((i+1)); done; ` + open + `; ` +
		`trap "echo T" INT; trap "echo U" USR1; ` +
		`{ printf %s "$c"; printf %s "$c"; kill -INT $$; ` + then + ` echo; echo tail; } >p & ` +
		read + `; s=$?; ` + next + `; echo "st=$s len=${#l} next=${#m}"; wait`
}

// aBlockStillArriving is aLineStillArriving with thenUSR1, for the row whose
// answer hangs on INT landing while there are bytes still to read: a read
// given up at the next byte keeps part of the line only if there is a next
// byte. Sent after the second block, INT has at most a fifo's worth left to
// land in, and a fifo on macOS holds 8 KiB: a reader that took those first
// met INT in its wait, and gave the whole line up with nothing left for the
// next read — which is what a read given up only in a wait does, so the row
// cannot accept it (#7184). So the writer says so through q just before a
// third block, of 128 KiB, and another job sends INT on that word while the
// block is being written. That widens the margin from what a fifo holds to
// the block; it does not order the two. ksh93u+ printed `T, U, st=258
// len=131072 next=131072` for it on 2026-10-31, as for the arriving rows.
func aBlockStillArriving(open, read, next string) string {
	return `mkfifo p q; c=a; i=0; while [ $i -lt 16 ]; do c=$c$c; i=$((i+1)); done; ` + open + `; ` +
		`trap "echo T" INT; trap "echo U" USR1; (read -r x <q; kill -INT $$) & ` +
		`{ printf %s "$c"; printf %s "$c"; echo >q; printf %s "$c$c"; ` + thenUSR1 + ` echo; echo tail; } >p & ` +
		read + `; s=$?; ` + next + `; echo "st=$s len=${#l} next=${#m}"; wait`
}

// partOfALine has `first` waiting before the read, sends INT 0.3 s in while
// the read waits for the rest, and writes `cd` and `end` half a second after
// that: a read given up has what was first, and the next one `cd`; a read
// that goes on has it and `cd`, and the next one `end`.
func partOfALine(open, first, read string) string {
	return `mkfifo p; ` + open + `; trap "echo T" INT; printf '` + first + `' >p; ` +
		`(sleep 0.3; kill -INT $$; sleep 0.5; printf 'cd\nend\n' >p) & ` +
		read + `; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`
}

const (
	bothWays    = `exec 3<>p`
	readingOnly = `exec 4<>p 3<p`
	moreBlocks  = `printf %s "$c"; printf %s "$c"; sleep 0.2;`
	// Three seconds rather than the half second the real shells need,
	// because a Linux pipe holds 64 KiB and this shell under the race
	// detector took 0.8 s to drain it: a USR1 landing with bytes still there
	// is, rightly, not a give-up.
	thenUSR1  = `sleep 3; kill -USR1 $$; sleep 0.3;`
	redirect  = `read -r l <&3`
	redirect2 = `read -r m <&3`
	byNumber  = `read -r -u3 l`
	byNumber2 = `read -r -u3 m`
)

func TestATrappedSignalGivesUpAReadWhereTheShellsDo(t *testing.T) {
	t.Parallel()
	const (
		readOn      = "T\nst=0 l=[abcd]\nnext [end]\n"
		readOnEmpty = "T\nst=0 l=[cd]\nnext [end]\n"
		gaveUpAB    = "T\nst=1 l=[ab]\nnext [cd]\n"
		gaveUpAB258 = "T\nst=258 l=[ab]\nnext [cd]\n"
	)
	type row struct {
		name, dialect, src string
		// want is the output; given, where it is set instead, is the length
		// the two reads share, for a give-up whose point is a matter of
		// timing — the status is checked and the first read must have part.
		want  string
		given int
	}
	rows := []row{
		// A signal already waiting as the read begins does not give it up in
		// any of the three — a wait that begins after it is not one it lands
		// in — except, in ksh93, on a descriptor open both ways, where the
		// read is given up before it has taken a byte and `ab` is left.
		{"pending, part in", "dash", pendingAsItBegins(readingOnly, "ab", redirect, redirect2), readOn, 0},
		{"pending, both ways", "dash", pendingAsItBegins(bothWays, "ab", redirect, redirect2), readOn, 0},
		{"pending, nothing in", "dash", pendingAsItBegins(readingOnly, "", redirect, redirect2), readOnEmpty, 0},
		{"pending, part in", "ash", pendingAsItBegins(readingOnly, "ab", redirect, redirect2), readOn, 0},
		{"pending, both ways", "ash", pendingAsItBegins(bothWays, "ab", redirect, redirect2), readOn, 0},
		{"pending, nothing in", "ash", pendingAsItBegins(readingOnly, "", redirect, redirect2), readOnEmpty, 0},
		{"pending, part in", "ksh", pendingAsItBegins(readingOnly, "ab", redirect, redirect2), readOn, 0},
		{"pending, nothing in", "ksh", pendingAsItBegins(readingOnly, "", redirect, redirect2), readOnEmpty, 0},
		{"pending, -u", "ksh", pendingAsItBegins(readingOnly, "ab", byNumber, byNumber2), readOn, 0},
		{"pending, -u, nothing in", "ksh", pendingAsItBegins(readingOnly, "", byNumber, byNumber2), readOnEmpty, 0},
		{"pending, -t", "ksh", pendingAsItBegins(readingOnly, "", `read -r -t 5 l <&3`, redirect2), readOnEmpty, 0},
		{
			"pending, both ways", "ksh", pendingAsItBegins(bothWays, "ab", redirect, redirect2),
			"T\nst=258 l=[]\nnext [abcd]\n", 0,
		},
		// A line still arriving, with INT landing as the read takes it.
		// ksh93 finishes it on its standard input from a descriptor open for
		// reading alone, and gives it up from one open both ways or read by
		// `-u` — measured at `st=258 len=131072 next=131072`, ksh93 having
		// taken what was written before the signal; this shell stops at the
		// next byte, so where the line splits differs and nothing is lost.
		{
			"arriving, reading only", "ksh", aLineStillArriving(readingOnly, moreBlocks, redirect, redirect2),
			"T\nst=0 len=262144 next=4\n", 0,
		},
		{"arriving, both ways", "ksh", aLineStillArriving(bothWays, moreBlocks, redirect, redirect2), "", 262144},
		{"arriving, -u", "ksh", aLineStillArriving(readingOnly, moreBlocks, byNumber, byNumber2), "", 262144},
		// The same, with no more bytes after INT and a USR1 once the read is
		// waiting. In dash the second signal lands in a wait and gives the
		// read up — and so does the first if it found the reader already
		// waiting, which leaves the same two lines; ksh93 reading a line it
		// has begun reads on. ksh93 both ways gives it up at INT, which has
		// to find bytes still to read for that to differ from giving it up
		// in a wait, so its row has a block after INT (aBlockStillArriving).
		{
			"second signal, both ways", "dash", aLineStillArriving(bothWays, thenUSR1, redirect, redirect2),
			"T\nU\nst=1 len=131072 next=0\n", 0,
		},
		{
			"second signal, reading only", "dash", aLineStillArriving(readingOnly, thenUSR1, redirect, redirect2),
			"T\nU\nst=1 len=131072 next=0\n", 0,
		},
		{
			"second signal, reading only", "ksh", aLineStillArriving(readingOnly, thenUSR1, redirect, redirect2),
			"T\nU\nst=0 len=131072 next=4\n", 0,
		},
		{"second signal, both ways", "ksh", aBlockStillArriving(bothWays, redirect, redirect2), "", 262144},
		// Part of a line in, and a wait the signal lands in. dash gives it
		// up whatever the descriptor; ksh93 reads on only on its standard
		// input from a descriptor open for reading alone, reading to a
		// delimiter.
		{"part, reading only", "dash", partOfALine(readingOnly, "ab", redirect), gaveUpAB, 0},
		{"part, both ways", "dash", partOfALine(bothWays, "ab", redirect), gaveUpAB, 0},
		{"part, reading only", "ksh", partOfALine(readingOnly, "ab", redirect), readOn, 0},
		{"part, both ways", "ksh", partOfALine(bothWays, "ab", redirect), gaveUpAB258, 0},
		{"part, -u", "ksh", partOfALine(readingOnly, "ab", byNumber), gaveUpAB258, 0},
		{"nothing, -u", "ksh", partOfALine(readingOnly, "", byNumber), "T\nst=258 l=[]\nnext [cd]\n", 0},
		{"part, -n", "ksh", partOfALine(readingOnly, "ab", `read -r -n 4 l <&3`), "T\nst=0 l=[ab]\nnext [cd]\n", 0},
		// Where the stream came from (#7030), measured 2026-10-31 on ksh93u+
		// with these scripts. A `-u` read of a descriptor the script opened
		// at that number is given up at the next byte, and one of a copy
		// reads on as standard input does — a copy of itself, and a copy
		// whose source has been closed, included; a move keeps what its
		// source was.
		{"part, -u on a copy", "ksh", partOfALine(readingOnly+`; exec 5<&3`, "ab", `read -r -u5 l`), readOn, 0},
		{"part, -u on a copy of itself", "ksh", partOfALine(readingOnly+`; exec 3<&3`, "ab", byNumber), readOn, 0},
		{"part, -u on a {name} copy", "ksh", partOfALine(readingOnly+`; exec {v}<&3`, "ab", `read -r -u$v l`), readOn, 0},
		{
			"part, -u on a copy whose source closed", "ksh",
			partOfALine(`exec 4<>p 5<p; exec 3<&5; exec 5<&-`, "ab", byNumber), readOn, 0,
		},
		{"part, -u on a move", "ksh", partOfALine(`exec 4<>p 5<p; exec 3<&5-`, "ab", byNumber), gaveUpAB258, 0},
		{
			"part, -u on a moved copy", "ksh",
			partOfALine(`exec 4<>p 5<p; exec 6<&5; exec 3<&6-`, "ab", byNumber), readOn, 0,
		},
		// `-u0` is standard input, which reads on — unless a builtin's own
		// redirection opened it, which `exec` and a function's or a group's
		// do not count as.
		{"part, -u0", "ksh", partOfALine(readingOnly+`; exec 0<&3`, "ab", `read -r -u0 l`), readOn, 0},
		{"part, its own <p", "ksh", partOfALine(readingOnly, "ab", `read -r l <p`), gaveUpAB258, 0},
		{"part, -u0 and its own <p", "ksh", partOfALine(readingOnly, "ab", `read -r -u0 l <p`), gaveUpAB258, 0},
		{"part, command's <p", "ksh", partOfALine(readingOnly, "ab", `command read -r l <p`), gaveUpAB258, 0},
		{"part, eval's <p", "ksh", partOfALine(readingOnly, "ab", `eval 'read -r l' <p`), gaveUpAB258, 0},
		{"part, a function's <p", "ksh", partOfALine(readingOnly, "ab", `f() { read -r l; }; f <p`), readOn, 0},
		{
			"part, the <p of a function named for a builtin", "ksh",
			partOfALine(readingOnly, "ab", `true() { command read -r l; }; true <p`), readOn, 0,
		},
		{"part, a group's <p", "ksh", partOfALine(readingOnly, "ab", `{ read -r l; } <p`), readOn, 0},
		// And a copy of a descriptor open both ways is open both ways: a
		// signal already waiting gives it up, `ab` left.
		{
			"pending, -u on a copy, both ways", "ksh",
			pendingAsItBegins(bothWays+`; exec 5<&3`, "ab", `read -r -u5 l`, `read -r -u5 m`),
			"T\nst=258 l=[]\nnext [abcd]\n", 0,
		},
		// A newline read starts the next line, so a continued line is given
		// up after its backslash-newline.
		{"continued", "ksh", partOfALine(readingOnly, `a\\\n`, `read l <&3`), "T\nst=258 l=[a]\nnext [cd]\n", 0},
		{"continued", "dash", partOfALine(readingOnly, `a\\\n`, `read l <&3`), "T\nst=1 l=[a]\nnext [cd]\n", 0},
		// `-t` reads are given up by the same readings, and stopped by their
		// deadline as well.
		{"timed, nothing in", "ksh", partOfALine(readingOnly, "", `read -r -t 5 l <&3`), "T\nst=258 l=[]\nnext [cd]\n", 0},
		{"timed, nothing in", "ash", partOfALine(readingOnly, "", `read -r -t 5 l <&3`), "T\nst=1 l=[]\nnext [cd]\n", 0},
		{"timed, part", "ksh", partOfALine(readingOnly, "ab", `read -r -t 5 l <&3`), readOn, 0},
		{"timed, part, both ways", "ksh", partOfALine(bothWays, "ab", `read -r -t 5 l <&3`), gaveUpAB258, 0},
		// A pipe the shell made, read in the current shell: open for reading
		// alone, so ksh93 reads on. The rest is written only once the signal
		// has been sent.
		//
		// The signal waits for `ab` to be in the pipe — the left side says so
		// through p — and then for a second more, so the read has taken it.
		// A clock alone raced: at 0.3 s from the start, a loaded runner under
		// the race detector had sometimes not yet written `ab`, and a read
		// still waiting for its first byte is rightly given up (#7042). The
		// second covers the read's own wake-up, since a byte there is seen
		// on a goroutine where ksh93 is handed it by the kernel.
		{
			"a pipe", "ksh",
			`mkfifo p q; trap "echo T" INT; (read y <p; sleep 1; kill -INT $$; echo go >q) & ` +
				`{ printf ab; echo >p; read x <q; echo cd; } | read -r l; echo "st=$? l=[$l]"; wait`,
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
