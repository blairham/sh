// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package zsh

import "github.com/blairham/sh/interp"

// A platform with no Unix-domain sockets to open. `zsocket` is not registered
// at all, so `zmodload -F zsh/net/socket b:zsocket` refuses by the builtin's
// name rather than loading a module with nothing behind it.
func registerSocketModule(*interp.Runner) {}
