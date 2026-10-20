// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// `unset PIPESTATUS` takes the value away and leaves the producer, and the
// *description* has to follow.
//
// `${PIPESTATUS[@]}` after the removal already read `0` here and a later
// pipeline already refilled it — the read was right throughout — while
// `declare -p PIPESTATUS` answered `not found` at 1 where this shell writes
// the row. Five sites asked `removed` in front of the producer (#4877).
//
// Measured 2026-09-27 on bash 5.3.20 under `--noprofile --norc`, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
func TestUnsettingThePipelineStatusLeavesItsDescription(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `false | true
unset PIPESTATUS
declare -p PIPESTATUS
echo "rc=$?"
echo "read=[${PIPESTATUS[@]}]"
false | true | false
declare -p PIPESTATUS`)
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	want := `declare -a PIPESTATUS=([0]="0")
rc=0
read=[0]
declare -a PIPESTATUS=([0]="1" [1]="0" [2]="1")
`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The control, and it is what makes the rows above about this one name rather
// than about `unset`: every other producer here really does end with the name,
// and so does an ordinary variable. All four agree with bash 5.3.20 exactly,
// before this change and after it.
func TestUnsettingTheOtherProducersEndsThem(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a produced integer", `unset RANDOM; declare -p RANDOM; echo "rc=$?"; echo "read=[$RANDOM]"`},
		{"a produced line number", `unset LINENO; declare -p LINENO; echo "rc=$?"`},
		{"a produced array", `unset GROUPS; declare -p GROUPS; echo "rc=$?"`},
		{"and an ordinary variable", `V=1; unset V; declare -p V; echo "rc=$?"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), tc.src)
			if !regexp.MustCompile(`(?m)^rc=1$`).MatchString(out) {
				t.Errorf("%s = %q, want the listing to report at 1", tc.src, out)
			}
			if !strings.Contains(out, "not found") {
				t.Errorf("%s = %q, want `not found`", tc.src, out)
			}
		})
	}
}
