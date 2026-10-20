// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestTheCallStackParametersAnswerABareName is this dialect's half of #1600.
//
// `$FUNCNAME` and `${BASH_SOURCE}` without a subscript are ordinary in real
// scripts — the second is how a great many of them find their own directory —
// and both answered *empty* rather than failing, so a script using one got a
// wrong path at status 0. `[[ -v FUNCNAME ]]` was worse in kind: a parameter
// this dialect registers reported that it did not exist, so a script guarding
// on it took the wrong branch on purpose.
//
// The rows are bash 5.3.15's, and the two element-selecting ones are the
// point of this dialect being the place to assert it: bash reads a bare array
// name as its *first element*, so these are `g` and not `g f`. The same fix
// gives zsh the joined list, which dialect/zsh asserts for itself.
func TestTheCallStackParametersAnswerABareName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "FUNCNAME is the innermost function",
			src:  `f(){ g; }; g(){ echo "[$FUNCNAME]"; }; f`,
			want: "[g]\n",
		},
		{
			name: "braced reads the same",
			src:  `f(){ g; }; g(){ echo "[${FUNCNAME}]"; }; f`,
			want: "[g]\n",
		},
		{
			// The length of the first element, not the count — which is what
			// makes this a different assertion from ${#FUNCNAME[@]}, and the
			// one that would pass on a fix that answered the count here.
			name: "the length is the first element's",
			src:  `f(){ g; }; g(){ echo "${#FUNCNAME} ${#FUNCNAME[@]}"; }; f`,
			want: "1 2\n",
		},
		{
			name: "a default is not taken",
			src:  `g(){ echo "${FUNCNAME:-DEFAULT}"; }; g`,
			want: "g\n",
		},
		{
			name: "-v says the parameter is there",
			src:  `g(){ [[ -v FUNCNAME ]] && echo yes || echo no; }; g`,
			want: "yes\n",
		},
		{
			name: "and is not there outside a call",
			src:  `[[ -v FUNCNAME ]] && echo yes || echo no`,
			want: "no\n",
		},
		{
			// Measured: bash 5.3.15 under `-c` at the top level answers
			// `${BASH_SOURCE}` empty and `${#BASH_SOURCE[@]}` 0, because
			// there is no frame — the same reason `-v FUNCNAME` is false
			// there. This row is the empty *producer*, which is the case
			// that separates the two dialects: reading the whole list would
			// make the name set, and bash's base-element reading does not.
			name: "BASH_SOURCE is unset with no frame",
			src:  `echo "[${BASH_SOURCE}][${BASH_SOURCE+SET}]${#BASH_SOURCE[@]}"`,
			want: "[][]0\n",
		},
		{
			// And the bare name is element zero rather than the list, said
			// without naming a path: under `-c` the file is whatever the
			// shell calls itself, which is the front end's business.
			name: "BASH_SOURCE in a call is its first element",
			src: `f(){ g; }; g(){ [[ $BASH_SOURCE == "${BASH_SOURCE[0]}" ]] && echo same || echo differs
	echo "n=${#BASH_SOURCE[@]} empty=$([[ -n $BASH_SOURCE ]] && echo no || echo yes)"; }; f`,
			want: "same\nn=2 empty=no\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}
