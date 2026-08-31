// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

// processTimes has no answer where there is no getrusage.
//
// Reporting zeros would be a pretence — four plausible numbers that mean
// nothing — so this says it has none and `times` refuses instead, the same
// way the parser refuses a construct a dialect does not have. procgroup_other
// takes the same line for process groups.
func processTimes() (self, children cpuTime, ok bool) {
	return cpuTime{}, cpuTime{}, false
}
