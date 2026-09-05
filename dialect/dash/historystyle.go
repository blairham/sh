// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import "github.com/blairham/sh/repl"

// HistoryStyle is how dash searches its history and what it declines to keep.
//
// dash has no history at all: measured on 2026-09-05, an interactive dash run
// with `HISTFILE` set leaves no file behind, and it has no line editor to
// search from either. There is therefore nothing to be compatible with, in
// either half.
//
// The zero value it returns is not "dash's answer" but "no answer" — this
// shell offers a search and a recall list because it has a line editor and
// dash does not, and there is no measurement that would make one wording more
// dash-like than another.
func HistoryStyle() repl.HistoryStyle { return repl.HistoryStyle{} }
