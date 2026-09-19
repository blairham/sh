// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"
)

// Ranking the directories a person works in, out of the records that are
// already here.
//
// The whole of this file is a query. A block already carries where the line
// was accepted and when it was accepted, so "which directories does this
// person use" is a fold over two fields the store has written since it
// existed — not a second database, not a second thing to keep in step with
// the first, and nothing new on the path of a command that runs. That is the
// property worth protecting: anything that wanted its own file would be a
// second record of the same events, and the first disagreement between them
// would be silent.
//
// # What the score is, and what it deliberately is not
//
// A visit count with an exponential decay on it: every record in a directory
// contributes 2^(-age/HalfLife), so a directory's score is how much recent
// use it has rather than how much use it has ever had. One half-life ago
// counts half, two ago a quarter.
//
// **Status and duration are read and deliberately not weighted**, and that is
// a decision rather than an omission, because the issue this came from
// (#1312) names both as fields a history file does not have and this store
// does.
//
//   - A failed command is as much evidence of presence as a successful one.
//     `Cwd` is where the line was *accepted*, so it records that somebody was
//     working there; a `git push` that was rejected does not make the
//     repository a place they visit less. Weighting it down would rank a
//     directory by how well the work in it went.
//   - A long command is a fact about the command and not about the place. A
//     six-hour build and a `cd` are one visit each.
//
// The honest statement is therefore that the extra fields let this *decide*
// rather than guess, and the decision was to rank on presence. A weighting
// that claimed to be better than the tools this is compared against would
// need a measurement against a real store, and the store has been opt-in
// since #2274 — so that measurement cannot be taken yet, and inventing one
// would be worse than saying so.
//
// # The half-life
//
// One week, and the arithmetic is the argument. A directory visited ten times
// a week ago scores 5 against a directory visited once a minute ago, so this
// week's project stays ahead of last week's until the new one has been used
// about as much; a month-old directory needs sixteen times the visits to
// compete. A half-life of a day would make ten visits yesterday lose to one
// visit now, which is a ranking that forgets what you were doing this
// morning.
//
// It is a stated default and not a measured one, for the reason above.

// HalfLife is how long it takes a visit to count half as much.
const HalfLife = 7 * 24 * time.Hour

// A DirRank is one directory and what the ranking made of it.
type DirRank struct {
	// Dir is the directory, exactly as the records spell it.
	Dir string
	// Score is the decayed visit count — see the file comment. It is
	// comparable between directories of one ranking and means nothing on its
	// own.
	Score float64
	// Visits is how many records named this directory, undecayed. It is what
	// a person reads when they want to know why something is ranked where it
	// is, and it is never what the order is taken from.
	Visits int
	// Last is the most recent of those records.
	Last time.Time
}

// RankDirs scores the directories the last limit records were typed in,
// highest first.
//
// Bounded by the same argument [Store.Find] is bounded by: the read is the
// newest limit records, walking the shards backwards, so ranking costs the
// same on a store somebody has been filling for a year as on one from this
// morning. It is also the right bound rather than merely an affordable one —
// what falls off the end is what the decay has already made worthless.
//
// now is passed in rather than read, so a test can say what time it is and so
// that one ranking is scored against one instant.
//
// Directories are not checked for existence here. A ranking is a fact about
// what was recorded, and a directory that has been deleted still was worked
// in; the caller that means to *go* somewhere is the one that has to care,
// and it cares about the first entry it can actually use rather than about
// every entry in the list. Doing it here would stat every directory in the
// store to answer a question about one.
func (s *Store) RankDirs(ctx context.Context, limit int, now time.Time) []DirRank {
	recs := s.Load(ctx, limit)
	if len(recs) == 0 {
		return nil
	}
	at := map[string]int{}
	out := make([]DirRank, 0, len(recs))
	for _, r := range recs {
		if r.Cwd == "" {
			// A session the front end never told where it was. There is no
			// directory here to rank, and the empty string is not one.
			continue
		}
		i, seen := at[r.Cwd]
		if !seen {
			i = len(out)
			at[r.Cwd] = i
			out = append(out, DirRank{Dir: r.Cwd})
		}
		out[i].Score += decay(now.Sub(r.Start))
		out[i].Visits++
		if r.Start.After(out[i].Last) {
			out[i].Last = r.Start
		}
	}
	slices.SortFunc(out, func(a, b DirRank) int {
		// Score decides, and the two tie-breaks below are there so that a
		// ranking is a total order rather than whatever the map iterated:
		// two directories with the same score would otherwise swap places
		// between two runs on the same store, and a jump that lands
		// somewhere different each time is the silent-wrong-answer failure
		// this feature is most exposed to.
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := b.Last.Compare(a.Last); c != 0 {
			return c
		}
		return cmp.Compare(a.Dir, b.Dir)
	})
	return out
}

// decay is what one visit of a given age is worth.
//
// A visit from the future is worth a whole one rather than more than one. It
// happens: a store is several sessions appending, clocks move, and a record
// written a second ahead of this reading must not outweigh the rest of the
// directory it is in.
func decay(age time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	return math.Exp2(-age.Seconds() / HalfLife.Seconds())
}
