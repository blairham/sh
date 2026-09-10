// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package zsh

import "errors"

// A platform whose stat fields this package has not been taught, which is the
// same shape errnotable_other.go has: the name is not registered at all, so
// `zmodload -F zsh/stat b:zstat` refuses by the builtin's name rather than
// loading a module with nothing behind it.

const statSupported = false

func statPath(string, bool) (statFields, error) { return statFields{}, errors.ErrUnsupported }

func statFd(int) (statFields, error) { return statFields{}, errors.ErrUnsupported }
