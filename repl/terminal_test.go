// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// A character device is not a terminal, and the difference is the whole of a
// reported bug: `sh < /dev/null` — the cron, systemd, CI and harness
// invocation — was read as a person at a keyboard, because the null device is
// a character device and that was the test.
//
// The question is an ioctl the kernel answers. Everything here except the last
// row would have passed under the old test as well; the last row is the reason
// the test cannot simply be `return false`.
func TestOnlyATerminalIsATerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		open func(t *testing.T) *os.File
		want bool
	}{
		{name: "nothing at all", open: func(*testing.T) *os.File { return nil }},
		{
			name: "the null device",
			open: func(t *testing.T) *os.File { return device(t, os.DevNull) },
		},
		{
			name: "another character device",
			open: func(t *testing.T) *os.File { return device(t, "/dev/zero") },
		},
		{
			name: "a regular file",
			open: func(t *testing.T) *os.File { return readerFile(t, "echo hello\n") },
		},
		{
			name: "a pipe",
			open: func(t *testing.T) *os.File {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
				return r
			},
		},
		{name: "a terminal", open: terminalFile, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.open(t)
			if got := IsTerminal(f); got != tc.want {
				t.Errorf("IsTerminal(%s) = %v, want %v", describe(f), got, tc.want)
			}
		})
	}
}

// Asking leaves the file as it was.
//
// The descriptor is borrowed through SyscallConn rather than taken with Fd,
// which would detach the file from the runtime's poller and leave it blocking
// for good — a real change to make to a pipe merely to ask it a question it
// answers no to. The observable half of that is that the pipe still reads.
func TestAskingAPipeDoesNotDisturbIt(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if IsTerminal(r) {
		t.Fatal("a pipe reported as a terminal")
	}
	go func() {
		_, _ = w.WriteString("still here\n")
		_ = w.Close()
	}()
	b := make([]byte, len("still here\n"))
	if _, err := r.Read(b); err != nil {
		t.Fatalf("reading the pipe after asking: %v", err)
	}
	if string(b) != "still here\n" {
		t.Errorf("read %q, want the pipe unchanged", b)
	}
}

func device(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func describe(f *os.File) string {
	if f == nil {
		return "nil"
	}
	return filepath.Base(f.Name())
}
