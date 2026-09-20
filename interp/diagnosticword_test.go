// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How `${x?word}` reads its word (#3876).
//
// The bash family expands it the way a command line is expanded and makes the
// sentence of the fields; zsh, ksh93 and dash take it as a value, where `$*`
// joins on the first character of IFS.
//
// **Every row sets IFS, and that is not decoration.** With the default IFS
// the two readings coincide — the first character is already the space the
// fields reading supplies — so a probe without it cannot tell them apart.
// The last row is that probe, kept as a control: it must answer the same
// under both answers, and a test written only that way would pass on any
// implementation at all.
func TestDiagnosticWordIsFieldsAxis(t *testing.T) {
	const fired = `set -- 'a:b' c
IFS=:
unset e1; echo ${e1?$*}`

	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    string
		refused string
	}{
		{
			// bash 5.3.20 and bash 3.2.57.
			name: "the word is fields", answer: Yes,
			src: fired, want: "e1: a b c",
		},
		{
			// zsh 5.9.2, ksh93u+, dash 0.5.12.
			name: "the word is a value", answer: No,
			src: fired, want: "e1: a:b:c",
		},
		{
			// CONTROL. The assigning form takes the same word and is
			// unanimous on the value reading in all four columns, bash
			// included — so it must not move with this axis. If it does,
			// the question has been put on the shared helper instead of on
			// the operator that disagrees.
			name: "an assigning word is a value even where the diagnostic is fields", answer: Yes,
			src: `set -- 'a:b' c
IFS=:
unset u; : ${u:=$*}; printf '[%s]' "$u"`,
			want: "[a:b:c]",
		},
		{
			// CONTROL. Without a non-default IFS the two readings are the
			// same string, so this row must answer identically under both.
			// It is here to say that the rows above earn their IFS line.
			name: "the default IFS cannot tell them apart, answered Yes", answer: Yes,
			src: `set -- 'a:b' c
unset e2; echo ${e2?$*}`,
			want: "e2: a:b c",
		},
		{
			name: "the default IFS cannot tell them apart, answered No", answer: No,
			src: `set -- 'a:b' c
unset e2; echo ${e2?$*}`,
			want: "e2: a:b c",
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     fired,
			refused: "reading its word as fields",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			// Not this axis's questions, but every row splits `$*` and
			// every fired row ends the script, and the runner refuses an
			// unanswered axis. These are the values of the columns the
			// rows are measured against.
			sem.SplitParamExpansion = Yes
			sem.FatalErrorStatusIsOne = Yes
			sem.DiagnosticWordIsFields = tc.answer
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			want := tc.want
			if tc.refused != "" {
				want = tc.refused
			}
			if !strings.Contains(out, want) {
				t.Fatalf("got %q, want it to carry %q", out, want)
			}
		})
	}
}
