// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestTheParseCacheIsTheFrontEndsToChoose is when this front end keeps a
// parse cache: where the environment names a cache directory that is
// already there, unless SH_NO_PARSE_CACHE says not to or a policy governs
// where the shell may write.
func TestTheParseCacheIsTheFrontEndsToChoose(t *testing.T) {
	xdg := t.TempDir()
	for _, c := range []struct {
		name string
		env  []string
		gate bool
		want bool
	}{
		{"a cache directory", []string{"XDG_CACHE_HOME=" + xdg}, false, true},
		{"switched off", []string{"XDG_CACHE_HOME=" + xdg, "SH_NO_PARSE_CACHE=1"}, false, false},
		{"switched off with anything", []string{"XDG_CACHE_HOME=" + xdg, "SH_NO_PARSE_CACHE=yes"}, false, false},
		{"0 is on", []string{"XDG_CACHE_HOME=" + xdg, "SH_NO_PARSE_CACHE=0"}, false, true},
		{"under a policy", []string{"XDG_CACHE_HOME=" + xdg}, true, false},
		{"nowhere to put it", []string{"XDG_CACHE_HOME=" + filepath.Join(xdg, "absent")}, false, false},
	} {
		sh := Shell{Env: c.env}
		if c.gate {
			sh.Gate = interp.GateFunc(func(context.Context, interp.Action) interp.Decision {
				return interp.Allow
			})
		}
		if got := sh.parseCache() != nil; got != c.want {
			t.Errorf("%s: a cache is %v, want %v", c.name, got, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(xdg, "absent")); err == nil {
		t.Fatal("the cache's parent was made")
	}
}
