// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// A Remark is something the parser has to say about input it nonetheless
// accepted — a third channel beside the error and the tree.
//
// It exists because a parse can be *wrong enough to mention and right enough
// to run*: a here-document whose delimiter never arrives is taken as
// everything to the end of the input and the command runs, and one shell in
// the panel says so on the way past while three say nothing.
//
// Two things about the shape were measured before it was built, and both rule
// out simpler designs:
//
//   - A remark can accompany a *fatal* error. `f() { cat <<X` with neither
//     the delimiter nor the `}` produces the warning and then the syntax
//     error, so remarks are not "what the parser says when it succeeds" and a
//     front end that reads them only in the else branch would drop this one.
//   - A remark carries *two* positions. The warning is located at the line
//     the input ran out on and names, inside its wording, the line the
//     here-document began on.
//
// Whether anything is said at all is the front end's to decide from its
// dialect: three of the four say nothing, so silence is an answer rather than
// a missing feature.
type Remark struct {
	// Kind is what happened. There is one today, and the type exists so a
	// second does not change the shape of everything that reads these.
	Kind RemarkKind
	// Pos is where the input ran out, which is where the remark is located.
	Pos Pos
	// At is where the construct it is about began.
	At Pos
	// Token is what was being waited for — the here-document's delimiter.
	Token string
}

// RemarkKind is which remark this is.
type RemarkKind int

const (
	// RemarkNone is the zero value and is never produced.
	RemarkNone RemarkKind = iota
	// RemarkHeredocAtEOF is a here-document whose delimiter never arrived.
	// The body is everything to the end of the input, and the command runs.
	RemarkHeredocAtEOF
)

func (k RemarkKind) String() string {
	if k == RemarkHeredocAtEOF {
		return "RemarkHeredocAtEOF"
	}
	return "RemarkNone"
}

// Remarks is what the parser has to say about input it accepted anyway.
//
// Read it whether or not Err returned something: a remark can accompany a
// fatal error, and that is measured rather than hypothetical.
func (p *Parser) Remarks() []Remark { return p.lex.remarks }
