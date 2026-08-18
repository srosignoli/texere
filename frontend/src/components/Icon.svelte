<script>
  import { ICONS } from '../lib/icons.js'

  export let name
  export let size = 16
  // Accessible name. Empty means decorative — correct when a visible text label
  // sits beside the icon, wrong when the icon is the button's only content.
  export let label = ''

  // Fail soft. A mistyped name must not blank a toolbar or throw inside a
  // button's render; icons.test.js catches the typo at test time instead.
  $: icon = ICONS[name]
  $: if (!icon) console.warn(`Icon: unknown name "${name}"`)
</script>

{#if icon}
  <svg
    viewBox={icon.viewBox}
    width={size}
    height={size}
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    role={label ? 'img' : undefined}
    aria-label={label || undefined}
    aria-hidden={label ? undefined : 'true'}
  >
    {#each icon.paths as d}
      <path {d} />
    {/each}
  </svg>
{/if}

<style>
  /* Sits on the text baseline next to a label, and never stretches in a flex row. */
  svg { display: inline-block; vertical-align: -0.125em; flex: none; }
</style>
