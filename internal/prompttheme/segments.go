// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The roster of compiled-in segments — the ones common enough that everyone
// would otherwise write them.
//
// They are not a privileged class. Every one of them reads the Context, the
// settings and nothing else, which is the constraint the spec puts on the
// extension path: a compiled-in segment may use no fact and no capability a
// segment arriving from outside the binary cannot also have. The moment one
// needs a private interface the extension path has become second-class, so
// the test that matters for this file is the one that drives a segment
// through the same Segment interface a shell function is reached through.
//
// None of them forks, dials or blocks. Each is arithmetic over the Context,
// a variable lookup, or — for the directory's writability — one stat-shaped
// syscall about a path the shell is already standing in.

// CoreSegments returns the compiled-in roster, by element name.
//
// prompt_char and the time segment are not here because each needs something
// from the caller — the character a shell prompts with, and a date formatter
// this package will not own a second copy of. They are registered beside
// these by whoever wires the engine.
func CoreSegments() map[string]Segment {
	return map[string]Segment{
		"dir":                    SegmentFunc(dirSegment),
		"status":                 SegmentFunc(statusSegment),
		"command_execution_time": SegmentFunc(durationSegment),
		"background_jobs":        SegmentFunc(jobsSegment),
		"context":                SegmentFunc(contextSegment),
	}
}

// dirSegment draws where the shell is.
//
// The home directory collapses to `~` because that is what every shell's own
// prompt does with it and because the alternative is a segment whose width is
// dominated by a constant. Truncation is off until a configuration asks for
// it: a prompt that silently drops the middle of a path is a prompt that can
// show two different directories identically, and that is a decision for the
// person whose screen it is.
func dirSegment(settings *Settings, ctx *Context) (Rendered, bool) {
	full := ctx.Dir
	if full == "" {
		// Nothing to draw and nothing to guess at. A shell whose working
		// directory is unknown has a bigger problem than its prompt.
		return Rendered{}, false
	}

	shown := full
	home := ctx.Home
	atHome := false
	if home != "" && home != "/" {
		switch {
		case full == home:
			shown, atHome = "~", true
		case strings.HasPrefix(full, home+"/"):
			shown = "~" + full[len(home):]
		}
	}
	shown = truncatePath(shown, settings.ParamInt("dir", "", "MAX_DEPTH", 0),
		settings.Param("dir", "", "TRUNCATION", "…"))

	state := ""
	if writable, known := writability(full); known && !writable {
		// The spec's own headline example of the three-step chain is
		// DIR_NOT_WRITABLE_FOREGROUND, and this is the state it names.
		// Unknown is not "writable": a platform that cannot answer draws the
		// plain state rather than claiming either way.
		state = "not_writable"
	}

	icon := "DIR"
	if atHome {
		icon = "DIR_HOME"
	}
	return Rendered{
		Content: shown,
		Icon:    icon,
		State:   state,
		Fields: map[string]string{
			"PATH": shown,
			"FULL": full,
			"LAST": filepath.Base(full),
		},
	}, true
}

// truncatePath keeps the last depth components and replaces what it dropped
// with one marker. A depth of zero is off, which is the default.
func truncatePath(path string, depth int, marker string) string {
	if depth <= 0 {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) <= depth {
		return path
	}
	return marker + "/" + strings.Join(parts[len(parts)-depth:], "/")
}

// statusSegment draws what the last command exited with.
//
// It declines after a success unless asked, because the ordinary case is
// success and a prompt that says so on every line is a prompt that says
// nothing on every line. The failure is never declined: a command that failed
// silently is the thing this segment exists to stop.
func statusSegment(settings *Settings, ctx *Context) (Rendered, bool) {
	state, icon := "ok", "STATUS_OK"
	if ctx.Status != 0 {
		state, icon = "error", "STATUS_ERROR"
	}
	if state == "ok" && !settings.ParamBool("status", "", "OK", false) {
		return Rendered{}, false
	}
	code := strconv.Itoa(ctx.Status)
	content := code
	if state == "ok" {
		// On success the number is always zero, so drawing it says nothing
		// the icon does not. The field is still there for a content template
		// that wants it.
		content = ""
	}
	return Rendered{
		Content: content,
		Icon:    icon,
		State:   state,
		Fields:  map[string]string{"CODE": code},
	}, true
}

// durationSegment draws how long the last command took, once it took long
// enough to be worth saying.
//
// The threshold is the whole of the design. Without one this segment draws a
// number on every line that nobody reads; with one it draws only where a
// person would have wondered, which is the question "did that really take
// that long" and nothing else.
func durationSegment(settings *Settings, ctx *Context) (Rendered, bool) {
	threshold := settings.ParamInt("command_execution_time", "", "THRESHOLD", 3)
	if ctx.Duration <= 0 {
		return Rendered{}, false
	}
	if threshold > 0 && ctx.Duration < time.Duration(threshold)*time.Second {
		return Rendered{}, false
	}
	precision := settings.ParamInt("command_execution_time", "", "PRECISION", 0)
	return Rendered{
		Content: formatDuration(ctx.Duration, precision),
		Icon:    "COMMAND_EXECUTION_TIME",
		Fields: map[string]string{
			"SECONDS": strconv.FormatFloat(ctx.Duration.Seconds(), 'f', max(precision, 0), 64),
		},
	}, true
}

// formatDuration writes a duration the way a person reads one: the largest
// unit that is not zero, and everything below it.
//
// Leading zero units are dropped rather than padded, because `0d 0h 2m 3s` is
// four fields of which two are noise, and a prompt is the one place where a
// field that is always zero is worth removing.
func formatDuration(d time.Duration, precision int) string {
	precision = min(max(precision, 0), 9)
	total := d.Seconds()
	days := int(total) / 86400
	hours := int(total) % 86400 / 3600
	minutes := int(total) % 3600 / 60
	seconds := total - float64(int(total)/60*60)

	var parts []string
	if days > 0 {
		parts = append(parts, strconv.Itoa(days)+"d")
	}
	if days > 0 || hours > 0 {
		parts = append(parts, strconv.Itoa(hours)+"h")
	}
	if days > 0 || hours > 0 || minutes > 0 {
		parts = append(parts, strconv.Itoa(minutes)+"m")
	}
	return strings.Join(append(parts, strconv.FormatFloat(seconds, 'f', precision, 64)+"s"), " ")
}

// jobsSegment draws how many jobs the shell is still looking after.
//
// It declines when there are none, which is the common case, and can be asked
// to draw a zero by a configuration that would rather the prompt keep the same
// shape from line to line.
func jobsSegment(settings *Settings, ctx *Context) (Rendered, bool) {
	if ctx.Jobs <= 0 && !settings.ParamBool("background_jobs", "", "ALWAYS", false) {
		return Rendered{}, false
	}
	count := strconv.Itoa(ctx.Jobs)
	return Rendered{
		Content: count,
		Icon:    "BACKGROUND_JOBS",
		Fields:  map[string]string{"COUNT": count},
	}, true
}

// contextSegment draws who and where this session is.
//
// It declines on an ordinary local session, which is the one case where the
// answer is the same every day and the person already knows it. Root and a
// session that arrived over a network are the two cases where not knowing is
// how a command ends up run in the wrong place, so those draw by default and
// carry their own state for their own colors.
func contextSegment(settings *Settings, ctx *Context) (Rendered, bool) {
	state := ""
	switch {
	case ctx.Root:
		state = "root"
	case ctx.Remote:
		state = "remote"
	}
	if state == "" && !settings.ParamBool("context", "", "ALWAYS", false) {
		return Rendered{}, false
	}
	user, host := ctx.User, ctx.Host
	content := user + "@" + host
	switch {
	case user == "" && host == "":
		return Rendered{}, false
	case user == "":
		content = host
	case host == "":
		content = user
	}
	return Rendered{
		Content: content,
		Icon:    "CONTEXT",
		State:   state,
		Fields:  map[string]string{"USER": user, "HOST": host},
	}, true
}

// Clock draws the time of day.
//
// The format language is the caller's, for the reason PromptChar's character
// is: this tree already has one strftime, written from POSIX and checked
// against a live shell, and a second reader of the same format language would
// drift the first time a conversion was fixed in one of them. So the engine
// takes the formatter rather than owning a copy.
func Clock(format func(layout string, t time.Time) string) Segment {
	return SegmentFunc(func(settings *Settings, ctx *Context) (Rendered, bool) {
		layout := settings.Param("time", "", "FORMAT", "%H:%M:%S")
		if format == nil || layout == "" {
			return Rendered{}, false
		}
		return Rendered{Content: format(layout, ctx.Now), Icon: "TIME"}, true
	})
}
