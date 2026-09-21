// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"slices"
	"strings"
	"time"
)

// What a segment is, and what it is allowed to know.
//
// The rule, which is where the speed comes from: a segment may read the
// context it is given, shell and environment variables, and files. It may not
// fork, may not dial, and may not block. Anything needing a subprocess or a
// network round trip arrives pre-computed and possibly stale, or does not
// arrive.
//
// That is the whole difference between a prompt that is drawn and a prompt
// that is computed. A segment running `node --version` has moved a process
// spawn onto the path between pressing return and seeing a prompt; reading the
// nearest pin file answers a different and usually better question — what the
// project is pinned to, rather than what happens to be on PATH.

// Context is the moment a prompt is being drawn for.
//
// A segment reads this and nothing else about the session, which is what makes
// a rendered prompt reproducible: the fidelity harness pins every field here
// on both sides of a comparison, and any fact a segment could reach around
// this struct would be a fact the harness could not pin.
type Context struct {
	// Continued reports whether this is the prompt for the rest of an
	// unfinished construct rather than for a new command.
	Continued bool

	// Dir is the shell's working directory, and PrevDir is what it was when
	// the previous prompt was drawn. The second is what tells a transient
	// prompt whether the last command moved.
	Dir     string
	PrevDir string

	// Home, User and Host are who and where this session is.
	Home string
	User string
	Host string

	// Status is what the last command exited with, Duration is how long it
	// took, and Jobs is how many the shell is still looking after.
	Status   int
	Duration time.Duration
	Jobs     int

	// Columns is the terminal's width, or zero when it is not known. Zero is
	// not a width: a layout that divides by it, or fills to it, is broken in
	// a way an unknown width is not, so the layout declines to place anything
	// that needs it rather than guessing at 80.
	Columns int

	// Root reports whether this shell is running as the superuser, and Remote
	// whether the session arrived over a network.
	Root   bool
	Remote bool

	// Now is the render clock. One value for the whole render, so two
	// segments drawing a time cannot disagree by the microsecond between
	// them.
	Now time.Time

	// Var reads a shell variable. The session's own, not the process's, for
	// the reason the settings themselves are read that way.
	Var func(name string) (string, bool)

	// memo is what an upward walk for a marker file found, for the life of
	// one render. A dozen version-manager segments each walking
	// independently is a hundred stats per prompt; one render is the memo's
	// whole lifetime, so nothing is ever served stale.
	memo map[string]string
}

// Memo returns what compute answered for key, computing it at most once per
// render.
func (c *Context) Memo(key string, compute func() string) string {
	if c.memo == nil {
		c.memo = map[string]string{}
	}
	if answer, ok := c.memo[key]; ok {
		return answer
	}
	answer := compute()
	c.memo[key] = answer
	return answer
}

// Variable reads a shell variable through the context's lookup, or empty when
// there is none.
func (c *Context) Variable(name string) string {
	if c.Var == nil {
		return ""
	}
	text, _ := c.Var(name)
	return text
}

// Rendered is what a segment produced.
type Rendered struct {
	// Content is the segment's text, in the value markup — so a segment may
	// color a piece of itself without the layout having to know which piece.
	Content string

	// Icon names an entry in the icon table. A segment that hardcoded its
	// glyph would make an icon table a lie and a global icon override have
	// nothing to default to, so a segment names one and never draws one.
	Icon string

	// State selects the middle step of the lookup chain: a directory that is
	// not writable and one that is are the same segment in two states, and
	// each takes its own colors.
	State string

	// Fields are the pieces the segment computed, reachable from the content
	// template as ${NAME}.
	//
	// The template is the configuration's and not the segment's, so a segment
	// that computed two things — a user and a host, a path and its last
	// component — has to offer both or the template can only ever draw the
	// arrangement the segment chose. CONTENT, ICON and STATE are provided by
	// the layout and win over anything a segment puts here, so a segment
	// cannot redefine what those three mean.
	//
	// Values are substituted and never expanded, exactly as Content is: a
	// directory holding a percent sign is drawn rather than read.
	Fields map[string]string
}

// Segment computes one element of a prompt.
//
// A segment may decline, and layout is a separate pass over whatever
// survived — which is why an absent tool costs no space rather than an empty
// box.
type Segment interface {
	Render(settings *Settings, ctx *Context) (Rendered, bool)
}

// SegmentFunc adapts a function to Segment.
type SegmentFunc func(*Settings, *Context) (Rendered, bool)

// Render calls f.
func (f SegmentFunc) Render(settings *Settings, ctx *Context) (Rendered, bool) {
	return f(settings, ctx)
}

// Roster resolves an element name to the segment that draws it.
//
// The segments compiled in are the ones common enough that everyone would
// otherwise write them. They are not a privileged class: a compiled-in segment
// may use no fact and no capability that a segment arriving from outside the
// binary cannot also have. The moment one needs a private interface, the
// extension path has become second-class.
type Roster struct {
	compiled map[string]Segment
	outside  []Resolver
	unknown  []string
	shadowed []string
}

// Resolver finds a segment that is not compiled in — one defined as a shell
// function in the session, or declared by a plugin.
type Resolver interface {
	// Name says where this resolver's segments come from, for the report.
	Name() string

	// Resolve answers an element name, or declines.
	Resolve(element string) (Segment, bool)
}

// NewRoster returns a roster holding the compiled-in segments.
func NewRoster() *Roster {
	return &Roster{compiled: map[string]Segment{}}
}

// Compile registers a compiled-in segment.
func (r *Roster) Compile(element string, segment Segment) {
	r.compiled[strings.ToLower(element)] = segment
}

// Consult adds a resolver ahead of the compiled-in segments.
//
// The order a name is resolved in is most local first: a resolver added later
// is asked earlier, because the person who defined a segment in their own
// session is the most present author of it.
func (r *Roster) Consult(resolver Resolver) {
	r.outside = append(r.outside, resolver)
}

// Resolve answers an element name, and records the ones nothing answered.
//
// A configured element with no segment renders nothing and is named under
// "not yet" by the report. The person configured something, the tool accepted
// it, and the prompt quietly did something else is the failure this
// repository treats as its worst, so the third state — set and ignored — is
// the one that must not be invisible.
func (r *Roster) Resolve(element string) (Segment, bool) {
	name := strings.ToLower(strings.TrimSpace(element))
	for i := len(r.outside) - 1; i >= 0; i-- {
		segment, ok := r.outside[i].Resolve(name)
		if !ok {
			continue
		}
		if _, also := r.compiled[name]; also && !slices.Contains(r.shadowed, name) {
			// A collision is named rather than quietly resolved. Somebody
			// whose segment stopped drawing because a release added a
			// built-in of the same name has been silently overruled by their
			// own shell, and the reverse — a built-in that stopped drawing
			// because a startup file defined a function — is the same
			// surprise from the other side. The local one still wins; what
			// changes is that it is said out loud.
			r.shadowed = append(r.shadowed, r.outside[i].Name()+" "+name)
		}
		return segment, true
	}
	if segment, ok := r.compiled[name]; ok {
		return segment, true
	}
	if !slices.Contains(r.unknown, name) {
		r.unknown = append(r.unknown, name)
	}
	return nil, false
}

// NotYet names every element a configuration asked for that nothing answered,
// in the order they were first asked for.
func (r *Roster) NotYet() []string { return slices.Clone(r.unknown) }

// Shadowed names every element an outside resolver answered that a
// compiled-in segment also answers, as the resolver's own name and the
// element.
func (r *Roster) Shadowed() []string { return slices.Clone(r.shadowed) }
