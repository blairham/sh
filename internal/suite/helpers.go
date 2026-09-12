// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoCompiler is returned when the suite's helpers cannot be built.
//
// A skip rather than a failure, and a loud one. A suite whose helpers are
// missing does not score badly — it scores *zero*, because the calls fail
// under both shells and the two failures match, so a run without them reads
// as agreement on an error message. That is the failure mode this whole class
// of instrument has, and it is worse than no number at all.
var ErrNoCompiler = errors.New("no C compiler, so the suite's helpers cannot be built")

// BuildHelpers compiles the suite's C programs into its test directory and
// returns their names.
//
// They are compiled, never read. That is the same line the rest of this
// instrument stands on: handing a file to a compiler is running a tool over
// bytes, and it produces a program rather than knowledge.
func BuildHelpers(ctx context.Context, s Suite, dir string) ([]string, error) {
	if len(s.Helpers) == 0 {
		return nil, nil
	}
	cc := compiler()
	if cc == "" {
		return nil, ErrNoCompiler
	}
	tests := filepath.Join(dir, filepath.FromSlash(s.TestDir))
	var built []string
	for _, src := range s.Helpers {
		name := strings.TrimSuffix(filepath.Base(src), ".c")
		out := filepath.Join(tests, name)
		cmd := exec.CommandContext(ctx, cc, "-o", out, filepath.Join(dir, filepath.FromSlash(src)))
		if combined, err := cmd.CombinedOutput(); err != nil {
			// The compiler's own diagnostic is quoted here and nowhere else
			// in this instrument, because it is about a build of ours going
			// wrong rather than about the suite's content — and without it a
			// reader has "helpers failed" and no way to act on it.
			return nil, fmt.Errorf("%s %s: %v: %s", cc, name, err, strings.TrimSpace(string(combined)))
		}
		if err := os.Chmod(out, 0o700); err != nil {
			return nil, err
		}
		built = append(built, name)
	}
	return built, nil
}

// compiler is the C compiler to use, honoring CC the way a build system does
// and falling back to the two names a Unix has.
func compiler() string {
	names := []string{os.Getenv("CC"), "cc", "gcc", "clang"}
	for _, name := range names {
		if name == "" {
			continue
		}
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
