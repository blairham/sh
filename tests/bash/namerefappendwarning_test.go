// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An **append** through a reference aimed at its own name is one event and
// says one sentence: the write's.
//
// A write through a self reference says `maximum nameref depth (8) exceeded`
// and a read says `circular name reference` — two sentences for two events.
// An append does both halves of one write: it reads the old text to join to,
// and this shell let that read say the read's sentence, so `ref+=X` wrote a
// `circular name reference` line bash does not write. On a file graded by
// counting lines that is a differing line of its own.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20 and bash 5.3.15, which agree, with
// an outer `ref=B`:
//
//	f() { typeset -n ref=ref; ref+=X; }
//	  bash     the declaration's two warnings, then the depth sentence alone
//	  before   a `circular name reference` in front of the depth sentence
//
//	f() { typeset -n ref=ref; declare -g ref+=X; }
//	  bash     the declaration's two warnings and nothing at all for the
//	           append
//	  before   a `circular name reference` for the join's read
//
// The second is the sharper of the two: with no depth sentence to hide behind,
// the extra line stood on its own. The value was right all along on both —
// `[BX]` inside and out — so what was wrong was the diagnostic alone, which is
// what `nameref15.sub:33` grades (#4178).
func TestAnAppendThroughASelfAimedReferenceSaysOnlyTheWritesSentence(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row. The depth sentence stands and the read's does not.
			"a plain append says the depth sentence alone",
			"ref=B\nf() { typeset -n ref=ref; ref+=X; }\nf",
			"warning: ref: maximum nameref depth (8) exceeded", "",
		},
		{
			// Counted, because the row is about how many lines there are:
			// the declaration writes two and the append writes one.
			"and writes three warning lines in all, not four",
			"ref=B\nf() { typeset -n ref=ref; ref+=X; }\nf 2>&1 | grep -c 'warning:'",
			"3", "",
		},
		{
			// The sharper row: no depth sentence to hide behind.
			"a global append says nothing at all",
			"ref=B\nf() { typeset -n ref=ref; declare -g ref+=X; }\nf 2>&1 | grep -c 'warning:'",
			"2", "",
		},
		{
			// The value was never wrong and must stay right: the append joins
			// what the **global** cell holds, which is what a self-aimed
			// reference is a handle on.
			"the join is the global cell's text",
			"ref=B\nf() { typeset -n ref=ref; ref+=X; printf '<%s>' \"$ref\"; }\nf\nprintf '[%s]' \"$ref\"",
			"<BX>[BX]", "",
		},
		{
			// A **read** still says the read's sentence, which is the line
			// this change must not have taken away.
			"a read still says the read's sentence",
			"ref=B\nf() { typeset -n ref=ref; printf '<%s>' \"$ref\"; }\nf",
			"warning: ref: circular name reference", "",
		},
		{
			// And an ordinary aimed reference joins its target's text with no
			// warning at all — the path this leaves alone.
			"an ordinary aimed reference is untouched",
			"v=V\nf() { typeset -n r=v; r+=X; printf '<%s><%s>' \"$r\" \"$v\"; }\nf",
			"<VX><VX>", "warning:",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
