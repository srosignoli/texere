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

  // throwOnError:false makes KaTeX render bad LaTeX in red with its source
  // showing — but only for its own ParseError. A RangeError from pathologically
  // nested input, or one of KaTeX's internal Errors, escapes that option
  // entirely and would take the whole card down. So the call is wrapped, and a
  // segment that cannot be typeset falls back to its source text.
  //
  // trust:false is KaTeX's default and is set explicitly: it is what stops
  // \href, \url and \html* turning note text into arbitrary markup, and this
  // component's {@html} depends on it. An implicit invariant that a future
  // major version could flip is not one worth relying on silently.
  function render(value, displayMode) {
    try {
      return { ok: true, html: katex.renderToString(value, { displayMode, throwOnError: false, trust: false }) }
    } catch {
      return { ok: false }
    }
  }
</script>

{#each segments as seg}
  {#if seg.type === 'text'}{seg.value}
  {:else}
    {@const r = render(seg.value, seg.type === 'display')}
    <!-- The fallback goes through ordinary interpolation, never {@html}: only a
         string KaTeX produced is ever injected. -->
    {#if r.ok}{@html r.html}{:else}<span class="math-error">{seg.value}</span>{/if}
  {/if}
{/each}

<style>
  .math-error { color: #ff8080; font-family: monospace; font-size: 0.9em; }
</style>
