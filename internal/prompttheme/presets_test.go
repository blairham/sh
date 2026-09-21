// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestEveryCarriedPresetLoadsAndNamesElements(t *testing.T) {
	t.Parallel()
	// A preset that named no elements would be a theme that does not draw,
	// which is the one thing a preset cannot be.
	for _, name := range prompttheme.Presets() {
		layer, trouble := prompttheme.LoadPreset(name)
		if trouble != "" || layer == nil {
			t.Errorf("%s: %s", name, trouble)
			continue
		}
		if v, ok := layer.Lookup("LEFT_ELEMENTS"); !ok || v.Text() == "" {
			t.Errorf("%s names no left elements", name)
		}
	}
}

func TestTheTwoCarriedLooksDifferOnlyInTheirSettings(t *testing.T) {
	t.Parallel()
	// The structural claim the whole engine rests on: one layout loop, and a
	// new look costs data and no code. If a third look ever needs code, the
	// loop has stopped being generic and that is the bug rather than the
	// look.
	lean, _ := prompttheme.LoadPreset("lean")
	frame, _ := prompttheme.LoadPreset("frame")

	sameElements := func(a, b prompttheme.Layer, key string) bool {
		x, _ := a.Lookup(key)
		y, _ := b.Lookup(key)
		return x.Text() == y.Text()
	}
	for _, key := range []string{"LEFT_ELEMENTS", "RIGHT_ELEMENTS"} {
		if !sameElements(lean, frame, key) {
			t.Errorf("the two looks draw different %s, so the comparison is not about the loop", key)
		}
	}
	// And they do differ: a background per segment is what makes one framed.
	if _, ok := frame.Lookup("DIR_BACKGROUND"); !ok {
		t.Error("the framed look has no backgrounds in it")
	}
	if _, ok := lean.Lookup("DIR_BACKGROUND"); ok {
		t.Error("the lean look has a background in it")
	}
}

func TestAPresetMayBeAFileSomewhere(t *testing.T) {
	t.Parallel()
	// A look somebody publishes is a file you point a variable at, and it is
	// read by the same reader the carried ones are seeded through.
	path := filepath.Join(t.TempDir(), "mine.prompt")
	if err := os.WriteFile(path, []byte("LEFT_ELEMENTS = dir\nDIR_FOREGROUND = 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	layer, trouble := prompttheme.LoadPreset(path)
	if trouble != "" || layer == nil {
		t.Fatalf("a preset in a file: %s", trouble)
	}
	if v, _ := layer.Lookup("DIR_FOREGROUND"); v.Text() != "9" {
		t.Errorf("the file's own setting read as %q", v.Text())
	}
}

func TestAPresetThatIsNeitherCarriedNorAFileIsNamed(t *testing.T) {
	t.Parallel()
	// A person who misspelled a preset asked for a look and got the plain
	// prompt. The difference between "there is no such preset" and "presets
	// do not work" is the whole of what a report is for.
	layer, trouble := prompttheme.LoadPreset("not-a-look")
	if layer != nil {
		t.Error("a preset nobody has drew anyway")
	}
	if !strings.Contains(trouble, "not-a-look") {
		t.Errorf("the trouble was %q, and does not name what was asked for", trouble)
	}
	for _, carried := range prompttheme.Presets() {
		if !strings.Contains(trouble, carried) {
			t.Errorf("the trouble does not say that %s is carried: %q", carried, trouble)
		}
	}
}

func TestNoPresetIsNoLayerAndNoTrouble(t *testing.T) {
	t.Parallel()
	layer, trouble := prompttheme.LoadPreset("")
	if layer != nil || trouble != "" {
		t.Errorf("an unset preset answered %v / %q", layer != nil, trouble)
	}
}
