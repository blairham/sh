// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import "strconv"

// The kernel resolves /proc/self/fd/N itself, for this process and for a
// child chdir-ing before its exec, so nothing here has to recognize it.
// The caller verifies the answer against the descriptor before using it: a
// system with no /proc gets "" from that check, not a dead path.
func heldName(fd int) string { return "/proc/self/fd/" + strconv.Itoa(fd) }

func forgetHeld(int) {}

func splitHeld(string) (int, string, bool) { return 0, "", false }

func fromHeldThread(int, func()) error { return nil }

func heldChdir(int) error { return nil }
