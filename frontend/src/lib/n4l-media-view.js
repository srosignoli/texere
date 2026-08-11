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

import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import { RangeSetBuilder, StateField } from '@codemirror/state'
import { GetSourceBytes } from '../../wailsjs/go/main/App'
import { findMedia, mediaMime } from './n4l-media.js'

// path -> Promise<string|null>, the object URL, or null for a file that could
// not be read. Failures are cached too: without that, typing a path fires a
// fresh load on every keystroke.
//
// One URL per path, made once and never revoked. Per-widget URLs were the
// obvious design and were wrong: CodeMirror reuses a widget's DOM when eq()
// matches but installs the NEW instance, dropping the old one without calling
// destroy() (index.js:2536-2549, 3446-3453). The instance holding the URL was
// therefore discarded silently, and the one eventually destroyed had none —
// so editing at the end of a media line, which is the ordinary case since the
// path is usually last, leaked a URL every time. This cache already pins the
// decoded bytes for the life of the window; holding one URL beside them costs
// nothing more and removes the whole class of lifecycle bug.
const media = new Map()

function loadMedia(path) {
  if (media.has(path)) return media.get(path)
  const p = GetSourceBytes(path)
    .then((b64) => URL.createObjectURL(new Blob([bytesFrom(b64)], { type: mediaMime(path) })))
    .catch(() => null)
  media.set(path, p)
  return p
}

function bytesFrom(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

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
  // between mount and decode; requestMeasure below corrects it exactly.
  get estimatedHeight() { return this.kind === 'audio' ? 54 : 240 }

  toDOM(view) {
    const box = document.createElement('div')
    box.className = 'n4l-media'
    loadMedia(this.path).then((url) => {
      if (!url) {
        const miss = document.createElement('span')
        miss.className = 'n4l-media-missing'
        miss.textContent = `media not found: ${this.path}`
        box.appendChild(miss)
        view.requestMeasure()
        return
      }
      const el = document.createElement(this.kind === 'image' ? 'img' : this.kind)
      if (this.kind !== 'image') el.controls = true
      // Appending schedules a measure, but at that instant the element has no
      // intrinsic size. Decoding is what grows it to its cap, and decoding
      // mutates no DOM, so nothing would re-measure: the height map would
      // under-report every media block and drift the line numbers beside it.
      el.addEventListener(this.kind === 'image' ? 'load' : 'loadedmetadata',
        () => view.requestMeasure(), { once: true })
      el.src = url
      box.appendChild(el)
    })
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

const mediaField = StateField.define({
  create: (state) => mediaDecorations(state),
  update: (deco, tr) => (tr.docChanged ? mediaDecorations(tr.state) : deco),
  provide: (f) => EditorView.decorations.from(f),
})

export function n4lMedia() {
  return [mediaField, theme]
}
