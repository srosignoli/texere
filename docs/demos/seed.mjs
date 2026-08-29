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


// Supporting documents, imported off-camera so the reading list is populated
// before the viewer sees it. Deliberately short: they are set dressing, and
// every one of them costs a second of import time behind the title card.
const LIBRARY = {
  'encoding.md': `# Encoding

Encoding is the first step of memory: turning what you perceive into a trace
the brain can store. Elaborative encoding — connecting new material to what you
already know — produces a far more durable trace than rote repetition.
`,
  'retrieval-practice.md': `# Retrieval Practice

Testing yourself is not merely a way to measure learning; it changes it. The
act of retrieving a memory strengthens the path back to it, which is why a
quiz outperforms rereading even when no feedback is given.
`,
  'interleaving.md': `# Interleaving

Studying one topic to exhaustion feels productive and performs poorly. Mixing
related topics within a session — interleaving — slows you down in the moment
and improves retention weeks later.
`,
  'sleep-and-memory.md': `# Sleep and Memory

Consolidation happens largely during sleep, when the hippocampus replays the
day's traces to the cortex. A night of poor sleep after learning costs more
than the same hours lost before it.
`,
}

function seedFixture() {
  rmSync(FIXTURE, { recursive: true, force: true });
  mkdirSync(join(FIXTURE, 'sources'), { recursive: true });
  mkdirSync(join(FIXTURE, 'reading'), { recursive: true });
  writeFileSync(join(FIXTURE, 'memory.n4l'), `- Memory research

 :: _sequence_ ::

encoding (leads to) storage
storage (leads to) retrieval
`);
  // A collection with one document in it looks like a demo. Texere opened on a
  // real collection shows a populated library, so the fixture carries several
  // documents; the plot imports them behind the title card and features one on
  // camera. See LIBRARY below.
  for (const [name, body] of Object.entries(LIBRARY)) {
    writeFileSync(join(FIXTURE, name), body);
  }
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
  const current = cfg.collectionDir ?? '';

  // Seeding twice without restoring in between would save the fixture as the
  // directory to go back to — and seedFixture() has just deleted it. Restore
  // would then point the app at a path that no longer exists, which is the one
  // state it cannot recover from without picking a collection by hand.
  //
  // So only record `previous` when it is something other than the fixture, and
  // never clobber a good saved value with a bad one.
  if (current !== FIXTURE) {
    writeFileSync(SAVED, current);
  } else if (!existsSync(SAVED)) {
    throw new Error(
      `the config already points at the fixture (${FIXTURE}) and no previous ` +
        'collection is on record.\nSet collectionDir back to your own ' +
        `collection in ${CONFIG} before seeding.`,
    );
  }

  cfg.collectionDir = FIXTURE;
  writeFileSync(CONFIG, JSON.stringify(cfg, null, 2));
  return readFileSync(SAVED, 'utf8').trim();
}

seedFixture();
const previous = pointConfigAtFixture();
console.log('fixture   :', FIXTURE);
console.log('config    :', CONFIG);
console.log('previous  :', previous || '(none)');
console.log('\nNow start the app:  make dev-demo');
console.log('Then run        :  npm run study-session');
console.log('Afterwards      :  npm run restore');
