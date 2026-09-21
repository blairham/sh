// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/blairham/sh/repl"
)

// The segment role: a prompt segment whose work happens in another process.
//
// It is the third role and, per #1315, the last — no further provider roles
// are coming. docs/spec/prompt-theme.md asked for it and said it wanted its
// own issue when it was picked up; this is that, built on the contract #3980
// landed rather than on a bespoke one.
//
// # How it clears the hot-path exclusion
//
// docs/design/plugins.md excludes anything on the path between pressing
// return and seeing a prompt, and the exclusion is not argued with here. It
// is answered by construction: **a plugin segment is never asked a question
// the shell waits on.** The host *tells* the plugin what the prompt is being
// drawn for, as a notification, and the plugin publishes an answer whenever
// it has one. The prompt draws whatever is already held — which before a
// first answer is nothing, and an element that draws nothing costs no space
// — and is redrawn in place when something arrives.
//
// So the boundary is not on that path at all, ever, and the latency of a
// slow or wedged plugin is bounded by nothing because nothing is waiting for
// it. A plugin that dies loses its segments and they draw nothing; the
// host's existing failure reporting says so once, and the prompt does not
// report a dead plugin on every line.
//
// That is the same difference the observer role turns on, from the other
// side. Gate.Allow returns a Decision and has no third answer available;
// Sink.Emit returns nothing; and a segment is not even called — it is a
// value the host is holding. Each role is remotable exactly to the degree
// that nothing is waiting for its answer.
//
// # What is bounded, and what deliberately is not
//
// **At most one context is ever waiting.** A newer one replaces an older one
// rather than queueing behind it, which is not a weaker version of the
// observer role's bounded buffer but an answer to a different question: an
// event is a record and losing one leaves a gap the schema can say, while a
// context is a *state* and only the newest one means anything. A plugin that
// is behind is told the current state when it catches up, and the prompt
// after this one says the same thing again.
//
// **How long a plugin takes to answer is not bounded**, deliberately. That
// is the point of it not being asked. A deadline here would report something
// other than what happened — #493's rule, which promptprovider.go and
// repl.Completer both already state — and there is nothing to abandon,
// because nothing was requested.

// segments is one plugin's segment role.
//
// Safe from every goroutine that touches it, which is three: the session's,
// drawing a prompt and reading latest; the read loop, delivering what the
// plugin published; and the feed below, writing contexts out.
type segments struct {
	h *Host

	// names is what the plugin declared, fixed for its life. Written once
	// during the handshake, before Launch returns and therefore before
	// anything else can hold this host, and only read afterwards.
	names []string

	mu sync.Mutex
	// latest is what each segment now holds. A name the plugin has not
	// published is absent, which is what a segment with no answer yet looks
	// like — there is no placeholder here and there must not be one, because
	// inventing one would be this implementation's taste presented as
	// behavior.
	latest map[string]promptSegmentParams
	// pending is the context waiting to go out, or nil. One, not a queue:
	// see the bound above.
	pending *promptContextParams
	// notify is how the session is told a redraw would differ, or nil
	// between sessions. A publish with no session listening is an ordinary
	// outcome and not an error: a plugin that answers after the shell has
	// gone is a plugin that was never told to stop.
	notify func()

	// wake carries "there is a context to send". One slot and a
	// non-blocking send, which is the same shape repl's own wake pipe has
	// and for the same reason: what the reader wants is "something happened
	// since I last looked" and not a count.
	// said is what has already been relayed about this plugin's segments, so
	// a complaint about a publish that happens every prompt is made once.
	said map[string]bool

	wake    chan struct{}
	closing chan struct{}
	done    chan struct{}
}

func newSegments(h *Host, names []string) *segments {
	return &segments{
		h:       h,
		names:   names,
		latest:  map[string]promptSegmentParams{},
		wake:    make(chan struct{}, 1),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Name is what `prompt show` prints beside an element this plugin draws, and
// what a collision is reported as. The plugin's own declared name, because a
// person whose segment stopped drawing is owed the name of what claimed it.
func (s *segments) Name() string { return "plugin " + s.h.Name() }

// PromptSegments is every element this plugin declared.
func (s *segments) PromptSegments() []string { return append([]string(nil), s.names...) }

// PromptSegment is what the plugin has for an element right now.
//
// Immediately, from memory, with nothing asked of the plugin. This runs on
// the path to drawing a prompt, and the whole role rests on nothing here
// reaching the process.
func (s *segments) PromptSegment(element string) (repl.PromptSegment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, ok := s.latest[strings.ToLower(element)]
	if !ok {
		return repl.PromptSegment{}, false
	}
	out := repl.PromptSegment{Content: held.Content, Icon: held.Icon, State: held.State}
	if len(held.Fields) > 0 {
		out.Fields = make(map[string]string, len(held.Fields))
		for k, v := range held.Fields {
			out.Fields[k] = v
		}
	}
	return out, true
}

// PromptContext says what the next prompt is being drawn for.
//
// It returns immediately whatever the plugin is doing. The write to the
// plugin's standard input happens on the feed goroutine, which is the same
// arrangement the observer role has and for the same reason: a plugin that
// lets that stream back up had already stopped serving the protocol, and the
// host's answer to that is Close taking the stream away rather than the
// shell waiting.
func (s *segments) PromptContext(info repl.PromptInfo) {
	p := promptContextParams{
		Dir:        info.Dir,
		PrevDir:    info.PrevDir,
		Status:     info.Status,
		DurationMS: info.Duration.Milliseconds(),
		Jobs:       info.Jobs,
		Columns:    info.Columns,
		Root:       info.Root,
		Remote:     info.Remote,
		Continued:  info.Continued,
	}
	s.mu.Lock()
	s.pending = &p
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
		// A wake is already up and the context it will carry is the one just
		// stored. That is the whole of the bound.
	}
}

// PublishPrompt takes the session's wake, and gives it back at the end.
func (s *segments) PublishPrompt(notify func()) {
	s.mu.Lock()
	s.notify = notify
	s.mu.Unlock()
}

// promptSegment takes what a plugin published.
//
// Nothing is answered, because a notification has nobody to answer. What a
// plugin can get wrong is therefore said through the relay — the same route
// every other plugin diagnostic takes — and said **once per name**: a
// publish happens every prompt, and a complaint repeated every prompt is a
// broken shell rather than a report.
func (h *Host) promptSegment(params json.RawMessage) {
	if h.segs == nil {
		// A plugin publishing a segment it never declared the role for. Said
		// out loud rather than dropped, because from the plugin author's
		// side a segment that silently never draws is indistinguishable from
		// a shell that does not have the feature.
		h.noRoleOnce.Do(func() {
			h.say("it published a prompt segment without declaring the segment role")
		})
		return
	}
	var p promptSegmentParams
	if err := json.Unmarshal(params, &p); err != nil {
		h.say(MethodPromptSegment + ": " + err.Error())
		return
	}
	if !h.segs.declared(p.Name) {
		// A name outside the declaration. Refused rather than accepted,
		// because the declaration is what makes a collision reportable: an
		// element that appeared partway through a session would make a
		// prompt's shape depend on what had already run, and two sources
		// claiming one name could never be told apart.
		h.segs.sayOnce(MethodPromptSegment + ": " + p.Name +
			" is not one of the segments it declared")
		return
	}
	h.segs.published(p)
}

// sayOnce relays a sentence the first time it is said, for a plugin that
// would otherwise say it on every prompt.
func (s *segments) sayOnce(text string) {
	s.mu.Lock()
	if s.said == nil {
		s.said = map[string]bool{}
	}
	already := s.said[text]
	s.said[text] = true
	s.mu.Unlock()
	if !already {
		s.h.say(text)
	}
}

// declared reports whether a name is one the plugin stated at the handshake.
func (s *segments) declared(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, declared := range s.names {
		if declared == lower {
			return true
		}
	}
	return false
}

// published records what the plugin said and tells the session a redraw
// would differ.
//
// **Only when it would.** A plugin that republishes the same answer costs
// nothing at all, which matters because the obvious way to write one is a
// poll: the comparison is here rather than in the session because this is
// the only place that holds both the old answer and the new one. The session
// makes the same comparison one level up over the whole rendered prompt, and
// two cheap comparisons are what keep a chatty plugin from being a
// flickering one.
func (s *segments) published(p promptSegmentParams) {
	name := strings.ToLower(strings.TrimSpace(p.Name))
	s.mu.Lock()
	held, had := s.latest[name]
	same := had && sameSegment(held, p)
	if !same {
		p.Name = name
		s.latest[name] = p
	}
	notify := s.notify
	s.mu.Unlock()
	if same || notify == nil {
		return
	}
	notify()
}

// sameSegment reports whether two answers would draw identically.
func sameSegment(a, b promptSegmentParams) bool {
	if a.Content != b.Content || a.Icon != b.Icon || a.State != b.State {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for k, v := range a.Fields {
		if other, ok := b.Fields[k]; !ok || other != v {
			return false
		}
	}
	return true
}

// feed puts contexts on the wire, on a goroutine the session is not running
// on.
//
// It exists so that PromptContext does not, and everything about it follows
// from that — the marshaling and the write happen here.
func (s *segments) feed() {
	defer close(s.done)
	for {
		select {
		case <-s.wake:
			s.mu.Lock()
			p := s.pending
			s.pending = nil
			s.mu.Unlock()
			if p == nil {
				continue
			}
			if err := s.h.conn.Notify(MethodPromptContext, *p); err != nil {
				// The stream is gone, which usually means the plugin is. Not
				// reported as its death: die keeps the first reason for the
				// reason it keeps it.
				s.h.die(err)
				return
			}
		case <-s.closing:
			// Nothing is drained here, and that is the difference from the
			// observer role's shutdown. A record is something the plugin is
			// owed; a context is a statement about a prompt that is not
			// going to be drawn again, so delivering it at shutdown would be
			// telling a plugin about a shell that has gone.
			return
		}
	}
}

// stop tells the feed to end, and does not wait.
//
// The waiting is the caller's and happens after the plugin's input is
// closed — see Host.Close, where the reason is written down: a feed parked
// inside a write to a plugin that stopped reading is unblocked by the stream
// going and by nothing else, so a wait before that would be a wait for an
// event nothing had caused.
func (s *segments) stop() {
	select {
	case <-s.closing:
	default:
		close(s.closing)
	}
}

// PromptSegments is the repl.PromptSegmentSource this plugin's segment role
// supplies, or nil for a plugin that did not declare one.
//
// Nil rather than an empty source, and it is the same typed-nil care Sink
// needs: returning s.segs directly when it is nil would hand back an
// interface holding a nil pointer, which is not nil, and every caller's
// `if source != nil` would be wrong.
func (h *Host) PromptSegments() repl.PromptSegmentSource {
	if h.segs == nil {
		return nil
	}
	return h.segs
}
