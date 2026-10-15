// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package opened

// No path leads to a descriptor's directory here, so callers keep the names
// they had.
func heldName(int) string { return "" }

func forgetHeld(int) {}

func splitHeld(string) (int, string, bool) { return 0, "", false }

func fromHeldThread(int, func()) error { return nil }

func heldChdir(int) error { return nil }
