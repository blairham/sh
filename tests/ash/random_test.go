// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"regexp"
	"testing"
)

// TestRandomIsAProducedParameter: `RANDOM` draws, a seed is not stored, the
// same seed draws the same pair, `unset` makes it an ordinary name, and a
// bare `set` lists the last reading and nothing before one. Measured
// 2026-10-03 in the pinned image. The numbers themselves are not BusyBox's:
// see the note in Apply.
func TestRandomIsAProducedParameter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[ -n "${RANDOM-}" ] && echo have || echo none`, `^have\n$`},
		{`RANDOM=5; a=$RANDOM; [ "$a" = 5 ] && echo stored || echo "produced $a"`, `^produced [0-9]+\n$`},
		{`RANDOM=42; a="$RANDOM $RANDOM"; RANDOM=42; b="$RANDOM $RANDOM"; [ "$a" = "$b" ] && echo "same $a"`, `^same [0-9]+ [0-9]+\n$`},
		{`unset RANDOM; RANDOM=9; echo "$RANDOM $RANDOM"`, `^9 9\n$`},
		{`set | grep ^RANDOM; echo --; v=$RANDOM; set | grep "^RANDOM='$v'\$"`, `^--\nRANDOM='[0-9]+'\n$`},
	} {
		out, _ := run(t, tc.src)
		if !regexp.MustCompile(tc.want).MatchString(out) {
			t.Errorf("%s\n got %q, want %s", tc.src, out, tc.want)
		}
	}
}
