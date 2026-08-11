// Renders media referenced from an N4L line, as a block widget BELOW that line.
//
// Below, never replacing: this pane is an editor, and hiding the path would
// make a broken link invisible and unfixable, and the text awkward to edit.
//
// The decorations come from a StateField, NOT a ViewPlugin. CodeMirror splits
// the decorations facet in two: sets provided directly may affect vertical
// layout, sets provided as a function are computed after the viewport is known
// and may not. A ViewPlugin registers its decorations as a function
// (@codemirror/view index.js:1420), so a block decoration from one throws
// `RangeError: Block decorations may not be specified via plugins`
// (index.js:2721) — out of the EditorView constructor on open, or out of the
// keystroke that first types a media path. A StateField provides its value
// directly and is allowed block widgets.
//
// A StateField has no viewport, so this scans the whole document rather than
// the visible ranges. Reading notes files are small, and correctness here is
// not optional.
//
// Nothing here loads bytes: every asset is an ordinary src the webview fetches,
// so http.ServeFile's range support makes a video seek for free and the browser
// does the caching.

import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import { RangeSetBuilder, StateField } from '@codemirror/state'
import { findMedia, srcFor } from './n4l-media.js'

class MediaWidget extends WidgetType {
  constructor(path, kind) {
    super()
    this.path = path
    this.kind = kind
  }

  // Without this CodeMirror rebuilds the widget on every update, which would
  // restart a playing video on each keystroke.
  eq(other) { return other.path === this.path && other.kind === this.kind }

  // Reserves roughly the final height so the height map is not wildly wrong
  // before the media loads; requestMeasure below corrects it exactly.
  get estimatedHeight() { return this.kind === 'audio' ? 54 : 240 }

  toDOM(view) {
    const box = document.createElement('div')
    box.className = 'n4l-media'
    const el = document.createElement(this.kind === 'image' ? 'img' : this.kind)
    if (this.kind !== 'image') el.controls = true

    // Loading grows the element to its cap and mutates no DOM, so nothing else
    // would re-measure: the height map would under-report every media block and
    // drift the line numbers beside it.
    el.addEventListener(this.kind === 'image' ? 'load' : 'loadedmetadata',
      () => view.requestMeasure(), { once: true })

    // Without this a failed load shows the browser's broken-image icon. The
    // chip names the path instead, so a typo is diagnosable in place.
    el.addEventListener('error', () => {
      el.remove()
      const miss = document.createElement('span')
      miss.className = 'n4l-media-missing'
      miss.textContent = `media not found: ${this.path}`
      box.appendChild(miss)
      view.requestMeasure()
    }, { once: true })

    el.src = srcFor(this.path)
    box.appendChild(el)
    return box
  }

  // Governs DOM events, not diffing: without it the video scrubber's pointer
  // events would be treated as editor input.
  ignoreEvent() { return true }
}

function mediaDecorations(state) {
  const b = new RangeSetBuilder()
  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    for (const m of findMedia(line.text)) {
      b.add(line.to, line.to, Decoration.widget({
        widget: new MediaWidget(m.path, m.kind), side: 1, block: true,
      }))
    }
  }
  return b.finish()
}

// The 240px cap keeps a note with twenty assets navigable.
const theme = EditorView.theme({
  '.n4l-media': { padding: '4px 0 8px 0' },
  '.n4l-media img, .n4l-media video': {
    maxHeight: '240px', maxWidth: '100%', display: 'block', borderRadius: '4px',
  },
  '.n4l-media audio': { width: '100%', maxWidth: '420px', display: 'block' },
  '.n4l-media-missing': { color: '#8899aa', fontStyle: 'italic', fontSize: '11px' },
})

export function n4lMedia() {
  const field = StateField.define({
    create: (state) => mediaDecorations(state),
    update: (deco, tr) => (tr.docChanged ? mediaDecorations(tr.state) : deco),
    provide: (f) => EditorView.decorations.from(f),
  })
  return [field, theme]
}
