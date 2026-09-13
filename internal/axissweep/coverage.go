// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/blairham/sh/interp"
)

// The rest of this package reports **vacuity**: an axis nothing objects to,
// and a legal value no dialect holds. This file reports **absence**: an axis
// a dialect never answers at all.
//
// They are not the same question and the second one shipped a bug. A
// `Semantics` axis added after a dialect is written holds that type's
// Unspecified constant there, and an unanswered axis does not fail a test —
// it refuses at run time, in the shipped binary, at status 2. #2272 is the
// worked example: ReadTimeoutBoundsReadability landed within an hour of
// dialect/ash, from another session, neither change wrong on its own, and
// `ash -c 'read -t 1 x'` came back `no dialect was chosen` with `go test
// ./...` green. A sweep afterwards found nineteen more.
//
// # Why this is a ledger and not a bare assertion
//
// "Every dialect answers every axis" is false today and ought to be: dash has
// no `$'…'` at all, so DollarSingleBackslashC is a question dash is never
// asked, and writing a value there would be inventing a measurement. 429 of
// the 480 axes can be left unanswered — the ones whose type has an
// Unspecified constant — and dash leaves 126 of them so. Demanding a
// paragraph for each is how a gate gets switched off.
//
// So the shape is the one corpusguard uses: a committed record of what is
// unanswered today, and a check that the vector and the record still agree.
// A **new** pair fails — which is the whole point, because a new axis is
// unanswered in every dialect at once and lands here on the commit that adds
// it. A pair that is **no longer** unanswered also fails, so the record
// cannot rot into a list of things that used to be true.
//
// # Where the reasons live
//
// Not in the ledger. A reason belongs where the omission is, which is the
// dialect's own source, as a line in a comment:
//
//	// unanswered BraceRescanEntersFailedGroup: this shell has no brace
//	// expansion, so nothing ever resumes a scan — `@{x}{a,b}@` is one word.
//
// read back by DialectNotes and printed under the entry it answers. That is
// the same discipline the unpinned and unexhibited lists already use, and for
// the same stated reason: an answer that lives only in a pull request is one
// the next sweep cannot see.
//
// Untriaged is therefore the number that means something, exactly as it is
// for the other two lists. The ledger's length measures the shape of the
// panel — dash will never answer a question dash cannot be asked — while the
// count of entries nobody has explained measures the work.

// Gap is one axis one dialect does not answer.
type Gap struct {
	Dialect string `json:"dialect"`
	Field   string `json:"field"`
	Type    string `json:"type"`
	// Doc is the axis's own first sentence, so the report says what is
	// missing without the reader opening interp/semantics.go.
	Doc string `json:"doc,omitempty"`
	// Why is the reason recorded in the dialect's source, and empty when
	// nobody has written one.
	Why string `json:"why,omitempty"`
}

// CoverageResult is what every dialect answers and does not answer.
type CoverageResult struct {
	// Axes is every field of the vector, and Askable the subset whose type
	// has an Unspecified constant — the only kind a dialect can leave
	// unanswered. A bool, a string or an int always holds something, so
	// silence is indistinguishable from an answer there and this cannot
	// speak about them.
	Axes    int `json:"axes"`
	Askable int `json:"askable"`
	// Gaps is every unanswered dialect/axis pair, ledger or not, sorted by
	// dialect and then by field.
	Gaps []Gap `json:"gaps"`
	// New are the gaps the ledger does not record: a new axis nobody gave
	// this dialect a value for. This is the failure #2272 needed.
	New []Gap `json:"new,omitempty"`
	// Stale are the ledger's entries that the dialect now answers, or that
	// name no axis at all. Also a failure: a record that is allowed to drift
	// stops being evidence of anything.
	Stale []Gap `json:"stale,omitempty"`
	// Orphaned are recorded reasons for axes the dialect does answer. A
	// value quietly appearing under a comment that still says the question
	// is open is how a guess comes to read like a measurement.
	Orphaned []Gap `json:"orphaned,omitempty"`
	// Answered is how many askable axes each dialect answers.
	Answered map[string]int `json:"answered"`
}

// Coverage asks every dialect, of every axis, whether it has an answer.
//
// It reuses the walk the flip sweep uses — Fields over interp.Semantics — so
// an axis the sweep can move is an axis this can miss only by the two of them
// disagreeing about what a field is, which they cannot, because there is one
// walk. And it runs no shells at all, which is why it can be a test.
func Coverage() (*CoverageResult, error) {
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		return nil, err
	}
	consts, err := typeConstants()
	if err != nil {
		return nil, err
	}
	out := &CoverageResult{Axes: len(fields), Answered: map[string]int{}}
	var askable []Field
	for _, f := range fields {
		if unspecifiedConstant(consts, f.Type) != nil {
			askable = append(askable, f)
		}
	}
	out.Askable = len(askable)

	ledger, err := readLedger()
	if err != nil {
		return nil, err
	}
	recorded := map[Gap]bool{}
	for _, e := range ledger {
		recorded[e] = true
	}
	seen := map[Gap]bool{}

	for _, p := range Presets() {
		notes, err := DialectNotes(p.Name)
		if err != nil {
			return nil, err
		}
		held := reflect.ValueOf(p.Semantics)
		byPath := map[string]Field{}
		for _, f := range askable {
			byPath[f.Path] = f
		}
		for _, f := range askable {
			cur, err := At(held, f.Path)
			if err != nil {
				return nil, err
			}
			if !unspecifiedNow(f, cur) {
				out.Answered[p.Name]++
				if why, ok := notes[f.Path]; ok {
					out.Orphaned = append(out.Orphaned, Gap{
						Dialect: p.Name, Field: f.Path, Type: f.Type, Doc: f.Doc, Why: why,
					})
				}
				continue
			}
			g := Gap{Dialect: p.Name, Field: f.Path, Type: f.Type, Doc: f.Doc, Why: notes[f.Path]}
			out.Gaps = append(out.Gaps, g)
			key := Gap{Dialect: p.Name, Field: f.Path}
			seen[key] = true
			if !recorded[key] {
				out.New = append(out.New, g)
			}
		}
		// A reason recorded against a name that is not an axis at all —
		// a typo, or an axis that was renamed — is silence wearing the
		// shape of an answer, so it is reported rather than ignored.
		for path, why := range notes {
			if _, ok := byPath[path]; !ok {
				out.Orphaned = append(out.Orphaned, Gap{
					Dialect: p.Name, Field: path,
					Type: "(no such axis)", Why: why,
				})
			}
		}
	}
	for _, e := range ledger {
		if !seen[e] {
			out.Stale = append(out.Stale, e)
		}
	}
	sortGaps(out.Gaps)
	sortGaps(out.New)
	sortGaps(out.Stale)
	sortGaps(out.Orphaned)
	return out, nil
}

// Untriaged are the gaps no dialect comment has explained.
//
// The count and not the list is the deliverable, for the reason the other two
// lists give: the ledger's length is a fact about the panel, while the number
// of entries nobody has re-measured is a fact about the work.
func (c *CoverageResult) Untriaged() []Gap {
	var out []Gap
	for _, g := range c.Gaps {
		if g.Why == "" {
			out = append(out, g)
		}
	}
	return out
}

// Failures are the reasons this check is a check: a gap the ledger does not
// record, a recorded gap that is no longer one, and a reason attached to an
// axis that is answered or does not exist.
func (c *CoverageResult) Failures() int {
	return len(c.New) + len(c.Stale) + len(c.Orphaned)
}

func sortGaps(g []Gap) {
	sort.Slice(g, func(i, j int) bool {
		if g[i].Dialect != g[j].Dialect {
			return g[i].Dialect < g[j].Dialect
		}
		return g[i].Field < g[j].Field
	})
}

// unspecifiedConstant is the type's "no shell has been chosen here" value, or
// nil when the type has none.
//
// Its absence is what makes an axis unaskable here. A bool axis holds false
// whether that is dash's measured answer or nobody's, and this cannot tell
// those apart — which is a real hole and is reported as a number rather than
// papered over: see the Askable field.
func unspecifiedConstant(consts map[string][]Value, typeName string) *Value {
	for _, v := range consts[typeName] {
		if v.Unspecified {
			return &v
		}
	}
	return nil
}

// unansweredLine matches the reason a dialect records for an axis it leaves
// alone. The name has to look like a field path so that a sentence beginning
// with the word cannot be mistaken for a marker.
var unansweredLine = regexp.MustCompile(`^unanswered ([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*):[ \t]*(.*)$`)

// The scan is done once for the whole roster and shared, behind a sync.Once
// for the reason fields.go gives about its three: the callers are tests that
// run in parallel, and a lazily filled package-level map is a data race
// however obviously idempotent the work is. Filling it per dialect on demand
// was written first and the race detector caught it in one run.
var (
	noteOnce       sync.Once
	dialectNotes   map[string]map[string]string
	dialectNoteErr error
)

// DialectNotes is what one dialect's source says about the axes it does not
// answer, by field path.
//
// Every comment in the package is scanned rather than only the doc comment of
// something, because the omission has no declaration to hang a comment on:
// the axis is simply not assigned, and the natural place to say why is beside
// the assignments that are there.
func DialectNotes(name string) (map[string]string, error) {
	noteOnce.Do(readAllDialectNotes)
	if dialectNoteErr != nil {
		return nil, dialectNoteErr
	}
	out, ok := dialectNotes[name]
	if !ok {
		return nil, fmt.Errorf("no dialect named %q is in the roster; see Presets", name)
	}
	return out, nil
}

func readAllDialectNotes() {
	dialectNotes = map[string]map[string]string{}
	for _, p := range Presets() {
		notes, err := readDialectNotes(p.Name)
		if err != nil {
			dialectNotes, dialectNoteErr = nil, err
			return
		}
		dialectNotes[p.Name] = notes
	}
}

func readDialectNotes(name string) (map[string]string, error) {
	dir := dialectDir(name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	out := map[string]string{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Join(dir, n), err)
		}
		for _, cg := range f.Comments {
			scanMarkers(cg.Text(), unansweredLine, func(m []string, body string) {
				out[m[1]] = body
			})
		}
	}
	return out, nil
}

// readLedger reads the committed record of what is unanswered today.
func readLedger() ([]Gap, error) {
	path := ledgerFile()
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the coverage ledger: %w", err)
	}
	var out []Gap
	for n, line := range strings.Split(string(blob), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: want `<dialect> <axis>`, got %q", path, n+1, line)
		}
		out = append(out, Gap{Dialect: parts[0], Field: parts[1]})
	}
	sortGaps(out)
	return out, nil
}

// ledgerHeader is written into the file so that whoever opens it next reads
// what it is for before deciding to regenerate it.
const ledgerHeader = `# Every axis of interp.Semantics that a dialect does not answer.
#
# An axis holding its type's Unspecified constant is not a default: the shell
# refuses, by name, wherever the axis is consulted. That is right when the
# question cannot be put to the shell at all — dash has no $'...', so
# DollarSingleBackslashC is not dash's to answer — and it is a shipped bug
# when the axis is one the dialect reaches. #2272 was the second kind, in a
# binary, with go test green.
#
# So this file exists to make the second kind fail. TestEveryDialectAnswers-
# EveryAxis compares it against the vector and fails on any difference: a pair
# here that the dialect now answers, and — the one that matters — a pair the
# vector has and this file does not, which is what a newly added axis looks
# like in every dialect at once.
#
# If you added an axis and landed here: measure it in each dialect and write
# the value. Where a dialect genuinely cannot be asked, add the pair below AND
# say why in that dialect's own source, as
#
#     // unanswered SomeAxisName: what was measured and what stands in the way.
#
# beside the assignments it sits among. ` + "`make axis-coverage`" + ` prints the
# reasons under the entries they answer and counts the entries that have none.
# Regenerating this file with -write to make a failure go away records a
# refusal in the shipped binary as if it were a decision.
#
# Generated by ` + "`make axis-coverage ARGS=-write`" + `. Sorted; do not hand-order.
`

// Ledger renders the file this result would write.
func (c *CoverageResult) Ledger() string {
	var b strings.Builder
	b.WriteString(ledgerHeader)
	last := ""
	for _, g := range c.Gaps {
		if g.Dialect != last {
			b.WriteString("\n")
			last = g.Dialect
		}
		fmt.Fprintf(&b, "%-5s %s\n", g.Dialect, g.Field)
	}
	return b.String()
}

// WriteLedger rewrites the committed record.
func (c *CoverageResult) WriteLedger() (string, error) {
	path := ledgerFile()
	return path, os.WriteFile(path, []byte(c.Ledger()), 0o644)
}

// Report renders the coverage of every dialect.
func (c *CoverageResult) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "axis coverage: %d axes, %d of them askable\n", c.Axes, c.Askable)
	b.WriteString("  an axis is askable when its type has an Unspecified constant, which is\n" +
		"  the only way a dialect can be silent about one. A bool, a string or an\n" +
		"  int always holds something, so silence there is indistinguishable from\n" +
		"  an answer and this instrument cannot speak about it.\n\n")
	for _, p := range Presets() {
		gaps := c.Askable - c.Answered[p.Name]
		note := ""
		if p.Against == "" {
			note = "   (no oracle column — nothing else grades this dialect)"
		}
		fmt.Fprintf(&b, "  %-5s answers %3d of %d, unanswered %3d%s\n",
			p.Name, c.Answered[p.Name], c.Askable, gaps, note)
	}

	if len(c.New) > 0 {
		fmt.Fprintf(&b, "\nUNANSWERED AND UNRECORDED (%d) — this is the failure:\n", len(c.New))
		b.WriteString("  the vector has an axis this dialect gives no value for, and the ledger\n" +
			"  does not know about it. In a shipped binary that is not a default; it is\n" +
			"  `no dialect was chosen` at status 2, wherever the axis is consulted, with\n" +
			"  `go test ./...` green. Measure it and write the value. If the dialect\n" +
			"  cannot be asked at all, record it — see internal/axissweep/testdata.\n")
		for _, g := range c.New {
			fmt.Fprintf(&b, "  %-5s %-52s %s\n", g.Dialect, g.Field, g.Type)
			b.WriteString(wrapNote(g.Doc))
		}
	}
	if len(c.Stale) > 0 {
		fmt.Fprintf(&b, "\nRECORDED BUT ANSWERED NOW (%d) — also a failure:\n", len(c.Stale))
		b.WriteString("  the ledger says this dialect is silent here and it is not. Drop the\n" +
			"  line: a record allowed to drift stops being evidence of anything.\n")
		for _, g := range c.Stale {
			fmt.Fprintf(&b, "  %-5s %s\n", g.Dialect, g.Field)
		}
	}
	if len(c.Orphaned) > 0 {
		fmt.Fprintf(&b, "\nREASONS WITH NOTHING TO EXPLAIN (%d) — also a failure:\n", len(c.Orphaned))
		b.WriteString("  a dialect's source records why an axis was left unanswered, and the axis\n" +
			"  is answered — or is not an axis. If it was measured and closed, delete\n" +
			"  the note. If a value was copied from a neighboring dialect to quiet a\n" +
			"  refusal, it is a guess in a place that reads exactly like a measurement.\n")
		for _, g := range c.Orphaned {
			fmt.Fprintf(&b, "  %-5s %-52s %s\n", g.Dialect, g.Field, g.Type)
			b.WriteString(wrapNote(g.Why))
		}
	}

	untriaged := c.Untriaged()
	explained := len(c.Gaps) - len(untriaged)
	fmt.Fprintf(&b, "\nunanswered on purpose, with a recorded reason (%d):\n", explained)
	for _, g := range c.Gaps {
		if g.Why == "" {
			continue
		}
		fmt.Fprintf(&b, "  %-5s %-52s %s\n", g.Dialect, g.Field, g.Type)
		b.WriteString(wrapNote(g.Why))
	}
	if explained == 0 {
		b.WriteString("  (none)\n")
	}
	fmt.Fprintf(&b, "\n%d unanswered pairs in all, %d of them with no recorded reason.\n", len(c.Gaps), len(untriaged))
	b.WriteString("The second number is the one to drive down, and the first one mostly is\n" +
		"not: dash will never answer a question dash cannot be asked. A pair with a\n" +
		"reason states what is unmeasured; a pair without one states nothing.\n")
	return b.String()
}
