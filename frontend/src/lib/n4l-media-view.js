// Renders media referenced from an N4L line, as a block widget BELOW that line.
//
// Below, never replacing: this pane is an editor, and hiding the path would
// make a broken link invisible and unfixable, and the text awkward to edit.

import { Decoration, EditorView, ViewPlugin, WidgetType } from '@codemirror/view'
import { RangeSetBuilder } from '@codemirror/state'
import { GetSourceBytes } from '../../wailsjs/go/main/App'
import { findMedia, mediaMime } from './n4l-media.js'

// path -> Promise<Blob|null>. Failures are cached too: without that, typing a
// path fires a fresh load on every keystroke.
//
// A Blob rather than an object URL, because one URL shared between widgets
// would break every other widget the moment the first one revoked it. Each
// widget makes its own URL from this blob and revokes only that.
const blobs = new Map()

function loadBlob(path) {
  if (blobs.has(path)) return blobs.get(path)
  const p = GetSourceBytes(path)
    .then((b64) => new Blob([bytesFrom(b64)], { type: mediaMime(path) }))
    .catch(() => null)
  blobs.set(path, p)
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
    this.url = null
  }

  // Without this CodeMirror rebuilds the widget on every update, which would
  // restart a playing video on each keystroke.
  eq(other) { return other.path === this.path && other.kind === this.kind }

  toDOM() {
    const box = document.createElement('div')
    box.className = 'n4l-media'
    loadBlob(this.path).then((blob) => {
      if (!blob) {
        const miss = document.createElement('span')
        miss.className = 'n4l-media-missing'
        miss.textContent = `media not found: ${this.path}`
        box.appendChild(miss)
        return
      }
      this.url = URL.createObjectURL(blob)
      const el = document.createElement(this.kind === 'image' ? 'img' : this.kind)
      if (this.kind !== 'image') el.controls = true
      el.src = this.url
      box.appendChild(el)
    })
    return box
  }

  destroy() {
    if (this.url) URL.revokeObjectURL(this.url)
    this.url = null
  }

  // The widget owns its own async content; CodeMirror must not diff into it.
  ignoreEvent() { return true }
}

function mediaDecorations(view) {
  const b = new RangeSetBuilder()
  for (const { from, to } of view.visibleRanges) {
    for (let pos = from; pos <= to;) {
      const line = view.state.doc.lineAt(pos)
      for (const m of findMedia(line.text)) {
        b.add(line.to, line.to, Decoration.widget({
          widget: new MediaWidget(m.path, m.kind), side: 1, block: true,
        }))
      }
      pos = line.to + 1
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
  return [
    ViewPlugin.fromClass(
      class {
        constructor(view) { this.decorations = mediaDecorations(view) }
        update(u) { if (u.docChanged || u.viewportChanged) this.decorations = mediaDecorations(u.view) }
      },
      { decorations: (v) => v.decorations },
    ),
    theme,
  ]
}
