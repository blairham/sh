// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package repl

// resizeWaking is nothing where there is no window-size signal: the line is
// drawn at the new width on the next key.
func resizeWaking() (*promptSignal, func()) { return nil, func() {} }
