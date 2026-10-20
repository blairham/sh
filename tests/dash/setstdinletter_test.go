// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `set -s` is the short spelling of this shell's `stdin` option, and the
// letter and the name are one request with one answer.
//
// Measured 2026-09-17 on dash 0.5.12, from a script file under
// `env -i PATH=/usr/bin:/bin`:
//
//	set -s; echo "st=$? [$-]"; set +s; echo "[$-]"
//	  st=0 [s]
//	  []
//
// and the listing agrees with `$-` in both directions — `stdin on` after the
// first and `stdin off` after the second. Nothing else was observed to move:
// the program is not re-read, so what the letter carries is a bit of `$-`
// rather than a mode.
//
// Before #3411 the letter reached this dialect's unimplemented-letter table
// and the script ended at 2, while `set -o stdin` beside it worked — the two
// spellings of one option answering differently.
func TestTheStdinLetterMovesTheStdinOption(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -s; echo "st=$? [$-]"`, "st=0 [s]\n"},
		{`set -s; set +s; echo "[$-]"`, "[]\n"},
		// The name writes the same state the letter does, which is what
		// makes them one request: neither can be observed to disagree.
		{`set -o stdin; echo "[$-]"`, "[s]\n"},
		{`set -s; set +o stdin; echo "[$-]"`, "[]\n"},
		// And a shell asking about itself reads the same bit.
		{`set -s; case "$-" in *s*) echo yes ;; *) echo no ;; esac`, "yes\n"},
	} {
		out, status, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if status != 0 {
			t.Errorf("%s ended at %d, want 0", tc.src, status)
		}
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// And the listing moves with it, which is the half `$-` cannot show: a
// script reads `set -o` to find out how the shell was started, so a letter
// that left the row behind would be two states under one name.
func TestTheStdinLetterMovesTheListingRow(t *testing.T) {
	for _, tc := range []struct{ src, row string }{
		{`set -s; set -o`, "stdin           on"},
		{`set -s; set +s; set -o`, "stdin           off"},
	} {
		out, status, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if status != 0 {
			t.Errorf("%s ended at %d, want 0", tc.src, status)
		}
		if !strings.Contains(out, tc.row+"\n") {
			t.Errorf("%s listed %q, want a row %q", tc.src, out, tc.row)
		}
	}
}
