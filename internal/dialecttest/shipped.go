// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialecttest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ShippedFunctionDir is share/sh/functions, the function files the zsh binary
// installs, found from this source file rather than from the working
// directory so that a test in any package reaches the same directory.
func ShippedFunctionDir(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information: cannot find the shipped function files")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "share", "sh", "functions")
	for _, name := range []string{"add-zle-hook-widget", "add-zsh-hook", "bashcompinit", "colors", "compaudit", "compdump", "compinit", "is-at-least", "regexp-replace"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("shipped function %s: %v", name, err)
		}
	}
	return dir
}
