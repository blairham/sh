// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package secret finds credentials in text.
//
// It exists because a shell writes two things down that a person did not ask
// it to keep: the lines they typed, in the history file, and — once there is
// somewhere to put it — what those lines printed. Both are ordinary files
// with ordinary permissions, and a token that reaches either one has been
// written to disk by a program the person was only talking to.
//
// The rules are patterns for credentials whose *shape* is public: an AWS key
// id begins with four known letters and is twenty characters long because
// AWS says so, a PEM private key starts with a line that is part of the
// format. None of that is a secret, and none of it is anyone's expression of
// it — the table here is written from those facts and from nothing else.
//
// # Two postures, one table
//
// The callers want different things done with a match, and the difference is
// the whole reason this is a package rather than a function:
//
//   - A *command line* that matches is rejected whole. The credential
//     essentially is the command — `export TOKEN=…` is nothing else — so
//     keeping a redacted skeleton of it saves nobody anything, and dropping
//     the line is proportionate. [Scanner.Match] answers that question.
//
//   - *Output* that matches is redacted, never rejected. Dropping a
//     two-hundred-kilobyte build log because one line of it echoed a token
//     destroys exactly what the person wanted to keep. [Scanner.Redact]
//     replaces the credential and leaves the rest of the text standing.
//
// The table has to be the same table for the two to mean anything together.
// A history that refuses a line whose output would have been kept, or the
// reverse, is a policy nobody can describe in a sentence.
//
// # What it is not
//
// It is not an entropy scanner and does not try to be. A rule that fires on
// "this looks random" fires on a commit hash, a UUID and a base64 image, and
// a scrubber that rejects ordinary lines is worse than no scrubber at all:
// people turn it off, or worse, stop reading what it says. Every rule here
// either names its credential's own prefix or requires the text to say what
// the value is — `password=`, `Authorization:`, a password inside a URL.
//
// The cost of that choice is real and worth stating: a bare secret, pasted
// with nothing around it to say what it is, does not match. There is nothing
// in the text to match on.
package secret

import (
	"regexp"
	"sort"
	"strings"
)

// Placeholder is what [Scanner.Redact] leaves where a credential was.
//
// It says something happened rather than eliding it silently — a log with a
// hole in it where a token used to be is a log someone will read as corrupt
// and go looking for the original of.
const Placeholder = "[redacted]"

// A Scanner matches text against a table of credential rules.
//
// The zero value matches nothing, which is deliberate: a scanner is either
// the default table or one a caller assembled on purpose, and an empty one
// that silently approves everything is not a state to arrive at by accident.
type Scanner struct {
	rules []rule
}

// rule is one credential pattern.
//
// The submatch, where a rule has one, is the credential itself rather than
// the whole match: `password=hunter2` matches entirely and only `hunter2` is
// the secret, and redacting the name along with the value would leave output
// nobody can read. Where a rule has no submatch — a key id, a PEM header —
// the whole match is the credential.
type rule struct {
	name string
	re   *regexp.Regexp
}

// Default is the standard table.
//
// One compiled copy, shared: the patterns are constant, matching does not
// mutate them, and a shell that recompiled a dozen regular expressions per
// line typed would be paying for this feature at the prompt.
func Default() *Scanner { return defaultScanner }

var defaultScanner = newScanner(defaultRules)

func newScanner(table []ruleSource) *Scanner {
	s := &Scanner{rules: make([]rule, 0, len(table))}
	for _, r := range table {
		s.rules = append(s.rules, rule{name: r.name, re: regexp.MustCompile(r.pattern)})
	}
	return s
}

// Match reports the first rule that finds a credential in the text, naming
// it.
//
// The name is for the person, not for the program: a line that vanishes from
// the history with no reason given is indistinguishable from a shell that has
// lost it, and "aws-access-key-id" is also the only way anyone can tell us a
// rule is wrong.
//
// First rather than every rule, because the answer is a yes or a no and the
// remaining rules cannot change it. The table is ordered so that the specific
// rules are asked before the general ones, which makes the name the most
// informative one available.
func (s *Scanner) Match(text string) (string, bool) {
	for _, r := range s.rules {
		if r.re.MatchString(text) {
			return r.name, true
		}
	}
	return "", false
}

// Redact replaces every credential in the text with [Placeholder], returning
// the result and how many it replaced.
//
// Text with nothing in it comes back untouched and unallocated, which is the
// case that happens every time: almost no output holds a credential, and this
// runs over all of it.
func (s *Scanner) Redact(text string) (string, int) {
	spans := s.spans(text)
	if len(spans) == 0 {
		return text, 0
	}
	var b strings.Builder
	b.Grow(len(text))
	prev := 0
	for _, sp := range spans {
		b.WriteString(text[prev:sp.lo])
		b.WriteString(Placeholder)
		prev = sp.hi
	}
	b.WriteString(text[prev:])
	return b.String(), len(spans)
}

// span is a half-open range of the text holding one credential.
type span struct{ lo, hi int }

// spans finds every credential in the text, in order and without overlaps.
//
// Overlaps are not hypothetical: a URL holding a password matches the
// connection-string rule, and the same password matches the generic
// assignment rule if the URL was on the right of one. Two replacements over
// one region would cut the text in half at an index that no longer exists, so
// the wider of the two wins and the other is dropped.
func (s *Scanner) spans(text string) []span {
	var found []span
	for _, r := range s.rules {
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			lo, hi := m[0], m[1]
			// Group one is the credential where a rule isolates it; a rule
			// without one means the whole match.
			if len(m) >= 4 && m[2] >= 0 {
				lo, hi = m[2], m[3]
			}
			found = append(found, span{lo: lo, hi: hi})
		}
	}
	if len(found) < 2 {
		return found
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].lo != found[j].lo {
			return found[i].lo < found[j].lo
		}
		return found[i].hi > found[j].hi
	})
	merged := found[:1]
	for _, sp := range found[1:] {
		last := &merged[len(merged)-1]
		if sp.lo <= last.hi {
			// Touching or overlapping: one replacement covering both, which
			// is also the safe direction to be wrong in.
			last.hi = max(last.hi, sp.hi)
			continue
		}
		merged = append(merged, sp)
	}
	return merged
}
