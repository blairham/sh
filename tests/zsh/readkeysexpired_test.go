// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `read -k` or `read -q` whose `-t` deadline passes with nothing read leaves
// its name exactly as it was, as an expired line read does, and reports its
// short read's status: 1 for `-k`, 2 for `-q` (#7038). Its end of input is not
// a deadline and assigns, empty for `-k` and `n` for `-q`. See
// interp.Semantics.ReadTimeoutKeepsWhatArrived.
//
// Every row is the script below run from -c on zsh 5.9.2 under `-f` on
// 2026-10-31, with its output copied. The stream is a fifo on descriptor 3
// opened for reading alone, with nothing in it until `late` and `end` are
// written a second in, so a read that has given up leaves `late` to the next
// one. The same rows read the terminal the same way through a pseudo-terminal
// with nothing typed, which is the source these letters have without `-u`.
//
// The last row is the reason a zsh deadline only ever finds nothing to assign:
// its `-t` bounds the wait for the first byte, so with `ab` waiting the read
// goes on for its third character and takes the `l` of `late`.
func TestAnExpiredKeyReadLeavesItsNameAlone(t *testing.T) {
	const left = "st=1 l=[old] R=[seed]\nnext [late]\n"
	for _, r := range []struct{ name, before, read, want string }{
		{"-k", "", `read -t 0.5 -k -u 3 l`, left},
		{"-k 3", "", `read -t 0.5 -k 3 -u 3 l`, left},
		{"-q", "", `read -t 0.5 -q -u 3 l`, "st=2 l=[old] R=[seed]\nnext [late]\n"},
		{"-q -k 2", "", `read -t 0.5 -q -k 2 -u 3 l`, "st=2 l=[old] R=[seed]\nnext [late]\n"},
		{"REPLY by -k", "", `read -t 0.5 -k -u 3`, left},
		{"REPLY by -q", "", `read -t 0.5 -q -u 3`, "st=2 l=[old] R=[seed]\nnext [late]\n"},
		{"an unset name", `unset l; `, `read -t 0.5 -k -u 3 l`, "st=1 l=[unset] R=[seed]\nnext [late]\n"},
		{"not a name, never reached", "", `read -t 0.5 -k -u 3 1x`, left},
		{"-k at the end of its input", "", `read -t 5 -k -u 3 l 3</dev/null`, "st=1 l=[] R=[seed]\nnext [late]\n"},
		{"-q at the end of its input", "", `read -t 5 -q -u 3 l 3</dev/null`, "st=2 l=[n] R=[seed]\nnext [late]\n"},
		{"a byte first", `printf ab >p; `, `read -t 0.5 -k 3 -u 3 l`, "st=0 l=[abl] R=[seed]\nnext [ate]\n"},
	} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			src := `mkfifo p; exec 4<>p 3<p; l=old; REPLY=seed; ` + r.before +
				`(sleep 1; printf 'late\nend\n' >p) & ` + r.read +
				`; echo "st=$? l=[${l-unset}] R=[$REPLY]"; read -r m <&3; echo "next [$m]"; wait`
			if got := runReleasingAfter(t, src, nil); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}
