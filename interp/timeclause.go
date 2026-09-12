// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/syntax"
)

// This file runs `time [-p] pipeline`, the reserved word. The measuring is
// the core's — real from the wall clock, user and sys from what the process
// can truthfully know — and every shape the report takes is a dialect's,
// which is why the layouts live on [Diagnostics] beside the `times` ones.
//
// What can be truthfully known is worth stating, because it is not what a
// forking shell knows. A real shell forks every element, waits, and reads the
// children's rusage; our builtins and compound commands run in this process,
// so their CPU appears in the *shell's* own rusage instead. The default
// report therefore sums both deltas — everything the pipeline spent, wherever
// it ran — and the per-command layout reports the externals each element ran,
// staying silent for an element that ran none, which parallels the shell it
// was measured from: zsh prints no line for an element that did not fork.

// TimeLayout is how a dialect arranges the `time` keyword's report.
type TimeLayout int

const (
	// TimeRealUserSys is a blank line, then `real`, `user` and `sys` lines,
	// tab-separated, in the minutes-and-seconds form. bash and ksh93, apart
	// only in decimals, and the substrate's own.
	TimeRealUserSys TimeLayout = iota
	// TimePerCommand is one line per pipeline element, labeled with the
	// element as written: `wc -l  0.00s user 0.00s system 63% cpu 0.002
	// total`. zsh alone — and only for an element that forked, which here
	// means an element that ran an external command.
	TimePerCommand
)

// TimeBareLayout is what a `time` with no pipeline reports. Three shells,
// three answers, and none of them is an error.
type TimeBareLayout int

const (
	// TimeBareTimesNothing reports a run of nothing: near-zero real, user
	// and sys, in the dialect's ordinary layout. bash, and the substrate's
	// own.
	TimeBareTimesNothing TimeBareLayout = iota
	// TimeBareShellUserSys reports the shell's own accumulated user and sys
	// — two labeled lines, no real, no leading blank line. ksh93 alone.
	TimeBareShellUserSys
	// TimeBareShellAndChildren reports a `shell` and a `children` line in
	// the per-command shape. zsh alone.
	TimeBareShellAndChildren
)

// cpuAccum sums the CPU the external commands of one pipeline element used.
// A mutex rather than a pair of atomics because a process substitution or a
// nested pipeline inside the element can finish on its own goroutine.
type cpuAccum struct {
	mu          sync.Mutex
	user, sys   time.Duration
	sawExternal bool
}

func (a *cpuAccum) add(user, sys time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sawExternal = true
	a.user += user
	a.sys += sys
}

// elemTiming is one pipeline element's figures for the per-command layout.
type elemTiming struct {
	// text is the element as the printer writes it, which is what labels
	// the line — the shell measured labels it with the element as written,
	// redirections included.
	text string
	// wall is written by the goroutine that ran the element and read after
	// the pipeline has been waited for, so it needs no lock.
	wall time.Duration
	cpu  cpuAccum
}

// pipelineTiming is the collection for one timed pipeline, one slot per
// element, allocated before anything runs so the goroutines never resize it.
type pipelineTiming struct {
	elems []elemTiming
}

func (t *pipelineTiming) grow(cmds []syntax.Command) {
	t.elems = make([]elemTiming, len(cmds))
	for i, c := range cmds {
		t.elems[i].text = syntax.PrintCommand(c)
	}
}

// timeClause runs the pipeline and reports to the shell's own standard error
// — the stream as it stands *here*, outside the pipeline, which is the whole
// of the measured fact: `time true 2>&1 | wc -l` counts nothing, because the
// redirection belongs to an element and the report does not.
func (r *Runner) timeClause(ctx context.Context, tc *syntax.TimeClause) error {
	d := r.diag()
	if tc.Pipeline == nil && d.TimeBare != TimeBareTimesNothing {
		// A dialect that answers a bare `time` with accumulated state
		// rather than with a run of nothing. The run-of-nothing answer
		// falls through: it is the ordinary report over an empty interval.
		r.reportBareTime(d)
		r.status = 0
		if tc.Negated {
			r.status = 1
		}
		return nil
	}

	selfBefore, childrenBefore, okBefore := processTimes()
	var timing *pipelineTiming
	if d.TimeLayout == TimePerCommand {
		timing = &pipelineTiming{}
	}
	start := time.Now()
	if tc.Pipeline != nil {
		r.timedPipeline = timing
		err := r.expr(ctx, tc.Pipeline)
		r.timedPipeline = nil
		if err != nil {
			return err
		}
	} else {
		r.status = 0
	}
	elapsed := time.Since(start)
	selfAfter, childrenAfter, okAfter := processTimes()

	if !okBefore || !okAfter {
		// No rusage on this platform. Refused rather than reported as
		// zeros, the same line `times` takes — but only the report is
		// refused: the pipeline has already run and keeps its status.
		r.diagf("time: not available on this platform\n")
	} else {
		user := (selfAfter.user - selfBefore.user) + (childrenAfter.user - childrenBefore.user)
		sys := (selfAfter.system - selfBefore.system) + (childrenAfter.system - childrenBefore.system)
		r.reportTime(d, tc.Posix, elapsed, user, sys, timing)
	}
	if tc.Negated {
		// `! time x` negates the timed pipeline's status, exactly as the
		// pipeline's own `!` would — measured, both spellings report and
		// both carry the inverted status.
		if r.status == 0 {
			r.status = 1
		} else {
			r.status = 0
		}
	}
	return nil
}

// reportTime writes the report in the dialect's layout — or in the POSIX one,
// which `-p` selects identically in both shells that read the flag.
func (r *Runner) reportTime(d Diagnostics, posix bool, elapsed time.Duration, user, sys time.Duration, timing *pipelineTiming) {
	var b strings.Builder
	if format, ok := r.timeFormat(); ok && !posix {
		// A format the script named. `-p` is not one of the things it may
		// override: measured, `TIMEFORMAT='X %R'; time -p sleep 0` prints
		// the POSIX three lines in bash and ksh93 alike, so the flag is
		// asked first.
		text, usable := r.timeFormatReport(format, elapsed, user, sys)
		if !usable {
			return
		}
		if text != "" {
			text += "\n"
		}
		_, _ = fmt.Fprint(r.stderr(), text)
		return
	}
	switch {
	case posix:
		// `real 0.00`, one space, two decimals, no leading blank line:
		// bash and ksh93 byte for byte.
		fmt.Fprintf(&b, "real %.2f\nuser %.2f\nsys %.2f\n",
			elapsed.Seconds(), user.Seconds(), sys.Seconds())
	case d.TimeLayout == TimePerCommand && timing != nil:
		for i := range timing.elems {
			e := &timing.elems[i]
			if !e.cpu.sawExternal {
				// Nothing forked for this element, and the shell this
				// layout was measured from prints nothing for one of
				// those — a lone builtin reports nothing at all.
				continue
			}
			b.WriteString(perCommandLine(e.text, e.cpu.user, e.cpu.sys, e.wall))
		}
	default:
		dec := d.timeDecimals()
		fmt.Fprintf(&b, "\nreal\t%s\nuser\t%s\nsys\t%s\n",
			clockTime(elapsed, dec), clockTime(user, dec), clockTime(sys, dec))
	}
	_, _ = fmt.Fprint(r.stderr(), b.String())
}

// reportBareTime answers a bare `time` for the dialects whose answer is
// accumulated state rather than a run of nothing.
func (r *Runner) reportBareTime(d Diagnostics) {
	self, children, ok := processTimes()
	if !ok {
		r.diagf("time: not available on this platform\n")
		return
	}
	var b strings.Builder
	switch d.TimeBare {
	case TimeBareShellUserSys:
		// The shell's own user and sys, labeled, no real and no leading
		// blank line.
		dec := d.timeDecimals()
		fmt.Fprintf(&b, "user\t%s\nsys\t%s\n", clockTime(self.user, dec), clockTime(self.system, dec))
	case TimeBareShellAndChildren:
		// The shell's and its children's accumulated CPU in the
		// per-command shape. The measured shell divides by its own age;
		// a Runner has no age, so the elapsed column reports the only
		// interval this construct has — its own, which is next to none.
		b.WriteString(perCommandLine("shell", self.user, self.system, 0))
		b.WriteString(perCommandLine("children", children.user, children.system, 0))
	}
	_, _ = fmt.Fprint(r.stderr(), b.String())
}

// perCommandLine is one line of the per-command layout: the element as
// written, then user, sys, a CPU percentage and the elapsed total.
func perCommandLine(text string, user, sys, wall time.Duration) string {
	pct := 0
	if wall > 0 {
		pct = int(float64(user+sys) / float64(wall) * 100)
	}
	return fmt.Sprintf("%s  %.2fs user %.2fs system %d%% cpu %.3f total\n",
		text, user.Seconds(), sys.Seconds(), pct, wall.Seconds())
}

// addChildTime bills a finished external command to the pipeline element it
// ran under, when a per-element `time` report is collecting. The figures are
// the process's own, from the wait that reaped it.
func (r *Runner) addChildTime(ps *os.ProcessState) {
	if r.elemCPU == nil || ps == nil {
		return
	}
	r.elemCPU.add(ps.UserTime(), ps.SystemTime())
}
