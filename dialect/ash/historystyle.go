// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

import "github.com/blairham/sh/repl"

// HistoryStyle is how ash searches its history and what it declines to keep.
//
// The zero value, and it means "no answer" rather than "ash's answer". This
// shell's line editing and history are a build-time option in BusyBox, so
// there is no one behavior to be compatible with — and, with no ash on this
// machine, no way to measure which build a reader has. We offer a search and a
// recall list because we have a line editor; nothing measured makes one
// wording more ash-like than another.
func HistoryStyle() repl.HistoryStyle { return repl.HistoryStyle{} }
