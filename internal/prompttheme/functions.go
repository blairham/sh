// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import "strings"

// A segment written as a shell function in the session.
//
// This is the everyday answer to "expandable without ever building the
// shell", and it is a capability no external prompt program has: the
// interpreter is in this process, so a segment's content can be produced by a
// function the person defined in their own startup file, called in-process
// with **no fork**, in whatever dialect that session is running.
//
// The common case for "a segment that does not exist yet" is a few lines of
// shell over a file or a variable, and that should cost a person nothing but
// their own rc.
//
// It is also what keeps the no-fork rule honest. Somebody who genuinely needs
// to run something gets to, deliberately and visibly, instead of the engine
// growing a forking segment for every tool in the world — and what it costs
// is theirs, on their own prompt, rather than everyone's.
//
// Off unless configured: nothing here is consulted for an element no
// configuration named, and a function that is not there is not a segment.

// Functions is a Resolver over the shell functions in a session.
//
// has reports whether a name is a function — not a builtin, not an alias, not
// a command on the path, which is the same rule the interpreter's own
// hook-calling follows — and call runs one and answers what it wrote.
//
// Both are functions rather than a Runner, for the reason the clock's format
// language is: this package holds no interpreter, and a segment reached
// through this interface is reached through the same Segment interface a
// compiled-in one is. That is the constraint the extension path rests on.
func Functions(layer string, has func(name string) bool, call func(name string) (string, bool)) Resolver {
	return functionSegments{layer: layer, has: has, call: call}
}

type functionSegments struct {
	layer string
	has   func(string) bool
	call  func(string) (string, bool)
}

func (f functionSegments) Name() string { return f.layer }

func (f functionSegments) Resolve(element string) (Segment, bool) {
	if f.has == nil || f.call == nil || !f.has(element) {
		return nil, false
	}
	return SegmentFunc(func(_ *Settings, _ *Context) (Rendered, bool) {
		text, ok := f.call(element)
		if !ok {
			// A function that failed or has gone costs its segment and
			// nothing else. A non-zero return and an unset name are ordinary
			// outcomes, not errors to report on a prompt.
			return Rendered{}, false
		}
		text = strings.TrimRight(text, "\n")
		if text == "" {
			// Nothing to say is declining, which is what every other segment
			// does with nothing to say — and it is what makes a conditional
			// segment written in three lines of shell cost no space on the
			// lines it has nothing for.
			return Rendered{}, false
		}
		return Rendered{Content: text}, true
	}), true
}
