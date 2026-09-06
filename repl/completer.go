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

// Completer answers what the word under the cursor could become.
//
// The answers are whole replacement words in the line's own quoting, not
// display text: the editor puts one straight into the line when it is the only
// one, and trims the directory already typed off all of them when it lists.
// Returning nothing means this completer has nothing to say about this word,
// which is different from saying the word cannot be completed — see the
// composition rule on Shell.Completers.
//
// Called on the editor's goroutine while a key is being handled. Read the note
// at the top of this file before writing one that does I/O.
type Completer interface {
	Complete(c Completion) []string
}

// CompleterFunc adapts a function to Completer.
type CompleterFunc func(Completion) []string

// Complete calls f.
func (f CompleterFunc) Complete(c Completion) []string { return f(c) }

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

func (cs completers) Complete(c Completion) []string {
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
