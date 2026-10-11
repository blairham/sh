// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a match starting at one position of a substitution could end.
//
// `${v//pat/rep}` wants the longest match at each position, and finds it by
// handing the matcher every span from the longest down and taking the first
// it accepts. For a pattern with no upper bound on what it consumes that is
// one whole match attempt per remaining unit of the subject, at every
// position, and nearly all of them fail: #1398's prompt-theme pattern makes
// 285 attempts and 44,000 matcher steps to find nine matches in 82 bytes
// (#6356).
//
// This file answers the question those attempts are asking — *which* spans
// starting here could the pattern fill — in one forward pass over the
// subject, so that the walk asks the matcher only about those.
//
// **It never decides a match.** That is the hazard #1398 recorded against an
// extent-reporting matcher: the matcher explores in an order that decides
// what a `(#b)` reports, and its first success is not the longest one, so a
// replacement that took a different engine's answer would be a different
// substitution. Here the matcher is still asked about every span this says
// is possible, in the same order as before, and its answer is the one used.
// The pass only removes spans from the walk, and the property it has to keep
// is one-sided: **every span the matcher would accept is in the set.** A set
// that is too large costs the attempts it failed to skip; one that is too
// small drops a replacement. TestTheReachSetHoldsEveryEndTheMatcherAccepts
// compares it against an enumeration of every span.
//
// The pass reads the pattern with the matcher's own readers — splitGroup,
// splitClosableItem, closureBounds, bracketEnd, matchBracket — in the order
// matchBranch asks them, so the two cannot read the same text two ways. And
// it reads only a subset of the language: anything outside it, and every
// option that changes how a byte compares, refuses the analysis and leaves
// the walk exactly as it was. The subset is what a substitution in a prompt
// theme is written in — literals, `?`, `*`, bracket expressions, groups with
// alternation, the `#` and `##` closures and the quantified groups, and the
// reporting flags.

// reachDisabled turns the analysis off, so that a test can run a
// substitution both ways and compare. Nothing outside a test writes it.
var reachDisabled bool

type reachKind uint8

const (
	// reachByte consumes one byte equal to b.
	reachByte reachKind = iota
	// reachUnit consumes one unit, whatever it holds: a `?`, or the unit a
	// `*` repeats.
	reachUnit
	// reachBracket consumes one unit the bracket expression text admits.
	reachBracket
	// reachSplit consumes nothing and goes to both out and alt.
	reachSplit
	// reachAccept is the end of the pattern.
	reachAccept
)

type reachNode struct {
	kind reachKind
	b    byte
	// text is a bracket expression and everything behind it, which is the
	// shape matchBracket takes, and after is how much of it the expression
	// leaves — the check that matchBracket read the bracket where this
	// analysis did.
	text     string
	after    int
	out, alt int
}

// reachFrag is a piece of the automaton with its entry and the slots still
// waiting to be pointed at whatever follows it. A hole is a node's index
// times two, plus one where it is the node's alt rather than its out.
type reachFrag struct {
	start int
	holes []int
}

// reachSet is a pattern read into an automaton: a Thompson construction,
// whose states are simulated together so that no position is read twice.
type reachSet struct {
	nodes []reachNode
	start int
	o     patternOpts
	// The simulation's scratch, reused from one position to the next.
	cur     []int
	stamp   []int
	gen     int
	slots   [5][]int
	slotGen [5][]int
	// base keeps one call's queue marks from being read as another's: a
	// mark is base plus the offset plus one, and base moves past every
	// offset a call could have written.
	base int
}

// newReachSet reads pattern for the analysis, or reports that it cannot.
func newReachSet(pattern string, o patternOpts) (*reachSet, bool) {
	if reachDisabled || o.tilde || o.tildeFold || o.fold || o.litFold != caseExact ||
		o.period || o.classEscapes || o.matchSkipsToAnyBracket || o.bracketMember {
		return nil, false
	}
	// The prepared tables are keyed on positions in the pattern the matcher
	// was handed, and the readers below are asked about the same text — but
	// a scan from scratch is what cannot be wrong about it.
	o.where = nil
	r := &reachSet{o: o}
	var f reachFrag
	var ok bool
	if o.topGroup {
		arms, armAt := topAlternatives(pattern, o.emptyBracket, o.bareParenIsText)
		f, ok = r.alternation(arms, armAt, o)
	} else {
		f, ok = r.branch(pattern, 0, o)
	}
	if !ok {
		return nil, false
	}
	r.patch(f, r.add(reachNode{kind: reachAccept}))
	r.start = f.start
	r.stamp = make([]int, len(r.nodes))
	for i := range r.slotGen {
		r.slotGen[i] = make([]int, len(r.nodes))
	}
	return r, true
}

func (r *reachSet) add(n reachNode) int {
	n.out, n.alt = -1, -1
	r.nodes = append(r.nodes, n)
	return len(r.nodes) - 1
}

func (r *reachSet) patch(f reachFrag, to int) {
	for _, h := range f.holes {
		if h&1 == 0 {
			r.nodes[h>>1].out = to
		} else {
			r.nodes[h>>1].alt = to
		}
	}
}

func outHole(i int) int { return i << 1 }
func altHole(i int) int { return i<<1 | 1 }

// unit is a fragment of one consuming node.
func (r *reachSet) unit(n reachNode) reachFrag {
	i := r.add(n)
	return reachFrag{start: i, holes: []int{outHole(i)}}
}

func (r *reachSet) concat(a, b reachFrag) reachFrag {
	if a.start < 0 {
		return b
	}
	r.patch(a, b.start)
	return reachFrag{start: a.start, holes: b.holes}
}

// empty is the fragment that matches nothing at all and consumes nothing.
func (r *reachSet) empty() reachFrag {
	i := r.add(reachNode{kind: reachSplit})
	// A split whose both arms are the same hole is an epsilon.
	return reachFrag{start: i, holes: []int{outHole(i), altHole(i)}}
}

func (r *reachSet) either(a, b reachFrag) reachFrag {
	i := r.add(reachNode{kind: reachSplit})
	r.nodes[i].out, r.nodes[i].alt = a.start, b.start
	return reachFrag{start: i, holes: append(append([]int(nil), a.holes...), b.holes...)}
}

// star is zero or more of f.
func (r *reachSet) star(f reachFrag) reachFrag {
	i := r.add(reachNode{kind: reachSplit})
	r.nodes[i].out = f.start
	r.patch(f, i)
	return reachFrag{start: i, holes: []int{altHole(i)}}
}

// plus is one or more of f.
func (r *reachSet) plus(f reachFrag) reachFrag {
	i := r.add(reachNode{kind: reachSplit})
	r.nodes[i].out = f.start
	r.patch(f, i)
	return reachFrag{start: f.start, holes: []int{altHole(i)}}
}

// optional is zero or one of f.
func (r *reachSet) optional(f reachFrag) reachFrag {
	i := r.add(reachNode{kind: reachSplit})
	r.nodes[i].out = f.start
	return reachFrag{start: i, holes: append(f.holes, altHole(i))}
}

func (r *reachSet) alternation(arms []string, armAt []int, o patternOpts) (reachFrag, bool) {
	var all reachFrag
	for k, a := range arms {
		f, ok := r.branch(a, armAt[k], o)
		if !ok {
			return reachFrag{}, false
		}
		if k == 0 {
			all = f
		} else {
			all = r.either(all, f)
		}
	}
	return all, true
}

// branch reads one branch of a pattern, asking the matcher's readers in the
// order matchBranch asks them. Every construct matchBranch reads that is not
// handled here refuses.
func (r *reachSet) branch(p string, pp int, o patternOpts) (reachFrag, bool) {
	f := reachFrag{start: -1}
	for len(p) > 0 {
		if o.extended {
			if _, _, ok := splitExclusion(p, &o); ok {
				return reachFrag{}, false
			}
			if p[0] == '^' {
				return reachFrag{}, false
			}
			if body, rest, ok := splitPatternFlags(p, o.emptyBracket); ok {
				if anchorPatternFlag(body) != anchorNone {
					return reachFrag{}, false
				}
				next, unknown := applyPatternFlags(body, o)
				if unknown != 0 || next.litFold != caseExact {
					return reachFrag{}, false
				}
				o, pp, p = next, pp+len(p)-len(rest), rest
				continue
			}
			if item, rest, ok := splitClosableItem(p, pp, &o); ok {
				if lo, hi, after, isClosure := closureBounds(rest, &o); isClosure {
					if hi != unboundedRepeat || lo > 1 {
						return reachFrag{}, false
					}
					g, ok := r.branch(item, pp, o)
					if !ok {
						return reachFrag{}, false
					}
					if lo == 0 {
						g = r.star(g)
					} else {
						g = r.plus(g)
					}
					f = r.concat(f, g)
					pp, p = pp+len(p)-len(after), after
					continue
				}
			}
		}
		if _, ok := splitCountedGroup(p, pp, &o); ok {
			return reachFrag{}, false
		}
		if body, quant, rest, ok := splitGroup(p, pp, &o); ok {
			lead := 0
			if quant != 0 {
				lead = 1
			}
			armOpts := o
			armOpts.noBareGroup = false
			arms, armAt := alternativesAt(body, pp+lead+1, o.emptyBracket)
			g, ok := r.alternation(arms, armAt, armOpts)
			if !ok {
				return reachFrag{}, false
			}
			switch quant {
			case 0, '@':
			case '*':
				g = r.star(g)
			case '+':
				g = r.plus(g)
			case '?':
				g = r.optional(g)
			default:
				return reachFrag{}, false
			}
			f = r.concat(f, g)
			o.noBareGroup = !o.group && !o.operandParens && !o.armParens
			pp, p = pp+len(p)-len(rest), rest
			continue
		}
		if _, _, _, ok := splitNumericRange(p, &o); ok {
			return reachFrag{}, false
		}
		switch p[0] {
		case '*':
			p, pp = p[1:], pp+1
			for len(p) > 0 && p[0] == '*' && !quantifierOpensAGroup(p, o) {
				p, pp = p[1:], pp+1
			}
			f = r.concat(f, r.star(r.unit(reachNode{kind: reachUnit})))
		case '?':
			f = r.concat(f, r.unit(reachNode{kind: reachUnit}))
			p, pp = p[1:], pp+1
		case '[':
			end, ok := bracketSpanEnd(p, 0, o.emptyBracket)
			if !ok {
				return reachFrag{}, false
			}
			f = r.concat(f, r.unit(reachNode{kind: reachBracket, text: p, after: len(p) - end - 1}))
			p, pp = p[end+1:], pp+end+1
		case '\\':
			if len(p) < 2 {
				return reachFrag{}, false
			}
			if !o.escapeReaches(p[1]) {
				f = r.concat(f, r.unit(reachNode{kind: reachByte, b: '\\'}))
				p, pp = p[1:], pp+1
				continue
			}
			f = r.concat(f, r.unit(reachNode{kind: reachByte, b: p[1]}))
			p, pp = p[2:], pp+2
		default:
			// One byte of pattern for one of subject. Where a unit is a
			// character the matcher compares whole characters, and two equal
			// runs of bytes are equal characters, so a match there is a match
			// here — the reverse need not hold, which costs only attempts.
			f = r.concat(f, r.unit(reachNode{kind: reachByte, b: p[0]}))
			p, pp = p[1:], pp+1
		}
	}
	if f.start < 0 {
		return r.empty(), true
	}
	return f, true
}

// ends marks in end every offset j for which value[i:j] could be one the
// pattern fills, and reports false where it could not say — a unit read
// somewhere that is not a unit boundary, or a bracket matchBracket read
// differently — in which case every end has to be treated as possible.
//
// stop says which offsets are unit boundaries, and is nil where every offset
// is one. end is indexed by offset and must be all false on entry; the
// offsets set are appended to marked so that the caller can clear them.
func (r *reachSet) ends(value string, i int, stop, end []bool, marked []int) ([]int, bool) {
	// Five slots, because no step moves further than a unit and a unit is
	// at most four bytes: the slot for offset x is x%5, and the slot about
	// to be read is always the lowest offset still pending.
	for k := range r.slots {
		r.slots[k] = r.slots[k][:0]
	}
	r.base += len(value) + 2
	pending := r.push(i, r.start)
	for x := i; x <= len(value) && pending > 0; x++ {
		slot := x % 5
		if len(r.slots[slot]) == 0 {
			continue
		}
		pending--
		r.gen++
		r.cur = r.cur[:0]
		for _, s := range r.slots[slot] {
			r.closure(s)
		}
		r.slots[slot] = r.slots[slot][:0]
		for _, s := range r.cur {
			n := &r.nodes[s]
			switch n.kind {
			case reachAccept:
				if !end[x] {
					end[x] = true
					marked = append(marked, x)
				}
			case reachByte:
				if x < len(value) && value[x] == n.b {
					pending += r.push(x+1, n.out)
				}
			case reachUnit, reachBracket:
				if x == len(value) {
					continue
				}
				if stop != nil && !stop[x] {
					return marked, false
				}
				w := r.o.unitWidth(value[x:])
				if n.kind == reachBracket {
					rest, ok := matchBracket(n.text, value[x:x+w], &r.o)
					if len(rest) != n.after {
						return marked, false
					}
					if !ok {
						continue
					}
				}
				pending += r.push(x+w, n.out)
			}
		}
	}
	return marked, true
}

// reachEnds is a reachSet bound to one subject, with the scratch a walk over
// that subject's positions reuses from one position to the next.
type reachEnds struct {
	r      *reachSet
	value  string
	stop   []bool
	end    []bool
	marked []int
	sure   bool
}

// newReachEnds binds r to value, whose unit boundaries are stops.
func newReachEnds(r *reachSet, value string, stops []int) *reachEnds {
	e := &reachEnds{r: r, value: value, end: make([]bool, len(value)+1)}
	if len(stops) != len(value)+1 {
		e.stop = make([]bool, len(value)+1)
		for _, x := range stops {
			e.stop[x] = true
		}
	}
	return e
}

// from works out the ends of a match beginning at i, and reports whether
// there could be any at all.
func (e *reachEnds) from(i int) bool {
	for _, x := range e.marked {
		e.end[x] = false
	}
	e.marked, e.sure = e.r.ends(e.value, i, e.stop, e.end, e.marked[:0])
	return !e.sure || len(e.marked) > 0
}

// may is whether the span from the last position asked to j could be one the
// pattern fills.
func (e *reachEnds) may(j int) bool {
	return !e.sure || e.end[j]
}

// push queues state s at offset x, reporting 1 where that slot had been
// empty, which is what pending counts.
func (r *reachSet) push(x, s int) int {
	slot := x % 5
	gen := r.base + x + 1
	if r.slotGen[slot][s] == gen {
		return 0
	}
	r.slotGen[slot][s] = gen
	was := len(r.slots[slot])
	r.slots[slot] = append(r.slots[slot], s)
	return boolInt(was == 0)
}

// closure adds s and everything reachable from it without consuming.
func (r *reachSet) closure(s int) {
	if s < 0 || r.stamp[s] == r.gen {
		return
	}
	r.stamp[s] = r.gen
	n := &r.nodes[s]
	if n.kind == reachSplit {
		r.closure(n.out)
		r.closure(n.alt)
		return
	}
	r.cur = append(r.cur, s)
}
