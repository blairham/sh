// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// TestNoClobberOverrideMarker pins the side of #1247 that is easy to lose,
// and the reason this dialect had it the other way round until #5119.
//
// The flag was set true here on the strength of one comment: *"`set -C; echo
// x >| f` overwrites"*. **`>|` is TokClobber and is core**, so every dialect
// reads it and that row says the same thing whichever way the flag is set —
// a correct instrument pointed at an input that cannot exercise the thing it
// was pointed at. The rows below are the ones that can.
func TestNoClobberOverrideMarker(t *testing.T) {
	if ash.Dialect().ClobberOverrideMarker {
		t.Error("BusyBox accepts none of the marker's other spellings; `>!` here is `>` and a file named `!out`")
	}
}

// What the seven spellings actually do, **graded on which file was written**.
//
// Three of these rows are silent — the script reports 0 whichever way the
// flag is set, and the only difference is where the bytes landed — so status
// cannot grade them and neither can standard error. That is the whole reason
// the defect survived: a wrong answer with no diagnostic looks like success.
//
// Measured in `alpine:3.20` on 2026-09-29, BusyBox v1.36.1 multi-call binary,
// each row a script holding `( echo O; echo E >&2 ) OPout` in an empty
// directory:
//
//	>|      writes `out`, `E` to the terminal      (core, and agrees either way)
//	>!      writes a file named `!out`             silent
//	>>!     writes a file named `!out`             silent
//	&>!     writes a file named `!out`, `E` in it  silent
//	>>|     syntax error at the `|`, status 2
//	&>|     syntax error at the `|`, status 2
//	&>>|    syntax error, status 2
//	&>>!    syntax error, status 2
//
// The two fallbacks are what make three loud and three silent: a `|` marker
// falls back to a pipe with nothing on its left and is refused, and a `!`
// marker falls back to an ordinary word and is not.
func TestTheMarkerSpellingsAreFilenamesAndPipes(t *testing.T) {
	for _, tc := range []struct {
		name, op  string
		wantFiles []string
		wantFail  bool
	}{
		// The core control. It agrees either way, which is precisely why it
		// could not have been evidence for the flag.
		{"the core clobber operator still writes its file", ">|", []string{"out"}, false},

		// The three silent rows: `!` glues to the word and becomes the name.
		{"a marked truncate is a file named for the marker", ">!", []string{"!out"}, false},
		{"a marked append likewise", ">>!", []string{"!out"}, false},
		{"and a marked both-streams likewise", "&>!", []string{"!out"}, false},

		// The loud rows: `|` cannot start a word, so the text is refused.
		{"an appending pipe marker is a syntax error", ">>|", nil, true},
		{"a both-streams pipe marker likewise", "&>|", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "( echo O; echo E >&2 ) " + tc.op + "out\n"
			// A `|` marker is refused while the text is *read*, so that row
			// is a question for the parser and never reaches a runner. The
			// two halves are asserted separately rather than folded into
			// "it did not work", so a refusal moving from the reader to the
			// runtime would be visible rather than absorbed.
			if tc.wantFail {
				if parses(t, src) {
					t.Errorf("%q parsed, want the text refused while reading", src)
				}
				return
			}
			dir := t.TempDir()
			_, st, err := preset.Combined(t, dialecttest.Base{
				Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
			}, src)
			if err != nil {
				t.Fatalf("could not run: %v", err)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			// The file system is the assertion, not the status: on the three
			// silent rows the status is 0 whichever file was written.
			got := lsNonScript(t, dir)
			if strings.Join(got, ",") != strings.Join(tc.wantFiles, ",") {
				t.Errorf("files %v, want %v", got, tc.wantFiles)
			}
		})
	}
}

// The control for the other flag on that line, which stays on.
//
// `&>` is real evidence for AmpersandRedirect and is measured: BusyBox leaves
// `out` holding both lines and writes nothing to the terminal. It is here so
// that "turn the marker off" cannot be read as "turn both off".
//
// **`&>>` is still wrong and is deliberately not asserted here.** BusyBox
// refuses it and we take it, because AmpersandRedirect gates `&>` and `&>>`
// together and no value of that flag is right for this shell. Splitting it is
// a change to a core axis and wants the panel re-measured for the split; see
// the note on #5119.
func TestBothStreamsToOneFileIsStillTaken(t *testing.T) {
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
	}, "( echo O; echo E >&2 ) &>out\n")
	if err != nil {
		t.Fatalf("could not run: %v", err)
	}
	if st != 0 || out != "" {
		t.Errorf("out %q status %d, want nothing on either stream at 0", out, st)
	}
	body, rerr := os.ReadFile(filepath.Join(dir, "out"))
	if rerr != nil || string(body) != "O\nE\n" {
		t.Errorf("out file %q (%v), want both lines in it", body, rerr)
	}
}

// lsNonScript is what makes the three silent rows gradable: the name of the
// file that was written, which is the only thing that moves on them.
func lsNonScript(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}
