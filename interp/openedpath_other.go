// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package interp

// pathOfFd has no answer on a platform with no way to ask one, and says so
// rather than guessing. The consequence is written down instead of inferred:
// there the gate matches the name a script wrote and nothing verifies what
// that name reached, which is the state every platform was in before this
// file's neighbors existed. docs/design/sandboxing.md carries the list of
// platforms the check is real on.
func pathOfFd(uintptr) (string, bool) { return "", false }
