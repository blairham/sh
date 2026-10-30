// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import "testing"

// What a `read` that ends short of its line leaves in its names: at its `-t`
// deadline, and given up for a trapped signal (#7028). BusyBox ash keeps only
// the fields whose separator arrived, each in its own name, and leaves the
// field still arriving, the last name and every name after it as they were;
// dash and ksh93 assign what arrived at a give-up as an end of input does, and
// at a deadline bash assigns it and ksh93 assigns nothing. See
// interp.Semantics.ReadEndedShortKeepsOnlyCompleteFields.
//
// Every row is a script run from -c on 2026-10-30 against BusyBox ash 1.37.0
// in the pinned alpine image, dash 0.5.12, ksh93u+ 2012-08-01 and bash
// 5.3.20, and the wanted output is what that shell printed. The stream is a
// fifo opened both ways on descriptor 3, with the text written before the
// read. A deadline is `-t 0.3` with nothing more coming, and `LATE` is
// written by the script once the read is over. A signal is INT a second in,
// which every one of these shells gives a read up for while it waits, and
// `LATE` comes half a second after it; the second is a margin for a loaded
// runner, and a read the signal found not yet waiting reads `LATE` and fails
// rather than hanging.
func TestAReadThatEndsShortKeepsWhatTheShellsKeep(t *testing.T) {
	t.Parallel()
	const vars = `a=A b=B c=C l=old REPLY=R; `
	const show = `echo "st=$s a=[$a] b=[$b] c=[$c] l=[$l] R=[$REPLY]"; read -r m <&3; echo "next [$m]"`
	signaled := func(setup, first, read string) string {
		return `mkfifo p; exec 3<>p; trap "echo T" INT; ` + setup + vars + `printf '` + first + `' >p; ` +
			`(sleep 1; kill -INT $$; sleep 0.5; echo LATE >p) & ` + read + ` <&3; s=$?; ` + show + `; wait`
	}
	expired := func(setup, first, read string) string {
		return `mkfifo p; exec 3<>p; ` + setup + vars + `printf '` + first + `' >p; ` +
			`read -t 0.3 ` + read[len("read "):] + ` <&3; s=$?; echo LATE >p; ` + show
	}
	// left is what the script prints of the names once the read is over, at
	// status st.
	left := func(st, a, b, c, l string) string {
		return "st=" + st + " a=[" + a + "] b=[" + b + "] c=[" + c + "] l=[" + l + "] R=[R]\n"
	}
	type row struct{ name, dialect, src, want string }
	var rows []row
	for _, end := range []struct {
		name string
		src  func(setup, first, read string) string
		tag  string
	}{{"expired", expired, ""}, {"given up", signaled, "T\n"}} {
		ash := func(name, setup, first, read, want string) {
			rows = append(rows, row{end.name + ", " + name, "ash", end.src(setup, first, read), end.tag + want})
		}
		ash("x y into two", "", "x y", "read -r a b", left("1", "x", "B", "C", "old"))
		ash("x y_ into three", "", "x y ", "read -r a b c", left("1", "x", "y", "C", "old"))
		ash("x y z_ into two", "", "x y z ", "read -r a b", left("1", "x", "B", "C", "old"))
		ash("x y into one", "", "x y", "read -r l", left("1", "A", "B", "C", "old"))
		ash("nothing into one", "", "", "read -r l", left("1", "A", "B", "C", "old"))
		ash("an escaped blank", "", `x\\ y z `, "read a b c", left("1", "x y", "z", "C", "old"))
		ash("an escaped blank last", "", `x\\ `, "read a b", left("1", "A", "B", "C", "old"))
		ash("an empty field", "IFS=:; ", ":y", "read -r a b c", left("1", "", "B", "C", "old"))
		ash("two separators", "IFS=:; ", "x::", "read -r a b c", left("1", "x", "", "C", "old"))
		ash("into REPLY", "", "x y ", "read -r", left("1", "A", "B", "C", "old"))
	}
	rows = append(rows,
		row{"given up", "dash", signaled("", "x y ", "read -r a b c"), "T\n" + left("1", "x", "y", "", "old")},
		row{"given up, one name", "dash", signaled("", "x y", "read -r l"), "T\n" + left("1", "A", "B", "C", "x y")},
		row{"given up, nothing", "dash", signaled("", "", "read -r l"), "T\n" + left("1", "A", "B", "C", "")},
		row{"given up", "ksh", signaled("", "x y ", "read -r a b c"), "T\n" + left("258", "x", "y", "", "old")},
		row{"given up, one name", "ksh", signaled("", "x y", "read -r l"), "T\n" + left("258", "A", "B", "C", "x y")},
		row{"expired", "ksh", expired("", "x y ", "read -r a b c"), left("1", "A", "B", "C", "old")},
		row{"expired", "bash", expired("", "x y ", "read -r a b c"), left("142", "x", "y", "", "old")},
		row{"expired, two", "bash", expired("", "x y z ", "read -r a b"), left("142", "x", "y z", "C", "old")},
	)
	for _, r := range rows {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			t.Parallel()
			want := r.want + "next [LATE]\n"
			if got := runReleasingFifos(t, presets[r.dialect], r.src); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
