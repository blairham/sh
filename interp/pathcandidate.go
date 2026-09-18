// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// PathCandidateReport is which failed candidate a PATH search names once it
// has found nothing it could run. See Semantics.PathCandidateReported.
//
// A place rather than a bool, because a bool is what
// Semantics.DirectoryOnPathIsACandidate already is and it could not say
// ksh93's answer. That probe put the directory alone on PATH, and in that
// shape ksh93 and zsh are byte for byte the same decision — both report the
// directory — so the observation could not tell "keeps the directory as the
// failed candidate" from "reports whatever the last candidate was". A second
// PATH entry is what parts them.
//
// Measured 2026-09-16 and again 2026-09-18, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`, with `$d` a directory
// holding a *directory* named `zzcmd`, `$e` an empty directory and `$g` a
// directory holding a non-executable file named `zzcmd`:
//
//	PATH               bash 5.3   zsh 5.9.2   dash/ash    ksh93u+
//	$d                 127        126 dir     127 perm    126 dir
//	$e:$d              127        126 dir     127 perm    126 dir
//	$d:$e              127        126 dir     127 perm    **127 not found**
//	$d:$g              127        126 perm    126 perm    **126 perm**
//	$g:$d              126 perm   126 perm    126 perm    **126 dir**
//	$g:$e              126 perm   126 perm    126 perm    126 perm
//	$e:$g              126 perm   126 perm    126 perm    126 perm
//
// The last four rows are the ones a directory alone cannot reach. Rows four
// and five are the same two entries in the two orders and ksh93 answers them
// differently, which no "first interesting candidate" reading can produce.
type PathCandidateReport uint8

const (
	// FirstInterestingCandidate is the core answer and four of the five
	// columns: the first candidate that existed and could not be run is the
	// one reported, whatever came after it. A candidate that simply is not
	// there is not interesting, and whether a *directory* is counts as one
	// is Semantics.DirectoryOnPathIsACandidate's question.
	FirstInterestingCandidate PathCandidateReport = iota

	// LastSearchedEntry is ksh93u+ 2012-08-01: the failure reported is the
	// one from the **last PATH entry actually searched**, whatever it was,
	// including a plain "not there".
	//
	// Searched is narrower than written. A PATH entry that is not an
	// existing directory is skipped entirely and does not overwrite what an
	// earlier one left: measured, `$d:/nonexistent` and `$d:relnope` both
	// report the directory at 126 where `$d:$e` and `$d:.` report `not
	// found` at 127, and an entry that is a regular *file* is skipped the
	// same way. So the test is on the entry and not on the candidate's
	// errno, which cannot tell a missing file from a missing directory
	// apart.
	LastSearchedEntry
)
