// The reference plot: a full study session — document → extract → card →
// review — against the real Go backend.
//
// This one stays in texere as the kit's smoke test: if it records, the kit and
// the selector contract still work. Tutorial plots live in the separate
// texere-tutorials repository.
//
//   npm run seed          create the fixture, point the app at it
//   make dev-demo         (repo root, another terminal)
//   npm run study-session
//   npm run restore       put your own collection back

import { join } from 'node:path'
import { session } from './kit.mjs'
import { SEL } from './selectors.mjs'
import { PHRASE } from './fixture.mjs'

const t = await session({
  title: 'Texere',
  subtitle: 'A study session, start to finish',
  slug: 'study-session',
})

await t.open()

await t.beat('Open a document', async () => {
  await t.say('Opening a document. The file dialog is bypassed by the demo seam.', 2400)
  const rv = await t.call('DemoImportTextSource', join(t.fixture, 'study-doc.md'))
  t.note(`imported ${rv.id} → ${rv.refPath}`)
  await t.until((c) => c.read > 0, { label: 'queued for reading' })
  await t.say('Imported and queued for reading.', 1900)
  await t.clear()
})

await t.beat('The study queue', async () => {
  await t.say('Study — the Read · Process · Recall queue.', 2000)
  await t.click(SEL.studyButton)
  await t.until(SEL.studyDialog)
  await t.wait(900)
  await t.click(SEL.studyTab('Read'))
  await t.wait(1200)
  await t.clear()
})

await t.beat('Open it in the reader', async () => {
  await t.say('Open the document.', 1600)
  await t.click(SEL.sourceRow)
  await t.until(SEL.sourceText)
  await t.page.waitForFunction(
    `document.querySelector('${SEL.sourceText}')?.innerText?.length > 50`,
    null,
    { timeout: 12000 },
  )
  await t.wait(1200)
  await t.clear()
})

await t.beat('Extract a passage', async () => {
  await t.say('Select a passage worth keeping.', 1800)
  const sel = await t.selectText(PHRASE, SEL.sourceText)
  t.note(`selected "${sel.text.slice(0, 48)}…"`)
  await t.wait(1600)

  await t.frame() // the highlighted passage, before it is consumed
  await t.say('Extract it.', 1500)
  await t.click(SEL.extract)
  await t.until((c) => c.process > 0, { label: 'extract registered (process queue > 0)' })
  await t.wait(1200)
  await t.clear()
})

await t.beat('Turn it into a card', async () => {
  await t.say('Process turns the extract into knowledge.', 2100)
  await t.click(SEL.studyTab('Process'))
  await t.wait(2000)
  await t.clear()

  // makeCard seeds "<passage> >> " and parks the cursor for the back. Clicking
  // Create card before typing materialises nothing — an empty back is not a card.
  await t.say('Make card seeds the front and waits for the answer.', 2200)
  await t.click(SEL.makeCard)
  await t.wait(1600)
  await t.clear()

  await t.say('Type the back of the card.', 1500)
  await t.type(' it beats passive rereading')
  await t.frame() // the seeded front and the typed back, both on screen
  await t.wait(1400)
  await t.clear()

  await t.say('Commit it to the deck.', 1600)
  await t.click(SEL.createCard)
  await t.until((c) => c.recall > 0, { label: 'card materialised (recall queue > 0)' })
  await t.wait(1800)
  await t.clear()
})

await t.beat('Recall and grade', async () => {
  await t.say('Recall — the card comes back on schedule.', 2100)
  await t.click(SEL.studyTab('Recall'))
  await t.until(SEL.showAnswer)
  await t.wait(1400)
  await t.click(SEL.showAnswer)
  await t.frame() // question and answer visible, before grading empties the queue
  await t.wait(1800)
  await t.say('Grade it, and FSRS schedules the next review.', 2100)
  await t.click(SEL.grade('Good'))
  await t.wait(1800)
  await t.clear()
})

await t.beat('Close', async () => {
  await t.say('Document → extract → card → review. One script, no hands.', 3000)
})

await t.finish()
