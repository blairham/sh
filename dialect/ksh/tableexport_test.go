// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// An exported keyed table reaches no child here, whatever its key, where an
// exported array hands one its first element. Measured 2026-10-03 on
// ksh93u+; see Semantics.ExportedTableReachesAChild.
func TestAnExportedTableReachesNoChild(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m=([k]=v); export m; /usr/bin/env | grep '^m=' || echo none`, "none\n"},
		{`typeset -A m=([0]=z); export m; /usr/bin/env | grep '^m=' || echo none`, "none\n"},
		{`typeset -A m; m[k]=v; typeset -i m; export m; /usr/bin/env | grep '^m=' || echo none`, "none\n"},
		// The control: an indexed array still hands its first element on.
		{`a=(x y); export a; /usr/bin/env | grep '^a='`, "a=x\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}
