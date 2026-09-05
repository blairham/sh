// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// This file implements `times`, the last of the four builtins specialBuiltins
// called special before any of them existed. `eval`, `.` and `exec` closed the
// other three; with this the table stops asserting anything untrue.
//
// It is also the most divergent builtin in the panel for its size. Four shells,
// four different renderings of the same two facts.

func init() {
	builtins["times"] = biTimes
}

// cpuTime is the pair every one of these reports, however it lays them out.
type cpuTime struct{ user, system time.Duration }

// TimesLayout is how a dialect arranges what `times` prints.
//
// It is a type rather than an Answer because the two shapes are not two
// answers to one question — they carry different *amounts* of information, and
// a boolean would hide that.
type TimesLayout int

const (
	// TimesSelfAndChildren is two lines of two figures: the shell's own user
	// and system time, then its children's. dash, bash and zsh, and the
	// substrate's own.
	TimesSelfAndChildren TimesLayout = iota
	// TimesUserAndSystem is two *labeled* lines, `user` and `sys`, each with
	// one figure and separated by a tab — and it reports only the shell's own
	// time. ksh93 alone, and it is genuinely less information rather than the
	// same information differently arranged.
	TimesUserAndSystem
)

// biTimes prints accumulated CPU time.
//
// Unanimous: it goes to standard output, not to standard error, and reports 0.
// An earlier probe suggested zsh used stderr; writing each stream to its own
// file rather than through a pipeline showed all four agree, and the pipeline
// had been the thing measured.
func biTimes(r *Runner, _ context.Context, args []string) int {
	if len(args) > 0 {
		// dash and bash ignore what they are given; zsh refuses it. ksh93
		// makes it a *syntax* error — `times` is a reserved word there, so the
		// argument never reaches a builtin at all, which is a grammar question
		// and not this one. That answer is measured and recorded in the corpus
		// rather than implemented here.
		if r.ask(r.sem().TimesRejectsArguments, "`times` refusing arguments") {
			r.diagf("%s\n", Wording(r.diag().TimesArguments, "times: too many arguments"))
			return 1
		}
	}

	self, children, ok := processTimes()
	if !ok {
		// No getrusage on this platform. Refused rather than answered with
		// zeros, which would be four plausible numbers meaning nothing — the
		// same line procgroup_other takes for process groups.
		r.diagf("times: not available on this platform\n")
		return 2
	}

	d := r.diag()
	var b strings.Builder
	switch d.TimesLayout {
	case TimesUserAndSystem:
		// One figure per line, labeled, tab-separated — and the children's
		// times are not reported at all.
		fmt.Fprintf(&b, "user\t%s\n", clockTime(self.user, d.timesDecimals()))
		fmt.Fprintf(&b, "sys\t%s\n", clockTime(self.system, d.timesDecimals()))
	default:
		fmt.Fprintf(&b, "%s %s\n", clockTime(self.user, d.timesDecimals()),
			clockTime(self.system, d.timesDecimals()))
		fmt.Fprintf(&b, "%s %s\n", clockTime(children.user, d.timesDecimals()),
			clockTime(children.system, d.timesDecimals()))
	}
	// Through printf, so a failed write is recorded for the dispatcher rather
	// than discarded here — `times >&-` reports it in dash and in both bash
	// builds, and this answered 0 in silence.
	r.printf("%s", b.String())
	return 0
}

// clockTime renders a duration the way every shell in the panel does: whole
// minutes, then seconds with a fixed number of decimals.
//
//	0m0.000s     3 decimals, bash
//	0m0.000000s  6, dash
//	0m0.00s      2, ksh93 and zsh
//
// Minutes are not wrapped at 60: a shell that has burned two hours prints
// `120m`, which is what the format means and what makes the number readable
// without arithmetic.
func clockTime(d time.Duration, decimals int) string {
	if d < 0 {
		d = 0
	}
	minutes := int64(d / time.Minute)
	seconds := (d - time.Duration(minutes)*time.Minute).Seconds()
	return fmt.Sprintf("%dm%.*fs", minutes, decimals, seconds)
}
