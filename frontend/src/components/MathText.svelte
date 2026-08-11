<script>
  // Renders a note string with its LaTeX typeset.
  //
  // This is the ONLY place in the feature where escaping is bypassed, and it is
  // bypassed only for a string KaTeX itself produced from the LaTeX source.
  // Prose segments go through ordinary interpolation and stay escaped — never
  // pass raw note text to {@html}.
  //
  // KaTeX defaults to trust:false, so it emits no scripts and no javascript:
  // links even from hostile input.
  import katex from 'katex'
  import 'katex/dist/katex.min.css'
  import { splitMath } from '../lib/n4l-math.js'

  export let text

  $: segments = splitMath(text)

  // throwOnError:false renders bad LaTeX in red with its source showing, rather
  // than throwing and taking the whole card down.
  const render = (value, displayMode) =>
    katex.renderToString(value, { displayMode, throwOnError: false })
</script>

{#each segments as seg}
  {#if seg.type === 'text'}{seg.value}
  {:else if seg.type === 'inline'}{@html render(seg.value, false)}
  {:else}{@html render(seg.value, true)}
  {/if}
{/each}
