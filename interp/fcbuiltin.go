// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// biFc is `fc`, honest about the history this shell does not keep.
//
// The name is POSIX and every shell on the panel answers to it, so a script
// that calls it defensively must find a builtin — `builtin fc` was reporting
// something untrue. With no history there is nothing to list, edit or rerun:
// bash and dash answer a script's `fc -l` with silence at 0, zsh reports the
// event it cannot find at 1, and ksh93 reads a history file this shell keeps
// no equivalent of — recorded as a divergence rather than reproduced.
func biFc(r *Runner, _ context.Context, args []string) int {
	args, _, code := r.builtinOptions("fc", args, "lnrse:")
	if code != 0 {
		return code
	}
	_ = args
	if r.ask(r.sem().FcEmptyHistoryIsAnError, "`fc` with no history to answer from") {
		r.diagf("%s\n", Wording(r.diag().FcNoSuchEvent, "fc: no such event: 1"))
		return 1
	}
	if r.unspecified {
		return 2
	}
	return 0
}

func init() { builtins["fc"] = biFc }

var _ = strings.TrimSpace
