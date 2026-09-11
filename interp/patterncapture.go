// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// The half of the position-aware flag family that has to *report* a position
// rather than merely ask about one: `(#b)` and `(#B)` fill `$match`,
// `$mbegin` and `$mend` from the groups of a pattern, and `(#m)` and `(#M)`
// fill `$MATCH`, `$MBEGIN` and `$MEND` from the whole of it.
//
// docs/spec/grammar/patterns.md records the measurements. Three of them shape
// everything here:
//
//   - **A group's number is a fact about the pattern text**, not about the
//     path the matcher took. `[[ abc == (#b)((x)|a(b)c) ]]` numbers `(x)` as
//     2 and reports it as empty, even though the arm holding it is never
//     taken. So the numbering is a pre-pass over the pattern, keyed by where
//     each `(` stands, and the matcher looks a group up rather than counting
//     as it goes.
//   - **A group that did not participate is the empty string with -1 for its
//     bounds**, which is why a dropped `(#b)` was never acceptable: empty is
//     a real answer here and reads exactly like a feature that is absent.
//   - **Nothing is written unless the pattern matched and had a group.**
//     `match=(zz); [[ abc == (#b)abc ]]` leaves `match` as `(zz)`, and so
//     does a `(#b)` pattern that fails.

// capSpan is where one group matched, in bytes of the subject.
//
// It is bytes here and characters by the time it reaches a parameter: the two
// differ, and which one a script sees is measured — `x=aébc;
// ${x//(#m)?/<$MATCH:$MBEGIN>}` reports `<é:2>`, so the é is the second
// *character* and not the second byte.
type capSpan struct {
	begin, end int
	set        bool
}

// capturePlan is the backreference number of every group in a pattern, by the
// byte offset of its opening parenthesis.
//
// count is the number of groups the pattern has, which is what fills `$match`
// with an entry per group whether or not each one was reached.
type capturePlan struct {
	index map[int]int
	count int
	// whole says the pattern asked for `$MATCH` — a `(#m)` in effect at the
	// end of the pattern's own top level. See planCaptures for why that is
	// the reading, and where it diverges.
	whole bool
}

// reports says the pattern fills a parameter when it matches, which is what
// decides whether a replacement has to be expanded again for each match.
func (p capturePlan) reports() bool { return p.count > 0 || p.whole }

// matchWhere is everything the position-aware flags need that is *not* about
// how a pattern reads: how long the whole subject is, which groups it
// numbers, and where a match reports itself.
//
// It is held behind a pointer, and that is a measurement rather than a style.
// [patternOpts] is threaded **by value** through a recursive matcher, so
// every byte of it is copied on every step and again into each of the
// helpers that take one. Inlining these three fields grew the struct from 72
// bytes to 104 and cost four times the run time of `${v//abc/xyz}` over a
// value using none of them — 0.12s to 0.49s for twenty passes over 1200
// characters, measured with the shell binary and confirmed in a profile,
// where the tell was `splitGroup` tripling in cost without its body
// changing: its whole cost is the argument copy.
//
// The pointer is made where the pattern is resolved and reused for every
// trial against the same subject, so a trim that tries a thousand prefixes
// allocates once rather than a thousand times.
type matchWhere struct {
	// total is the length of the whole subject, which is not always the
	// length of the string handed to the matcher: `${x#pat}` tries the
	// prefixes of x and each trial is a piece. It is what `(#e)` compares a
	// position against.
	total int
	// plan is the group numbering this pattern's `(#b)` implies, worked out
	// once rather than on every trial.
	plan capturePlan
	// caps is where the groups of the trial now running report themselves,
	// and is replaced for each trial.
	caps *captures
	// asked counts the questions put to matchHere against the pattern and
	// subject below, and dead is the ones that came back false. See
	// memoThreshold for why the count exists at all.
	asked int
	dead  deadSet
	// deadWide is the same memo for a pattern or subject too large to pack
	// into one integer. Two maps rather than one, and the reason is
	// measured: Go specializes a map whose key is a single 64-bit integer
	// and that specialization is the whole of the win here — 1.14s to 0.78s
	// on the #1383 configuration. Narrowing the struct's fields to uint32
	// instead, which is the obvious cheaper change, bought nothing at all
	// (1.13s), because what costs is hashing a struct rather than the
	// number of bytes in it.
	//
	// The fallback exists rather than a size limit because the alternative
	// to a memo is not slowness, it is the startup that never finished. A
	// 64KB subject in a substitution is an ordinary thing for a script to
	// produce, and it must not quietly become unbounded work — nor may the
	// packing silently alias two questions into one, which is what a
	// fixed-width field does when the value overflows it.
	deadWide map[matchKey]struct{}
	// packable is whether this pattern and subject fit the packed key, asked
	// once per match rather than per question.
	packable bool
	// noTilde and noParen are facts about the *whole* pattern,
	// worked out once when it is set rather than rediscovered at every
	// position.
	//
	// Every string matchHere is handed is a substring of the pattern — the
	// same fact matchKey is built on — so a character absent from the whole
	// is absent from every piece, and a negative answer here is a negative
	// answer everywhere. They are only ever read to *skip* a scan, never to
	// take a branch, so the direction that could be wrong is the one that
	// says "not present" for a pattern that has one, which a Contains over
	// the whole text cannot do.
	//
	// This is #1398's first half. The prescans matchBranch opens with —
	// splitExclusion above all, which walked the remaining pattern looking
	// for a `~` most patterns do not contain at all — were between a fifth
	// and a quarter of the samples on the prompt-theme substitution, and
	// every one of them was rediscovering the same absence at every
	// position of every trial.
	noTilde bool
	noParen bool
	// parenEnd and brackEnd are, for each byte of the pattern that opens a
	// group or a bracket expression, where it closes — worked out once for
	// the whole pattern instead of by a scan from every position.
	//
	// Absolute offsets into the pattern, and -1 where nothing closes. A
	// lookup is only *believed* when the answer lies inside the piece being
	// matched; a group body or an arm is a truncation of the pattern rather
	// than a suffix of it, and a closer past the end of the piece is not a
	// closer at all. That case falls back to the scan, which is exact, so
	// the cache can only ever save work and never change an answer.
	parenEnd []int32
	brackEnd []int32
	// reachAt and reachLen memoize patternReach, which is asked of the same
	// item at every level of a closure's recursion — `[^\}]##` asks it once
	// per repetition and per position, and the answer is a fact about six
	// bytes of pattern that cannot move.
	//
	// Two arrays rather than a map keyed on both: the item is always the
	// pattern from some offset for some length, so the offset indexes and
	// the length says whether the entry is about this item. -1 in reachLen
	// is "not asked yet", which is a length no item has.
	reachAt  []int32
	reachLen []int32
	// arms is each group body's split into alternatives, by where the body
	// starts. See armsOf.
	arms map[int]armSplit
	// pattern and subject are what dead's answers are *about*, and what
	// says when they have to be thrown away.
	//
	// Not per trial, which is the whole point and was worth a measurement to
	// learn. A trim tries every prefix of one subject and each prefix is a
	// trial, so a memo dropped per trial is rebuilt from nothing several
	// hundred times over — 509 times, in the configuration #1383 came from,
	// which is why that startup still did not finish once the constructs in
	// front of it stopped refusing and the string reached its full length.
	//
	// Sharing them is sound because a key indexes the **subject**, not the
	// piece: matchHere's `at` is an absolute offset into the subject and `s`
	// is always a slice of it, so two trials asking (pp, plen, at, slen) are
	// asking about literally the same two substrings and must get the same
	// answer. `(#e)` is the case that could have broken it and does not —
	// it compares a position against total, which is len(subject) and so is
	// the same in every trial, which is exactly why total is the subject's
	// length rather than the piece's.
	//
	// They are compared rather than assumed equal because one options value
	// is reused across *different* subjects too: pathname expansion matches
	// every name in a directory with one of them. That is the direction that
	// gives wrong answers rather than slow ones, so the check is the guard,
	// not an optimisation of it.
	pattern string
	subject string
	// ready also says whether the prepared tables above are about the
	// pattern now being matched. Everything that reads one asks it first,
	// because a caller may build options and scan a pattern with them before
	// any match has run — see scanExtendedPattern, which reads a group with
	// no offset to give.
	//
	// ready distinguishes "no pattern and no subject seen yet" from "the
	// empty pattern against the empty subject", which are the same two
	// strings and not the same state. Without it a matchWhere's zero value
	// *is* a legitimate pair, so the first match of `""` against `""` skips
	// the setup below it and runs with packable false — harmless there, and
	// exactly the kind of accident that stops being harmless when someone
	// adds a third field to this block.
	ready bool
}

// matchKey names one question put to matchHere, exactly.
//
// A question is "does the pattern from pp, for plen bytes, match the subject
// from at, for slen bytes, folding literals this way" — and those five
// numbers are the whole of it, which is a fact about the matcher that had to
// be established rather than assumed:
//
//   - Every non-empty string matchHere is handed is a *substring* of the
//     pattern or of the subject, never a fresh one, so a (offset, length)
//     pair names it. Measured: instrumented against the whole of interp's
//     and every dialect's tests, and against the configuration in #1383,
//     not one string of either kind fell outside its root.
//   - pp and at are those offsets and not approximations of them. Same
//     instrumentation, comparing each against the offset recovered from the
//     string's own data pointer: zero disagreements in 172,647 pattern and
//     189,208 subject checks on the #1383 pattern alone. They are exact
//     because the backreference numbering already depends on it — a `(#b)`
//     group's number is where its `(` stands in the pattern text.
//   - litFold is the only field of patternOpts a recursion can change.
//     applyPatternFlags writes that and nothing else; `b`, `B`, `m` and `M`
//     are read by planCaptures rather than by the matcher, and every other
//     field is the dialect's answer, fixed before the match began.
//
// litFold is in the key **defensively**, and that is worth stating plainly
// rather than leaving as an implied claim. Removing it survives every test
// here, and the search for a case where it matters came back empty: one key
// is reached under two different foldings often enough — 9,686 times over
// this package's own patterns — but never at a position where the answer
// could depend on it, and 400,000 randomly generated patterns of flags,
// alternation and closures over case-varying subjects produced no
// disagreement. That fits what the code says: the folding in force at a
// pattern position is decided by the flags textually in front of it inside
// the enclosing arm, and matchGroup hands the tail after a group the *outer*
// options, so a flag never leaks out of the arm that set it and the folding
// at a position is path-independent.
//
// It stays because it costs nothing — the key is compared as a unit either
// way — and because the property keeping it unnecessary is a subtle one
// nothing enforces. If flag scoping ever changes, a key without this field
// answers with the wrong folding and reports no error at all.
//
// The subject and pattern themselves are not in the key because a trial is
// against one of each, and the memo is reset when the trial is.
type matchKey struct {
	pp, plen, at, slen int
	fold               caseFolding
}

// packBase is the largest pattern or subject length the packed key admits.
//
// Four fields at 15 bits each and the folding in two leaves the product below
// 2^62, so the arithmetic below is exact rather than nearly exact — every
// distinct question gets a distinct number, which is the only property a memo
// key needs and the one a bit-shift silently loses on overflow.
const packBase = 1 << 15

// known reports whether this question has already been answered false.
func (w *matchWhere) known(pp, plen, at, slen int, f caseFolding) bool {
	if w.packable {
		return w.dead.has(packKey(pp, plen, at, slen, f))
	}
	_, dead := w.deadWide[matchKey{pp: pp, plen: plen, at: at, slen: slen, fold: f}]
	return dead
}

// remember writes down that this question came back false.
func (w *matchWhere) remember(pp, plen, at, slen int, f caseFolding) {
	if w.packable {
		w.dead.add(packKey(pp, plen, at, slen, f))
		return
	}
	if w.deadWide == nil {
		w.deadWide = make(map[matchKey]struct{})
	}
	w.deadWide[matchKey{pp: pp, plen: plen, at: at, slen: slen, fold: f}] = struct{}{}
}

// deadSet is the memo's store: an open-addressed set of packed keys.
//
// A Go map was what #1383 used and it answered the question — the blowup went
// with it — but the *map* then became the cost rather than the questions it
// answers. On the prompt-theme substitution of #1398 it was near half the
// samples: hashing, probing, and the rehash of a table that grows to a
// hundred thousand entries in one substitution.
//
// The keys are already one uint64 each and a dead end is a *set* membership
// with no value to carry, so almost everything a map does here is unused.
// Linear probing over a flat slice removes the indirection, the bucket
// metadata and the per-entry allocation, and keeps the property that matters:
// the key is exact, so a hit is the same question and never a collision.
//
// Zero is the empty slot, so the one key that packs to zero — the whole
// pattern against the whole subject at the start, unfolded — is held in a
// field of its own rather than given a sentinel.
//
// The store is **reused** across matches rather than reallocated. A pattern
// matched against every name in a directory resets once per name, and a reset
// that has nothing to clear costs nothing.
type deadSet struct {
	slots []uint64
	mask  uint64
	count int
	zero  bool
}

// deadSetMin is the smallest table worth allocating. The memo does not engage
// until a trial has asked memoThreshold questions, so a table is only ever
// built for a match that is already in the thousands.
const deadSetMin = 1 << 12

func (d *deadSet) has(k uint64) bool {
	if k == 0 {
		return d.zero
	}
	if len(d.slots) == 0 {
		return false
	}
	for i := mixKey(k) & d.mask; ; i = (i + 1) & d.mask {
		switch d.slots[i] {
		case 0:
			return false
		case k:
			return true
		}
	}
}

func (d *deadSet) add(k uint64) {
	if k == 0 {
		d.zero = true
		return
	}
	// Grown at three quarters full, which keeps the probe runs short without
	// doubling more often than a match needs.
	if len(d.slots) == 0 {
		d.resize(deadSetMin)
	} else if (d.count+1)*4 > len(d.slots)*3 {
		d.resize(len(d.slots) * 2)
	}
	d.insert(k)
}

func (d *deadSet) insert(k uint64) {
	for i := mixKey(k) & d.mask; ; i = (i + 1) & d.mask {
		switch d.slots[i] {
		case 0:
			d.slots[i], d.count = k, d.count+1
			return
		case k:
			return
		}
	}
}

func (d *deadSet) resize(n int) {
	old := d.slots
	d.slots, d.mask, d.count = make([]uint64, n), uint64(n-1), 0
	for _, k := range old {
		if k != 0 {
			d.insert(k)
		}
	}
}

// deadSetKeep is the largest table a reset clears rather than gives up.
//
// Both directions cost something and the sizes decide which. One options
// value is reused across every name in a directory, so a reset happens once
// per *subject*, and clearing a table that has grown to a megabyte on one
// pathological subject would charge every later name for it. Below the cap,
// keeping the memory is the cheaper half — a substitution resets once and
// then fills the table again.
const deadSetKeep = 1 << 14

// reset empties the set, keeping the memory it has grown into where that is
// the cheaper of the two. A set nothing wrote to has nothing to do at all,
// which is the ordinary case: most patterns never reach the memo.
func (d *deadSet) reset() {
	d.zero = false
	if d.count == 0 {
		return
	}
	if len(d.slots) > deadSetKeep {
		d.slots, d.mask, d.count = nil, 0, 0
		return
	}
	clear(d.slots)
	d.count = 0
}

// mixKey spreads a packed key over the table. The packing is a mixed-radix
// number, so its low bits move with one field and its high bits with
// another; taking the low bits of it directly would put every question about
// one position in one run of slots.
func mixKey(k uint64) uint64 {
	k ^= k >> 33
	k *= 0xff51afd7ed558ccd
	k ^= k >> 29
	return k
}

// packKey folds the five numbers into one, and is only ever called where
// packable said they fit.
func packKey(pp, plen, at, slen int, f caseFolding) uint64 {
	n := uint64(pp)
	n = n*packBase + uint64(plen)
	n = n*packBase + uint64(at)
	n = n*packBase + uint64(slen)
	return n*4 + uint64(f)
}

// captures is where a match reports itself, and is shared by pointer because
// the matcher threads its options by value.
//
// journal makes backtracking cheap and correct: the matcher writes a span the
// moment a group and everything after it have matched, and unwinds the
// journal when the attempt around it turns out not to be the one that wins.
// Without it a group that matched down a branch the subject later left would
// keep its span — a plausible wrong answer, and one no status would report.
type captures struct {
	plan    capturePlan
	spans   []capSpan
	journal []capUndo
}

type capUndo struct {
	slot int
	was  capSpan
}

// newCaptures is the state for one pattern, or nil where the pattern asks for
// nothing — which is the common case and costs nothing at all.
func newCaptures(plan capturePlan) *captures {
	if plan.count == 0 {
		return nil
	}
	return &captures{plan: plan, spans: make([]capSpan, plan.count)}
}

// mark is a point the journal can be wound back to.
func (c *captures) mark() int {
	if c == nil {
		return 0
	}
	return len(c.journal)
}

// rollback undoes every span written since mark.
func (c *captures) rollback(mark int) {
	if c == nil {
		return
	}
	for i := len(c.journal) - 1; i >= mark; i-- {
		c.spans[c.journal[i].slot] = c.journal[i].was
	}
	c.journal = c.journal[:mark]
}

// record notes where the group whose `(` stands at offset gp matched.
//
// A later write wins, which is measured rather than incidental: a group under
// a closure reports its **last** repetition, so `[[ abab == (#b)(ab)# ]]`
// answers `mbegin=(3)` and not `(1)`. The repetitions record in order, so
// overwriting is what leaves the last one standing.
func (c *captures) record(gp, begin, end int) {
	if c == nil {
		return
	}
	slot, ok := c.plan.index[gp]
	if !ok {
		return
	}
	c.journal = append(c.journal, capUndo{slot: slot, was: c.spans[slot]})
	c.spans[slot] = capSpan{begin: begin, end: end, set: true}
}

// planCaptures numbers the groups of a pattern and says whether it asked for
// the whole match.
//
// The walk mirrors the matcher's own reading of a pattern, because a `(` that
// the matcher would not treat as a group must not be numbered as one — a
// bracket expression holding a parenthesis is the case that says so.
//
// `(#b)` and `(#B)` are **scoped to the group they stand in**, exactly as the
// case flags are: measured, `[[ abc == (#b)(a)((#B)(b))(c) ]]` reports three
// groups and not four, because the `(#B)` inside the second one does not
// reach the third.
func planCaptures(pattern string, o patternOpts) capturePlan {
	if !o.extended {
		return capturePlan{}
	}
	pl := capturePlan{index: map[int]int{}}
	pl.whole = planWalk(pattern, 0, false, false, o, &pl)
	return pl
}

// planWalk reads one branch of a pattern, numbering the groups it finds.
//
// capturing and whole are the two flags as they stand where the branch
// begins; the returned value is `whole` as it stands where the branch ends,
// which is what the *top level* of a pattern is asked for and what a nested
// group's answer is deliberately thrown away.
func planWalk(p string, base int, capturing, whole bool, o patternOpts, pl *capturePlan) bool {
	if left, rights, ok := splitExclusion(p, &o); ok {
		planWalk(left, base, capturing, whole, o, pl)
		at := base + len(left)
		for _, x := range rights {
			at++
			planWalk(x, at, capturing, whole, o, pl)
			at += len(x)
		}
		return whole
	}
	for len(p) > 0 {
		if p[0] == '^' {
			p, base = p[1:], base+1
			continue
		}
		if body, rest, ok := splitPatternFlags(p); ok {
			if anchorPatternFlag(body) == anchorNone {
				capturing, whole = planFlags(body, capturing, whole)
			}
			base, p = base+len(p)-len(rest), rest
			continue
		}
		if p[0] == '*' || p[0] == '#' {
			p, base = p[1:], base+1
			continue
		}
		item, rest, ok := splitClosableItem(p, -1, &o)
		if !ok {
			break
		}
		if body, quant, after, isGroup := splitGroup(item, -1, &o); isGroup && after == "" {
			bp := base + 1
			if quant != 0 {
				bp = base + 2
			}
			if capturing {
				pl.index[base] = pl.count
				pl.count++
			}
			arms, armAt := alternativesAt(body, bp)
			for k, a := range arms {
				planWalk(a, armAt[k], capturing, whole, o, pl)
			}
		}
		base, p = base+len(item), rest
	}
	return whole
}

// planFlags folds one flag group's letters into the two answers this pass
// carries. Everything else in a group is the matcher's business.
//
// The last letter wins where two disagree, which is measured: `(#bB)` reports
// no groups at all.
func planFlags(body string, capturing, whole bool) (bool, bool) {
	for i := range len(body) {
		switch body[i] {
		case 'b':
			capturing = true
		case 'B':
			capturing = false
		case 'm':
			whole = true
		case 'M':
			whole = false
		}
	}
	return capturing, whole
}

// matchReport is what a surface hands back to the shell after a match: the
// spans a `(#b)` captured and the one a `(#m)` asked for.
//
// It is a value rather than the state itself so that a surface which matched
// several times — a replacement — can publish one match at a time.
type matchReport struct {
	// subject is the whole string the pattern was matched against, which is
	// what turns a byte offset into text and into a character index.
	subject string
	// groups are the `(#b)` spans, one per group in the pattern whether it
	// participated or not.
	groups []capSpan
	// whole is the `(#m)` span, and set says the pattern asked for it.
	whole    capSpan
	wantsAll bool
}

// wanted reports whether this pattern asks a surface for anything at all.
func (m matchReport) wanted() bool { return len(m.groups) > 0 || m.wantsAll }

// report reads the state out after a successful match.
func (c *captures) report(subject string, whole capSpan, wantsAll bool) matchReport {
	m := matchReport{subject: subject, whole: whole, wantsAll: wantsAll}
	if c != nil {
		m.groups = append(m.groups, c.spans...)
	}
	return m
}

// text is what a span matched, or the empty string for one that never did.
func (m matchReport) text(sp capSpan) string {
	if !sp.set {
		return ""
	}
	return m.subject[sp.begin:sp.end]
}

// bounds are a span's begin and end as a script reads them: **character**
// indices, one-based, with the end being the index of the last character —
// so an empty match has an end one below its begin, which is measured
// (`[[ ac == (#b)(a)(b|)c ]]` reports `mbegin=(1 2)` and `mend=(1 1)`).
//
// A group that did not participate is -1 on both, and that is the whole point
// of the flag reporting rather than the matcher dropping it.
func (m matchReport) bounds(sp capSpan, base int) (int, int) {
	if !sp.set {
		return -1, -1
	}
	return charIndex(m.subject, sp.begin) + base, charIndex(m.subject, sp.end) + base - 1
}

// charIndex is how many characters of s lie before byte offset n.
//
// Characters rather than bytes because that is what a script is told:
// `x=aébc; ${x//(#m)?/<$MATCH:$MBEGIN>}` reports the é at 2, where its byte
// offset is also 2 but the `b` after it is at 3 and not at 4.
func charIndex(s string, n int) int {
	if n <= 0 {
		return 0
	}
	if n > len(s) {
		n = len(s)
	}
	count := 0
	for i := 0; i < n; count++ {
		i += characterWidth(s[i:])
	}
	return count
}

// hasPatternFlagGroup is a cheap "is any of this worth asking about" over a
// pattern, so that the ordinary pattern — which is nearly all of them — never
// builds a plan or a report.
func hasPatternFlagGroup(pattern string) bool {
	return strings.Contains(pattern, "(#")
}

// publishMatch writes a successful match into the parameters the pattern
// asked for, and writes nothing at all where it asked for none.
//
// The silence is measured and load-bearing: `match=(zz); [[ abc == (#b)abc ]]`
// leaves `match` as `(zz)` in real zsh, and so does a `(#b)` pattern that
// fails — so a script that reads `$match` after a match with no group sees
// what it put there rather than an emptied array. A publish that always ran
// would clear it, which is the same class of wrong answer as a `(#b)` that
// never filled it.
func (r *Runner) publishMatch(m matchReport) {
	if !m.wanted() {
		return
	}
	// The indices a script reads are the dialect's own: the shell with these
	// flags counts arrays from one, and turning that option off moves the
	// reported positions with it — measured, `(#m)` on `abc` reports
	// `MBEGIN=1 MEND=3` and `MBEGIN=0 MEND=2` under `ksharrays`.
	base := r.arrayBase()
	if len(m.groups) > 0 {
		texts := make([]string, len(m.groups))
		begins := make([]string, len(m.groups))
		ends := make([]string, len(m.groups))
		for i, sp := range m.groups {
			b, e := m.bounds(sp, base)
			texts[i] = m.text(sp)
			begins[i] = strconv.Itoa(b)
			ends[i] = strconv.Itoa(e)
		}
		r.setArray("match", texts)
		r.setArray("mbegin", begins)
		r.setArray("mend", ends)
	}
	if m.wantsAll {
		b, e := m.bounds(m.whole, base)
		r.setVar("MATCH", m.text(m.whole))
		r.setVar("MBEGIN", strconv.Itoa(b))
		r.setVar("MEND", strconv.Itoa(e))
	}
}

// prepare works out the facts about a pattern that no position, subject or
// trial can change. Called when the pattern changes and not when the subject
// does.
//
// This is #1398's shape: the matcher re-derived the pattern's structure at
// every step, and half the samples on a prompt theme's substitution were
// scans rediscovering the same answers. Reading them once is the cheap half
// of "read the pattern once"; what it does not do is remove the re-derivation
// of *which* construct stands at a position, which wants a compiled form and
// a design note of its own.
func (w *matchWhere) prepare(pattern string) {
	if !patternPrepares {
		// Every table below is consulted through a lookup that falls back to
		// the scan it replaces, so leaving them empty is the matcher as it
		// was. A test drives the same patterns both ways and requires the
		// same answers — see TestPreparingThePatternChangesNoAnswer.
		w.noTilde, w.noParen = false, false
		w.parenEnd, w.brackEnd, w.reachLen = nil, nil, nil
		clear(w.arms)
		return
	}
	w.noTilde = strings.IndexByte(pattern, '~') < 0
	w.noParen = strings.IndexByte(pattern, '(') < 0
	w.parenEnd = grow32(w.parenEnd, len(pattern))
	w.brackEnd = grow32(w.brackEnd, len(pattern))
	clear(w.arms)
	w.reachAt = grow32(w.reachAt, len(pattern)+1)
	w.reachLen = grow32(w.reachLen, len(pattern)+1)
	for i := range w.reachLen {
		w.reachLen[i] = -1
	}
	for i := range len(pattern) {
		w.parenEnd[i], w.brackEnd[i] = -1, -1
		switch pattern[i] {
		case '(':
			if end, ok := closingParen(pattern[i:]); ok {
				w.parenEnd[i] = int32(i + end)
			}
		case '[':
			if end, ok := bracketEnd(pattern, i); ok {
				w.brackEnd[i] = int32(end)
			}
		}
	}
}

// patternPrepares turns the prepared tables off, for the reason
// memoThreshold and patternReachBounds are vars: it lets a test run the same
// pattern down both paths and require the same answer, which is the only
// reference for "reading the pattern once changed nothing" that cannot drift
// away from the code it is checking. Nothing outside a test writes it.
var patternPrepares = true

// grow32 reuses the slice when it is already long enough, so a matcher that
// runs against one pattern and many subjects allocates these once.
func grow32(s []int32, n int) []int32 {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]int32, n)
}

// closerWithin turns a prepared absolute offset into one relative to the
// piece, and reports false when the answer is not this piece's to give.
func closerWithin(table []int32, pp, n int) (int, bool) {
	if pp < 0 || pp >= len(table) {
		return 0, false
	}
	end := int(table[pp])
	if end < 0 || end >= pp+n {
		return 0, false
	}
	return end - pp, true
}

// closingParenAt is closingParen with the prepared table consulted first. pp
// is where p begins in the pattern, or -1 for a caller that does not know —
// the one-time scans that read a pattern before any match, where the table is
// not built yet and the answer is wanted once.
func closingParenAt(o *patternOpts, p string, pp int) (int, bool) {
	if o.where != nil && o.where.ready {
		if end, ok := closerWithin(o.where.parenEnd, pp, len(p)); ok {
			return end, true
		}
	}
	return closingParen(p)
}

// bracketEndAt is bracketEnd for the bracket standing at p[i], with the
// prepared table consulted first. The result is an offset into p, as
// bracketEnd's is.
func bracketEndAt(o *patternOpts, p string, i, pp int) (int, bool) {
	if o.where != nil && o.where.ready && pp >= 0 {
		if end, ok := closerWithin(o.where.brackEnd, pp+i, len(p)-i); ok {
			return i + end, true
		}
	}
	return bracketEnd(p, i)
}

// armsOf is alternativesAt with the arms of each group remembered by where
// the group's body stands.
//
// A group is split into its arms at every position it is tried from, and the
// split allocates two slices each time. The body is a fixed piece of the
// pattern, so the answer is too — the length is compared as well as the
// offset because a one-time scan reads bodies the table was not built for.
func (w *matchWhere) armsOf(body string, bp int) ([]string, []int) {
	if w == nil || !w.ready {
		return alternativesAt(body, bp)
	}
	if !patternPrepares {
		return alternativesAt(body, bp)
	}
	if a, ok := w.arms[bp]; ok && a.length == len(body) {
		return a.arms, a.offsets
	}
	arms, offsets := alternativesAt(body, bp)
	if w.arms == nil {
		w.arms = map[int]armSplit{}
	}
	w.arms[bp] = armSplit{length: len(body), arms: arms, offsets: offsets}
	return arms, offsets
}

// armSplit is one group body's arms, kept for the length of a match.
type armSplit struct {
	length  int
	arms    []string
	offsets []int
}

// reachAt is patternReach with the item's answer remembered by where it
// stands, which is what a closure's recursion asks for over and over.
func (w *matchWhere) reachOf(pp, n int) (int, bool) {
	if w == nil || !w.ready || pp < 0 || pp >= len(w.reachLen) || w.reachLen[pp] != int32(n) {
		return 0, false
	}
	return int(w.reachAt[pp]), true
}

func (w *matchWhere) rememberReach(pp, n, reach int) {
	if w == nil || !w.ready || pp < 0 || pp >= len(w.reachLen) {
		return
	}
	w.reachLen[pp], w.reachAt[pp] = int32(n), int32(reach)
}
