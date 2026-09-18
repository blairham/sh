// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// The public completion seam: how something other than this package answers
// Tab.
//
// It is deliberately in-process and deliberately synchronous, and the two are
// the same decision. Completion happens on the keystroke path — a Tab is a
// key, and the editor is holding a half-drawn line while it waits — so a
// completer that blocks blocks the person typing. That is stated here rather
// than defended against, and the reason it is not defended against is worth
// having in one place:
//
//   - A deadline would report something other than what happened. Abandoning
//     a slow completer and drawing "no matches" says the word cannot be
//     completed, when what is true is that nobody answered in time. This is
//     #493's finding about a deadline on a permission decision, and it reads
//     the same way one word to the left.
//   - Abandoning it costs a goroutine. A Go function that ignores
//     cancellation goes on running after the value that was waiting for it is
//     gone, and #690 is this repository's standing example of a leak per
//     occurrence in a package meant to be embedded in a long-lived program.
//     A leak per Tab is worse than a slow Tab.
//
// So a Completer is called on the editor's own goroutine and its answer is
// waited for. What follows from that is the answer to the question
// docs/design/plugins.md left open: this seam **cannot** be remoted as it
// stands, because a process round trip per keystroke is the hot-path
// exclusion that document already writes down. A future plugin role has to
// bring its own answer to slowness — a cancel notification and a peer that
// may be dropped — and it may do that additively without changing this
// interface, which is why the two were not designed together.

// Completion is what a completer is asked: which word, where it sits, and the
// line around it.
//
// A struct rather than an argument list because it is the thing most likely to
// gain a field, and a surface is easier to widen than to narrow. Everything in
// it is a fact the editor already holds at the moment Tab is pressed; nothing
// here is computed for the seam.
type Completion struct {
	// Line is the whole line as it has been typed, without the prompt.
	Line string

	// Point is where the cursor sits: an index into Line, in bytes, between
	// characters. Bytes rather than runes because Line is a string and every
	// caller will be slicing it; the editor holds runes and converts here, so
	// the conversion happens once rather than in each completer.
	Point int

	// Start is where the word under the cursor begins, also a byte index into
	// Line, so Line[Start:Point] is Word.
	Start int

	// Word is the text a completion replaces: everything from the start of
	// the word to the cursor, quoting and backslashes as typed. A completer
	// returns whole replacement words, not suffixes, so what it returns has
	// to begin with whatever of Word it means to keep.
	Word string

	// Command reports whether Word is the first word of a command rather than
	// an argument to one. It is the distinction that makes completion feel
	// like it understands the line, and it is decided by what precedes the
	// word rather than by what is in it — see commandPosition.
	Command bool

	// Dir is the shell's working directory.
	//
	// Here because it is not the process's. A Runner holds its own directory
	// and `cd` moves only that, so a completer resolving a relative path has
	// no other way to ask — the same reason the plugin protocol has a
	// `shell/dir` and for want of it would be resolving against whatever
	// directory the embedding program happened to be started in.
	Dir string
}

// Escape writes a literal name as a whole replacement word for this one.
//
// It is here because the alternative is every completer reinventing the
// shell's own quoting, and getting it wrong the same way each time: a
// completer that returns `surprising name.txt` has offered two arguments, and
// the person who pressed Tab sees a word split in half by a space they never
// typed. Which characters need a backslash depends on the quotation the word
// is already inside — a single quote inside double quotes is an ordinary
// character and a dollar sign is not — and that is a fact about this
// Completion rather than about the name, which is why this is a method.
//
// The whole word, opening quote included, because that is what a completer's
// answer replaces. A name with a directory in it is written out in full: the
// substrate's own completion keeps the directory a person typed so that
// `~/Deve` finishes as `~/Developer/` rather than as the home directory
// spelled out, and a completer that wants that keeps its own prefix.
func (c Completion) Escape(name string) string {
	quote := wordQuote(c.Word)
	if quote == 0 {
		return escapeName(name, 0, true)
	}
	return string(quote) + escapeName(name, quote, true)
}

// Candidate is one thing the word under the cursor could become, and what a
// listing draws for it.
//
// Two fields where this seam had one string, and the reason the string was not
// enough is that a listing and an insertion are different texts. A shell with
// a completion system draws `checkout  -- checkout branch or paths to working
// tree` and inserts `checkout`; computing the common prefix over the first of
// those, or putting it into the line, is what a single string forces. So the
// word a completion *is* and the row it is *drawn as* are separated here, and
// neither is derived from the other.
//
// The zero Candidate is not a completion. A Candidate with no Word is a row
// the listing draws and the editor never inserts — a heading's worth of text
// from a completion system with something to say and nothing to offer — and
// one with neither Word nor Display draws no row at all, which is how a Group
// comes to exist without a match in it.
type Candidate struct {
	// Word is the whole replacement word, in the line's own quoting: what
	// goes into the line, and what the common prefix is computed over.
	//
	// Empty is a row that is listed and never inserted. It takes no part in
	// the prefix, it is not what a lone match means, and a listing of nothing
	// but these is still a listing.
	Word string

	// Display is the row a listing draws. Empty is Word itself, which is what
	// a completer with nothing extra to say leaves it as — so `[]Candidate`
	// built from names alone behaves exactly as `[]string` did.
	//
	// Whole rather than a description beside a name, because that is the
	// shape the measurement has: zsh's `compadd -d` replaces the drawn text
	// outright, and the `name  -- sentence` rows people mean when they say
	// zsh's completion is better than bash's are those strings, already laid
	// out and padded by the function that built them. A `Description` field
	// would be this package inventing a layout nobody asked it for.
	Display string

	// Group is the block of the listing this candidate is drawn in.
	Group Group
}

// Group is a block of a listing: candidates whose Group compares equal are
// drawn together, under one heading, in one arrangement.
//
// By value rather than by pointer or by index, so that a completer builds one
// and puts it on each candidate without holding anything. Equality is the
// whole of the identity — Name is what distinguishes two blocks that are
// otherwise alike, and nothing here reads it.
//
// The zero Group is one block, sorted, packed into columns and unheaded,
// which is the listing this package drew before groups existed.
type Group struct {
	// Name distinguishes this block from another with the same heading and
	// the same arrangement. Nothing draws it.
	Name string

	// Heading is drawn above the block, or none.
	//
	// Rows rather than a row: a newline in it draws another line. A
	// completion system that reaches the same block twice has two things to
	// say about it and says both — measured on zsh, where two explanations
	// for one group draw two rows over one sorted block — and a string keeps
	// a Group comparable, which is what makes it the identity of a block
	// rather than a thing looked up beside one.
	Heading string

	// Unsorted draws the block in the order the candidates arrived rather
	// than sorted by the word. A completion system that has already ordered
	// its answer — by relevance, by a definition's own order — asks for this;
	// anything else is better read alphabetically.
	Unsorted bool

	// OnePerLine draws one row per line instead of packing the block into
	// columns. What a row carrying a sentence needs and what a bare name does
	// not, which is why it is a property of the block rather than of the
	// listing.
	OnePerLine bool
}

// Completer answers what the word under the cursor could become.
//
// The words are whole replacements in the line's own quoting, not display
// text: the editor puts one straight into the line when it is the only one,
// and trims the directory already typed off all of them when it lists.
// Returning nothing means this completer has nothing to say about this word,
// which is different from saying the word cannot be completed — see the
// composition rule on Shell.Completers.
//
// Called on the editor's goroutine while a key is being handled. Read the note
// at the top of this file before writing one that does I/O.
type Completer interface {
	Complete(c Completion) []Candidate
}

// CompleterFunc adapts a function to Completer.
type CompleterFunc func(Completion) []Candidate

// Complete calls f.
func (f CompleterFunc) Complete(c Completion) []Candidate { return f(c) }

// Words is the candidates for a list of replacement words that carry nothing
// else — the shape every completer had before a candidate could carry a row
// of its own, and the shape most of them still want.
func Words(words ...string) []Candidate {
	if len(words) == 0 {
		return nil
	}
	out := make([]Candidate, len(words))
	for i, w := range words {
		out[i] = Candidate{Word: w}
	}
	return out
}

// completers is a list of them consulted as one.
//
// The rule is that the first one with anything to say is the whole answer, and
// the lists are not merged. That is measured rather than chosen: both shells
// with a completion system keep exactly **one** completer per command name and
// a second registration replaces the first — bash 5.3.15 and 3.2.57 given two
// `complete -W` for the same name report only the second from `complete -p`,
// and zsh 5.9.2 given two `compdef` for the same name leaves only the second
// in `$_comps`. Neither offers the union.
//
// Merging would also be wrong here for a reason of our own. A completion is a
// replacement word in the line's own quoting, so two completers that disagree
// about how to escape a space produce a list in which the common prefix — the
// thing a second Tab fills in — is computed across two conventions. One
// answer, from one source, is a list that means something.
type completers []Completer

func (cs completers) Complete(c Completion) []Candidate {
	for _, one := range cs {
		if one == nil {
			continue
		}
		if matches := one.Complete(c); len(matches) > 0 {
			return matches
		}
	}
	return nil
}
