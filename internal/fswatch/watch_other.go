// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package fswatch

// Watcher is the shape the supported systems have, so that a caller compiles
// everywhere and degrades in one place rather than in every use of it.
type Watcher struct{}

// Open says there is no watch here rather than answering a watcher that never
// fires. A caller told "unsupported" can say so and fall back to the weaker
// guarantee; a caller handed a silent watcher would report a branch it stopped
// checking, which is the failure this repository treats as its worst.
func Open(func()) (*Watcher, error) { return nil, ErrUnsupported }

// Add has nothing to add to.
func (w *Watcher) Add(string) error { return ErrUnsupported }

// Close has nothing to close.
func (w *Watcher) Close() error { return nil }
