<script>
  // Always-visible legend strip for the 2.5D Isometric Graph Explorer.
  //
  // Compact horizontal row of chips, one per edge meta-type. Each
  // chip shows a tiny SVG glyph that mirrors the actual marker
  // shape drawn on the edge (cone / diamond / disc / pair-of-cones)
  // so the legend communicates direction-vocabulary, not just colour.
  // The line swatch on the left is the same edge colour the renderer
  // uses (Theme.js).

  import {
    COLOR_CONTAINS,
    COLOR_LEADS_TO,
    COLOR_PROPERTIES,
    COLOR_SIMILAR_TO,
    COLOR_UNCLASSIFIED,
    COLOR_UNKNOWN,
  } from './explorer/Theme.js'

  function hex(rgb) {
    return '#' + rgb.toString(16).padStart(6, '0')
  }

  // `marker` codes mirror the EdgeMeshes.js dispatcher:
  //   cone-end    → leadsto         (cone at destination)
  //   diamond-src → contains        (diamond at source)
  //   disc-end    → properties      (sphere at destination)
  //   cone-both   → similarto       (two cones, both ends)
  //   none        → unclassified
  const ENTRIES = [
    { key: 'contains',     label: 'Contains',     color: hex(COLOR_CONTAINS),     marker: 'diamond-src' },
    { key: 'leadsto',      label: 'Leads to',     color: hex(COLOR_LEADS_TO),     marker: 'cone-end'    },
    { key: 'properties',   label: 'Properties',   color: hex(COLOR_PROPERTIES),   marker: 'disc-end'    },
    { key: 'similarto',    label: 'Similar to',   color: hex(COLOR_SIMILAR_TO),   marker: 'cone-both'   },
    { key: 'unclassified', label: 'Other',        color: hex(COLOR_UNCLASSIFIED), marker: 'none'        },
    { key: 'unknown',      label: 'Unknown arrow code', color: hex(COLOR_UNKNOWN), marker: 'dashed'     },
  ]
</script>

<div class="legend" role="img" aria-label="Edge color legend">
  {#each ENTRIES as e (e.key)}
    <div class="chip">
      <svg class="glyph" viewBox="0 0 40 12" width="40" height="12" aria-hidden="true">
        <!-- Edge line. Unknown edges are dashed to match the renderer. -->
        <line x1="4" y1="6" x2="36" y2="6" stroke={e.color} stroke-width="1.6" stroke-linecap="round"
          stroke-dasharray={e.marker === 'dashed' ? '4 2.5' : null} />
        {#if e.marker === 'cone-end'}
          <!-- Filled triangle at the end. -->
          <polygon points="32,3 38,6 32,9" fill={e.color} />
        {:else if e.marker === 'diamond-src'}
          <!-- Filled diamond at the start. -->
          <polygon points="4,3 8,6 4,9 0,6" fill={e.color} />
        {:else if e.marker === 'disc-end'}
          <!-- Small filled disc at the end. -->
          <circle cx="35" cy="6" r="2.8" fill={e.color} />
        {:else if e.marker === 'cone-both'}
          <!-- Filled triangles at both ends, each pointing toward its endpoint. -->
          <polygon points="8,3 2,6 8,9" fill={e.color} />
          <polygon points="32,3 38,6 32,9" fill={e.color} />
        {/if}
      </svg>
      <span class="label">{e.label}</span>
    </div>
  {/each}
</div>

<style>
  .legend {
    position: absolute;
    top: 12px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 14px;
    background: rgba(14, 20, 24, 0.85);
    border: 1px solid #334455;
    border-radius: 6px;
    padding: 6px 12px;
    pointer-events: none;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    z-index: 4;
  }
  .chip {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: #e8eaf0;
    white-space: nowrap;
  }
  .glyph {
    display: block;
    flex: 0 0 40px;
  }
  .label {
    line-height: 1;
  }
</style>
