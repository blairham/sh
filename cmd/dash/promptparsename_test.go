// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// The other side of #6244: dash names a line that will not parse at its prompt
// by the whole of the name it was started as, as it names everything else, so
// the shortening bash does must not reach it.
//
// Measured 2026-10-06 on dash (/bin/dash) on a pipe, `env -i`, started as
// `links/dfoo -i`: `links/dfoo: 1: Syntax error: "fi" unexpected`.
func TestADashPromptNamesALineThatWillNotParseByItsWholeName(t *testing.T) {
	_, errs, _ := prompt(t, "fi\n", "./weird/mydash", "-i")
	if !strings.Contains(errs, "./weird/mydash: 1: Syntax error: \"fi\" unexpected\n") {
		t.Errorf("stderr %q, want the whole of argv[0] in front of the refusal", errs)
	}
}
