// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/repl"

// HistoryStyle is how ksh93 searches its history and what it declines to keep.
//
// The answer to both is "nothing of its own", and it is measured rather than
// assumed. `C-r` in ksh93's emacs mode is not an incremental search: measured
// under a pseudo-terminal on 2026-09-05 with ksh93u+ 2012-08-01, it echoes
// `^R` and takes a whole string afterwards, so there is nothing here to
// reproduce a query-as-you-type rendering of. And neither knob exists — a line
// typed with a leading space is recorded, and setting `HISTIGNORE`, a name
// ksh93 does not have, changes nothing at all.
//
// So this dialect leaves both halves at the substrate's own answer: the search
// is offered, drawn in wording that names no shell, and nothing is filtered. A
// name a shell does not have must not be half-implemented — reading HISTIGNORE
// here would give ksh93 a knob real ksh93 ignores, which is worse than the gap
// it would be closing.
func HistoryStyle() repl.HistoryStyle { return repl.HistoryStyle{} }
