// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/interp"
)

// The policy file: one rule per line.
//
//	version 1
//	default deny
//
//	# the build reads its own tree and writes only into out/
//	allow exec  /usr/bin/git
//	allow read  /srv/build/**
//	allow write /srv/build/out/**
//	deny  read  /srv/build/.env
//
// JSON was the obvious alternative — encoding/json is in the standard library
// and this module has no runtime dependencies at all — and it lost on three
// counts, all of which are about a policy being a security artifact rather
// than a config file. A policy is defended by a second person reading the
// diff, and one rule per line makes a changed rule exactly one changed line. A
// policy accumulates reasons, and JSON cannot carry a comment without
// inventing a JSON dialect. And every value in this file is a path or a glob,
// so a format whose quoting turns `\` and `"` into escapes is a format in
// which the common case is misread.
//
// The parser we own in exchange has no quoting, no continuation, no nesting
// and no includes, and that is the whole of the bargain: the grammar is in
// docs/design/sandboxing.md and it fits in a paragraph.
//
// Two consequences of "the pattern is the rest of the line":
//
//   - A path containing spaces is written as it stands, with no quoting. This
//     is the case a whitespace-delimited third field gets wrong.
//   - A comment is a whole line. There are no trailing comments, because `#`
//     is a legal character in a path and stripping one would silently narrow
//     a rule about a file whose name contains it.
//
// Every failure is fatal and names the line. There is no "unknown directive
// ignored": an unrecognized word in a security file is a typo, and reading
// past it drops a rule that somebody believed was in force.

// version is the only file format version this parser understands.
//
// The directive is required, and required first. A parser that met a file it
// does not understand has to refuse it rather than read the parts it
// recognizes — a policy half-understood is a policy that allows what it was
// written to refuse — and the requirement is also what lets a later format
// change be detected instead of misread.
const version = 1

// ParseFile reads a policy from a named file.
//
// The policy file does not pass the gate, and neither does the audit stream.
// That is an exemption against docs/design.md's rule that an access is inside
// the boundary when the path was chosen by whoever the policy is about, and
// the reason is subject versus apparatus: the script is what the policy is
// about, while the policy file is the policy's own machinery. A boundary that
// could be told to refuse to read its own rules is not one. It is also read
// before there is a gate to ask.
func ParseFile(name string) (*Policy, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	// Nothing was written, so there is nothing a close can report that
	// matters: a read-only descriptor's close fails only for a descriptor
	// that was already gone.
	defer func() { _ = f.Close() }()
	p, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return p, nil
}

// Parse reads a policy.
func Parse(r io.Reader) (*Policy, error) {
	p := &Policy{}
	seenVersion := false
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(strings.TrimSuffix(sc.Text(), "\r"), " \t")
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		word, rest := cut(trimmed)
		if !seenVersion {
			if word != "version" {
				return nil, lineErr(n, errors.New(
					`a policy starts with "version 1"; a file this parser cannot vouch for is refused rather than half-read`))
			}
			if rest != fmt.Sprint(version) {
				return nil, lineErr(n, fmt.Errorf("unknown policy version %q, want %d", rest, version))
			}
			seenVersion = true
			continue
		}
		if err := p.directive(word, rest); err != nil {
			return nil, lineErr(n, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !seenVersion {
		return nil, errors.New(`a policy starts with "version 1"`)
	}
	// Defaults are settled once, after the whole file, so that the meaning of
	// a policy does not depend on the order its lines were written in — the
	// same property deny-overrides gives the rules.
	for i := range p.allowSlot {
		if !p.setSlot[i] {
			p.allowSlot[i] = p.allowBase
		}
	}
	return p, nil
}

func (p *Policy) directive(word, rest string) error {
	switch word {
	case "version":
		return errors.New("the version is stated once, on the first line")
	case "default":
		return p.defaultDirective(rest)
	case "allow", "deny":
		return p.rule(word, rest)
	}
	return fmt.Errorf("unknown directive %q, want allow, deny or default", word)
}

// defaultDirective is `default <decision>` or `default <decision> <selector>`.
//
// The bare form is the base and may appear once. The selector form overrides
// the base for the slots that selector names, and each slot may be named once.
// Both restrictions exist for one reason: a file where a later `default` could
// overwrite an earlier one would have an order-dependent meaning, and the
// rules deliberately do not.
func (p *Policy) defaultDirective(rest string) error {
	word, sel := cut(rest)
	decision, err := decisionOf(word)
	if err != nil {
		return err
	}
	if sel == "" {
		if p.baseSet {
			return errors.New("the base default is stated once")
		}
		p.baseSet, p.allowBase = true, decision == interp.Allow
		return nil
	}
	s, err := selectorOf(sel)
	if err != nil {
		return err
	}
	for _, sl := range s.slots() {
		if p.setSlot[sl] {
			return fmt.Errorf("the default for %s is set twice", sel)
		}
		p.setSlot[sl], p.allowSlot[sl] = true, decision == interp.Allow
	}
	return nil
}

func (p *Policy) rule(word, rest string) error {
	decision, err := decisionOf(word)
	if err != nil {
		return err
	}
	name, pattern := cut(rest)
	if name == "" {
		return fmt.Errorf("%s needs something to act on: a selector, then a pattern", word)
	}
	sel, err := selectorOf(name)
	if err != nil {
		return err
	}
	if !sel.takesPattern() {
		if pattern != "" {
			// A signal names a process, not a file, so a pattern here could
			// never match anything. Refused rather than ignored: a rule that
			// silently never fires is the worst outcome in a policy.
			return fmt.Errorf("%s takes no pattern — a signal names a process, not a path", name)
		}
		p.rules = append(p.rules, Rule{Decision: decision, Sel: sel})
		return nil
	}
	if pattern == "" {
		return fmt.Errorf("%s needs a pattern", name)
	}
	if !strings.HasPrefix(pattern, "/") {
		// A relative pattern would mean "relative to a working directory",
		// and the policy owns none: the shell's directory moves with `cd` and
		// is not the process's, so the same rule would mean different things
		// at different points in one script.
		return fmt.Errorf("pattern %q must be absolute", pattern)
	}
	if err := validPattern(pattern); err != nil {
		return fmt.Errorf("pattern %q: %w", pattern, err)
	}
	p.rules = append(p.rules, Rule{Decision: decision, Sel: sel, Pattern: pattern})
	return nil
}

func decisionOf(word string) (interp.Decision, error) {
	switch word {
	case "allow":
		return interp.Allow, nil
	case "deny":
		return interp.Deny, nil
	}
	return interp.Deny, fmt.Errorf("%q is not a decision, want allow or deny", word)
}

func selectorOf(name string) (Selector, error) {
	if s, ok := selectorNames[name]; ok {
		return s, nil
	}
	if name == "inherit" {
		// Named rather than lumped in with the typos, because someone writing
		// it has a reasonable model and a wrong one. ActionInherit is recorded
		// and never gated — interp/seams.go gives the reason at length — so a
		// rule about it would never be consulted.
		return 0, errors.New(
			"inherit is recorded and never gated: a descriptor the shell was handed is already in the process's table")
	}
	return 0, fmt.Errorf("unknown selector %q, want exec, read, write, open, stat, list, path or signal", name)
}

// cut splits the first whitespace-delimited word off a line and returns the
// rest with its leading whitespace removed and nothing else done to it. The
// rest is a pattern, and a pattern is a path: splitting it into fields would
// break every path containing a space.
func cut(line string) (word, rest string) {
	i := strings.IndexAny(line, " \t")
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.TrimLeft(line[i:], " \t")
}

func lineErr(n int, err error) error {
	return fmt.Errorf("line %d: %w", n, err)
}
