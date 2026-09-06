// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The fixtures are POSIX shell scripts in testdata, and that is deliberate
// rather than convenient. The claim this whole design rests on is that a
// plugin can be written in any language on the machine, with no library and
// no code generator; a fixture written in Go would test the host against a
// peer built from the same message types, which is the one peer that cannot
// check the claim. Each of these reads a line and writes a line.

// fixture is the absolute path of one, which is what Launch requires.
func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("resolving the fixture: %v", err)
	}
	return path
}

// launch starts a fixture and closes it when the test ends.
func launch(t *testing.T, name string, o plugin.Options) *plugin.Host {
	t.Helper()
	o.Path = fixture(t, name)
	if o.Stderr == nil {
		o.Stderr = &syncBuffer{}
	}
	h, err := plugin.Launch(t.Context(), o)
	if err != nil {
		t.Fatalf("Launch(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// shell is a runner with the plugin's commands registered on it, plus the two
// streams a test wants to read back.
type shell struct {
	r    *interp.Runner
	out  *strings.Builder
	errs *strings.Builder
}

// newShell builds one. POSIX semantics, because a plugin's command must not
// depend on which dialect is above it — internal/plugin knows nothing about
// dialects and dialectblind_test.go holds that at any depth.
func newShell(t *testing.T, h *plugin.Host) *shell {
	t.Helper()
	sem := interp.PosixSemantics()
	sh := &shell{out: &strings.Builder{}, errs: &strings.Builder{}}
	sh.r = &interp.Runner{
		Semantics: &sem,
		Stdout:    sh.out,
		Stderr:    sh.errs,
		Dir:       t.TempDir(),
		Name:      "testsh",
		// PATH and nothing else. The tests below pipe a plugin's output
		// through a real command, so the shell has to be able to find one —
		// and a Runner handed the whole environment would be a test reading
		// the machine it happens to be on.
		Env: []string{"PATH=" + os.Getenv("PATH")},
	}
	if h != nil {
		h.Register(sh.r)
	}
	return sh
}

// run parses and runs a script, and answers the status the shell would exit
// with. The whole point of going through the interpreter rather than calling
// the stub directly is that a plugin's command has to be a *builtin*: found
// by name resolution, shadowed by a function, working under a pipeline and a
// redirection.
func (sh *shell) run(t *testing.T, src string) int {
	t.Helper()
	parser := syntax.NewParser(src, syntax.Dialect{})
	f := parser.Parse()
	if err := parser.Err(); err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	code, err := sh.r.Run(t.Context(), f)
	if err != nil {
		t.Fatalf("running %q: %v", src, err)
	}
	return code
}

// syncBuffer is a bytes.Buffer a relay goroutine and a test may both touch.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runCtx is run with a context of the test's own, for the cancellation paths.
func (sh *shell) runCtx(t *testing.T, ctx context.Context, src string) int {
	t.Helper()
	parser := syntax.NewParser(src, syntax.Dialect{})
	f := parser.Parse()
	if err := parser.Err(); err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	code, _ := sh.r.Run(ctx, f)
	return code
}
