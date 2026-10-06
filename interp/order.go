// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// What order this shell puts words in, decided in one place.
//
// Three surfaces ask the question and they must not answer it three ways: a
// pathname expansion's matches (sortMatches in glob.go), the words a glob
// qualifier list's modifiers produced and then re-sorted, and the `o` and `O`
// flags of a parameter expansion (orderflags.go). All three are ordering the
// *same kind of thing* — words a shell is about to hand a command — and a
// shell that sorted `*` one way and `${(o)a}` another would be wrong about
// one of them whichever answer is right.
//
// It matters because the answer is contested, and a contested answer written
// down three times is an answer that drifts (#1675).

// shellOrder compares two words the way this shell orders them, which is
// **byte order** — and that is a decision rather than the absence of one.
//
// **In the C locale it is unanimous.** All four shells give
// `1digit A_upper Cherry Z _under a b banana date` for a directory holding
// those names, on macOS and on Linux alike, and that is what this produces.
// Both sweeps here run under `LC_ALL=C`, so it is also the only ordering the
// corpus can record.
//
// **Outside it, three of the four collate and dash never does.** Measured
// 2026-09-15 on this machine, the same directory, each shell started with
// `env -i` and nothing but the row's own variables:
//
//	set                              bash 5.3  ksh93   zsh     dash
//	nothing at all                   collates  bytes   bytes   bytes
//	LC_ALL=C                         bytes     bytes   bytes   bytes
//	LANG=C                           bytes     bytes   bytes   bytes
//	LANG=en_US.UTF-8                 collates  collat  collat  bytes
//	LC_ALL=en_US.UTF-8               collates  collat  collat  bytes
//	LC_COLLATE=en_US.UTF-8           collates  collat  collat  bytes
//	LC_COLLATE=C LANG=en_US.UTF-8    bytes     bytes   bytes   bytes
//	LC_ALL=C LC_COLLATE=en_US.UTF-8  bytes     bytes   bytes   bytes
//
// Three things come out of that table and the issue asked for all three:
//
//   - **It is a locale question and a dialect one at once.** dash is the
//     holdout in every row that has one, and the other three move together.
//   - **`LC_COLLATE=C` is byte order everywhere**, and it wins over a UTF-8
//     `LANG` while losing to `LC_ALL`, which is the ordinary precedence and
//     not a special rule about ordering.
//   - **An unset locale is not unanimous**, and that is the row a continuous
//     integration runner is actually in: bash reads no locale as the
//     system's default and collates, where ksh93, zsh and dash read it as C.
//     So a script whose output is a sorted glob can answer two ways on one
//     machine depending on which shell ran it and whether anything set LANG.
//
// **What cannot be done is the collation itself**, and the reason is kept
// next to the code so it is not attempted again:
//
//   - The platforms disagree. Same shells, same locale name, opposite
//     answers: macOS gives `_under 1digit a A_upper b banana Cherry date Z`
//     and glibc gives `1digit A_upper a b banana Cherry date _under`. No
//     single table is right on both.
//   - A dependency does not settle it. golang.org/x/text/collate implements
//     CLDR, which is close to glibc and not to macOS — so taking this
//     library's first direct dependency would buy a third answer, and be
//     wrong on the platform the panel is measured on. Generating a table the
//     way widthgen and normgen do runs into the same wall from the other
//     side: there is no *one* table to generate.
//   - An approximation is not close enough, and this was tried rather than
//     assumed. "Digits before letters, letters case-insensitively" gets the
//     obvious cases right and is still wrong twice over on an ordinary
//     directory: macOS orders `_` before `-`, which needs the real
//     punctuation weights, and sorts `Ápple` next to `Apple` and `éclair`
//     next to `date`, which needs base-letter folding. Both come from the
//     full table and neither can be derived from what the standard library
//     ships.
//
// So the shell sorts by byte, which is right in the C locale, right for one
// dialect everywhere, and wrong for three outside it — knowingly, in one
// place, and in a place that says so.
//
// **The completion listing is the fourth surface, and it answers the same
// way on purpose** (#6168). It lives in the line editor rather than here —
// repl's block.rows, which this package cannot be imported into — so it is
// recorded rather than shared. zsh orders the two together: measured
// 2026-10-05 and 2026-10-06 against zsh 5.9.2, a `compadd -J` listing of
// `README.md readme.md Beta alpha _x b-c bc B a1 a10 a2` draws `_x a1 a10 a2
// alpha B b-c bc Beta readme.md README.md` under `LANG=en_US.UTF-8` and `B
// Beta README.md _x a1 a10 a2 alpha b-c bc readme.md` under `LC_ALL=C`, and
// `print -r -- *` over the same names gives the same two orders. (Over ten
// of them: this machine's file system is case-insensitive and holds one of
// the two READMEs.) This shell gives the second order for both, under either
// locale.
// Collating the listing alone would sort one kind of word two ways, which is
// what this file exists to prevent, so the listing keeps byte order with
// globbing, and the two are one deliberate divergence rather than two. A
// collation, if one is ever built, is one table for both surfaces, per
// platform and measured — the maintainer's decision, 2026-10-06.
func shellOrder(a, b string) int { return strings.Compare(a, b) }

// shellOrderFolded is shellOrder with case put aside, which one flag of a
// parameter expansion asks for and nothing else does. The same function
// underneath, so a change to the order reaches both.
func shellOrderFolded(a, b string) int {
	return shellOrder(strings.ToLower(a), strings.ToLower(b))
}

// numericSegmentOrder is shellOrder with each run of digits read as the
// number it spells, which one shell in the panel asks for when it generates
// filenames — see Runner.SortsGlobMatchesNumerically.
//
// The same comparator the `n` flag of a parameter expansion reaches, and one
// implementation rather than two: the two surfaces were measured against each
// other before this was written and the reference answers them identically,
// including on the pair that decides the rule. Measured on zsh 5.9.2
// (`/opt/homebrew/bin/zsh`, `-f`), 2026-09-26, over a directory holding the
// same nine names as the array:
//
//	setopt numericglobsort; print -r -- *   f01z f1a f010 f10 g001 g01 g1 h01y h1x
//	print -r -- ${(on)a}                    f01z f1a f010 f10 g001 g01 g1 h01y h1x
//
// A second comparator beside compareNatural would have been a second place
// for the rule to be wrong in, which is what happened to the first draft of
// this: it was written out again here and got the `f01z` row right while the
// flag next door went on getting it wrong.
func numericSegmentOrder(a, b string) int { return compareNatural(a, b, false, false) }
