// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"testing"
	"time"
)

// now is the instant every ranking in this file is scored against, far enough
// from the epoch that a record can be a month old without a negative age.
var rankNow = at(90 * 24 * 3600)

// ago is a start time for a record that many hours before rankNow.
func ago(hours float64) time.Time {
	return rankNow.Add(-time.Duration(hours * float64(time.Hour)))
}

// visit is one record in a directory at a time.
func visit(t *testing.T, s *Store, dir string, start time.Time) {
	t.Helper()
	mustAppend(t, s, Record{ID: "R" + start.String() + dir, Command: "ls", Cwd: dir, Start: start})
}

// dirs is the ranking's order, which is the only thing a caller reads.
func dirs(ranked []DirRank) []string {
	out := make([]string, 0, len(ranked))
	for _, d := range ranked {
		out = append(out, d.Dir)
	}
	return out
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ranking %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ranking %v, want %v", got, want)
		}
	}
}

// More use ranks higher when the use is equally recent, which is the half of
// the score a plain visit count would also get right.
func TestMoreVisitsRankHigher(t *testing.T) {
	s, _ := newStore(t)
	visit(t, s, "/a", ago(1))
	visit(t, s, "/b", ago(1))
	visit(t, s, "/b", ago(1))
	same(t, dirs(s.RankDirs(t.Context(), 100, rankNow)), []string{"/b", "/a"})
}

// And recent use outranks more use, which is the half a visit count gets
// wrong. Ten visits a half-life ago are worth five; one visit now is worth
// one, and four more today carry it.
func TestRecentUseOutranksMoreUse(t *testing.T) {
	s, _ := newStore(t)
	for range 10 {
		visit(t, s, "/old", rankNow.Add(-HalfLife))
	}
	for range 6 {
		visit(t, s, "/new", ago(0.5))
	}
	ranked := s.RankDirs(t.Context(), 100, rankNow)
	same(t, dirs(ranked), []string{"/new", "/old"})
	// The arithmetic the file comment states, rather than only the order it
	// produces: a half-life old visit is worth half of one.
	if got := ranked[1].Score; got < 4.99 || got > 5.01 {
		t.Fatalf("ten visits one half-life ago scored %v, want 5", got)
	}
	if ranked[1].Visits != 10 {
		t.Fatalf("visits %d, want 10", ranked[1].Visits)
	}
}

// A failed command counts as much as a successful one: the record says
// somebody was working there, and this ranks presence. Stated as a test
// because it is a decision rather than an accident — see the file comment.
func TestAFailedCommandCountsAsAVisit(t *testing.T) {
	s, _ := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "git push", Cwd: "/repo", Start: ago(1), Status: 1})
	mustAppend(t, s, Record{ID: "B", Command: "ls", Cwd: "/other", Start: ago(1), Status: 0})
	ranked := s.RankDirs(t.Context(), 100, rankNow)
	if len(ranked) != 2 {
		t.Fatalf("ranked %v, want both directories", dirs(ranked))
	}
	if ranked[0].Score != ranked[1].Score {
		t.Fatalf("scores %v and %v differ, so status weighed something",
			ranked[0].Score, ranked[1].Score)
	}
}

// The same for duration: a six-hour build and an `ls` are one visit each.
func TestALongCommandCountsAsOneVisit(t *testing.T) {
	s, _ := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "make", Cwd: "/slow", Start: ago(1), DurationMs: 21600000})
	mustAppend(t, s, Record{ID: "B", Command: "ls", Cwd: "/fast", Start: ago(1), DurationMs: 1})
	ranked := s.RankDirs(t.Context(), 100, rankNow)
	if len(ranked) != 2 || ranked[0].Score != ranked[1].Score {
		t.Fatalf("ranking %v is not flat in duration", ranked)
	}
}

// A record with no directory is not a directory called "". A session the
// front end never told where it was writes one.
func TestARecordWithNoDirectoryIsNotRanked(t *testing.T) {
	s, _ := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "ls", Cwd: "", Start: ago(1)})
	visit(t, s, "/a", ago(1))
	same(t, dirs(s.RankDirs(t.Context(), 100, rankNow)), []string{"/a"})
}

// Two directories nothing separates come back in the same order twice. A
// ranking taken from a map would not, and a jump that landed somewhere
// different each time is the failure this feature is most exposed to.
func TestATieIsBrokenTheSameWayEveryTime(t *testing.T) {
	s, _ := newStore(t)
	for _, d := range []string{"/m", "/b", "/z", "/a"} {
		visit(t, s, d, ago(1))
	}
	want := []string{"/a", "/b", "/m", "/z"}
	for range 5 {
		same(t, dirs(s.RankDirs(t.Context(), 100, rankNow)), want)
	}
}

// A clock that ran backwards does not make one record outweigh a directory.
// Several sessions append to one store and they are not on one clock.
func TestARecordFromTheFutureIsWorthOneVisit(t *testing.T) {
	s, _ := newStore(t)
	visit(t, s, "/ahead", rankNow.Add(time.Hour))
	visit(t, s, "/here", rankNow)
	ranked := s.RankDirs(t.Context(), 100, rankNow)
	if len(ranked) != 2 {
		t.Fatalf("ranked %v", dirs(ranked))
	}
	if ranked[0].Score != 1 || ranked[1].Score != 1 {
		t.Fatalf("scores %v, want one visit each", ranked)
	}
}

// An empty store ranks nothing rather than failing.
func TestAnEmptyStoreRanksNothing(t *testing.T) {
	s, _ := newStore(t)
	if got := s.RankDirs(t.Context(), 100, rankNow); got != nil {
		t.Fatalf("ranked %v on an empty store", got)
	}
}

// The read is bounded by the limit, which is what makes ranking cost the same
// after a year of use. The oldest records fall off the end.
func TestTheRankingReadsAtMostTheLimit(t *testing.T) {
	s, _ := newStore(t)
	visit(t, s, "/old", ago(3))
	visit(t, s, "/new", ago(2))
	visit(t, s, "/newer", ago(1))
	same(t, dirs(s.RankDirs(t.Context(), 2, rankNow)), []string{"/newer", "/new"})
}
