// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// What `set -n` does with the *words* of a simple command it is not going to
// run.
//
// Six of the seven panel columns do nothing with them: `sh -n file` reads the
// program, answers whether it parses, and writes nothing else. zsh reads the
// words of a simple command it reaches far enough to raise the refusals a
// word's own reading makes, which is a narrower thing than expanding it — the
// controls below are what say so.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C zsh -f -n -c …`
// against zsh 5.9.2 at /opt/homebrew, each probe on its own. Three constructs
// reach it, with the same sentence, status and location they have at run time:
//
//	echo =nosuchcmd     nosuchcmd not found      1
//	: ${(P)::=y}        not an identifier:       1
//	echo ${9nope}       bad substitution         1
//
// **And it is a property of the position, not of the construct.** The same
// word, moved:
//
//	echo =nosuchcmd                        1, message
//	true; echo =nosuchcmd                  1, message
//	true && echo =nosuchcmd                1, message
//	! echo =nosuchcmd                      1, message
//	echo =nosuchcmd > /dev/null            1, message
//	echo =nosuchcmd | cat                  0, message
//	echo =nosuchcmd &                      0, message
//	{ echo =nosuchcmd; }                   0, silent
//	( echo =nosuchcmd )                    0, silent
//	if true; then echo =nosuchcmd; fi      0, silent
//	while false; do echo =nosuchcmd; done  0, silent
//	for i in a; do echo =nosuchcmd; done   0, silent
//	case a in a) echo =nosuchcmd;; esac    0, silent
//	f() { echo =nosuchcmd; }               0, silent
//	() { echo =nosuchcmd; }                0, silent
//	time echo =nosuchcmd                   0, silent
//	v==nosuchcmd                           0, silent
//
// which this implementation reproduces without a rule of its own for most of
// it: Runner.command already declines to walk into a compound under `set -n`,
// so a body is never reached. What needs saying is the two rows that carry a
// message at status 0 and the `time` row.
//
// The message-at-0 rows are a **fork**: the refusal ends the shell it is
// raised in, and a non-last pipeline element and a backgrounded statement are
// each a shell of their own — which this engine reconstructs with a clone, so
// the rows come out right with no rule of their own. Measured to be exactly that rather than a rule
// about pipelines — `echo =nosuchA | echo =nosuchB` writes **both** messages
// and exits 1, because the last element is the one that runs in this shell,
// and `echo =nosuchA | cat | cat` writes one and exits 0.
//
// The fatality is the other half and is measured the same way: `echo
// =nosuchA; echo =nosuchB` writes only the first message, and `echo =nosuchA;
// true` exits 1 — so the refusal ends the script exactly as it does at run
// time, rather than being reported and walked past. `true || echo
// =nosuchcmd` is silent at 0 and `echo =nosuchcmd || true` is 1, which is the
// chain's own short-circuit reading a status nothing set.
//
// **The controls, and they are what make this narrow.** All at the top level,
// all 0 and silent under `zsh -n`:
//
//	echo $(touch /tmp/marker)      the file is not created
//	echo ${nope:?boom}
//	echo $(( 1/0 ))
//	echo /nonexistentdir*/zzz
//	x=(1 2); echo $x[a]
//	typeset -r r=1; r=2
//	echo ~nosuchuser
//
// So no substitution runs, no arithmetic is evaluated, no pattern is matched,
// no value is fetched and no store happens — which is why this is three shape
// checks and a path lookup rather than a second expander. `setopt noequals;
// echo =nosuchcmd` is the sharpest row: under `-n` it still refuses, and at
// run time it prints `=nosuchcmd`, so the option cannot be in effect, nothing
// ran, and the refusal is still raised.
//
// Two rows are **measured and deliberately not reproduced**, recorded here
// rather than left to be rediscovered. `echo $[1/0]` is `division by zero` at
// 1 under `zsh -n` where `echo $(( 1/0 ))` is silent, and `echo
// $[nosuchfunc(1)]` is `unknown function` — so that one spelling of
// arithmetic is evaluated while the other is not. Evaluating an expression
// under a mode whose contract is that it does not act is a larger question
// than this one, and reproducing it would mean deciding what `$[x=5]` does
// there, which nothing measured answers (#3823).

// readWordsWithoutRunning raises, for a simple command `set -n` will not run,
// the refusals that reading its words makes.
//
// Three checks and no expansion at all, which is the controls above written
// as code: a `=word` head, a `${…}` the grammar could not read, and a
// name-flagged assignment whose target is no name. Everything else a word can
// hold needs a value, a process or the filesystem, and none of those is
// touched.
func (r *Runner) readWordsWithoutRunning(c *syntax.SimpleCmd) {
	if c == nil {
		return
	}
	if r.noexecUnread > 0 {
		// A timed command, which is the one shape that reaches here and is
		// still silent — measured above, `time echo =nosuchcmd` writes
		// nothing where `! echo =nosuchcmd` writes the refusal. Read off a
		// counter rather than the node, because the clause's body is a
		// pipeline and the element is what arrives here.
		return
	}
	if !unrunWordsCouldRefuse(c) {
		// Nothing in this command could refuse however the axis is
		// answered, so the axis is not asked. That keeps it off the common
		// path, which matters more here than at most sites: a question
		// asked of every simple command under `set -n` would make `sh -n
		// file` refuse every file, where the answer is only ever visible for
		// the three shapes below.
		return
	}
	if !r.ask(r.sem().UnrunSimpleCommandReadsItsWords,
		"`set -n` reading the words of a simple command it will not run") {
		return
	}
	for _, w := range c.Args {
		if r.readWordWithoutRunning(w) {
			// The first refusal is the whole of the answer: it ends the
			// shell, so nothing after it in the command or in the file is
			// read. Measured — `echo =nosuchA =nosuchB` writes one message.
			return
		}
	}
}

// unrunWordsCouldRefuse reports whether any word of this command carries one
// of the three shapes, read off the tree and running nothing.
//
// It is the guard that keeps the axis at the disagreement: a `=` head with no
// command behind it, a `${…}` the grammar could not read, or a name-flagged
// assignment are the only words whose reading can refuse, and every other
// command under `set -n` asks nothing at all.
func unrunWordsCouldRefuse(c *syntax.SimpleCmd) bool {
	for _, w := range c.Args {
		if w == nil {
			continue
		}
		if equalsHeadWord(w) != nil {
			return true
		}
		for i := range w.Spans {
			if e := w.Spans[i].Param; e != nil && (unrunBadShape(e) || unrunNameFlagShape(e)) {
				return true
			}
		}
	}
	return false
}

// unrunBadShape is the `${…}` half: an expansion whose operator the grammar
// did not recognize.
//
// BadTransform is deliberately not one of these. Its own documentation says
// why: where the transformation family exists, a bad letter is a failure of
// the *expansion*, checked against the value at the point the value is read —
// and no value is read here.
func unrunBadShape(e *syntax.ParamExpr) bool {
	return e.Bad && !e.BadTransform
}

// unrunNameFlagShape is the third shape: an expansion whose result is read as
// a **parameter name** and which assigns through it.
//
// `${(P)::=y}` is the measured spelling — the flag is there, the operator
// assigns, and the name between them is empty. Only where the name is written
// out, because a name that comes out of a value is not decidable without
// fetching one, and fetching one is what this whole file does not do; and only
// for a plain name, because an element is a reading of its own and needs the
// table.
func unrunNameFlagShape(e *syntax.ParamExpr) bool {
	return e.HasFlags && strings.ContainsRune(e.Flags, 'P') &&
		assignsThroughTheName(e.Op) && e.Index == nil && !e.Indirect
}

// readWordWithoutRunning is the three checks over one word. It reports whether
// one of them refused.
func (r *Runner) readWordWithoutRunning(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	// The `=word` head, which is the same rewrite expandEquals makes and is
	// made on a copy: the tree is the program, and a check that edited it
	// would leave `set -n` having changed what a later reader sees. It is
	// the one check that touches the filesystem, and it touches it the way
	// the word itself would — a PATH lookup and nothing more.
	if head := equalsHeadWord(w); head != nil && r.unrunRefusal(func() { r.expandEquals(head) }) {
		return true
	}
	for i := range w.Spans {
		e := w.Spans[i].Param
		if e == nil {
			continue
		}
		if unrunBadShape(e) && r.unrunRefusal(func() { r.reportBadSubstitution(e) }) {
			return true
		}
		if unrunNameFlagShape(e) &&
			r.unrunRefusal(func() { r.assignableParamName(e.Name) }) {
			return true
		}
	}
	return false
}

// assignsThroughTheName reports whether this operator writes the parameter
// rather than only reading it.
func assignsThroughTheName(op syntax.ParamOp) bool {
	switch op {
	case syntax.ParamAssign, syntax.ParamAssignAlways:
		return true
	}
	return false
}

// unrunRefusal runs one check and settles what its refusal costs: the shell
// it is raised in ends, as it does at run time.
//
// **The two rows that carry a message at status 0 need no rule here**, which
// is worth saying because they look like one. A pipeline element that is not
// the last and a backgrounded statement each run on a clone of this runner,
// so the control word this sets is the clone's and dies with it — the message
// is written on the shared stream and the shell that reports a status never
// saw one. That is the same boundary the reference forks at, reached by the
// same means, and a flag threaded through the pipeline would be a second
// answer to a question already answered. See the pipeline and background rows
// above, and the last-element row, which is the control: a refusal there ends
// the script, because the last element runs in this shell.
//
// Which of the three checks refused is read off the runner rather than
// returned, because each of them reports the dialect's own way and none of
// them was written for this caller: an expansion failure sets expandErr, and
// a refusal that ends the script sets the control word.
func (r *Runner) unrunRefusal(report func()) bool {
	before, ctl := r.expandErr, r.ctl
	report()
	if !r.expandErr && r.ctl == ctl {
		return false
	}
	r.expandErr = before
	r.status = 1
	r.ctl, r.abandon, r.errexitStopped = controlExit, abandonError, false
	return true
}

// equalsHeadWord is a copy of w holding only its first span, where that span
// is an unquoted literal opening with `=` and carrying something behind it,
// and nil otherwise.
func equalsHeadWord(w *syntax.Word) *syntax.Word {
	if len(w.Spans) == 0 {
		return nil
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "=") || len(s.Value) == 1 {
		return nil
	}
	return &syntax.Word{Spans: []syntax.Span{s}}
}
