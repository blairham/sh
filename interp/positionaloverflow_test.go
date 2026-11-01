// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A positional index too wide for the machine's integer is past the end of
// the list — unset, and stored nowhere — on every route that reads one. Its
// digits used to wrap to a negative index, which panicked the shell in every
// dialect (#6849). Measured 2026-10-16: bash 5.3.20, bash 3.2.57 and dash
// expand `${9223372036854775808}` to nothing.
func TestAPositionTooWideForAnyListIsUnset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`set -- a b; echo "[${9223372036854775808}][${9223372036854775807}]"`, "[][]\n"},
		{`set -- a b; echo "[${99999999999999999999999}]"`, "[]\n"},
		{`set -- a b; echo "[${9223372036854775808-unset}]"`, "[unset]\n"},
		{`set -- a b; echo "[${00000000000000000000002}]"`, "[b]\n"},
	} {
		if out, st := run(t, tc.src, nil); out != tc.want || st != 0 {
			t.Errorf("%s: %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
