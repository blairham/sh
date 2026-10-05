// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `compadd`: the builtin a completion function offers a candidate with.
//
// # Measured on zsh 5.9.2, 2026-09-15
//
// Through a pseudo-terminal, from inside a `zle -C` widget's function, with
// `git che` typed — so `PREFIX` is `che` — and the status and
// `$compstate[nmatches]` printed after each call:
//
//	compadd checkout cherry commit     0, nmatches 2, lists `checkout cherry`
//	compadd -P XX -- checkout cherry   0, nmatches 2, line becomes `XXche`
//	compadd -S = -- checkout           0, nmatches 1, line becomes `checkout=`
//	compadd -U -- zzz yyy              0, nmatches 2
//	compadd -- 'a b'                   1, nmatches 0
//	compadd -Q -- 'a b'                1, nmatches 0
//	compadd -p HID -- checkout         1, nmatches 0
//	compadd -d '(one two)' -- checkout cherry   0, lists `one two`
//
// and outside a completion function, on a `-c` line with `zmodload
// zsh/complete` first:
//
//	compadd x       can only be called from completion function, 1
//
// Four rules come out of that, and three of them are ones a reading of the
// manual gets wrong in the same direction:
//
//  1. **The candidates are filtered against `$PREFIX` as they are added.**
//     `commit` never becomes a match, and `nmatches` says 2 rather than 3. So
//     the status is "did anything match", not "was anything handed over".
//  2. **`-P` is not part of the match and `-p` is.** `-P XX` still matched
//     `checkout` against `che`; `-p HID` did not, because the candidate being
//     matched was `HIDcheckout`. Both are inserted. That is the whole of the
//     difference between the two, and it is the one worth measuring because
//     the names do not carry it.
//  3. **`-U` turns the filtering off** and nothing else: `zzz` and `yyy` are
//     both matches against a `che` that neither begins with.
//  4. **A match is quoted for the line unless `-Q`**, which is why `'a b'`
//     would have gone in as `a\ b` had it matched at all.
//
// # What is drawn, measured
//
// The listing options were consumed and dropped until #3041 and #3232, on the
// reasoning that repl's completion seam is answered with replacement words and
// a replacement word has nowhere to put a row. The seam carries a row now —
// see repl.Candidate — so what follows is what each of them means, measured on
// zsh 5.9.2, 2026-09-18 through a pseudo-terminal with a widget of my own, so
// that what is being read is this builtin and not a shipped function:
//
//	compadd -d '(one two)' -- checkout cherry        lists `one  two`
//	compadd -d disp -- alpha beta   disp=(DA DB)     lists `DA  DB`
//	compadd -J g1 -X 'first group' -- delta alpha    a heading, then the block
//	compadd -V g2 -- zulu bravo yankee               that order, unsorted
//	compadd -l … -d '(delta:D alpha:A charlie:C)'    one row per line, sorted
//	compadd -J g -x 'a message'                      the message, no matches
//	compadd -J g -X 'no matches here'                nothing at all
//
// Five rules, and the last two are the pair a reading of the manual gets
// backwards:
//
//  1. **`-d` replaces the drawn text outright**, and takes either an array's
//     name or a literal `(…)`. It is not a description appended to a name:
//     `checkout` drawn as `one` is what the first row above says. What the
//     shipped completions draw as `name  -- sentence` is a display string
//     `compdescribe` built and padded before it got here.
//  2. **A display string goes where its match goes.** The `-l` row sorts to
//     `alpha:A charlie:C delta:D`, which is the *matches* in order carrying
//     their own rows.
//  3. **`-J` sorts the block and `-V` keeps the order it was given.** That is
//     the whole of the difference between the two letters.
//  4. **`-x` is drawn even when the group has no matches, and `-X` is not.**
//     A message is a completion system saying something; an explanation heads
//     a block and there is nothing to head.
//  5. **`-x` wins over `-X`** where a call carries both.
//
// **Menus are still read and ignored.** `-1`, `-2` and `-o` name behavior
// this editor has not got — there is no menu completion here and no second
// sort order — so they are consumed rather than refused, for compctl.go's
// reason: a builtin that refused every call naming
// one would stop functions that are otherwise entirely servable.

func registerCompadd(r *interp.Runner) { r.Register("compadd", compaddBuiltin) }

// compaddArgumentOptions are the letters that take a following word, so that
// what follows one is never mistaken for a candidate. Getting this list short
// is how `compadd -J group -- x` comes to offer `group` as a completion.
const compaddArgumentOptions = "PSpsiIWdJVXxrRDOAFMEy"

// compaddFlagOptions are the letters that stand alone and may cluster.
const compaddFlagOptions = "akqQfenUl12CTuzo"

// compaddArray reads `-d`'s argument, which is an array's **name** or a
// literal list in parentheses.
//
// Both spellings, because both are written: `compdescribe`'s caller hands the
// name of the array it filled, and a completion function writing two rows
// inline writes `-d '(one two)'`. Measured, both draw the strings — see the
// file comment.
//
// Split on blanks and nothing cleverer. A display string with a blank in it —
// which is every `name  -- sentence` row — reaches here through the array
// name, where no splitting happens; the literal form is what a function writes
// for short words, and this is what that function means by it.
func compaddArray(r *interp.Runner, value string) []string {
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		return strings.Fields(value[1 : len(value)-1])
	}
	if values, ok := r.GetArray(value); ok {
		return values
	}
	if one, ok := r.GetVar(value); ok && one != "" {
		return []string{one}
	}
	return nil
}

// compaddOrders are the words `-o` takes as its argument, and the whole of
// how that letter is told from a candidate.
//
// `-o` is the one letter here whose argument is optional, and the rule is not
// "is there a next word" — it is **whether the next word is an order name**.
// Measured on zsh 5.9.2 from inside a widget with an empty `$PREFIX`, so that
// everything offered is counted:
//
//	compadd -o nosort -- alpha        1 match
//	compadd -o match -- alpha         1 match
//	compadd -o numeric -- alpha       1 match
//	compadd -o reverse -- alpha       1 match
//	compadd -o -- alpha               1 match — `--` is not eaten
//	compadd -o alpha                  1 match — `alpha` is not eaten
//	compadd -o zzz -- alpha           3 matches: `zzz`, `--` and `alpha`
//
// The last row is the one that pins the rule from the other side, and it is
// also what a parser that got this wrong would look like: a word that is not
// an order ends the options, so the `--` after it is an ordinary candidate.
// `compadd -o nosort` is written by real completions; offering `nosort` as a
// completion is what this table stops.
var compaddOrders = map[string]bool{
	"match": true, "nosort": true, "numeric": true, "reverse": true,
}

// compaddOptions is one call's letters, gathered.
type compaddOptions struct {
	prefix, suffix       string   // -P, -S: inserted, not matched
	suffixSet            bool     // -S given at all, even empty
	files                bool     // -f: the candidates are file names
	fileDir              string   // -W: what a file name is tested under
	hiddenPre, hiddenSuf string   // -p, -s: inserted *and* matched
	arrays               bool     // -a: the words name arrays
	keys                 bool     // -k: the words name associations
	unfiltered           bool     // -U: do not match against PREFIX
	raw                  bool     // -Q: insert the candidate unquoted
	into                 []string // -O or -A: store rather than offer
	filter               []string // -D: strike the non-matching out, in place
	withhold             bool     // -O, -A or -D: add nothing

	// What the listing draws. display is `-d`, unresolved: the argument as
	// written, since an array named there is read when the call runs and not
	// when the letter is seen.
	display    string // -d: the rows, an array's name or a literal list
	group      string // -J or -V: the block these matches are drawn in
	unsorted   bool   // -V: keep the order rather than sorting the block
	onePerLine bool   // -l: a row each rather than packed into columns
	heading    string // -X: the row drawn above the block
	message    string // -x: the same, and drawn with no matches under it
	hasMessage bool   // -x was given, which an empty message still is
	fillers    int    // -E: how many empty cells to add after the matches
	keepAll    bool   // -2: a block that keeps every match it is given

	// matchSpec is every `-M` given, joined: the completion system writes
	// the letter more than once in one call — measured, `_path_files` adds
	// `-M 'r:|/=* r:|=*'` after the matcher-list's own — and the matchers
	// are one list. See compmatch.go.
	matchSpec string
}

func compaddBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	cs, completing := completionFrom(ctx)
	if !completing {
		r.Diagnosef("can only be called from completion function\n")
		return 1
	}
	opts, rest, ok := compaddParse(r, args)
	if !ok {
		return 1
	}
	if opts.fillers < 0 {
		r.Diagnosef("invalid number: %d\n", opts.fillers)
		return 1
	}
	group := cs.group(opts)
	packed := slices.Contains(strings.Fields(cs.state["list"]), "packed")
	if packed {
		cs.packGroup(group)
	}
	candidates := compaddCandidates(r, opts, rest)
	offered := cs.add(r, ctx, opts, group, candidates)
	if opts.fillers > 0 {
		group = cs.fillerGroup(opts, group)
		if packed {
			cs.packGroup(group)
		}
		offered += cs.addFillers(r, opts, group, len(candidates))
	}
	// After the matches and not with the group, because what an explanation
	// draws depends on them: its `%n` is how many this call added, and an
	// explanation whose call added none is not drawn at all. See explain.
	cs.explain(r, opts, group, offered)
	if opts.hasMessage && offered == 0 {
		// A message is drawn whether or not the block has matches, so a call
		// that offered none of its own still leaves the block behind. A
		// candidate with neither a word nor a row is exactly that: the block
		// exists, and nothing is drawn under the heading.
		//
		// Only where nothing was offered, because a block with a match in it
		// is already there and a second empty candidate would be a row this
		// editor has to know to skip rather than one it never sees.
		cs.matches = append(cs.matches, repl.Candidate{Group: group})
	}
	return boolStatus(offered > 0)
}

// compaddParse reads the letters off the front, stopping at `--`, at a word
// that is not an option, and at a `-` on its own — which is a candidate.
func compaddParse(r *interp.Runner, args []string) (compaddOptions, []string, bool) {
	var o compaddOptions
	i := 0
	for ; i < len(args); i++ {
		word := args[i]
		if word == "--" || word == "-" {
			// A lone `-` ends the options exactly as `--` does, which is the
			// old spelling the manual's own example uses — `complete-files ()
			// { compadd - * }` — and which `_arguments` writes to this day:
			// `compadd … -D _a_11 - -a -m -n …` offers the seven option names
			// and not an eighth candidate spelled `-`. Measured on zsh 5.9.2,
			// 2026-09-15: with `PREFIX` empty, `compadd - alpha` offers
			// `alpha` alone.
			i++
			break
		}
		if len(word) < 2 || word[0] != '-' {
			break
		}
		letters := word[1:]
		for j := 0; j < len(letters); j++ {
			letter := letters[j]
			switch {
			case letter == 'o' && j == len(letters)-1 &&
				i+1 < len(args) && compaddOrders[args[i+1]]:
				// The one optional argument — see compaddOrders. Only where
				// the letter ends its word, since `-o` joined to its order
				// would have been read as a cluster.
				i++
			case strings.IndexByte(compaddFlagOptions, letter) >= 0:
				compaddFlag(&o, letter)
			case strings.IndexByte(compaddArgumentOptions, letter) >= 0:
				// The rest of the word if there is any, and the next word
				// otherwise — the two spellings every letter here accepts.
				value := letters[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						r.Diagnosef("argument expected after -%c\n", letter)
						return o, nil, false
					}
					i++
					value = args[i]
				}
				compaddArgument(&o, letter, value)
				j = len(letters)
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return o, nil, false
			}
		}
	}
	return o, args[i:], true
}

func compaddFlag(o *compaddOptions, letter byte) {
	switch letter {
	case 'a':
		o.arrays = true
	case 'k':
		o.keys = true
	case 'U':
		o.unfiltered = true
	case 'Q':
		o.raw = true
	case 'l':
		o.onePerLine = true
	case 'f':
		o.files = true
	case '2':
		o.keepAll = true
	}
}

func compaddArgument(o *compaddOptions, letter byte, value string) {
	switch letter {
	case 'P':
		o.prefix = value
	case 'S':
		o.suffix, o.suffixSet = value, true
	case 'W':
		o.fileDir = value
	case 'p':
		o.hiddenPre = value
	case 's':
		o.hiddenSuf = value
	case 'O', 'A':
		o.into, o.withhold = append(o.into, value), true
	case 'D':
		// More than once is allowed and is what `_arguments` writes: each
		// named array is struck through in parallel with the candidates.
		o.filter, o.withhold = append(o.filter, value), true
	case 'd':
		o.display = value
	case 'J':
		o.group = value
	case 'V':
		// The same field: a block is named once and the letter that named it
		// says whether it sorts. Measured — see the file comment.
		o.group, o.unsorted = value, true
	case 'X':
		o.heading = value
	case 'x':
		o.message, o.hasMessage = value, true
	case 'E':
		o.fillers = leadingCount(value)
	case 'M':
		if o.matchSpec != "" {
			o.matchSpec += " "
		}
		o.matchSpec += value
	}
}

// compaddCandidates is the words themselves, which are the arguments unless
// `-a` or `-k` said they are the names of parameters holding them.
//
// `-k` takes the *keys* of an association and the elements of an ordinary
// array, which is what makes it usable against either — a completion function
// writes `compadd -k mymap` without checking which it has.
func compaddCandidates(r *interp.Runner, o compaddOptions, words []string) []string {
	if !o.arrays && !o.keys {
		return words
	}
	var out []string
	for _, name := range words {
		if o.keys {
			// Stored or produced: `compadd -k commands` is how the command
			// names are offered, and `commands` is a produced table.
			if keys, ok := r.AssocKeys(name); ok {
				out = append(out, keys...)
				continue
			}
		}
		if values, ok := r.GetArray(name); ok {
			out = append(out, values...)
			continue
		}
		if value, ok := r.GetVar(name); ok && value != "" {
			out = append(out, value)
		}
	}
	return out
}

// add takes the candidates that match, and answers how many were **offered**.
//
// Offered and not matched, which is the measured rule and the discriminating
// one. On zsh 5.9.2, 2026-09-15, from inside a widget with `PREFIX` of `al`
// and candidates `alpha zzz alright`:
//
//	compadd -D arr -- alpha zzz alright   1, arr goes (A1 A2 A3) → (A1 A3)
//	compadd -O out -- alpha zzz alright   1, out=(alpha alright)
//	compadd -A a2 -- alpha zzz alright    1, a2=(alpha alright)
//	compadd -- alpha                      0
//	compadd -- zzz                        1
//
// So a call that stores rather than offers is **1 even though two candidates
// matched**, which is the manual's sentence read strictly: "the return status
// is zero if at least one match was added". This file used to answer 0 there,
// on the argument that `-O` needs to report whether anything matched; the
// argument is wrong and the three rows above are why — and it matters,
// because `_arguments` writes four `compadd -D` calls in a row and a 0 from
// any of them is a claim that something reached the line.
//
// `-D` strikes the non-matching out of each named array **by position**: the
// nth element goes when the nth candidate does not match, and an element past
// the end of the candidate list is left alone. Measured with a two-element
// array against three candidates, above.
func (cs *completionState) add(
	r *interp.Runner, ctx context.Context, o compaddOptions, group repl.Group, candidates []string,
) int {
	var matched []string
	kept := make([]bool, len(candidates))
	offered := 0
	// The rows, read now rather than when `-d` was seen: the array a caller
	// names is filled between the two.
	displays := compaddArray(r, o.display)
	spec := parseMatchSpec(o.matchSpec)
	for i, candidate := range candidates {
		// What is matched is the hidden prefix and suffix around the
		// candidate; what `-P` and `-S` add is not part of it. Measured —
		// see the file comment.
		subject := o.hiddenPre + candidate + o.hiddenSuf
		inserted := subject
		if !o.unfiltered {
			// Under the call's match specification, which an upper-case
			// matcher can answer with the word's own characters written
			// into what is inserted. Only into what is inserted: measured,
			// `-O` under `M:_=` with `f_o` typed stores `foo`, and `f_oo`
			// is what Tab puts on the line. See compmatch.go.
			written, ok := spec.matchCandidate(cs.prefix, subject)
			if !ok {
				continue
			}
			inserted = written
		}
		kept[i] = true
		matched = append(matched, subject)
		if !o.withhold {
			// The nth candidate's row, by position and not by match: a `-d`
			// array is as long as the candidate list the caller passed, so a
			// candidate the prefix struck out still costs its own row. That
			// is the same by-position rule `-D` is measured to follow.
			open, slash := fileSuffix(r, ctx, o, candidate)
			row := display(displays, i)
			if row == "" && o.files {
				row = fileRow(r, ctx, o, candidate)
			}
			cs.offer(inserted, candidate, o, row, group, open, slash)
			offered++
		}
	}
	for _, into := range o.into {
		r.SetArray(into, matched)
	}
	for _, from := range o.filter {
		if values, ok := r.GetArray(from); ok {
			r.SetArray(from, strikeUnmatched(values, kept))
		}
	}
	// The matches and not the rows: a block's message is a candidate here so
	// that the block exists with nothing in it, and `$compstate[nmatches]` is
	// what a completion function tests to decide whether anything was
	// offered.
	cs.state["nmatches"] = strconv.Itoa(len(insertableWords(cs.matches)) + cs.fillers)
	return offered
}

// insertableWords are the candidates that are matches: a listing-only row is
// drawn and never inserted, and never counted.
func insertableWords(candidates []repl.Candidate) []repl.Candidate {
	out := candidates[:0:0]
	for _, c := range candidates {
		if c.Word != "" {
			out = append(out, c)
		}
	}
	return out
}

// strikeUnmatched is `-D`'s array, with the elements whose candidate did not
// match taken out and everything past the candidates left where it was.
func strikeUnmatched(values []string, kept []bool) []string {
	out := make([]string, 0, len(values))
	for i, value := range values {
		if i >= len(kept) || kept[i] {
			out = append(out, value)
		}
	}
	return out
}

// offer writes one whole replacement word, quoted for the line unless the
// caller asked for it raw.
//
// Whole, because that is what repl's completion seam is answered with: a
// completer returns replacement words and not suffixes, so everything already
// on the line that this completion keeps — the opening quote, and whatever
// `compset` moved into `IPREFIX` — has to be written out again here.
//
// `-P` and `-S` go on unescaped and the candidate between them does not. They
// are delimiters a completion function chose — `=`, `/`, `:` — and escaping
// them would insert a backslash the function did not ask for; the candidate is
// a name and has to survive being read back by the parser.
//
// Duplicates are dropped rather than offered twice, which is what a listing
// with one entry per name needs and what makes a second Tab fill in from a
// set rather than from a bag.
func (cs *completionState) offer(subject, candidate string, o compaddOptions, row string, group repl.Group, open, slash bool) {
	body := cs.iprefix + o.prefix + subject
	word := cs.c.Escape(body) + o.suffix
	if o.raw {
		word = cs.qiprefix + body + o.suffix
	}
	if slash {
		word += "/"
	}
	if row == "" {
		if listed := cs.plainRow(candidate, o); listed != word {
			row = listed
		}
	}
	for _, have := range cs.matches {
		if have.Word == word {
			return
		}
	}
	cs.matches = append(cs.matches, repl.Candidate{Word: word, Display: row, Group: group, Open: open})
}

// plainRow is how a listing draws a candidate that brought no row of its own:
// the candidate alone, quoted the way it would be inserted, and nothing that
// is on the line around it (#6187).
//
// The word offer builds is the whole replacement, so drawing that instead puts
// in front of every row whatever `compset` moved into `IPREFIX`, and `-P`'s
// and `-p`'s prefixes and `-S`'s suffix around it. Measured 2026-10-05
// through a pseudo-terminal against zsh 5.9.2, a `zle -C` widget listing two
// matches:
//
//	compset -P '*='; compadd always auto    x --c=a    always  auto
//	compadd -p qq always auto               x qqa      always  auto
//	compadd -S ss always auto               x a        always  auto
//	compadd 'a b' 'a d'                     x a\       a\ b  a\ d
//
// A row that would be the word itself is left empty, so that the editor
// draws the word the way it draws every other plain candidate.
func (cs *completionState) plainRow(candidate string, o compaddOptions) string {
	if o.raw {
		return candidate
	}
	return cs.c.Escape(candidate)
}

// fileSuffix is what a lone match is finished with, where it is not the
// space every other match gets: whether it is left open, and whether a slash
// goes on it first.
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, a
// completion widget adding one match to `x ` and then `Z` typed:
//
//	compadd ff                      x ff Z
//	compadd -S '' ff                x ffZ      a suffix of any kind,
//	compadd -S x ff                 x ffxZ     the empty one included,
//	compadd -f -S x dd              x ddxZ     is the whole of it
//	compadd -f dd                   x dd/Z     a directory: a slash
//	compadd -f ff                   x ff Z     a file: the space
//	compadd -f nosuch               x nosuchZ  neither: nothing at all
//	compadd -W <tree>/ -f inner     x inner/Z  tested under -W's directory,
//	compadd -W <tree> -f dd         x ddZ      joined with no slash between,
//	compadd -W <tree>/ -p pre/ -f dd x pre/dd/Z and without -p's prefix
//
// The shipped `_path_files` adds every file with `-f` and `-W`, so without
// this a directory completed through it ended in a space where zsh leaves
// the slash and the word open.
func fileSuffix(r *interp.Runner, ctx context.Context, o compaddOptions, candidate string) (open, slash bool) {
	if o.suffixSet {
		return true, false
	}
	if !o.files {
		return false, false
	}
	info, err := fileStat(r, ctx, filePath(o, candidate))
	switch {
	case err != nil:
		return true, false
	case info.IsDir():
		return true, true
	}
	return false, false
}

// filePath is where a `-f` candidate is tested: `-W`'s directory with the
// candidate written straight after it. A relative one is the shell's own
// directory's, which the gated stat resolves; going through the gate means a
// sandbox that refuses the probe answers "nothing there".
func filePath(o compaddOptions, candidate string) string {
	return o.fileDir + candidate
}

// fileRow is how a listing draws a `-f` candidate: the name alone — not the
// directory `-p` carries in front of it — and, under LIST_TYPES, the mark `ls
// -F` would give it. Measured 2026-10-05 through a pseudo-terminal against
// zsh 5.9.2, `ls <TAB>` over a directory, a file, an executable, a FIFO, a
// link to the file and a link to the directory:
//
//	dlink@  exe*    fifo|   link@   plain   sub/
//
// and with `setopt nolisttypes` none of the marks, the directory's slash
// included — though a lone directory still inserts with one, and the column a
// mark would take is still taken. The link to a
// directory is drawn as a link and inserted as a directory (`ls dl<TAB>` is
// `ls dlink/`), so the mark is the link's own and the slash fileSuffix adds is
// its target's.
func fileRow(r *interp.Runner, ctx context.Context, o compaddOptions, candidate string) string {
	mark := fileMark(r, ctx, o, candidate)
	if mark == "" {
		return candidate
	}
	if recordedDeviates(r, "listtypes") {
		// The mark is not drawn, but its column is kept: measured 2026-10-05
		// at 40 columns, `dlink exe fifo link plain sub` under `nolisttypes`
		// draws `dlink   fifo    plain` in cells of eight, the width it has
		// with the marks, where `aa bb cc` — files with no mark — draws
		// `aa  bb  cc` either way. A blank is what the cell holds there, so
		// that is the row (#6157).
		return candidate + " "
	}
	return candidate + mark
}

// fileMark is the character LIST_TYPES draws after a `-f` candidate, or none.
func fileMark(r *interp.Runner, ctx context.Context, o compaddOptions, candidate string) string {
	info, err := fileLstat(r, ctx, filePath(o, candidate))
	if err != nil {
		return ""
	}
	return repl.FileTypeMark(info.Mode())
}

// display is the nth row of a `-d` list, or none where the caller gave no
// list or a shorter one than its candidates.
//
// Empty rather than an error for a short list, because the editor reads an
// empty row as "draw the word", which is what a candidate past the end of a
// display array has to be: there is nothing else to draw it as.
func display(rows []string, i int) string {
	if i >= len(rows) {
		return ""
	}
	return rows[i]
}

// group is the block this call's matches are drawn in.
//
// The identity of a block is its name and its arrangement, and not its
// heading. Both halves are measured, on zsh 5.9.2, 2026-09-18:
//
//   - **Two calls naming one group are one block, and both explanations are
//     drawn.** `-J gx -X 'first heading'` and `-J gx -X 'second heading'`
//     draw two heading rows and then `delta  gamma` — one sorted block under
//     two headings, not two blocks.
//   - **Two calls naming no group are one block too**, and the explanation
//     the first of them gave heads it: `-X 'unnamed heading' -- alpha` then a
//     plain `compadd -- beta` draws the heading once over `alpha  beta`.
//   - **An arrangement splits a name.** The `gzip -c` measurement in
//     compdescribe.go is one `-default-` definition drawn as two blocks,
//     because the described half carries `-l` and the bare half does not.
//
// So the heading cannot be part of what identifies a block — a second call
// with a second explanation would make a second block — and it cannot be
// settled when the call runs either, since a later call may add to it. It is
// recorded by explain and stamped on at the end; see
// completionState.groupedMatches.
func (cs *completionState) group(o compaddOptions) repl.Group {
	key := repl.Group{Name: o.group, Unsorted: o.unsorted, OnePerLine: o.onePerLine}
	if _, seen := cs.groups[key]; !seen {
		cs.groups[key] = nil
	}
	if o.keepAll {
		if cs.keepingAll == nil {
			cs.keepingAll = map[string]repl.Group{}
		}
		cs.keepingAll[o.group] = key
	}
	return key
}

// packGroup records that a block was added to while `$compstate[list]` said
// `packed`, which packs that block's columns whatever LIST_PACKED says.
//
// **When the call is made, not when the listing is drawn.** Measured on zsh
// 5.9.2, 2026-10-05, forty-two two-letter matches and one of fifty-eight
// letters added under `-J g`: two columns of twenty-two rows as they stand,
// three packed rows with `compstate[list]='list packed'` set before the
// `compadd`, and two columns of twenty-two again with it set after. It is
// how the completion system asks for the grid compdescribe.go builds to be
// drawn as one.
func (cs *completionState) packGroup(key repl.Group) {
	if cs.packedGroups == nil {
		cs.packedGroups = map[repl.Group]bool{}
	}
	cs.packedGroups[key] = true
}

// fillerGroup is the block an `-E` call's empty cells go into: the block a
// `-2` call made under the same name, where there is one, and the call's own
// otherwise.
//
// That is what lets the completion system add the names of a grid with
// `-2V ej` and its blanks and descriptions with `-J ej` and still have one
// grid. Measured on zsh 5.9.2, 2026-10-05, each a widget's whole body:
//
//	compadd -2V ej -- -b -a; compadd -E1 -J ej -d '(DESC)'   -b  -a  DESC
//	compadd -2V ej -- -b -a; compadd -E1 -V ej -d '(DESC)'   -b  -a  DESC
//	compadd -V ej -- -b -a;  compadd -E1 -J ej -d '(DESC)'   -b  -a / DESC
//	compadd -J ej -- -b -a;  compadd -E1 -J ej -d '(DESC)'   -a  -b / DESC
//	compadd -2V ej -- -b -a; compadd -E1 -J other -d …       -b  -a / DESC
//
// so the `-2` and the name decide it, and the filler's own `-J` or `-V`
// does not: without the `-2` the description is a block of its own, drawn on
// a line after the names — the fourth row's `DESC` even shares the `-J ej`
// the names were given, and is still not in their grid.
//
// That block is told apart by its name, which nothing draws: the call's own
// name with a byte no name can hold after it.
func (cs *completionState) fillerGroup(o compaddOptions, own repl.Group) repl.Group {
	if key, ok := cs.keepingAll[o.group]; ok {
		return key
	}
	own.Name += "\x00fillers"
	if _, seen := cs.groups[own]; !seen {
		cs.groups[own] = nil
	}
	return own
}

// addFillers is `-E n`: n cells that are listed and never inserted, each
// drawn as the `-d` row at its place after the call's own candidates, or
// blank.
//
// They are matches as far as `$compstate[nmatches]` is concerned, which is
// measured — `compadd -E2 -J g` answers 0 and leaves `nmatches` at 2, and a
// further `compadd -E1 -J g -- aa` takes it to 4 — and an explanation is
// drawn over them as over any other: `compadd -E2 -J g -X HEAD -d d` lists
// `HEAD` and then the two rows.
func (cs *completionState) addFillers(r *interp.Runner, o compaddOptions, group repl.Group, after int) int {
	displays := compaddArray(r, o.display)
	for i := range o.fillers {
		cs.matches = append(cs.matches, repl.Candidate{
			Display: display(displays, after+i), Group: group, Filler: true,
		})
	}
	cs.fillers += o.fillers
	cs.state["nmatches"] = strconv.Itoa(len(insertableWords(cs.matches)) + cs.fillers)
	return o.fillers
}

// leadingCount reads `-E`'s number the way zsh does: the digits it starts
// with, a minus sign in front of them included, and nothing at all as zero.
// Measured — `compadd -Ex` adds nothing and answers 1, and `compadd -E-1` is
// `invalid number: -1`.
func leadingCount(s string) int {
	sign, i := 1, 0
	if strings.HasPrefix(s, "-") {
		sign, i = -1, 1
	}
	n := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return sign * n
}

// explain records a call's explanation against its block, drawn the way the
// listing will show it. See compexplain.go for the language.
//
// Two rules, both measured on zsh 5.9.2, 2026-10-05:
//
//   - **A `-X` explanation is drawn only where its own call added a match.**
//     `compadd -X 'H' -J g nomatch` followed by `compadd -J g alfa` draws
//     `alfa` with no heading over it: the block is there, and the call that
//     explained it had nothing to explain.
//   - **A `-x` message is drawn either way, and wins over `-X`**, measured
//     earlier — see the file comment's fourth and fifth rules. Its `%n` is -1
//     whether or not the call added anything.
func (cs *completionState) explain(r *interp.Runner, o compaddOptions, key repl.Group, offered int) {
	text, count := o.heading, offered
	switch {
	case o.hasMessage:
		text, count = o.message, -1
	case offered == 0:
		return
	}
	if text == "" {
		return
	}
	drawn := explanationText(r, text, count)
	for _, have := range cs.groups[key] {
		if have == drawn {
			// **The same heading twice is drawn once.** Measured on zsh
			// 5.9.2, 2026-10-05: `-J g -X HH -- alpha` then `-J g -X HH --
			// beta` draws `HH` over `alpha  beta`, and `H1`, `H2`, `H1`
			// over three calls draws `H1` and `H2`. The completion system
			// writes one explanation on every cell of a grid, and drew it
			// once a cell here.
			return
		}
	}
	cs.groups[key] = append(cs.groups[key], drawn)
}

// groupedMatches is what `compadd` collected with each block's headings
// stamped on, which is the answer repl's seam is given.
//
// The last step of a completion rather than something offer() could do,
// because a block's headings are not all known until the last `compadd` has
// run — see group() for the two measurements that say so.
func (cs *completionState) groupedMatches() []repl.Candidate {
	out := make([]repl.Candidate, len(cs.matches))
	for i, c := range cs.matches {
		packed := cs.packedGroups[c.Group]
		if headings := cs.groups[c.Group]; len(headings) > 0 {
			c.Group.Heading = strings.Join(headings, "\n")
		}
		c.Group.Packed = packed
		out[i] = c
	}
	// And into the order `compgroups` asked for, where it was called. A
	// stable sort, so that a block nobody named keeps the place it was added
	// in and the candidates inside every block keep theirs — the listing
	// sorts within a block itself, and a block asked to stay unsorted must
	// come out of here in the order it went in.
	if len(cs.groupOrder) > 0 {
		sort.SliceStable(out, func(i, j int) bool {
			return cs.groupRank(out[i].Group.Name) < cs.groupRank(out[j].Group.Name)
		})
	}
	return out
}

// groupRank is where `compgroups` put a block, or the end for one it did not
// name.
//
// Measured on zsh 5.9.2, 2026-09-18: `compgroups second first` followed by a
// `compadd -J first` and a `compadd -J second` draws `SECOND beta` above
// `FIRST alpha`, so the declaration and not the order of the calls decides.
func (cs *completionState) groupRank(name string) int {
	for i, declared := range cs.groupOrder {
		if declared == name {
			return i
		}
	}
	return len(cs.groupOrder)
}
