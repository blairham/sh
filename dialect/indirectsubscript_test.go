// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `${!name[@]}` is the subscript listing in the bare form and nothing else.
//
// Measured 2026-09-14, `env -i PATH=/usr/bin:/bin HOME=<scratch>`, a script
// file and again under `-c` and on standard input, the same answers on all
// three routes. With `typeset -A w; w[k]=tgt; tgt=HELLO`:
//
//	written        bash 5.3.15   ksh93u+
//	${!w[@]}       k             k
//	${!w[@]#H}     ELLO          bad substitution, and the script ends
//	${!w[@]:1:2}   EL            bad substitution
//	${!w[@]/L/x}   HExLO         bad substitution
//	${!w[@]+SET}   SET           bad substitution
//
// So bash puts the `!` back to being an ordinary indirection: the operand is
// `w[@]`, that expands to `tgt`, and the operator then runs on `HELLO`. Here
// the subscripts were listed whatever followed, so every row above came back
// as a filtered listing at status 0 (#2821).
//
// The rows are in one table because the bare form is the control. A reading
// that listed either way answers `[k]` for the first two, which is why the
// issue this came from recorded one row and read it as a substitution corner.
func TestAnOperatorAfterTheSubscriptListing(t *testing.T) {
	const prelude = `typeset -A w; w[k]=tgt; tgt=HELLO; `
	bashPreset := dialecttest.Preset{
		Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
		Diagnostics: bash.Diagnostics, Apply: bash.Apply,
	}
	for _, tc := range []struct{ written, want string }{
		{`${!w[@]}`, "[k]"},
		{`${!w[@]#H}`, "[ELLO]"},
		{`${!w[@]:1:2}`, "[EL]"},
		{`${!w[@]/L/x}`, "[HExLO]"},
		{`${!w[@]+SET}`, "[SET]"},
	} {
		t.Run(tc.written, func(t *testing.T) {
			out, st, err := bashPreset.Combined(t, dialecttest.Base{},
				prelude+`printf "[%s]" "`+tc.written+`"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("%s answered %q at %d, want %q at 0", tc.written, out, st, tc.want)
			}
		})
	}
}

// And the other shell that has `${!…}` refuses the same text.
//
// Measured the same day: every operator form above is `bad substitution` in
// ksh93u+ and ends the script at 1, while the bare form answers the key. The
// status and the silence after it are both asserted, because a refusal
// carried as a status alone would leave the text after it running.
func TestAnOperatorAfterTheSubscriptListingMayBeRefused(t *testing.T) {
	kshPreset := dialecttest.Preset{
		Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
		Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
	}
	const prelude = `typeset -A w; w[k]=tgt; tgt=HELLO; `
	bare, st, err := kshPreset.Combined(t, dialecttest.Base{}, prelude+`printf "[%s]" "${!w[@]}"`)
	if err != nil || bare != "[k]" || st != 0 {
		t.Fatalf("the bare listing answered %q at %d (err %v), want %q at 0", bare, st, err, "[k]")
	}
	out, st, _ := kshPreset.Combined(t, dialecttest.Base{},
		prelude+`printf "[%s]" "${!w[@]#H}"; echo AFTER`)
	if !strings.Contains(out, "bad substitution") {
		t.Errorf("answered %q, want a bad substitution", out)
	}
	if strings.Contains(out, "AFTER") {
		t.Errorf("answered %q — the refusal did not end the script", out)
	}
	if st == 0 {
		t.Errorf("status %d, want a failure", st)
	}
}

// One subscript under an indirection was never the listing either.
//
// Measured the same day, `a=(p q)`: bash answers `${!a[0]}` with nothing —
// `a[0]` is `p` and `p` is unset — and ksh93 answers `a[0]`, the
// name-with-subscript its `${!x}` yields, with the subscript expanded where
// one was written (`${!a[$i]}` with `i=1` is `a[1]`). Neither lists. Here both
// answered `0 1`, the subscripts of an array nobody asked to list.
//
// The whole-array spelling is asserted beside it in both columns, because
// that is the pair a wrong reading passes: a listing on any subscript answers
// `0 1` twice.
func TestOneSubscriptUnderAnIndirectionIsNotTheListing(t *testing.T) {
	for _, tc := range []struct {
		name          string
		preset        dialecttest.Preset
		single, whole string
	}{
		{
			name: "bash indirects through the element",
			preset: dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			single: "[][]",
			whole:  "[0][1]",
		},
		{
			name: "ksh yields the name with its subscript",
			preset: dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			single: "[a[0]][a[1]]",
			whole:  "[0][1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := tc.preset.Combined(t, dialecttest.Base{},
				`a=(p q); i=1; printf "[%s]" "${!a[0]}" "${!a[$i]}"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.single {
				t.Errorf("one subscript answered %q, want %q", out, tc.single)
			}
			out, _, err = tc.preset.Combined(t, dialecttest.Base{},
				`a=(p q); printf "[%s]" "${!a[@]}"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.whole {
				t.Errorf("the whole-array spelling answered %q, want %q", out, tc.whole)
			}
		})
	}
}
