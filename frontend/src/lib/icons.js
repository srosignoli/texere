// SVG icon geometry, kept as plain data so `node --test` can cover it — the
// test runner cannot parse .svelte, so anything testable has to live outside a
// component (the same reason cell-label.js exists).
//
// Geometry is adapted from Lucide (https://lucide.dev), ISC licensed. Only the
// path strings are copied; Lucide is deliberately NOT a dependency, because the
// app ships offline and self-contained.
//
// All icons are 24x24 outline paths meant to be stroked, never filled.
// Icon.svelte applies stroke="currentColor" so they follow the theme.
export const ICONS = {
  save: {
    viewBox: '0 0 24 24',
    paths: [
      'M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z',
      'M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7',
      'M7 3v4a1 1 0 0 0 1 1h7',
    ],
  },
  search: {
    viewBox: '0 0 24 24',
    paths: [
      'M11 3a8 8 0 1 0 0 16 8 8 0 0 0 0-16z',
      'm21 21-4.35-4.35',
    ],
  },
  globe: {
    viewBox: '0 0 24 24',
    paths: [
      'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z',
      'M2 12h20',
      'M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z',
    ],
  },
  'book-open': {
    viewBox: '0 0 24 24',
    paths: [
      'M12 7v14',
      'M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z',
    ],
  },
  books: {
    viewBox: '0 0 24 24',
    paths: [
      'M4 4v16',
      'M8 4v16',
      'M12 5v15',
      'm16 6 4 14',
    ],
  },
  graph: {
    viewBox: '0 0 24 24',
    paths: [
      'M12 2.7 21.3 12 12 21.3 2.7 12z',
    ],
  },
  expand: {
    viewBox: '0 0 24 24',
    paths: [
      'M8 3H5a2 2 0 0 0-2 2v3',
      'M21 8V5a2 2 0 0 0-2-2h-3',
      'M3 16v3a2 2 0 0 0 2 2h3',
      'M16 21h3a2 2 0 0 0 2-2v-3',
    ],
  },
  collapse: {
    viewBox: '0 0 24 24',
    paths: [
      'M8 3v3a2 2 0 0 1-2 2H3',
      'M21 8h-3a2 2 0 0 1-2-2V3',
      'M3 16h3a2 2 0 0 1 2 2v3',
      'M16 21v-3a2 2 0 0 1 2-2h3',
    ],
  },
  celebrate: {
    viewBox: '0 0 24 24',
    paths: [
      'M5.8 11.3 2 22l10.7-3.79',
      'M11 13c1.93 1.93 2.83 4.17 2 5-.83.83-3.07-.07-5-2-1.93-1.93-2.83-4.17-2-5 .83-.83 3.07.07 5 2z',
      'M15 3v2',
      'M19 5l-1.5 1.5',
      'M21 10h-2',
    ],
  },
}
