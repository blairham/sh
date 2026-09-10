// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package zsh

// errnoNames is empty on a platform whose error numbering has not been
// measured, so `$errnos` is an empty array there.
//
// Empty rather than another platform's table, which would be numbers that
// belong to a different kernel presented as this one's — the failure mode this
// parameter exists to avoid, since the whole use of it is turning a number a
// system call gave back into a name.
//
// The two this project ships for are darwin and linux; the file exists so that
// the module still builds everywhere the rest of the tree does.
var errnoNames []string
