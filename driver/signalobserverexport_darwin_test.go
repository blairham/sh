// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

// ObserverDescriptorForTest is the signal observer's kqueue while a watch is
// up, and 0 otherwise.
func ObserverDescriptorForTest() int { return int(observerDescriptor.Load()) }

// HeldByTheRuntimeForTest is heldByTheRuntime: whether a script's placement
// onto fd is refused rather than allowed to overwrite it.
func HeldByTheRuntimeForTest(fd int) bool { return heldByTheRuntime(fd) }
