// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// An **external** command's assignment prefix is a store, made in the child
// that is about to run the command — so a hook watching the name fires, what
// it leaves is what the child is handed, and nothing it did reaches this
// shell (#3159).

// The hook fires, and the value it leaves is the child's.
func TestAPrefixToAnExternalStoresInTheChild(t *testing.T) {
	out, status := runDisciplined(t,
		"function s.set { echo \"SET[${.sh.value}]\" >&2; }\n"+
			"s=5 /usr/bin/env | grep '^s=' || echo '(none)'")
	if !strings.Contains(out, "SET[5]") {
		t.Errorf("out %q does not run the hook for an external command's prefix", out)
	}
	if !strings.Contains(out, "s=5") {
		t.Errorf("out %q does not hand the child the value", out)
	}
	if status != 0 {
		t.Errorf("status %d, want 0", status)
	}
	// A hook that **replaces** the value replaces what the child is given,
	// which is what says the child is handed the store rather than the
	// expansion beside it.
	out, _ = runDisciplined(t,
		"function s.set { .sh.value=REPLACED; }\n"+
			"s=5 /usr/bin/env | grep '^s='")
	if !strings.Contains(out, "s=REPLACED") {
		t.Errorf("out %q hands the child a value the store did not make", out)
	}
}

// And nothing the hook did reaches this shell.
func TestAPrefixToAnExternalLeavesThisShellAlone(t *testing.T) {
	out, _ := runDisciplined(t, `s=1
function s.set { t=HOOKRAN; }
s=5 /usr/bin/env >/dev/null
echo "t=[$t] s=[$s]"`)
	if !strings.Contains(out, "t=[] s=[1]") {
		t.Errorf("out %q lets the child's store reach this shell", out)
	}
	// The hook's status is the child's too: the command reports its own.
	out, _ = runDisciplined(t,
		"function s.set { return 7; }\ns=5 /usr/bin/env >/dev/null\necho \"st=$?\"")
	if !strings.Contains(out, "st=0") {
		t.Errorf("out %q reports the hook's status for the command", out)
	}
}

// One child serves the whole prefix list, and what the hooks leave in it is
// what the child's **environment** is built from — so a hook that writes some
// other exported name is answered by the child.
func TestOneChildServesTheWholePrefixList(t *testing.T) {
	out, _ := runDisciplined(t, `function a.set { t=HOOK; .sh.value="A[$t]"; }
function b.set { .sh.value="B[$t]"; }
a=1 b=2 /usr/bin/env | grep -E '^(a|b)=' | sort`)
	if !strings.Contains(out, "a=A[HOOK]") || !strings.Contains(out, "b=B[HOOK]") {
		t.Errorf("out %q, want the second hook to see what the first wrote", out)
	}
	out, _ = runDisciplined(t, `export E=parent
function s.set { E=child; }
s=5 /usr/bin/env | grep '^E='`)
	if !strings.Contains(out, "E=child") {
		t.Errorf("out %q builds the child's environment before the hooks ran", out)
	}
}

// An append enters its own event with the part being appended, and the child
// is handed the join — which is the same rule the spellings that store in
// this shell already keep.
func TestAnAppendPrefixToAnExternalEntersTheAppendEvent(t *testing.T) {
	out, _ := runDisciplined(t, `s=base
function s.append { echo "APP[${.sh.value}]" >&2; }
s+=5 /usr/bin/env | grep '^s='
echo "parent=[$s]"`)
	if !strings.Contains(out, "APP[5]") {
		t.Errorf("out %q does not enter the append event with the part alone", out)
	}
	if !strings.Contains(out, "s=base5") || !strings.Contains(out, "parent=[base]") {
		t.Errorf("out %q, want the join in the child and nothing in this shell", out)
	}
}

// A command that does not exist still made the store, because the fork
// happens before the search does.
func TestAPrefixToACommandThatIsNotThereStillStores(t *testing.T) {
	out, _ := runDisciplined(t,
		"function s.set { echo SET; }\ns=5 /nonexistent/nosuchfile-3159")
	if !strings.Contains(out, "SET") {
		t.Errorf("out %q makes no store for a command that was never found", out)
	}
}
