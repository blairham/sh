// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// How a listing is arranged, against the measurements in completelist.go.
//
// Nothing here names a shell: what a block is called and which builtin letter
// asks for one belongs in dialect/zsh, beside the pty run that measured it.
// This is what the *editor* does with a block once it has been given one.

// The rows of one listing, in order, with the headings in place.
func TestAListingIsDrawnBlockByBlock(t *testing.T) {
	first := Group{Name: "g1", Heading: "first group"}
	second := Group{Name: "g2", Heading: "second group"}
	got := listingRows([]Candidate{
		{Word: "delta", Display: "delta", Group: first},
		{Word: "alpha", Display: "alpha", Group: first},
		{Word: "charlie", Display: "charlie", Group: first},
		{Word: "zulu", Display: "zulu", Group: second},
		{Word: "bravo", Display: "bravo", Group: second},
	}, 40, listLayout{})
	want := []string{
		"first group",
		"alpha    charlie  delta",
		"second group",
		// Spread across the 27 columns the first block's grid spans, which
		// is the row the measurement in completelist.go's comment draws and
		// what this test asserted the packed `bravo  zulu` against until
		// #6157.
		"bravo        zulu",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// Sorted within a block by the word, and the row a candidate carries goes
// where its word went.
//
// The discriminating part is that the rows are not in the order they arrived
// *and* not sorted among themselves: `delta:D` sorts under `alpha:A` because
// `delta` sorts under `alpha`, and sorting the rows would have put `alpha:A`
// first for a different reason. The `:Z` is what tells the two apart.
func TestARowGoesWhereItsWordGoes(t *testing.T) {
	got := listingRows([]Candidate{
		{Word: "delta", Display: "delta:A"},
		{Word: "alpha", Display: "alpha:Z"},
	}, 0, listLayout{})
	want := []string{"alpha:Z", "delta:A"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// A block that asked to keep its order keeps it, and one beside it still
// sorts. Two blocks in one listing, so that a listing-wide sort would fail
// rather than merely look right.
func TestAnUnsortedBlockKeepsTheOrderItWasGiven(t *testing.T) {
	kept := Group{Name: "kept", Unsorted: true}
	sorted := Group{Name: "sorted"}
	got := listingRows([]Candidate{
		{Word: "zulu", Display: "zulu", Group: kept},
		{Word: "bravo", Display: "bravo", Group: kept},
		{Word: "yankee", Display: "yankee", Group: kept},
		{Word: "zeta", Display: "zeta", Group: sorted},
		{Word: "beta", Display: "beta", Group: sorted},
	}, 0, listLayout{})
	want := []string{"zulu", "bravo", "yankee", "beta", "zeta"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// A block asked for one row per line gets one, and a block beside it is still
// packed — which is the whole of why the flag is on the block.
//
// The width is wide enough to pack both, so a packed row is the failure this
// can see.
func TestOneRowPerLineIsABlocksAnswerAndNotTheListings(t *testing.T) {
	described := Group{Name: "d", OnePerLine: true}
	bare := Group{Name: "b"}
	got := listingRows([]Candidate{
		{Word: "-d", Display: "-d  -- decompress", Group: described},
		{Word: "-f", Display: "-f  -- force overwrite", Group: described},
		{Word: "-1", Display: "-1", Group: bare},
		{Word: "-2", Display: "-2", Group: bare},
	}, 80, listLayout{})
	want := []string{"-d  -- decompress", "-f  -- force overwrite", "-1  -2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// A heading of several rows draws several rows, which is what a completion
// system with two things to say about one block leaves behind.
func TestAHeadingOfSeveralRowsDrawsSeveralRows(t *testing.T) {
	g := Group{Name: "gx", Heading: "first heading\nsecond heading"}
	got := listingRows([]Candidate{
		{Word: "gamma", Display: "gamma", Group: g},
		{Word: "delta", Display: "delta", Group: g},
	}, 0, listLayout{})
	want := []string{"first heading", "second heading", "delta", "gamma"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// A block with a heading and nothing under it still draws the heading, and a
// block with neither draws nothing at all.
//
// The first is a completion system that had something to say and nothing to
// offer; the second is what a Group costs when nobody filled it.
func TestABlockWithNothingInItDrawsOnlyWhatItHasToSay(t *testing.T) {
	said := Group{Name: "said", Heading: "a message"}
	silent := Group{Name: "silent"}
	got := listingRows([]Candidate{{Group: said}, {Group: silent}}, 0, listLayout{})
	if strings.Join(got, "|") != "a message" {
		t.Errorf("drew %q, want the message alone", got)
	}
}

// A candidate with no row of its own is drawn as its word, without the
// directory already typed — which is the listing this package drew before
// there were blocks, and is what an answer of plain words still gets.
func TestAWordWithNoRowIsDrawnWithoutTheDirectoryAlreadyTyped(t *testing.T) {
	got := displayCandidates(Words("sub/nested.txt", "sub/other.txt"), "sub/")
	want := []string{"nested.txt", "other.txt"}
	for i, c := range got {
		if c.Display != want[i] {
			t.Errorf("row %d is %q, want %q", i, c.Display, want[i])
		}
	}
}

// And a candidate that brought its own row keeps it whole. A completion
// system that padded `name  -- sentence` to a width did so knowing what it
// meant to draw, and trimming a path off the front of that would cut it in
// the middle of the padding.
func TestARowACompleterBuiltIsLeftAsItIs(t *testing.T) {
	got := displayCandidates([]Candidate{
		{Word: "sub/nested.txt", Display: "nested.txt  -- a file"},
	}, "sub/")
	if got[0].Display != "nested.txt  -- a file" {
		t.Errorf("row is %q, want it untouched", got[0].Display)
	}
}

// A listing-only row is not a match: it is never inserted, it takes no part in
// the common prefix, and one word beside two of them is still a lone match
// that gets filled straight in.
//
// This is the rule that makes a message safe to add. Without it a completion
// system that said `no matches for this` would have inserted that sentence
// into the line the moment it was the only thing there.
func TestARowThatIsNotAMatchIsNeverInserted(t *testing.T) {
	c := CompleterFunc(func(Completion) []Candidate {
		return []Candidate{
			{Display: "no files here", Group: Group{Name: "msg"}},
			{Word: "banana.txt", Display: "banana.txt"},
		}
	})
	e := &editor{line: []rune("echo ban"), pos: 8, out: &strings.Builder{}}
	if listed, _ := e.complete(c); listed != nil {
		t.Errorf("listed %v, want the lone match filled in instead", listed)
	}
	if got := string(e.line); got != "echo banana.txt " {
		t.Errorf("line is %q, want the one real match completed", got)
	}
}

// The blocks of one listing share one grid width, and the options that
// rearrange a block do what they were measured to do. Every row is zsh
// 5.9.2's, 2026-10-05, through a pseudo-terminal 40 columns wide with a
// `.list-choices` widget adding the blocks; this shell draws the same rows
// once trailing blanks are taken off (#6157).
func TestAListingsBlocksShareOneGrid(t *testing.T) {
	block := func(name string, words ...string) []Candidate {
		out := make([]Candidate, len(words))
		for i, w := range words {
			out[i] = Candidate{Word: w, Group: Group{Name: name}}
		}
		return out
	}
	join := func(blocks ...[]Candidate) []Candidate {
		var out []Candidate
		for _, b := range blocks {
			out = append(out, b...)
		}
		return out
	}
	four := block("a", "alpha", "beta", "gamma", "zeta")
	ten := block("a", "a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10")
	long := block("a", "aaaaaaaaaaaa", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	for _, c := range []struct {
		name   string
		in     []Candidate
		layout listLayout
		want   []string
	}{
		{
			"two across the first block's grid", join(four, block("b", "delta", "eps")),
			listLayout{},
			[]string{"alpha  beta   gamma  zeta", "delta         eps"},
		},
		{
			"three across it", join(four, block("b", "d1", "d2", "d3")),
			listLayout{},
			[]string{"alpha  beta   gamma  zeta", "d1       d2       d3"},
		},
		{
			"never narrower than its own cell", join(four, block("b", "d1", "d2", "d3", "d4", "d5", "d6", "d7")),
			listLayout{},
			[]string{"alpha  beta   gamma  zeta", "d1  d2  d3  d4  d5  d6  d7"},
		},
		{
			"a later block widens an earlier one", join(block("a", "alpha", "beta"), block("b", "d1", "d2", "d3", "d4", "d5", "d6", "d7")),
			listLayout{},
			[]string{"alpha         beta", "d1  d2  d3  d4  d5  d6  d7"},
		},
		{
			"the grid counts the cells that fit, used or not", join(ten, block("b", "d1", "d2", "d3")),
			listLayout{},
			[]string{"a1   a2   a4   a6   a8", "a10  a3   a5   a7   a9", "d1           d2           d3"},
		},
		{
			"a block a row each takes no part", join([]Candidate{{Word: "x", Display: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Group: Group{Name: "a", OnePerLine: true}}}, block("b", "d1", "d2", "d3")),
			listLayout{},
			[]string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "d1  d2  d3"},
		},
		{
			"packed where it takes fewer rows", long,
			listLayout{packed: true},
			[]string{"aaaaaaaaaaaa  c  e  g  i", "b             d  f  h  j"},
		},
		{
			"packed, and the last column counts", block("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "xxxxxxxxxxxxxxxxx"),
			listLayout{packed: true},
			[]string{"b  d  f  h  j  l  n  p", "c  e  g  i  k  m  o  xxxxxxxxxxxxxxxxx"},
		},
		{
			"packed, columns of their own widths", block("a", "aaaaaaaa", "bb", "cccccc", "d", "eeeeeeeee", "ff", "ggg", "h", "iiiiiii", "jj", "kkkkk", "l", "m"),
			listLayout{packed: true},
			[]string{"aaaaaaaa  d          ggg      jj     m", "bb        eeeeeeeee  h        kkkkk", "cccccc    ff         iiiiiii  l"},
		},
		{
			"not packed where it saves no row", join(ten, block("b", "d1", "d2", "d3")),
			listLayout{packed: true},
			[]string{"a1   a2   a4   a6   a8", "a10  a3   a5   a7   a9", "d1           d2           d3"},
		},
		{
			"a packed block still spans the shared grid", join(long, block("b", "d1", "d2", "d3")),
			listLayout{packed: true},
			[]string{"aaaaaaaaaaaa  c  e  g  i", "b             d  f  h  j", "d1       d2       d3"},
		},
		{
			"across the rows", join(ten, block("b", "d1", "d2", "d3")),
			listLayout{rowsFirst: true},
			[]string{"a1   a10  a2   a3   a4   a5   a6   a7", "a8   a9", "d1           d2           d3"},
		},
		{
			"across the rows, two to a row", long,
			listLayout{rowsFirst: true},
			[]string{"aaaaaaaaaaaa  b", "c             d", "e             f", "g             h", "i             j"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := listingRows(c.in, 40, c.layout)
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

// A grid of names and descriptions, as a completion system builds it for
// options that share a description: the names in an unsorted block, blank
// fillers where a row has fewer names than the widest, and the descriptions
// as a last column of fillers padded out to what the names leave. Packed,
// it draws a row per description; without the fillers taking their cells,
// every later cell of a column would rise a row (#6178).
//
// The rows are zsh 5.9.2's, measured 2026-10-05 on a 100-column terminal
// with the cells `compdescribe -g` answered for `-a:same -b:same -c:same
// -d:other` — see dialect/zsh/compdescribepack.go.
func TestAFillerKeepsItsCellInAPackedGrid(t *testing.T) {
	grid := Group{Name: "ej", Unsorted: true, Packed: true}
	pad := func(s string) string { return s + strings.Repeat(" ", 86-len(s)) }
	got := listingRows([]Candidate{
		{Word: "-c", Display: "-c", Group: grid},
		{Word: "-d", Display: "-d", Group: grid},
		{Word: "-b", Display: "-b", Group: grid},
		{Filler: true, Group: grid},
		{Word: "-a", Display: "-a", Group: grid},
		{Filler: true, Group: grid},
		{Filler: true, Display: pad("-- same"), Group: grid},
		{Filler: true, Display: pad("-- other"), Group: grid},
		{Word: "-y", Group: Group{Name: "ej-bare"}},
		{Word: "-z", Group: Group{Name: "ej-bare"}},
	}, 100, listLayout{})
	want := []string{
		"-c  -b  -a  " + pad("-- same"),
		"-d          " + pad("-- other"),
		// And the block after it spreads across the grid's span, which is
		// the whole screen less two: `-z` at 49.
		"-y" + strings.Repeat(" ", 47) + "-z",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The span a packed block lends the blocks after it: its own columns where
// they are wider than its uniform grid, capped two short of the screen.
// Measured on zsh 5.9.2, 2026-10-05; see listingRows.
func TestAPackedBlockLendsItsSpan(t *testing.T) {
	long := []Candidate{{Word: strings.Repeat("a", 49)}}
	for _, w := range []string{"b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z1", "z2", "z3", "z4", "z5", "z6", "z7", "z8", "z9"} {
		long = append(long, Candidate{Word: w})
	}
	next := []Candidate{{Word: "y1", Group: Group{Name: "b"}}, {Word: "y2", Group: Group{Name: "b"}}}
	got := listingRows(append(long, next...), 100, listLayout{packed: true})
	want := "y1" + strings.Repeat(" ", 42) + "y2"
	if last := got[len(got)-1]; last != want {
		t.Errorf("the block after drew %q, want %q (y2 at 44)", last, want)
	}
}
