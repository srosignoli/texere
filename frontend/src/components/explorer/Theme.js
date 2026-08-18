// Color theme for the 2.5D Isometric Graph Explorer (feature 006).
//
// Single source of truth: CSS variables declared in `frontend/src/style.css`
// (`:root { --explorer-* }`, plus the per-theme `[data-theme=...]` blocks).
//
// EVERY resolver in this module reads the document at CALL time. There are
// deliberately no eagerly-evaluated `COLOR_*` constants: this module used to
// export both, and the frozen constants were what the scene actually
// imported, so the palette was pinned to whichever theme happened to be
// active when the module was first imported — a theme switch (or even
// reopening the explorer afterwards) could not change it. One path only.
//
// Each lookup falls back to a hardcoded default so the explorer renders
// correctly even if a variable is missing (e.g. a test environment with no
// real document).
//
// Three.js consumes 24-bit hex integers, not CSS strings, so every colour
// resolver parses "#rrggbb" / "#rgb" into a number. The two label resolvers
// are the exception: they feed canvas2d fillStyle/strokeStyle and so return
// CSS strings (which may carry alpha).

const DEFAULTS = {
  edgeContains: 0xf59e0b,
  edgeLeadsTo: 0x06b6d4,
  edgeProperties: 0x10b981,
  edgeSimilarTo: 0xa855f7,
  edgeUnclassified: 0x6b7280,
  edgeUnknown: 0xe05fa0, // magenta-pink: stands apart from the four Compass hues

  nodeDefault: 0x6cb4ee,
  nodeAnchor: 0xffffff,
  label: '#e8eaf0',
  labelHalo: '#0e0f12',
  labelSub: 'rgba(200, 214, 230, 0.7)',
  planeGrid: 0x6b7c95,
  planeTint: 0x1f2a3a,
  cycleGlyph: '#f4b545',
  cycleHalo: '#1a1306',

  territory: [
    0xe07a3a,
    0x3aa8e0,
    0xc8e03a,
    0xb83ae0,
    0x3ae0a0,
    0xe03a8a,
    0xe0c83a,
    0x3ae0e0,
  ],
}

export const TERRITORY_OPACITY = 0.42

// Default opacity for the layered grid planes. The user can override
// via the Plane Opacity slider; this is the initial value.
export const PLANE_OPACITY_DEFAULT = 0.15

// readVar reads a CSS variable from the document root and returns its
// trimmed string value, or null if unavailable. Wrapped in a try/catch
// so a test environment without `document` doesn't crash.
function readVar(name) {
  try {
    if (typeof document === 'undefined') return null
    const v = getComputedStyle(document.documentElement).getPropertyValue(name)
    return v ? v.trim() : null
  } catch {
    return null
  }
}

// parseHex turns "#rgb" / "#rrggbb" into a 24-bit integer. Returns null
// on parse failure so callers can substitute their default.
function parseHex(s) {
  if (!s) return null
  const m = s.match(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/)
  if (!m) return null
  let h = m[1]
  if (h.length === 3) {
    h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2]
  }
  return parseInt(h, 16)
}

function colorFromVar(name, fallback) {
  return parseHex(readVar(name)) ?? fallback
}

function cssStringFromVar(name, fallback) {
  return readVar(name) || fallback
}

// =============================================================================
// Semantic hues — deliberately theme-INVARIANT.
//
// The edge colours encode arrow meta-types and the node / territory colours
// encode node and context identity. They are declared once in a shared
// :root block with no per-theme override, and ExplorerLegend renders swatches
// that must keep matching what Three.js draws. They are still resolved at
// call time (one path for everything), they simply return the same value in
// every theme.
// =============================================================================

export function getEdgeColor(meta) {
  switch (meta) {
    case 'contains':
      return colorFromVar('--explorer-edge-contains', DEFAULTS.edgeContains)
    case 'leadsto':
      return colorFromVar('--explorer-edge-leads-to', DEFAULTS.edgeLeadsTo)
    case 'properties':
      return colorFromVar('--explorer-edge-properties', DEFAULTS.edgeProperties)
    case 'similarto':
      return colorFromVar('--explorer-edge-similar-to', DEFAULTS.edgeSimilarTo)
    case 'unknown':
      return colorFromVar('--explorer-edge-unknown', DEFAULTS.edgeUnknown)
    default:
      return colorFromVar('--explorer-edge-unclassified', DEFAULTS.edgeUnclassified)
  }
}

export function getNodeColor(isAnchor) {
  return isAnchor
    ? colorFromVar('--explorer-node-anchor', DEFAULTS.nodeAnchor)
    : colorFromVar('--explorer-node-default', DEFAULTS.nodeDefault)
}

export function getTerritoryPalette() {
  const out = new Array(DEFAULTS.territory.length)
  for (let i = 0; i < out.length; i++) {
    out[i] = colorFromVar(`--explorer-territory-${i}`, DEFAULTS.territory[i])
  }
  return out
}

// =============================================================================
// Themed values — these DO change per theme, and every caller must therefore
// resolve them at the moment it paints (scene build, or a retheme pass).
// =============================================================================

// Grid lines and fill of the neutral floor (the synthetic "Untagged" layer,
// which has no palette colour of its own).
export function getPlaneGridColor() {
  return colorFromVar('--explorer-plane-grid', DEFAULTS.planeGrid)
}

export function getPlaneTintColor() {
  return colorFromVar('--explorer-plane-tint', DEFAULTS.planeTint)
}

// Label fill / halo / secondary fill. CSS strings — they are handed to
// canvas2d, and the sub colour intentionally carries alpha.
export function getLabelColor() {
  return cssStringFromVar('--explorer-label', DEFAULTS.label)
}

export function getLabelHaloColor() {
  return cssStringFromVar('--explorer-label-halo', DEFAULTS.labelHalo)
}

export function getLabelSubColor() {
  return cssStringFromVar('--explorer-label-sub', DEFAULTS.labelSub)
}

// Cycle-warning glyph fill/halo. Same theme-invariant family as
// getEdgeColor/getTerritoryPalette above — declared in the :root-only data
// block, no per-theme override — but CSS strings like the label resolvers
// since NodeMeshes.js feeds them straight to canvas2d fillStyle/strokeStyle.
export function getCycleGlyphColor() {
  return cssStringFromVar('--explorer-cycle-glyph', DEFAULTS.cycleGlyph)
}

export function getCycleGlyphHaloColor() {
  return cssStringFromVar('--explorer-cycle-halo', DEFAULTS.cycleHalo)
}
