// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// jobSpecMiss is the sentence a job builtin writes for a spec that names
// nothing, where the dialect words that by the spec's shape, and whether it
// does. See Diagnostics.JobSpecNoCurrent for the measurement.
//
// One function for every builtin that resolves a job spec, because the shell
// that has the wordings has them on all four — `kill`, `disown`, `jobs` and
// `wait` measured alike — and a builtin left on its old sentence is the
// second helper this tree keeps finding (#5140).
func jobSpecMiss(d Diagnostics, builtin, spec string) (string, bool) {
	body, ok := strings.CutPrefix(spec, "%")
	if !ok {
		return "", false
	}
	var format string
	switch {
	case body == "" || body == "%" || body == "+":
		format = d.JobSpecNoCurrent
	case body == "-":
		format = d.JobSpecNoPrevious
	case digitRun(body, false):
		// A job number keeps the builtin's ordinary sentence.
		return "", false
	default:
		format = d.JobSpecNotFound
	}
	if format == "" {
		return "", false
	}
	return Wording(format, "", builtin, body), true
}
