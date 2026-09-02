// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

// Prelude is the part of the dialect written as shell rather than as Go.
//
// Empty, and that is dash's answer rather than a gap: it defines no functions
// of its own and names itself in no variable. A script asking which shell it
// is under gets nothing from dash, which is how a script tells dash from the
// three that answer.
func Prelude() string { return "" }
