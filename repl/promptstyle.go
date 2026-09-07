// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "github.com/blairham/sh/interp"

// The prompt-escape table, which is [interp.PromptStyle] and not a second
// type.
//
// There used to be two tables. This package held the one a prompt was *drawn*
// from — about forty escapes, filled in by the dialect — and `interp` held a
// second one of four that `${(%)…}` and `print -P` read, refusing everything
// else by name. So the shell answered `%F{196}` when drawing a prompt and
// refused it when a script asked for the same expansion: one question, two
// answers, and the larger table was the one a script could not reach (#1090).
//
// The table moved to the package both readers can see — supplied by a dialect
// the way [interp.Semantics] and [interp.Diagnostics] are — and everything
// below is an **alias** rather than a copy. That is the whole point of the
// aliases: `repl.PromptStyle` and `interp.PromptStyle` are the same Go type,
// so "the two tables agree" is not a property anybody has to maintain. There
// is one table, and a dialect that fills it in fills in both readers at once.
//
// What is still two is the *resolver*, deliberately: a drawer knows the
// session's history number, the terminal's name and what construct the parser
// is still inside, and a script's expansion does not. See the note at the top
// of interp/prompt.go, and promptrender.go for this package's half.
type PromptStyle = interp.PromptStyle

// The rest of the table's vocabulary, aliased for the same reason. A caller
// that has only ever written `repl.FieldUser` keeps working, and it is naming
// the same constant the interpreter refuses or answers.
type (
	// OpenWord is what a dialect calls one thing the parser is inside.
	OpenWord = interp.OpenWord
	// UnknownCode is what becomes of an escape whose code is not in the table.
	UnknownCode = interp.UnknownCode
	// PromptField is something a prompt draws that is not literal text.
	PromptField = interp.PromptField
	// PromptColor is which half of the screen a color code paints.
	PromptColor = interp.PromptColor
	// PromptResolver is what one code draws, for the reader that has the facts.
	PromptResolver = interp.PromptResolver
)

// What becomes of a code the table does not know.
const (
	KeepBoth   = interp.KeepBoth
	DropEscape = interp.DropEscape
	DropBoth   = interp.DropBoth
	Foreground = interp.Foreground
	Background = interp.Background
)

// Every field a prompt can draw.
const (
	FieldNone             = interp.FieldNone
	FieldUser             = interp.FieldUser
	FieldHost             = interp.FieldHost
	FieldHostFull         = interp.FieldHostFull
	FieldCwd              = interp.FieldCwd
	FieldCwdFull          = interp.FieldCwdFull
	FieldCwdBase          = interp.FieldCwdBase
	FieldCwdBaseFull      = interp.FieldCwdBaseFull
	FieldPrivilege        = interp.FieldPrivilege
	FieldShellName        = interp.FieldShellName
	FieldNewline          = interp.FieldNewline
	FieldReturn           = interp.FieldReturn
	FieldTab              = interp.FieldTab
	FieldTime24           = interp.FieldTime24
	FieldTime12           = interp.FieldTime12
	FieldTime24HM         = interp.FieldTime24HM
	FieldTime12AMPM       = interp.FieldTime12AMPM
	FieldTime12Padded     = interp.FieldTime12Padded
	FieldTime24Unpadded   = interp.FieldTime24Unpadded
	FieldTime24HMUnpadded = interp.FieldTime24HMUnpadded
	FieldDate             = interp.FieldDate
	FieldDateShort        = interp.FieldDateShort
	FieldDateMonthDayYear = interp.FieldDateMonthDayYear
	FieldDateYearMonthDay = interp.FieldDateYearMonthDay
	FieldSourceFile       = interp.FieldSourceFile
	FieldUnitName         = interp.FieldUnitName
	FieldEscape           = interp.FieldEscape
	FieldOpenState        = interp.FieldOpenState
	FieldVersion          = interp.FieldVersion
	FieldVersionFull      = interp.FieldVersionFull
	FieldHistoryNumber    = interp.FieldHistoryNumber
	FieldCommandNumber    = interp.FieldCommandNumber
	FieldJobCount         = interp.FieldJobCount
	FieldTerminalName     = interp.FieldTerminalName
	FieldExitStatus       = interp.FieldExitStatus
	FieldNonPrintingStart = interp.FieldNonPrintingStart
	FieldNonPrintingEnd   = interp.FieldNonPrintingEnd
)
