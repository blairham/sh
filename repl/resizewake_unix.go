// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"os/signal"
	"syscall"
)

// resizeWaking wires the terminal's size changes to a wake the editor waits
// on beside the terminal, and answers how to take the wiring down. A nil
// signal is a session that cannot have one, which redraws on the next key as
// every session did before #5908.
func resizeWaking() (*promptSignal, func()) {
	wake := newPromptSignal()
	if wake == nil {
		return nil, func() {}
	}
	sizes := make(chan os.Signal, 1)
	signal.Notify(sizes, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sizes {
			wake.publish()
		}
	}()
	return wake, func() {
		signal.Stop(sizes)
		close(sizes)
		<-done
		wake.close()
	}
}
