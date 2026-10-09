// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// moduleDirs is what each reference said its module directory is, asked once
// per binary rather than once per file.
var moduleDirs sync.Map

// placeModules links the reference's module directory where the suite's
// driver looks for modules. See [Suite.ModulesAt].
//
// The directory comes from asking the reference rather than from a list of
// where a package manager might have put it, because the reference is the only
// thing that knows: it is `/opt/homebrew/Cellar/zsh/5.9.2/lib` on this Mac and
// `/usr/lib/aarch64-linux-gnu/zsh/5.9.2` in the column's image. A reference
// that names no directory, or one that is not there, leaves the layout as it
// was rather than failing the run. In that case the file reports what the
// reference does without its modules, which is what it reported before.
func placeModules(ctx context.Context, s Suite, run, reference string) error {
	if s.ModulesAt == "" || s.ModulesQuery == "" || reference == "" {
		return nil
	}
	dir, err := moduleDir(ctx, reference, s.ModulesQuery)
	if err != nil || dir == "" {
		return nil
	}
	at := filepath.Join(run, filepath.FromSlash(s.ModulesAt))
	if _, err := os.Lstat(at); err == nil {
		return fmt.Errorf("%s: %s is already in the test directory", s.Name, s.ModulesAt)
	}
	return os.Symlink(dir, at)
}

func moduleDir(ctx context.Context, reference, query string) (string, error) {
	if v, ok := moduleDirs.Load(reference); ok {
		if dir, isString := v.(string); isString {
			return dir, nil
		}
	}
	out, err := exec.CommandContext(ctx, reference, "-fc", query).Output()
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(string(out))
	if fi, serr := os.Stat(dir); serr != nil || !fi.IsDir() {
		dir = ""
	}
	moduleDirs.Store(reference, dir)
	return dir, nil
}
