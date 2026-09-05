// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Route is where a shell's program came from, which the front end reads off
// the invocation and hands to the Runner.
//
// It is a fact about the invocation and not about the language, so nothing
// here decides it — the same shape as Runner.Interactive, and carried in for
// the same reason: a Runner embedded in another program would otherwise be
// told about the embedder's command line.
//
// Three routes, because that is what the panel distinguishes: `$0` is named
// by a different rule on each, an alias is expanded on some routes and not
// others, and `$-` shows a letter for two of them in some shells.
type Route int

const (
	// RouteUnspecified is an embedder that has not said. It behaves as
	// neither a command string nor standard input — which is what a Runner
	// built by hand answered before there was a name for the question, so
	// the zero value keeps that answer rather than quietly becoming one of
	// the three.
	RouteUnspecified Route = iota
	// RouteCommandString is `-c`: the program is an argument.
	RouteCommandString
	// RouteScriptFile is a path operand: the program is a file, and the
	// path is `$0`.
	RouteScriptFile
	// RouteStandardInput is `-s`, or no operands at all: the program
	// arrives on the same descriptor the script itself reads from.
	RouteStandardInput
)

// String names the route, so a failing test says which one it meant.
func (r Route) String() string {
	switch r {
	case RouteCommandString:
		return "a command string"
	case RouteScriptFile:
		return "a script file"
	case RouteStandardInput:
		return "standard input"
	}
	return "an unspecified route"
}
