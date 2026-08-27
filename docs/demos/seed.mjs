// Prepare a throwaway collection and point the app at it, BEFORE starting the
// dev server.
//
// This is a separate step, and not something study-session.mjs can do itself,
// because App.svelte decides whether a collection is open from
// GetInitialSummaries() — a snapshot taken at startup(). Calling
// OpenCollection later updates Go but leaves the UI on whatever it booted
// with, and reloading the page re-reads the same startup snapshot. So the
// fixture has to be in the config before the app starts.
//
// The previous collectionDir is saved next to the fixture; `npm run restore`
// puts it back.

import { mkdirSync, writeFileSync, rmSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { FIXTURE, CONFIG, SAVED, PHRASE } from './fixture.mjs';

function seedFixture() {
  rmSync(FIXTURE, { recursive: true, force: true });
  mkdirSync(join(FIXTURE, 'sources'), { recursive: true });
  mkdirSync(join(FIXTURE, 'reading'), { recursive: true });
  writeFileSync(join(FIXTURE, 'memory.n4l'), `- Memory research

 :: _sequence_ ::

encoding (leads to) storage
storage (leads to) retrieval
`);
  writeFileSync(join(FIXTURE, 'study-doc.md'), `# The Forgetting Curve

Hermann Ebbinghaus discovered that memory retention declines sharply in the
hours after learning, then levels off. He called this the forgetting curve.

Spaced repetition counters the curve by scheduling reviews just before the
point of forgetting, which strengthens the memory trace each time.

${PHRASE},
and it produces markedly stronger retention than passive review.
`);
}

function pointConfigAtFixture() {
  if (!existsSync(CONFIG)) {
    throw new Error(`no app config at ${CONFIG} — run the app once first`);
  }
  const cfg = JSON.parse(readFileSync(CONFIG, 'utf8'));
  const previous = cfg.collectionDir ?? '';
  writeFileSync(SAVED, previous);
  cfg.collectionDir = FIXTURE;
  writeFileSync(CONFIG, JSON.stringify(cfg, null, 2));
  return previous;
}

seedFixture();
const previous = pointConfigAtFixture();
console.log('fixture   :', FIXTURE);
console.log('config    :', CONFIG);
console.log('previous  :', previous || '(none)');
console.log('\nNow start the app:  make dev-demo');
console.log('Then run        :  npm run study-session');
console.log('Afterwards      :  npm run restore');
