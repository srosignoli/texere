// Color theme for the 2.5D Isometric Graph Explorer (feature 006).
//
// Single source of truth: CSS variables declared in `frontend/src/style.css`
// (`:root { --explorer-* }`). This module reads them lazily so Three.js
// materials can be rebuilt against a fresh palette if a theme switcher
// is added later. Each lookup falls back to a hardcoded default so the
// explorer renders correctly even if a variable is missing (e.g. during
// SSR / test environments without a real document).
//
// Three.js consumes 24-bit hex integers, not CSS strings, so every
// resolver parses "#rrggbb" / "#rgb" into a number.

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
  groundPlane: 0x1b2636,
  planeGrid: 0x6b7c95,
  planeTint: 0x1f2a3a,
  extendedPlaneGrid: 0x7a78a5,
  extendedPlaneTint: 0x2c2a40,

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

// Lazy resolvers — call at scene-build time. Callers that want the
// value-at-import-time can also use the eagerly-evaluated exports
// below, which mirror the legacy module surface.
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

export function getGroundPlaneColor() {
  return colorFromVar('--explorer-ground', DEFAULTS.groundPlane)
}

export function getPlaneGridColor() {
  return colorFromVar('--explorer-plane-grid', DEFAULTS.planeGrid)
}

export function getPlaneTintColor() {
  return colorFromVar('--explorer-plane-tint', DEFAULTS.planeTint)
}

export function getExtendedPlaneTintColor() {
  return colorFromVar('--explorer-extended-plane-tint', DEFAULTS.extendedPlaneTint)
}

export function getExtendedPlaneGridColor() {
  return colorFromVar('--explorer-extended-plane-grid', DEFAULTS.extendedPlaneGrid)
}

export function getLabelColor() {
  return cssStringFromVar('--explorer-label', DEFAULTS.label)
}

export function getLabelHaloColor() {
  return cssStringFromVar('--explorer-label-halo', DEFAULTS.labelHalo)
}

export function getTerritoryPalette() {
  const out = new Array(DEFAULTS.territory.length)
  for (let i = 0; i < out.length; i++) {
    out[i] = colorFromVar(`--explorer-territory-${i}`, DEFAULTS.territory[i])
  }
  return out
}

// =============================================================================
// Legacy eager exports — kept so existing imports continue to work. New
// code should prefer the get*() resolvers above so it picks up theme
// changes if the document's :root variables are mutated at runtime.
// =============================================================================

export const COLOR_CONTAINS = colorFromVar('--explorer-edge-contains', DEFAULTS.edgeContains)
export const COLOR_LEADS_TO = colorFromVar('--explorer-edge-leads-to', DEFAULTS.edgeLeadsTo)
export const COLOR_PROPERTIES = colorFromVar('--explorer-edge-properties', DEFAULTS.edgeProperties)
export const COLOR_SIMILAR_TO = colorFromVar('--explorer-edge-similar-to', DEFAULTS.edgeSimilarTo)
export const COLOR_UNCLASSIFIED = colorFromVar('--explorer-edge-unclassified', DEFAULTS.edgeUnclassified)
export const COLOR_UNKNOWN = colorFromVar('--explorer-edge-unknown', DEFAULTS.edgeUnknown)

export const COLOR_NODE_DEFAULT = colorFromVar('--explorer-node-default', DEFAULTS.nodeDefault)
export const COLOR_NODE_ANCHOR = colorFromVar('--explorer-node-anchor', DEFAULTS.nodeAnchor)
export const COLOR_LABEL = cssStringFromVar('--explorer-label', DEFAULTS.label)
export const COLOR_LABEL_HALO = cssStringFromVar('--explorer-label-halo', DEFAULTS.labelHalo)
export const COLOR_GROUND_PLANE = colorFromVar('--explorer-ground', DEFAULTS.groundPlane)
export const COLOR_PLANE_GRID = colorFromVar('--explorer-plane-grid', DEFAULTS.planeGrid)
export const COLOR_PLANE_TINT = colorFromVar('--explorer-plane-tint', DEFAULTS.planeTint)
export const COLOR_EXTENDED_PLANE_GRID = colorFromVar('--explorer-extended-plane-grid', DEFAULTS.extendedPlaneGrid)
export const COLOR_EXTENDED_PLANE_TINT = colorFromVar('--explorer-extended-plane-tint', DEFAULTS.extendedPlaneTint)

export const TERRITORY_PALETTE = getTerritoryPalette()

export function colorForMeta(meta) {
  return getEdgeColor(meta)
}
