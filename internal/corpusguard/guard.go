// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package corpusguard checks that the oracle corpus never loses a case.
//
// The corpus is semantically a *set*, written down as a Go slice literal.
// Git merges it as text, and text merges cleanly in cases where the set does
// not: two trains appending cases in the same region, one of them resolved by
// taking a side, a rebase that replays an edit against a file that has moved.
// The campaign convention — "resolve case.go by keeping both sides" — is
// correct and is a convention, so nothing enforces it.
//
// What a dropped case costs is why this exists. It is lost coverage that
// reads as a passing build: every test still passes, the conformance total
// shifts by a few, and that shift is indistinguishable from the ordinary
// movement the campaign produces daily. There is no symptom to notice.
//
// # Why a set and not a count
//
// The cheap version of this guard is a committed integer that the corpus must
// meet or exceed. It is defeated by the exact shape of the incident it is
// meant to catch: a branch that adds two cases and merges away two others
// leaves the count unchanged, and a branch that adds five and loses four
// makes it *rise*. The failure is a set difference, and only a set difference
// detects it.
//
// # Why the baseline is git history and not a committed file
//
// A committed manifest of IDs is a set, so it fixes the paragraph above. It
// does not fix the mechanism: the manifest is another text file in the same
// tree, merged by the same merge, and a merge that drops a case from case.go
// is in a position to drop its manifest line in the same commit. A guard
// whose baseline can be edited by the event it is guarding against is not a
// guard.
//
// Git history is the one baseline the working tree cannot rewrite. What
// landed on main landed; a merge in a worktree cannot reach back and change
// it. So the question this package asks is "is every case ID that existed at
// the merge base still here", read from the object database rather than from
// a file anyone's merge can touch. It also needs no maintenance: there is no
// number to bump and no list to keep in step, so it cannot go stale.
//
// The invariant is not aspirational. Replaying every commit that has ever
// touched case.go, no case ID has ever left main by accident — the set has
// only grown, across the whole history of the file. Retired is the deliberate
// exception, and it holds only the cases whose *snippet* was found to be
// measuring something other than what their ID claimed.
package corpusguard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CasePath is the corpus, relative to the repository root.
const CasePath = "internal/oracle/case.go"

// Retired names case IDs that were deliberately removed from the corpus,
// each with the reason. A case is normally forever: the ID is the key in the
// golden record, so retiring one throws away recorded evidence about five
// real shells. Renaming a case is a retirement plus an addition, and it is
// the likeliest reason for an entry here.
//
// Losing an entry from this map is fail-safe — the guard starts reporting the
// ID again — which is the direction a merge hazard should point.
var Retired = map[string]string{
	// Both measured `a=(); set -- "${a[@]}"; echo "n=$#"` and recorded
	// ksh93 at n=1 against bash's and zsh's n=0, and a semantics axis was
	// built on the difference. `a=()` is not an array literal in ksh93: it
	// builds a *compound* variable whose value is the three bytes `(`,
	// newline, `)`, so the one field the count saw was not an empty one and
	// the divergence was between two unrelated behaviors. A count-only
	// snippet cannot be repaired in place — the evidence under the ID is
	// evidence about the wrong question — so each is retired for a row that
	// shows what the field holds, or that asks the question the ID claimed
	// (#1379).
	"array/a-quoted-empty-array-is-not-the-same-question": "the snippet measured ksh93's compound variable rather than an empty array; replaced by array/a-quoted-at-on-a-name-that-holds-nothing, array/a-declared-empty-array-quoted-at and param/an-empty-array-literal-shows-what-its-field-holds",
	"param/an-empty-array-quoted-at":                      "the same snippet and the same confound; replaced by param/an-empty-array-literal-shows-what-its-field-holds, which records the field's contents",
}

// idLine matches the ID field of a corpus entry in the *text* of case.go.
//
// Reading the base revision's IDs means reading a file that is not the one
// being compiled, so this is a scan of source text rather than a Go value.
// That is only trustworthy because the same scan is applied to the working
// tree's copy and checked against the compiled corpus on every run; see
// Check. An extractor that stops matching would otherwise report an empty
// baseline and pass, which is the failure mode a guard must not have.
var idLine = regexp.MustCompile(`(?m)^[ \t]*ID:[ \t]*"([^"]*)"`)

// IDs returns the case IDs in the text of case.go, in the order they appear.
func IDs(src []byte) []string {
	m := idLine.FindAllSubmatch(src, -1)
	ids := make([]string, 0, len(m))
	for _, g := range m {
		ids = append(ids, string(g[1]))
	}
	return ids
}

// Result is what one comparison found.
type Result struct {
	// Base is the resolved commit the corpus was compared against.
	Base string
	// BaseIDs and HeadIDs are the corpus sizes at each end.
	BaseIDs, HeadIDs int
	// Dropped are IDs present at Base, absent now, and not retired. A
	// non-empty Dropped is the failure this package exists for.
	Dropped []string
	// Duplicate are IDs the corpus now spells more than once. One would
	// silently overwrite the other in the golden record.
	Duplicate []string
}

// OK reports whether nothing was lost and nothing is written twice.
func (r *Result) OK() bool { return len(r.Dropped) == 0 && len(r.Duplicate) == 0 }

// Compare reports what base has that head does not, ignoring retirements,
// and any ID head spells twice.
func Compare(base, head []string) *Result {
	have := map[string]int{}
	for _, id := range head {
		have[id]++
	}
	r := &Result{BaseIDs: len(base), HeadIDs: len(head)}
	seen := map[string]bool{}
	for _, id := range base {
		if have[id] > 0 || Retired[id] != "" || seen[id] {
			continue
		}
		seen[id] = true
		r.Dropped = append(r.Dropped, id)
	}
	for id, n := range have {
		if n > 1 {
			r.Duplicate = append(r.Duplicate, id)
		}
	}
	sort.Strings(r.Dropped)
	sort.Strings(r.Duplicate)
	return r
}

// git runs one git command in dir and returns its standard output.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveBase names the commit to compare against.
//
// A preference is tried first and is allowed to fail: the caller passes what
// the continuous-integration event says the base is, and that commit is not
// always present in the checkout. Falling back to a merge base is still a
// real comparison against a real ancestor, so the fallback weakens the floor
// without ever skipping the check. Only running out of candidates is an
// error, because a guard that decides it cannot tell and passes is the same
// as no guard.
func ResolveBase(dir, prefer string) (string, error) {
	if prefer != "" {
		if sha, err := git(dir, "rev-parse", "--verify", "--quiet", prefer+"^{commit}"); err == nil && sha != "" {
			return sha, nil
		}
	}
	for _, ref := range []string{"origin/main", "main"} {
		if sha, err := git(dir, "merge-base", "HEAD", ref); err == nil && sha != "" {
			return sha, nil
		}
	}
	return "", fmt.Errorf("no base commit: %q did not resolve and neither origin/main nor main is a merge base of HEAD", prefer)
}

// Check compares the corpus in dir's working tree against the corpus at base.
//
// live is the compiled corpus's IDs. Checking the scan of the working tree's
// case.go against it is what makes the scan of the base revision credible: if
// the two ever disagree the extractor has stopped reading this file's shape,
// and every answer it gives about history is worthless. That is an error
// rather than a warning for the reason above — the broken form of a source
// scanner is silence, and silence here reads as "nothing was lost".
func Check(dir, prefer string, live []string) (*Result, error) {
	head, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(CasePath)))
	if err != nil {
		return nil, err
	}
	got, want := IDs(head), live
	if !sameMultiset(got, want) {
		return nil, fmt.Errorf("the ID scan read %d IDs from %s but the compiled corpus has %d; "+
			"the scan no longer matches this file's shape and cannot be trusted about history",
			len(got), CasePath, len(want))
	}
	base, err := ResolveBase(dir, prefer)
	if err != nil {
		return nil, err
	}
	src, err := git(dir, "show", base+":"+CasePath)
	if err != nil {
		return nil, err
	}
	r := Compare(IDs([]byte(src)), got)
	r.Base = base
	return r, nil
}

// sameMultiset reports whether two ID lists hold the same IDs the same number
// of times. Order is not compared: the corpus is grouped by category and a
// reordering is not a loss.
func sameMultiset(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	n := map[string]int{}
	for _, s := range a {
		n[s]++
	}
	for _, s := range b {
		n[s]--
		if n[s] == 0 {
			delete(n, s)
		}
	}
	return len(n) == 0
}
