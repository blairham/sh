// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"errors"
	"strings"
	"testing"
)

// normalize replaces each shell's full path and never its base name, so a line
// where a shell names *itself* by base name is compared literally. That is
// sound only while the two base names are equal, and nothing in the report says
// so when they are not — which is how five rows of bash's suite came to be
// reported as regressions on figures that were the binary's name.
//
// The three rows are the three answers this has to have, and the middle one is
// the reason it is not a flat refusal: a reference the machine keeps under
// another name cannot be made to match, and refusing would take the column away
// rather than fix anything.
func TestCheckBaseNames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		suite     Suite
		ours, ref string
		wantWarn  bool
		wantErr   bool
	}{
		{
			name:  "the names agree, at any depth",
			suite: Suite{Dialect: "bash"},
			ours:  "build/shells/bash", ref: "/bin/bash",
		},
		{
			name:  "the reference is under another name on this machine",
			suite: Suite{Dialect: "ksh"},
			ours:  "build/shells/ksh", ref: "/opt/homebrew/bin/ksh93",
			wantWarn: true,
		},
		{
			name:  "ours is named neither like the reference nor like its column",
			suite: Suite{Dialect: "bash"},
			ours:  "/alt/ourbash", ref: "/bin/bash",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warning, err := CheckBaseNames(tc.suite, tc.ours, tc.ref)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want an error: %v", err, tc.wantErr)
			}
			if (warning != "") != tc.wantWarn {
				t.Errorf("warning = %q, want one: %v", warning, tc.wantWarn)
			}
			if tc.wantErr {
				if !errors.Is(err, ErrBaseNameDiffers) {
					t.Errorf("err = %v, want it to wrap ErrBaseNameDiffers", err)
				}
				// Both names, so a reader can see which is which without
				// going to look. The remedy itself is the caller's to print
				// — see the ErrDigest branch it sits beside.
				if !strings.Contains(err.Error(), `"ourbash"`) ||
					!strings.Contains(err.Error(), `"bash"`) {
					t.Errorf("err = %v, want both base names named", err)
				}
			}
			if tc.wantWarn && !strings.Contains(warning, "ceiling") {
				t.Errorf("warning = %q, want it to say the figures are a ceiling", warning)
			}
		})
	}
}
