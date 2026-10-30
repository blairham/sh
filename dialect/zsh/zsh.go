// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

// prefixKeepingBuiltins is the roster of interp.Semantics.BuiltinsKeepingAnAssignmentPrefix,
// named so that `unsetopt posixbuiltins` can put back what `setopt` took away.
const prefixKeepingBuiltins = "alias hash builtin exec"

// posixSpecialDeclarations is the roster of interp.Semantics.SpecialBuiltinsBeyondPosix
// under `setopt posixbuiltins`. Without the option it is empty. See the option's
// entry in setopt.go.
const posixSpecialDeclarations = "typeset declare integer float local"
