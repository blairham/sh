// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package prompttheme is the prompt theme engine's configuration
// vocabulary: the keyed store a theme is written in, the lookup that
// resolves a setting for a segment, the colors a setting may name, and the
// markup a value may carry.
//
// docs/spec/prompt-theme.md is the design, and it is a design spec rather
// than a measurement: no external specification says what a themed prompt
// must draw, so most of what is implemented here was decided rather than
// observed. Where this package decides something the spec left open, the
// decision is written down next to the code that makes it.
//
// Nothing here draws. The store, the chain and the markup reader are the
// vocabulary everything else in the engine is expressed in, and they are
// separated from rendering for the reason the spec gives for the layout
// pass: a new look must cost data and no code, which is only true while
// the configuration surface is generic.
//
// The package imports no dialect, names no shell, and reads no process
// state. Settings arrive through a lookup function, so wiring them to a
// Runner's variables is the consumer's job and no dialect can become a
// prerequisite for having a prompt.
package prompttheme
