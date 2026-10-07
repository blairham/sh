// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompinitReadsAPlainLineAsCompdefWould pins the shortcut compinit takes
// for a `#compdef` line that is only command names (#5873): the tables it
// leaves are the ones calling `compdef -n` for every file in the same order
// leaves. The fixture holds plain lines, a name two files claim, names with
// punctuation and quotes, services, both pattern forms (one behind a plain
// name, which is the shape a test of the line's first word would miss), `-N`,
// a bare line and one separated by tabs, so both readings are exercised and
// the two have to agree on every key.
func TestCompinitReadsAPlainLineAsCompdefWould(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, line := range map[string]string{
		"_a1":     "#compdef alpha beta gamma",
		"_a2":     "#compdef alpha delta",
		"_dash":   "#compdef e-f g_h i.j",
		"_quote":  `#compdef 'q1' "q2" [x] a]b`,
		"_svc":    "#compdef svc=service other",
		"_svc2":   "#compdef svc=later other",
		"_pat":    "#compdef -p 'pat*' after",
		"_post":   "#compdef -P 'post*'",
		"_n":      "#compdef -N n1 -p 'p2*' n2",
		"_mid":    "#compdef midname -p 'mid*' -P 'midpost*' last",
		"_bare":   "#compdef",
		"_tab":    "#compdef\tt1\tt2",
		"_late":   "#compdef gamma zeta",
		"_auto":   "#autoload",
		"_nomark": "nothing",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(line+"\nprint in "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Sorted by the shell itself: the test's PATH has no sort in it.
	dump := `d=()
for k v in ${(kv)_comps}; do d+=("c $k=$v"); done
for k v in ${(kv)_services}; do d+=("s $k=$v"); done
for k v in ${(kv)_patcomps}; do d+=("p $k=$v"); done
for k v in ${(kv)_postpatcomps}; do d+=("P $k=$v"); done
print -r -- ${(j:;:)${(o)d}}`
	out, st := runShipped(t, `fpath=(`+dir+` $fpath); autoload -Uz compinit; compinit -D -u
local -a d
local file line k v
`+dump+`
_comps=() _services=() _patcomps=() _postpatcomps=()
for file in `+dir+`/_*(N-.); do
  IFS= read -r line < $file
  [[ $line == '#compdef '* || $line == '#compdef' ]] && compdef -n ${file:t} ${=${line#\#compdef}}
done
`+dump)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two dumps, got status %d:\n%s", st, out)
	}
	if lines[0] != lines[1] {
		t.Errorf("compinit and compdef -n disagree\ncompinit: %s\ncompdef:  %s", lines[0], lines[1])
	}
	for _, want := range []string{"c alpha=_a1", "c gamma=_a1", "c zeta=_late", "c e-f=_dash", "c [x]=_quote", "c 'q1'=_quote", "p 'pat*'=_pat", "P 'post*'=_post", "s svc=service", "p 'mid*'=_mid"} {
		if !strings.Contains(";"+lines[0]+";", ";"+want+";") {
			t.Errorf("%q is not in the tables, so the rows are not reaching what they test:\n%s", want, lines[0])
		}
	}
}
