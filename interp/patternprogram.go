// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// A pattern read once into a program, and the program the matcher runs (#6380).
//
// matchBranch reads the pattern text at every step of every trial: which
// construct stands at this position, where its group closes, what its arms
// are, whether a closure follows it, what a bracket expression holds. None of
// those answers depends on the subject. #1932 cached the *extents* — where a
// group or a bracket closes — and left the per-step branch chain in place;
// this reads the whole chain once, into a list of nodes per piece of the
// pattern, and runs the list.
//
// **The compiler is matchBranch with the subject taken out**, and that is the
// whole of the equivalence argument. Every decision matchBranch makes about
// what to do next is a function of the remaining pattern text and the options
// in force — splitExclusion, the caret, a flag group, a closure, a counted
// group, a group, a numeric range, then the first byte — and the compiler
// asks the same functions in the same order and writes down what they said.
// The executor then does what matchBranch would have done with the answer:
// the same splits, in the same order, with the same captures marked and
// rolled back, so a `(#b)` reports the same spans and the search stops at the
// same success. Nothing about *which* match is found is decided here.
//
// What the program does not model it hands back. A piece of pattern the
// compiler declines — a `~(…)` group, a class escape, a bracket whose
// reading asks the dialect a question or depends on the character it matched
// — becomes an interpret node, which runs matchBranch on that text exactly as
// the walk would have continued. So a pattern is never refused, only run
// partly by the old reader, and the equivalence argument for that part is
// that it *is* the old reader.
//
// Compiled once per pattern and per reading of the options, and only once a
// pattern has cost the interpreter enough steps to be worth it: a `case` arm
// matched once is a handful of steps and a program would be a loss there. See
// compileThreshold.

// patternCompiles turns the program off, for the reason memoThreshold and
// patternPrepares are vars: a test runs the same pattern down both paths and
// requires the same answer, which is the only reference for "the program
// changes nothing" that cannot drift from the code it checks. Nothing outside
// a test writes it.
var patternCompiles = true

// compileThreshold is how many interpreter steps a pattern may cost one
// matchWhere before it is compiled.
//
// The two costs it weighs are both per pattern: a program costs an allocation
// per construct to build and saves a re-read of the construct at every step.
// A `case` arm matched once against a short word is a few dozen steps and
// never reaches it; a substitution over a prompt theme reaches it on its first
// trial. A var so a test can compile everything (0) and compare.
var compileThreshold = 256

// compileNodeLimit bounds a program's size. The compiler reads each piece of
// the pattern once, so a program is linear in the pattern; the limit is a
// guard against a reading nobody anticipated rather than an expected case,
// and a pattern past it is simply interpreted.
const compileNodeLimit = 1 << 14

// patternReading is every field of patternOpts that is fixed for a match and
// can decide how a pattern reads. A program is built for one reading and run
// only under the same one.
//
// The fields of patternOpts it leaves out are the ones a match carries as
// state rather than reading — where the piece sits, the leading-period
// question a directory walk asks per name, and the three pointers — and the
// executor takes those from the match it is running. TestTheReadingNamesEveryOption
// keeps the two lists whole: a field added to patternOpts and to neither list
// fails it.
type patternReading struct {
	caret, tilde, tildeFold, whole, classEscapes  bool
	tildeGlobRead, tildeLeftUnread, emptyBracket  bool
	group, quantified, counted, operandParens     bool
	noBareGroup, armParens, topGroup              bool
	bareParenIsText, numericRange, fold, foldWide bool
	chars, record, extended, foldClass            bool
	bracketMember, matchSkipsToAnyBracket         bool
	nth                                           int
	tildePrefix, escapes                          string
	bracket                                       BracketPolicy
	unknownClass                                  UnknownClassPolicy
	unterminatedClass                             UnterminatedClassPolicy
	classes                                       patternClasses
	litFold                                       caseFolding
	collating                                     CollatingElementPolicy
}

// readingOf is the reading of o. See patternReading.
func readingOf(o *patternOpts) patternReading {
	return patternReading{
		caret: o.caret, tilde: o.tilde, tildeFold: o.tildeFold, whole: o.whole,
		classEscapes: o.classEscapes, tildeGlobRead: o.tildeGlobRead,
		tildeLeftUnread: o.tildeLeftUnread, emptyBracket: o.emptyBracket,
		group: o.group, quantified: o.quantified, counted: o.counted,
		operandParens: o.operandParens, noBareGroup: o.noBareGroup,
		armParens: o.armParens, topGroup: o.topGroup,
		bareParenIsText: o.bareParenIsText, numericRange: o.numericRange,
		fold: o.fold, foldWide: o.foldWide, chars: o.chars, record: o.record,
		extended: o.extended, foldClass: o.foldClass,
		bracketMember: o.bracketMember, matchSkipsToAnyBracket: o.matchSkipsToAnyBracket,
		nth: o.nth, tildePrefix: o.tildePrefix, escapes: o.escapes,
		bracket: o.bracket, unknownClass: o.unknownClass,
		unterminatedClass: o.unterminatedClass, classes: o.classes,
		litFold: o.litFold, collating: o.collating,
	}
}

// withMatchState is o with the fields a match carries as state taken from the
// match now running — the other half of patternReading.
func (x *cexec) withMatchState(o patternOpts) patternOpts {
	o.pieceBase, o.pieceEnd = x.pieceBase, x.pieceEnd
	o.period = x.period
	o.bad, o.where = x.bad, x.w
	o.askBracketAfterSub = x.ask
	return o
}

// stateless is o with the match state cleared, for a copy a program keeps.
func stateless(o patternOpts) patternOpts {
	o.pieceBase, o.pieceEnd, o.period = 0, 0, false
	o.bad, o.where, o.askBracketAfterSub = nil, nil, nil
	return o
}

// patternProgram is one pattern under one reading, compiled.
type patternProgram struct {
	reading patternReading
	// top is the pattern's top level: one piece, or one per arm where the
	// reading splits a live `|` there. See matchTopLevel.
	top []*cpiece
}

// cpiece is one piece of pattern text compiled — what matchHere is handed as
// p — and the unit the executor memoizes on, exactly as matchHere is the unit
// the interpreter memoizes on.
type cpiece struct {
	id    uint64
	first *cnode
	// text is the piece's pattern text. Only its emptiness and its reach are
	// read at run time; see cexec.star.
	text string
	// reach is patternReach of the text, which splitFloor and splitCeiling
	// read: a fact about the text that the interpreter caches and this
	// computes once.
	reach int
	// empty is the piece with no text, which matches exactly the empty span
	// and can write nothing — the closure's `after` and the group's `rest`
	// that end a pattern, asked at every position a repetition stops at.
	empty bool
	// units is how many units of the subject the piece can consume at most,
	// read off the compiled steps rather than off the text, or
	// unboundedReach. pure says nothing in the piece can act on reaching it:
	// no step hands text back to the walk, which is the only place a match
	// can ask the dialect a question or mark the pattern bad. See
	// cexec.floor for what the two together allow.
	units int
	pure  bool
	// unit says the piece is one step that consumes one unit and can write
	// nothing — a character, a `?`, a bracket — which is what a closure or a
	// group most often repeats or tries. Such a piece is asked directly
	// rather than through here, and where units are bytes it is asked only
	// about the one span it could fill. See cexec.item.
	unit bool
}

type cop uint8

const (
	// One subject byte that must be b or alt: a literal where units are
	// bytes, an escaped character, and a backslash the escape does not reach.
	opByte cop = iota
	// One pattern character against one subject character, where units are
	// characters.
	opChar
	// `?`.
	opAny
	// A bracket expression.
	opBracket
	// `(#s)` and `(#e)`.
	opAnchor
	// A trailing backslash, which matches a subject that is one backslash.
	opTrailingBackslash
	// A flag letter nothing answers: the branch fails.
	opFail
	// The terminals: each consumes the rest of its piece.
	opStar
	opGroup
	opRepeat
	opRange
	opExclude
	opNegate
	opInterpret
)

// cnode is one step of a piece. The steps up to opFail are a single
// consumption followed by next; the rest settle the piece themselves, as the
// branches of matchBranch that return do.
type cnode struct {
	op   cop
	next *cnode
	// o is the options in force where the node stands, for the steps that
	// call back into the matcher's own helpers. The match state on it is
	// stale and is replaced by withMatchState before it is used.
	o *patternOpts
	// opByte and opChar: the subject bytes an ASCII comparison accepts.
	b, alt byte
	// opChar: the pattern's character, and whether it is one byte.
	pat   string
	ascii bool
	// opBracket and opInterpret: the pattern text from here to the end of
	// the piece, and where it begins.
	text string
	pp   int
	// opBracket: which single bytes the expression holds, nil where the
	// expression cannot be answered a byte at a time, and the length of the
	// pattern left once it has matched.
	set     *[256]bool
	restLen int
	// opAnchor.
	anchor patternAnchor
	// opStar, opGroup, opRange, opNegate: what follows.
	rest *cpiece
	// opGroup. tight says the arms and the rest are all pure, so the floor
	// may be the compiled one; see cexec.floor.
	arms  []*cpiece
	gp    int
	quant byte
	bound repeatBound
	tight bool
	// opRepeat: lo and hi repetitions of item, then after.
	item, after *cpiece
	lo, hi      int
	// opRange.
	nlo, nhi int64
	// opExclude.
	left   *cpiece
	rights []*cpiece
}

// compilePattern reads a pattern into a program under o, which must carry the
// prepared matchWhere for this pattern. It reports false where the pattern is
// one the program must not run at all.
func compilePattern(pattern string, o patternOpts) (*patternProgram, bool) {
	// A `~(…)` group under the reading that consumes one mid-pattern is read
	// by matchHere *before* its memo, at every piece — see
	// matchTildeFlavorInPiece — so it is not a step a node can stand for.
	if o.tildeFold && strings.IndexByte(pattern, '~') >= 0 {
		return nil, false
	}
	c := &compiler{}
	prog := &patternProgram{reading: readingOf(&o)}
	if o.topGroup {
		if arms, armAt := topAlternatives(pattern, o.emptyBracket, o.bareParenIsText); len(arms) > 1 {
			for i, arm := range arms {
				prog.top = append(prog.top, c.piece(arm, armAt[i], o))
			}
		}
	}
	if prog.top == nil {
		prog.top = []*cpiece{c.piece(pattern, 0, o)}
	}
	if c.nodes > compileNodeLimit {
		return nil, false
	}
	return prog, true
}

type compiler struct {
	pieces uint64
	nodes  int
}

// piece compiles one piece of pattern text, which begins at offset pp of the
// pattern.
func (c *compiler) piece(p string, pp int, o patternOpts) *cpiece {
	pc := &cpiece{id: c.pieces, text: p, reach: patternReach(p, pp, &o)}
	c.pieces++
	if c.nodes <= compileNodeLimit {
		pc.first = c.branch(p, pp, o)
	}
	pc.empty = p == ""
	pc.units, pc.pure = pieceFacts(pc.first)
	if n := pc.first; n != nil && n.next == nil {
		switch n.op {
		case opByte, opChar, opAny:
			pc.unit = true
		case opBracket:
			// Not in the reading whose scan skips to the next `]` once a
			// member matches: there a match can hand the walk more pattern
			// than the piece's end, which is not one step.
			pc.unit = !n.o.matchSkipsToAnyBracket
		}
	}
	return pc
}

// branch is matchBranch's loop with the subject taken out. Each arm below is
// the arm of matchBranch it is named after, asked in the same order of the
// same helpers; the comments there say why each one is what it is.
func (c *compiler) branch(p string, pp int, o patternOpts) *cnode {
	var head *cnode
	tail := &head
	// A copy rather than &o: o moves as the walk goes, and a node emitted
	// before a flag must keep the options it was read under. The match
	// state is cleared from each copy, since a program outlives the match it
	// was compiled in; see cexec.withMatchState.
	first := stateless(o)
	opts := &first
	emit := func(n *cnode) {
		if n.o == nil {
			n.o = opts
		}
		c.nodes++
		*tail = n
		tail = &n.next
	}
	// changed is called whenever o moves, so that the nodes after it carry
	// the options they are matched under and the ones before keep theirs.
	changed := func() {
		cp := stateless(o)
		opts = &cp
	}
	for len(p) > 0 {
		if c.nodes > compileNodeLimit {
			return head
		}
		if o.extended {
			if left, rights, ok := splitExclusion(p, &o); ok {
				n := &cnode{op: opExclude, left: c.piece(left, pp, o)}
				xp := pp + len(left)
				for _, x := range rights {
					xp++
					n.rights = append(n.rights, c.piece(x, xp, o))
					xp += len(x)
				}
				emit(n)
				return head
			}
			if p[0] == '^' {
				emit(&cnode{op: opNegate, rest: c.piece(p[1:], pp+1, o)})
				return head
			}
			if body, rest, ok := splitPatternFlags(p, o.emptyBracket); ok {
				if a := anchorPatternFlag(body); a != anchorNone {
					emit(&cnode{op: opAnchor, anchor: a})
					pp, p = pp+len(p)-len(rest), rest
					continue
				}
				next, unknown := applyPatternFlags(body, o)
				if unknown != 0 {
					emit(&cnode{op: opFail})
					return head
				}
				o, pp, p = next, pp+len(p)-len(rest), rest
				changed()
				continue
			}
			if item, rest, ok := splitClosableItem(p, pp, &o); ok {
				if lo, hi, after, isClosure := closureBounds(rest, &o); isClosure {
					emit(&cnode{
						op: opRepeat, item: c.piece(item, pp, o), lo: lo, hi: hi,
						after: c.piece(after, pp+len(p)-len(after), o),
					})
					return head
				}
			}
		}
		// compilePattern declined every pattern holding a `~` under this
		// reading, so a group read here would be a pattern it let through.
		if o.tildeFold {
			if _, ok := splitTildeHereGroup(p); ok {
				emit(&cnode{op: opInterpret, text: p, pp: pp})
				return head
			}
		}
		if g, ok := splitCountedGroup(p, pp, &o); ok {
			o.noBareGroup = !o.operandParens
			changed()
			if !g.counts {
				spend := g.lead
				if !o.operandParens {
					spend++
				}
				for n := spend; n > 0; {
					pw := c.literal(p, o, emit)
					p, pp, n = p[pw:], pp+pw, n-pw
				}
				continue
			}
			armOpts := o
			armOpts.noBareGroup = false
			emit(&cnode{
				op: opGroup, gp: pp, bound: g.bound,
				arms: c.arms(g.body, pp+g.lead+1, armOpts),
				rest: c.piece(g.rest, pp+len(p)-len(g.rest), o),
			})
			return head
		}
		if body, quant, rest, ok := splitGroup(p, pp, &o); ok {
			lead := 0
			if quant != 0 {
				lead = 1
			}
			o.noBareGroup = !o.group && !o.operandParens && !o.armParens
			changed()
			armOpts := o
			armOpts.noBareGroup = false
			emit(&cnode{
				op: opGroup, gp: pp, quant: quant, bound: boundOf(quant),
				arms: c.arms(body, pp+lead+1, armOpts),
				rest: c.piece(rest, pp+len(p)-len(rest), o),
			})
			return head
		}
		if lo, hi, rest, ok := splitNumericRange(p, &o); ok {
			emit(&cnode{op: opRange, nlo: lo, nhi: hi, rest: c.piece(rest, pp+len(p)-len(rest), o)})
			return head
		}
		switch p[0] {
		case '*':
			p, pp = p[1:], pp+1
			for len(p) > 0 && p[0] == '*' && !quantifierOpensAGroup(p, o) {
				p, pp = p[1:], pp+1
			}
			emit(&cnode{op: opStar, rest: c.piece(p, pp, o)})
			return head
		case '?':
			emit(&cnode{op: opAny})
			p, pp = p[1:], pp+1
		case '[':
			n, ok := c.bracket(p, pp, o)
			if !ok {
				emit(&cnode{op: opInterpret, text: p, pp: pp})
				return head
			}
			emit(n)
			pp += len(p) - n.restLen
			p = p[len(p)-n.restLen:]
		case '\\':
			if len(p) < 2 {
				emit(&cnode{op: opTrailingBackslash})
				return head
			}
			if o.classEscapes {
				_, zero := kshGlobZeroWidth(p[1])
				_, control := kshGlobControlEscape(p[1])
				if zero || control || kshClassEscape(p[1]) {
					emit(&cnode{op: opInterpret, text: p, pp: pp})
					return head
				}
			}
			if !o.escapeReaches(p[1]) {
				emit(&cnode{op: opByte, b: '\\', alt: '\\'})
				p, pp = p[1:], pp+1
				continue
			}
			n, ok := byteNode(p[1], &o)
			if !ok {
				emit(&cnode{op: opInterpret, text: p, pp: pp})
				return head
			}
			emit(n)
			p, pp = p[2:], pp+2
		default:
			pw := c.literal(p, o, emit)
			p, pp = p[pw:], pp+pw
		}
	}
	return head
}

// arms compiles a group's body into its alternatives.
func (c *compiler) arms(body string, bp int, o patternOpts) []*cpiece {
	arms, armAt := alternativesAt(body, bp, o.emptyBracket)
	out := make([]*cpiece, len(arms))
	for k, a := range arms {
		out[k] = c.piece(a, armAt[k], o)
	}
	return out
}

// literal emits the node for the pattern character at the front of p and
// reports its width in the pattern — the pw eqPatternHere answers, which is a
// fact about the pattern alone.
func (c *compiler) literal(p string, o patternOpts, emit func(*cnode)) int {
	pw := 1
	if o.chars && p[0] >= utf8.RuneSelf {
		_, pw = utf8.DecodeRuneInString(p)
	}
	if !o.chars {
		if n, ok := byteNode(p[0], &o); ok {
			emit(n)
			return pw
		}
	}
	n := &cnode{op: opChar, pat: p[:pw], ascii: p[0] < utf8.RuneSelf}
	if n.ascii {
		n.b, n.alt = p[0], p[0]
		if set, ok := byteSet(p[0], &o); ok {
			n.b, n.alt = set[0], set[1]
		} else {
			n.ascii = false
		}
	}
	emit(n)
	return pw
}

// byteNode is a step accepting the subject bytes eqPatternByte accepts for
// pattern byte b, worked out by asking it of every byte.
func byteNode(b byte, o *patternOpts) (*cnode, bool) {
	set, ok := byteSet(b, o)
	if !ok {
		return nil, false
	}
	return &cnode{op: opByte, b: set[0], alt: set[1]}, true
}

// byteSet is the one or two subject bytes eqPatternByte accepts for pattern
// byte b. Asked of all 256 rather than derived from the fold rules, so that
// it is the comparison's own answer; more than two is a comparison this
// cannot hold and is reported as such.
func byteSet(b byte, o *patternOpts) ([2]byte, bool) {
	var set [2]byte
	n := 0
	for c := 0; c < 256; c++ {
		if o.eqPatternByte(b, byte(c)) {
			if n == 2 {
				return set, false
			}
			set[n] = byte(c)
			n++
		}
	}
	switch n {
	case 0:
		return set, false
	case 1:
		set[1] = set[0]
	}
	return set, true
}

// bracket compiles the bracket expression at the front of p.
//
// The expression is answered for every single byte ahead of time by asking
// matchBracket itself, under a copy of the options whose two side-effecting
// fields record rather than act. A bracket that would ask the dialect a
// question or mark the pattern bad is declined, and is interpreted where it
// stands: those are effects of *reaching* it, and a table built ahead would
// either lose them or invent them.
//
// What is left after a match is the same for every byte in every reading
// but one — the dialect whose scan skips to the next `]` once a member has
// matched — and the table is kept only where it is the same; a character the
// table cannot answer is matched at run time and its rest compared with the
// compiled one. See cexec.run.
func (c *compiler) bracket(p string, pp int, o patternOpts) (*cnode, bool) {
	probe := o
	var bad, asked bool
	probe.bad = &bad
	probe.askBracketAfterSub = func() BracketPolicy {
		asked = true
		return o.bracket
	}
	var set [256]bool
	restLen, consistent := -1, true
	for b := 0; b < 256; b++ {
		rest, ok := matchBracket(p, byteStrings[b], &probe)
		if ok {
			set[b] = true
			if restLen < 0 {
				restLen = len(rest)
			} else if len(rest) != restLen {
				consistent = false
			}
		}
	}
	if bad || asked {
		return nil, false
	}
	n := &cnode{op: opBracket, text: p, pp: pp, set: &set, restLen: restLen}
	if restLen < 0 {
		// No single byte is a member, so no byte says where the expression
		// ends. A miss says it as well as a hit does except where a hit moves
		// it, which the run-time comparison catches.
		rest, _ := matchBracket(p, byteStrings[0], &probe)
		n.restLen = len(rest)
	}
	if !consistent {
		n.set = nil
	}
	return n, true
}

// cexec is one run of a program against one piece of a subject.
type cexec struct {
	w *matchWhere
	// piece is what matchPatternIn was handed, and base where it begins in
	// the subject: every position below is absolute, as matchHere's at is,
	// and the text at a position is read through sub.
	piece string
	base  int
	// The state patternReading leaves out, from the options of the match
	// running. Held by value rather than as a pointer to those options so
	// that matchPatternIn's own copy of them stays on its stack.
	period              bool
	pieceBase, pieceEnd int
	bad                 *bool
	ask                 func() BracketPolicy
}

func (x *cexec) sub(at, end int) string { return x.piece[at-x.base : end-x.base] }

// periodAt is patternOpts.periodHere at this position.
func (x *cexec) periodAt(at, end int) bool {
	return x.period && at == 0 && end > at && x.piece[at-x.base] == '.'
}

// match is matchTopLevel.
func (p *patternProgram) match(w *matchWhere, piece string, base int, o *patternOpts) bool {
	x := cexec{
		w: w, piece: piece, base: base,
		period: o.period, pieceBase: o.pieceBase, pieceEnd: o.pieceEnd,
		bad: o.bad, ask: o.askBracketAfterSub,
	}
	end := base + len(piece)
	if len(p.top) == 1 {
		return x.here(p.top[0], base, end)
	}
	for _, arm := range p.top {
		mark := w.caps.mark()
		if x.here(arm, base, end) {
			return true
		}
		w.caps.rollback(mark)
	}
	return false
}

// here is matchHere: does the piece match the subject from at to end, with
// the dead ends remembered once the match has asked enough questions. The
// memo is keyed on the piece rather than on the pattern offsets, which names
// the same question — a piece is one pattern text under one set of options —
// and it is a store of its own, so a key of this kind never meets one of the
// interpreter's.
func (x *cexec) here(c *cpiece, at, end int) bool {
	w := x.w
	w.asked++
	if c.empty {
		return at == end
	}
	if w.asked <= memoThreshold*compiledMemoFactor {
		return x.run(c.first, at, end)
	}
	if w.compiledDeadEnd(c.id, at, end) {
		return false
	}
	if x.run(c.first, at, end) {
		return true
	}
	w.rememberCompiledDeadEnd(c.id, at, end)
	return false
}

// compiledMemoFactor is how much later than the walk the program starts
// writing its dead ends down. A compiled step costs a fraction of a walked
// one, so a memo that pays for itself at the walk's threshold is still a loss
// here — on the prompt theme's substitution its table was the largest
// allocation the match made. It scales memoThreshold rather than replacing
// it, so a test that sets that to nought drives both memos.
const compiledMemoFactor = 8

// run walks a piece's steps from n. It is the loop of matchBranch.
func (x *cexec) run(n *cnode, at, end int) bool {
	for ; n != nil; n = n.next {
		switch n.op {
		case opByte:
			if at == end {
				return false
			}
			if c := x.piece[at-x.base]; c != n.b && c != n.alt {
				return false
			}
			at++
		case opChar:
			if at == end {
				return false
			}
			if c := x.piece[at-x.base]; n.ascii && c < utf8.RuneSelf {
				if c != n.b && c != n.alt {
					return false
				}
				at++
				continue
			}
			_, sw, ok := n.o.eqPatternHere(n.pat, x.sub(at, end))
			if !ok {
				return false
			}
			at += sw
		case opAny:
			if at == end || x.periodAt(at, end) {
				return false
			}
			at += n.o.unitWidth(x.sub(at, end))
		case opBracket:
			if at == end || x.periodAt(at, end) {
				return false
			}
			s := x.sub(at, end)
			w := n.o.unitWidth(s)
			if w == 1 && n.set != nil {
				if !n.set[s[0]] {
					return false
				}
				at++
				continue
			}
			o := x.withMatchState(*n.o)
			rest, ok := matchBracket(n.text, s[:w], &o)
			if !ok {
				return false
			}
			if len(rest) != n.restLen {
				// The member moved where the pattern goes on from, which is
				// the one reading whose scan stops at the next `]`. The walk
				// carries on from there exactly as matchBranch's loop does.
				return matchBranch(rest, s[w:], n.pp+len(n.text)-len(rest), at+w, o)
			}
			at += w
		case opAnchor:
			// patternAnchor.holds, without copying the options to ask it.
			if (n.anchor == anchorStart && at != 0) || (n.anchor == anchorEnd && at != x.w.total) {
				return false
			}
		case opTrailingBackslash:
			return end-at == 1 && x.piece[at-x.base] == '\\'
		case opFail:
			return false
		case opStar:
			return x.star(n, at, end)
		case opGroup:
			return x.group(n, n.bound, true, at, end)
		case opRepeat:
			return x.repeat(n, 0, at, end)
		case opRange:
			return x.numericRange(n, at, end)
		case opExclude:
			if !x.here(n.left, at, end) {
				return false
			}
			for _, r := range n.rights {
				if x.here(r, at, end) {
					return false
				}
			}
			return true
		case opNegate:
			return !x.here(n.rest, at, end)
		case opInterpret:
			return matchBranch(n.text, x.sub(at, end), n.pp, at, x.withMatchState(*n.o))
		}
	}
	return at == end
}

// star is the `*` arm of matchBranch.
func (x *cexec) star(n *cnode, at, end int) bool {
	caps := x.w.caps
	if x.periodAt(at, end) {
		mark := caps.mark()
		if x.here(n.rest, at, end) {
			return true
		}
		caps.rollback(mark)
		return false
	}
	if n.rest.text == "" {
		return true
	}
	if !n.o.chars {
		for i := end; ; i-- {
			mark := caps.mark()
			if x.here(n.rest, i, end) {
				return true
			}
			caps.rollback(mark)
			if i == at {
				return false
			}
		}
	}
	s := x.sub(at, end)
	var buf [64]int
	bounds := buf[:0]
	for i := 0; ; i += n.o.unitWidth(s[i:]) {
		bounds = append(bounds, i)
		if i == len(s) {
			break
		}
	}
	for j := len(bounds) - 1; j >= 0; j-- {
		mark := caps.mark()
		if x.here(n.rest, at+bounds[j], end) {
			return true
		}
		caps.rollback(mark)
	}
	return false
}

// floor is splitFloor for the piece c over the subject from at to end, as an
// offset from at.
//
// tight says every attempt the floor rules out is one that could act on
// nothing — see pieceFacts — and then the bound is the compiled steps' own,
// which is never looser than the text's and usually much tighter: a bracket
// expression is one unit however many bytes it is written in. An attempt below
// the floor hands the rest more subject than it can consume, so it fails; one
// that could have set a verdict or asked a question on the way is the only
// kind whose skipping anyone could see, and tight is false for those.
func (x *cexec) floor(c *cpiece, chars, tight bool, at, end int) int {
	if !patternReachBounds {
		return 0
	}
	n := c.reach
	if tight {
		n = c.units
	}
	if n == unboundedReach {
		return 0
	}
	if chars {
		n *= utf8.UTFMax
	}
	return max(0, end-at-n)
}

// ceiling is splitCeiling for the piece c, as an offset from at, and tight
// where the piece is pure — the same argument as floor's.
func (x *cexec) ceiling(c *cpiece, o *patternOpts, at, end int) int {
	if !patternReachBounds {
		return end - at
	}
	n := c.reach
	if c.pure {
		n = c.units
	}
	if n == unboundedReach {
		return end - at
	}
	if !o.chars {
		return min(n, end-at)
	}
	s := x.sub(at, end)
	i := 0
	for range n {
		if i >= len(s) {
			break
		}
		i += o.unitWidth(s[i:])
	}
	return min(i, len(s))
}

// unitOnly says the only span of a unit piece worth asking about is one byte
// long: units are bytes, so every step a unit piece is consumes exactly one
// and fills no other span.
func unitOnly(c *cpiece, o *patternOpts) bool { return c.unit && !o.chars }

// pieceFacts is the most units a piece's steps can consume, and whether any
// of them hands text back to the walk.
//
// Each consuming step takes one unit: a byte where units are bytes, and at
// most one character where they are characters — an escaped byte is one
// byte, and a literal or a bracket takes the character it compared. The
// terminal steps add what their own pieces can, or are unbounded. A group
// also learns here whether its own floor may be the compiled one; see
// cexec.floor.
func pieceFacts(n *cnode) (units int, pure bool) {
	units, pure = 0, true
	add := func(k int) {
		if units == unboundedReach || k == unboundedReach {
			units = unboundedReach
			return
		}
		units += k
	}
	for ; n != nil; n = n.next {
		switch n.op {
		case opByte, opChar, opAny, opTrailingBackslash:
			add(1)
		case opBracket:
			add(1)
			if n.o.matchSkipsToAnyBracket {
				// A match can move where the pattern goes on from and hand
				// the rest to the walk; see cexec.run.
				units, pure = unboundedReach, false
			}
		case opAnchor, opFail:
		case opStar, opRange, opNegate:
			add(unboundedReach)
			pure = pure && n.rest.pure
		case opInterpret:
			units, pure = unboundedReach, false
		case opExclude:
			// The left side has to match the whole span, so it bounds it.
			add(n.left.units)
			pure = pure && n.left.pure
			for _, r := range n.rights {
				pure = pure && r.pure
			}
		case opRepeat:
			if n.hi == unboundedRepeat || n.item.units == unboundedReach {
				add(unboundedReach)
			} else {
				add(n.hi * n.item.units)
			}
			add(n.after.units)
			pure = pure && n.item.pure && n.after.pure
		case opGroup:
			widest := 0
			n.tight = n.rest.pure
			for _, a := range n.arms {
				n.tight = n.tight && a.pure
				pure = pure && a.pure
				if widest != unboundedReach && a.units != unboundedReach {
					widest = max(widest, a.units)
				} else {
					widest = unboundedReach
				}
			}
			switch {
			case n.quant == '!', n.bound.hi < 0, widest == unboundedReach:
				add(unboundedReach)
			default:
				add(n.bound.hi * widest)
			}
			add(n.rest.units)
			pure = pure && n.rest.pure
		}
	}
	return units, pure
}

// item is here for a piece that may be a unit: the same question, asked
// without the memo for a piece that is one step, since a step is cheaper to
// take than to look up.
func (x *cexec) item(c *cpiece, at, end int) bool {
	if c.unit {
		return x.run(c.first, at, end)
	}
	return x.here(c, at, end)
}

// group is matchGroupTimes.
func (x *cexec) group(n *cnode, b repeatBound, first bool, at, end int) bool {
	caps := x.w.caps
	if n.quant == '!' {
		lo, hi := x.floor(n.rest, n.o.chars, n.tight, at, end), end-at
		if x.periodAt(at, end) {
			lo, hi = 0, 0
		}
		for i := lo; i <= hi; i++ {
			mark := caps.mark()
			if !x.anyArm(n, at, at+i) && x.here(n.rest, at+i, end) {
				caps.record(n.gp, at, at+i)
				return true
			}
			caps.rollback(mark)
		}
		return false
	}
	repeat := b.mayRepeat()
	if first && b.mayStopHere() {
		mark := caps.mark()
		if x.here(n.rest, at, end) {
			return true
		}
		caps.rollback(mark)
	}
	if !b.mayTakeARepetition() {
		return false
	}
	floor := 0
	if !repeat {
		floor = x.floor(n.rest, n.o.chars, n.tight, at, end)
	}
	for _, a := range n.arms {
		top, bottom := x.ceiling(a, n.o, at, end), floor
		if unitOnly(a, n.o) {
			top, bottom = min(top, 1), max(bottom, 1)
		}
		for i := top; i >= bottom; i-- {
			mark := caps.mark()
			if !x.item(a, at, at+i) {
				caps.rollback(mark)
				continue
			}
			after := b.afterOne()
			if after.mayStopHere() && x.here(n.rest, at+i, end) {
				caps.record(n.gp, at, at+i)
				return true
			}
			if repeat && i > 0 && x.group(n, after, false, at+i, end) {
				caps.record(n.gp, at, at+i)
				return true
			}
			caps.rollback(mark)
		}
	}
	return false
}

// anyArm is matchesAnyArm.
func (x *cexec) anyArm(n *cnode, at, end int) bool {
	for _, a := range n.arms {
		if x.here(a, at, end) {
			return true
		}
	}
	return false
}

// repeat is repeatFrom: k repetitions of the item have been taken.
func (x *cexec) repeat(n *cnode, k, at, end int) bool {
	caps := x.w.caps
	if n.hi == unboundedRepeat || k < n.hi {
		s := x.sub(at, end)
		i := x.ceiling(n.item, n.o, at, end)
		if unitOnly(n.item, n.o) {
			i = min(i, 1)
		}
		for ; i > 0; i = n.o.unitBefore(s, i) {
			mark := caps.mark()
			if !x.item(n.item, at, at+i) {
				caps.rollback(mark)
				continue
			}
			if x.repeat(n, k+1, at+i, end) {
				return true
			}
			caps.rollback(mark)
		}
	}
	mark := caps.mark()
	if k >= n.lo && x.here(n.after, at, end) {
		return true
	}
	caps.rollback(mark)
	return false
}

// numericRange is matchNumericRange.
func (x *cexec) numericRange(n *cnode, at, end int) bool {
	caps := x.w.caps
	s := x.sub(at, end)
	for k := 1; k <= len(s) && isDigit(s[k-1]); k++ {
		v, err := strconv.ParseInt(s[:k], 10, 64)
		if err != nil {
			v = math.MaxInt64
		}
		if n.nlo != unbounded && v < n.nlo {
			continue
		}
		if n.nhi != unbounded && v > n.nhi {
			break
		}
		mark := caps.mark()
		if x.here(n.rest, at+k, end) {
			return true
		}
		caps.rollback(mark)
	}
	return false
}

// program is the compiled form of the pattern now set on w under o's
// reading, or nil where the interpreter is to run instead: compiling is off,
// the pattern has not yet cost enough to be worth it, or it is one the
// program declines.
//
// The pattern's entry in the process-wide cache is found once per matchWhere
// and pattern, under the first reading it is matched with. A match under a
// different reading — a substitution's one whole-subject question about the
// empty string, say — is interpreted rather than given an entry of its own,
// so two readings alternating on one matchWhere cannot thrash.
func (w *matchWhere) program(pattern string, o *patternOpts) *patternProgram {
	if !patternCompiles {
		return nil
	}
	rd := readingOf(o)
	e := w.entry
	if e == nil {
		e = programs.entry(pattern, &rd)
		w.entry = e
	} else if e.reading != rd {
		return nil
	}
	if prog := e.prog.Load(); prog != nil {
		if w.prog != prog {
			w.prog = prog
			w.resetCompiledDeadEnds()
		}
		return prog
	}
	if e.declined.Load() || e.spent.Load() < int64(compileThreshold) {
		return nil
	}
	prog, ok := compilePattern(pattern, *o)
	if !ok {
		e.declined.Store(true)
		return nil
	}
	e.prog.Store(prog)
	w.prog = prog
	w.resetCompiledDeadEnds()
	return prog
}

// spend charges the interpreter's steps on this matchWhere's pattern to its
// entry, which is what decides when the pattern is worth compiling.
func (w *matchWhere) spend(steps int) {
	if w.entry != nil {
		w.entry.spent.Add(int64(steps))
	}
}

// forgetProgram drops the program, for a matchWhere whose pattern has moved.
func (w *matchWhere) forgetProgram() {
	w.prog, w.entry = nil, nil
	w.resetCompiledDeadEnds()
}

// programCache holds every pattern's entry in this process, by pattern text
// and then by reading.
//
// A matchWhere lives for one resolution of a pattern — one substitution, one
// directory walk, one `case` arm — and a script resolves the same pattern
// afresh every time round a loop. So what a pattern has cost, and its program
// once it has one, are kept here rather than there: a substitution a loop
// runs two hundred times compiles once, and a `case` arm too cheap to compile
// on any one pass is compiled once its passes have added up.
//
// A program holds nothing about any one match: the state its nodes carry is
// replaced from the match running before it is read (see
// cexec.withMatchState), and is cleared when it is compiled so that a cached
// program keeps nothing of a finished match alive.
//
// Asked once per pattern per matchWhere, which for a `case` arm is once per
// match, so the read is kept cheap: a lock-free map from the pattern text to
// the readings it has been matched under, which is nearly always one. It is
// bounded, because a script can build any number of distinct patterns, and
// emptied rather than evicted piecemeal when it fills: the patterns worth
// keeping are the ones a loop asks for again, and those come straight back.
// A matchWhere holding an entry from before the emptying keeps using it.
type programCache struct {
	m     atomic.Pointer[sync.Map]
	count atomic.Int64
}

// programFamily is one pattern text's entries, one per reading, replaced
// whole when one is added so that a reader never takes a lock.
type programFamily struct {
	mu      sync.Mutex
	entries atomic.Pointer[[]*programEntry]
}

// programEntry is one pattern under one reading. Its counters are shared by
// every Runner in the process, which is why they are atomics.
type programEntry struct {
	reading  patternReading
	prog     atomic.Pointer[patternProgram]
	spent    atomic.Int64
	declined atomic.Bool
}

// programCacheSize is how many patterns the cache holds before it empties.
const programCacheSize = 512

var programs programCache

// entry answers the entry for pattern under rd, making one on a miss.
func (c *programCache) entry(pattern string, rd *patternReading) *programEntry {
	m := c.m.Load()
	if m == nil {
		c.m.CompareAndSwap(nil, &sync.Map{})
		m = c.m.Load()
	}
	f, ok := m.Load(pattern)
	if !ok {
		if c.count.Add(1) > programCacheSize {
			c.count.Store(1)
			m = &sync.Map{}
			c.m.Store(m)
		}
		f, _ = m.LoadOrStore(pattern, &programFamily{})
	}
	fam, isFamily := f.(*programFamily)
	if !isFamily {
		// Nothing else is ever stored, so this is unreachable; an entry of
		// its own is a correct answer that merely is not shared.
		return &programEntry{reading: *rd}
	}
	if list := fam.entries.Load(); list != nil {
		for _, e := range *list {
			if e.reading == *rd {
				return e
			}
		}
	}
	fam.mu.Lock()
	defer fam.mu.Unlock()
	var list []*programEntry
	if old := fam.entries.Load(); old != nil {
		for _, e := range *old {
			if e.reading == *rd {
				return e
			}
		}
		list = append(list, *old...)
	}
	e := &programEntry{reading: *rd}
	list = append(list, e)
	fam.entries.Store(&list)
	return e
}

// compiledDeadEnd and rememberCompiledDeadEnd are the memo of cexec.here,
// packed as the interpreter's is where the subject fits and in a map where it
// does not. See deadSet.
func (w *matchWhere) compiledDeadEnd(id uint64, at, end int) bool {
	if w.total < packBase {
		return w.cdead.has(packProgramKey(id, at, end))
	}
	_, dead := w.cdeadWide[[3]uint64{id, uint64(at), uint64(end)}]
	return dead
}

func (w *matchWhere) rememberCompiledDeadEnd(id uint64, at, end int) {
	if w.total < packBase {
		w.cdead.add(packProgramKey(id, at, end))
		return
	}
	if w.cdeadWide == nil {
		w.cdeadWide = make(map[[3]uint64]struct{})
	}
	w.cdeadWide[[3]uint64{id, uint64(at), uint64(end)}] = struct{}{}
}

func (w *matchWhere) resetCompiledDeadEnds() {
	w.cdead.reset()
	w.cdeadWide = nil
}

// byteStrings is every one-byte string, so that a bracket can be asked about
// each byte without building one per question.
var byteStrings = func() (t [256]string) {
	for b := range t {
		t[b] = string([]byte{byte(b)})
	}
	return t
}()

// packProgramKey folds a piece and a span into one key. Both positions are
// below packBase where it is used, so every question gets its own number.
func packProgramKey(id uint64, at, end int) uint64 {
	return (id*packBase+uint64(at))*packBase + uint64(end)
}
