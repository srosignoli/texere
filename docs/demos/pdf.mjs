// The fixture's PDF, generated rather than committed.
//
// The reading tutorial has to import one of each source kind, and a PDF is the
// only one that cannot be written as a string. Chromium renders it — the same
// browser the recordings already run in — so nothing new is installed and the
// text layer is real, which matters: the reader extracts by selecting text, and
// a scanned or image-only PDF would have nothing to select.
//
// Generated, not committed, for the reason the rest of the fixture is: seeding
// deletes and recreates the collection, so anything a take needs must be
// reproducible from this repo alone.
//
// Content is deliberately three short sections with a heading each, because the
// tutorial takes three extracts from it and they should come from different
// parts of the document rather than three consecutive sentences.

import { chromium } from 'playwright'

const CHROMIUM =
  process.env.TEXERE_CHROMIUM ??
  '/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome'

// PAGE SIZE IS A FIXTURE DECISION, because the reader has no zoom.
//
// PdfSourcePane renders at a fixed scale of 1.4 and offers no fit-to-width, so
// an A4 page becomes an 834px canvas inside a 570px pane (709px with the
// sources sidebar hidden) and is simply cut off at the margin. Nothing in the
// app can be adjusted to fix that from a plot.
//
// So the fixture's page is 5.2in wide: 5.2 * 96 * 1.4 = 699px, which fits the
// pane with the sidebar collapsed and leaves the whole width of the document
// visible. The height is set to keep the content on ONE page, so the tutorial
// is not obliged to paginate mid-demonstration.
const PAGE = { width: '6.6in', height: '7.4in' }

const HTML = `<style>
  body { font: 10.5pt/1.5 Georgia, 'Times New Roman', serif; margin: 34px 38px; color: #111 }
  h1 { font-size: 15pt; margin: 0 0 2px }
  .sub { color: #555; font-style: italic; margin: 0 0 14px; font-size: 9.5pt }
  h2 { font-size: 11.5pt; margin: 16px 0 4px }
  p { margin: 6px 0 }
</style>
<h1>Spacing, Testing and Forgetting</h1>
<p class="sub">Three findings that have survived replication.</p>

<h2>1. The spacing effect</h2>
<p>Study sessions separated by a delay produce more durable memory than the same
total study time massed together. The effect is large, old, and robust across
materials and age groups.</p>

<h2>2. Retrieval practice</h2>
<p>Being asked a question changes a memory more than being shown the answer again.
A quiz outperforms rereading even when no feedback is given, and the effect
survives when study time is held constant.</p>

<h2>3. Judgements of learning</h2>
<p>Fluency is mistaken for durability. Material that feels easy while you study it
is often the first to go, which is why learners reliably prefer the methods that
serve them worst.</p>`

/** Render the fixture's PDF to `path`. */
export async function writeReadingPdf(path) {
  const browser = await chromium.launch({ executablePath: CHROMIUM, args: ['--no-sandbox'] })
  try {
    const page = await browser.newPage()
    await page.setContent(HTML, { waitUntil: 'load' })
    await page.pdf({ path, ...PAGE, printBackground: true })
  } finally {
    await browser.close()
  }
  return path
}
