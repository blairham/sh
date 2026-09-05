// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package corpusguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/corpusguard"
	"github.com/blairham/sh/internal/oracle"
)

// corpusOf writes a case.go the way the real one is written, so the scan is
// exercised against the shape it has to read rather than against a fixture
// invented to suit it.
func corpusOf(ids ...string) string {
	var b strings.Builder
	b.WriteString("package oracle\n\nvar Corpus = []Case{\n")
	for _, id := range ids {
		b.WriteString("\t{\n\t\tID: \"" + id + "\", Category: \"c\",\n")
		b.WriteString("\t\tSnippet: `echo hi`,\n\t\tWhy:     \"because\",\n\t},\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// repo is a throwaway git repository holding a corpus. It is isolated from
// the developer's git configuration — no global config, no signing key, no
// hooks — because a test that borrows real user state measures the machine.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-b", "main")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// write puts a corpus in the working tree without committing it.
func (r *repo) write(ids ...string) {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(corpusguard.CasePath))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(corpusOf(ids...)), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// commit records the corpus and returns the commit.
func (r *repo) commit(msg string, ids ...string) string {
	r.t.Helper()
	r.write(ids...)
	r.git("add", "-A")
	r.git("-c", "commit.gpgsign=false", "commit", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

func (r *repo) check(base string, live ...string) *corpusguard.Result {
	r.t.Helper()
	res, err := corpusguard.Check(r.dir, base, live)
	if err != nil {
		r.t.Fatalf("Check: %v", err)
	}
	return res
}

// The incident in #708: four cases another train had added were gone after a
// clean merge, and nothing said so.
func TestADroppedCaseIsCaught(t *testing.T) {
	r := newRepo(t)
	base := r.commit("three cases", "a/one", "a/two", "a/three")
	r.write("a/one", "a/three")

	got := r.check(base, "a/one", "a/three")
	if got.OK() {
		t.Fatal("a corpus that lost a case passed the guard")
	}
	if len(got.Dropped) != 1 || got.Dropped[0] != "a/two" {
		t.Errorf("Dropped = %v, want [a/two]", got.Dropped)
	}
}

// The reason the guard compares sets and not a committed number. This corpus
// is exactly as large as it was, so a count — or a floor, or a monotonicity
// rule over the total — sees nothing at all.
func TestACaseAddedWhileAnotherIsDroppedIsStillCaught(t *testing.T) {
	r := newRepo(t)
	base := r.commit("three cases", "a/one", "a/two", "a/three")
	r.write("a/one", "a/three", "a/four")

	got := r.check(base, "a/one", "a/three", "a/four")
	if got.BaseIDs != got.HeadIDs {
		t.Fatalf("this test is only interesting when the size is unchanged: %d then %d", got.BaseIDs, got.HeadIDs)
	}
	if len(got.Dropped) != 1 || got.Dropped[0] != "a/two" {
		t.Errorf("Dropped = %v, want [a/two] — a count would have reported nothing", got.Dropped)
	}
}

// Two trains both appending is the ordinary case, and it must be silent.
func TestACorpusThatOnlyGrewPasses(t *testing.T) {
	r := newRepo(t)
	base := r.commit("two cases", "a/one", "a/two")
	r.write("a/one", "a/two", "b/one", "b/two")

	if got := r.check(base, "a/one", "a/two", "b/one", "b/two"); !got.OK() {
		t.Errorf("a corpus that only grew was reported as %v", got.Dropped)
	}
}

// Order is not a fact about the corpus: it is grouped by category, and a case
// moved into its group is not a loss.
func TestReorderingIsNotALoss(t *testing.T) {
	r := newRepo(t)
	base := r.commit("two cases", "a/one", "a/two")
	r.write("a/two", "a/one")

	if got := r.check(base, "a/two", "a/one"); !got.OK() {
		t.Errorf("a reordered corpus was reported as %v", got.Dropped)
	}
}

// A retirement is a deliberate, reviewable act, so it silences the guard for
// exactly the one ID it names and nothing else.
func TestARetiredCaseIsNotReportedButItsNeighborStillIs(t *testing.T) {
	corpusguard.Retired["a/two"] = "renamed to a/two-renamed"
	t.Cleanup(func() { delete(corpusguard.Retired, "a/two") })

	r := newRepo(t)
	base := r.commit("three cases", "a/one", "a/two", "a/three")
	r.write("a/one")

	got := r.check(base, "a/one")
	if len(got.Dropped) != 1 || got.Dropped[0] != "a/three" {
		t.Errorf("Dropped = %v, want [a/three] only", got.Dropped)
	}
}

// The corpus is keyed by ID in the golden record, so a repeated ID is one
// case's evidence written over another's.
func TestADuplicateIDIsReported(t *testing.T) {
	r := newRepo(t)
	base := r.commit("two cases", "a/one", "a/two")
	r.write("a/one", "a/two", "a/two")

	got := r.check(base, "a/one", "a/two", "a/two")
	if got.OK() || len(got.Duplicate) != 1 || got.Duplicate[0] != "a/two" {
		t.Errorf("Duplicate = %v, want [a/two]", got.Duplicate)
	}
}

// The broken form of a source scanner is silence, and silence here reads as
// "nothing was lost". So a scan that disagrees with the compiled corpus is an
// error and never a quiet pass.
func TestAScanThatNoLongerReadsTheFileIsAnErrorRatherThanAPass(t *testing.T) {
	r := newRepo(t)
	base := r.commit("three cases", "a/one", "a/two", "a/three")
	r.write("a/one")

	// The compiled corpus still has three; the file has one. Whatever went
	// wrong, the answer is not "no cases were lost".
	_, err := corpusguard.Check(r.dir, base, []string{"a/one", "a/two", "a/three"})
	if err == nil {
		t.Fatal("a scan disagreeing with the compiled corpus was accepted")
	}
	if !strings.Contains(err.Error(), "cannot be trusted") {
		t.Errorf("error = %v, want it to say the scan cannot be trusted", err)
	}
}

// A guard that decides it cannot tell and passes is the same as no guard.
func TestNoBaseCommitIsAnErrorRatherThanAPass(t *testing.T) {
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-b", "work")
	r.commit("one case", "a/one")

	if _, err := corpusguard.Check(r.dir, "", []string{"a/one"}); err == nil {
		t.Fatal("a repository with no base to compare against passed the guard")
	}
}

// A base the caller named but the checkout does not have falls back to the
// merge base rather than skipping: a weaker floor is still a floor.
func TestAnAbsentPreferredBaseFallsBackInsteadOfSkipping(t *testing.T) {
	r := newRepo(t)
	r.commit("two cases", "a/one", "a/two")
	r.write("a/one")

	got := r.check("0000000000000000000000000000000000000000", "a/one")
	if len(got.Dropped) != 1 || got.Dropped[0] != "a/two" {
		t.Errorf("Dropped = %v, want [a/two] from the fallback base", got.Dropped)
	}
}

// The base a caller names is the base that is used. In continuous
// integration that name comes from the event — the commit the pull request
// is based on — and it is a tighter floor than any ancestor the checkout
// happens to have. Here the fallback would find HEAD itself and report
// nothing, so the two answers differ and only one of them is right.
func TestTheNamedBaseIsTheOneCompared(t *testing.T) {
	r := newRepo(t)
	first := r.commit("two cases", "a/one", "a/two")
	r.commit("one case", "a/one")

	if got := r.check("", "a/one"); !got.OK() {
		t.Fatalf("this test is only interesting when the fallback is silent, got %v", got.Dropped)
	}
	got := r.check(first, "a/one")
	if len(got.Dropped) != 1 || got.Dropped[0] != "a/two" {
		t.Errorf("Dropped = %v, want [a/two] from the named base", got.Dropped)
	}
}

// The scan has to read the real file, not a fixture shaped to suit it. This
// is the same check Check makes at run time, pinned as a test so a change to
// how the corpus is written is caught here rather than by the guard refusing
// to run.
func TestTheScanReadsTheRealCorpus(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(corpusguard.CasePath)))
	if err != nil {
		t.Fatal(err)
	}
	got := corpusguard.IDs(src)
	if len(got) != len(oracle.Corpus) {
		t.Fatalf("scanned %d IDs from %s, the compiled corpus has %d", len(got), corpusguard.CasePath, len(oracle.Corpus))
	}
	for i, c := range oracle.Corpus {
		if got[i] != c.ID {
			t.Fatalf("ID %d: scanned %q, compiled %q", i, got[i], c.ID)
		}
	}
}
