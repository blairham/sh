// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package testenv

// closeInheritedDescriptors has nothing to do off unix, where a child is not
// handed its parent's descriptors by number. See descriptors_unix.go.
func closeInheritedDescriptors() {}
