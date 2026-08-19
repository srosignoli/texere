// css-var.js — read a CSS custom property off the document root at the
// moment it is called, for the code paths outside the graph explorer that
// need a themed colour (the sketch pen, and the background it gets baked
// onto). Mirrors the resolution strategy documented at the top of
// components/explorer/Theme.js: no eagerly-evaluated colour constant, so a
// theme switch — or simply a component mounting after one — sees the value
// actually painted, rather than whichever was live when a module first
// loaded. Falls back to `fallback` in a test environment with no real
// `document`, or if the property is unset.
export function cssVar(name, fallback) {
  try {
    if (typeof document === 'undefined') return fallback
    const v = getComputedStyle(document.documentElement).getPropertyValue(name)
    return v ? v.trim() || fallback : fallback
  } catch {
    return fallback
  }
}

// Fallbacks for when getComputedStyle returns nothing (see cssVar above) —
// same reasoning as Theme.js's DEFAULTS map. These two literals are the
// reason this file carries an entry in theme-tokens.test.js's ALLOW: they
// are not styling anything themselves, they are what SketchCell.svelte's pen
// and lib/bake.js's baked background fall back to if the token can't be read.
const SKETCH_INK_FALLBACK = '#e8eaf0'
const SKETCH_BG_FALLBACK = '#0b1220'

// The colour a new stroke is drawn in when no swatch override is picked —
// near-white in Blue/Dark, near-black in Light, so the default pen stays
// visible on the canvas in every theme. Read at call time (see cssVar above)
// so a freshly mounted sketch cell, or a stroke rendered after a theme
// switch, picks up the CURRENT theme's ink.
export function sketchInkColor() {
  return cssVar('--sketch-ink', SKETCH_INK_FALLBACK)
}

// The background baked behind a sketch's strokes when it is flattened to a
// PNG for an LLM turn (lib/bake.js). Resolved from the SAME theme the
// strokes were drawn against — see bake.js for why baking onto the wrong
// background can permanently destroy a saved sketch.
export function sketchBgColor() {
  return cssVar('--sketch-bg', SKETCH_BG_FALLBACK)
}
