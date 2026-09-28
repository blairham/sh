// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"testing"
)

// TestEveryGatedBuiltinIsAFeatureOfItsModule keeps the roster honest against
// the table it is cut from.
//
// The load path that brings a withdrawn builtin back is the **feature
// selection** — a plain `zmodload` widens to every feature the module names,
// and that is what puts the name back in the lookup. So a name gated here
// that the module does not name as a `b:` feature is a builtin nothing could
// ever restore: the shell would have lost it, silently, with every test that
// calls it after a `zmodload` still failing for a reason that looks like the
// builtin being broken.
func TestEveryGatedBuiltinIsAFeatureOfItsModule(t *testing.T) {
	for module, names := range zshGatedBuiltins {
		features, ok := zmodloadFeatures[module]
		if !ok {
			t.Errorf("%s is gated and is not a module this shell has", module)
			continue
		}
		for _, name := range names {
			if !slices.Contains(features, "b:"+name) {
				t.Errorf("%s gates %s, which is not a b: feature of it — "+
					"nothing would bring it back", module, name)
			}
		}
	}
}
