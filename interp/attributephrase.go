// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// attributePhraseHead is the second vocabulary a bare declaration listing has
// for the attributes on a name — see BareLocalListsAttributedNames — ready to
// sit in front of the name, with one trailing space, and empty where the name
// carries no attribute at all.
//
// It is a second function beside attributeWordHead rather than a flag on it
// because the two vocabularies **share no word**. The one shell says
// `association`, `integer 16`, `uppercase`, `exported`; the other says
// `associative array`, `integer base 16`, `toupper`, `export`. There is no
// list with a switch in it that yields both, and a renderer that tried would
// be a table of pairs pretending to be a rule.
//
// What the two do share is the *risk*, and that is guarded rather than
// commented: an attribute a declaration can carry must be spelled by both
// heads, because a second helper omitting what the first carries is this
// tree's commonest defect. See TestBothAttributeVocabulariesSpellEveryAttribute.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME,
// one name per combination, read back out of a bare `typeset`:
//
//	typeset -x v       export v
//	typeset -r v       readonly v
//	typeset -a v       indexed array v
//	typeset -A v       associative array v
//	typeset -i v       integer v
//	typeset -i10 v     integer v            a named base of ten is the default
//	typeset -i16 v     integer base 16 v    and any other base is a word
//	typeset -u v       toupper v
//	typeset -l v       tolower v
//	typeset -li v      long integer v       the case letters are not case
//	typeset -ui v      unsigned integer v   letters beside `-i`
//	typeset -uli v     long unsigned integer v
//	typeset -li16 v    long integer base 16 v
//	typeset -xr v      export readonly v    and `-rx` is the same two words
//	typeset -ri v      readonly integer v
//	typeset -xa v      export indexed array v
//	typeset -ua v      indexed array toupper v
//	typeset -uA v      associative array toupper v
//	typeset -ru v      readonly toupper v
//
// So the order is: export, readonly, the kind, then the case — which is not
// the order the other vocabulary writes and is the second reason the two are
// two functions.
//
// **One word is out of reach for a reason that is not this listing's.**
// `typeset -uli v` lists as `long unsigned integer` there and as `unsigned
// integer` here, because this engine holds the two case letters as exclusive
// — the later one clears the earlier — where that shell keeps both when `-i`
// is on the name. The divergence is in the *attribute* and not in the words:
// `typeset -p v` already writes `typeset -l -u -i v=7` against our `typeset
// -u -i v=7`, on main and before any of this. Left where it was found rather
// than fixed in passing, because making the letters non-exclusive is a
// question about every declaration listing and not about this one.
//
// **The words this shell has that no name here can carry are left out rather
// than guessed at**, and they are left out because its own letter set cannot
// reach them: `nameref`, `tagged`, `filename`, `long integer`, `short
// integer`, `exponential`, `float`, `zerofill`, `leftjust` and `rightjust`
// come from `-n`, `-t`, `-H`, `-li`, `-si`, `-E`, `-F`, `-Z`, `-L` and `-R`,
// and none of those letters is in this dialect's DeclareOptions. A name that
// cannot be declared cannot be listed, so every word above is reachable and
// every word left out is not (#1461, #2345).
func (r *Runner) attributePhraseHead(d declaration) string {
	var words []string
	if d.exported {
		words = append(words, "export")
	}
	if d.readonly {
		words = append(words, "readonly")
	}
	switch {
	case d.isAssoc:
		words = append(words, "associative array")
	case d.isArr:
		words = append(words, "indexed array")
	}
	if d.integer {
		// **Neither case letter is a case letter on an integer**, and this
		// is the one place the two halves of this head are not independent.
		// `-l` beside `-i` is the *width* and `-u` beside it is the *sign*:
		// `typeset -li v` lists as `long integer`, `typeset -ui v` as
		// `unsigned integer`, and the two together as `long unsigned
		// integer` whichever order they were written in. `typeset -l v` on
		// its own is `tolower` and `typeset -u v` is `toupper`.
		//
		// The letters themselves are not in dispute — both shells write
		// `typeset -l -i n=3` for `integer n=3` — so this is the listing
		// reading a pair it already holds, not a second attribute. It is
		// also why `integer n=3` must not come back as `integer tolower n`:
		// that word would name a folding this name does not do.
		if d.lower {
			words = append(words, "long")
		}
		if d.upper {
			words = append(words, "unsigned")
		}
		words = append(words, "integer")
		if d.base != 0 && d.base != 10 {
			words = append(words, "base", strconv.Itoa(d.base))
		}
	} else {
		if d.upper {
			words = append(words, "toupper")
		}
		if d.lower {
			words = append(words, "tolower")
		}
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " ") + " "
}
