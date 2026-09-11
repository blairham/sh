// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build shaxissweep

package driver

import (
	"github.com/blairham/sh/internal/axismutate"
	"github.com/blairham/sh/interp"
)

// axisMutate moves the one axis the sweep is asking about.
//
// Built only under `-tags shaxissweep`, which nothing ships. See
// driver/axissweep_off.go for why the front end needs a hook of its own.
func axisMutate(s interp.Semantics) interp.Semantics {
	axismutate.ApplyEnv(&s)
	return s
}
