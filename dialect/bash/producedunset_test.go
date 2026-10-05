// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A produced array `unset` takes away is unset, not just emptied. Measured
// 2026-10-05 on bash 5.3: after `pushd /tmp`, `unset DIRSTACK` leaves
// `${DIRSTACK+set}` empty and `${#DIRSTACK[@]}` 0, `unset GROUPS` leaves
// `${GROUPS+set}` empty, and `unset FUNCNAME` inside a function does the
// same. This shell kept answering every read from the producer (#5961).
func TestAnUnsetProducedArrayIsUnset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"DIRSTACK", `pushd / >/dev/null; unset DIRSTACK; echo "[${DIRSTACK+set}] ${#DIRSTACK[@]}"`, "[] 0\n"},
		{"GROUPS", `unset GROUPS; echo "[${GROUPS+set}]"`, "[]\n"},
		{"FUNCNAME", `f() { unset FUNCNAME; echo "[${FUNCNAME+set}]"; }; f`, "[]\n"},
		// The control: before the unset each one is set.
		{"control", `pushd / >/dev/null; f() { echo "[${FUNCNAME+set}]"; }; f; echo "[${DIRSTACK+set}][${GROUPS+set}]"`, "[set]\n[set][set]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := runEmptyArray(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}
