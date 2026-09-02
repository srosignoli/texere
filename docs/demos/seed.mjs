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
import { writeCurveSvg, writeCurveVideo } from './media.mjs';


// The .n4l side of the fixture.
//
// A collection needs more than one file, and a file more than one chapter,
// before "Files / Chapters" means anything on camera — the columns tutorial is
// about structure, and structure is invisible in a three-line collection.
//
// Every arrow code below resolves in ../../SSTconfig: the nodes column renders
// an arrow's LongName from the registry, so an unregistered code shows the raw
// code and reads as a typo rather than as a feature. `(fwd)` displaying as
// "leads to" is the point, not an accident.
// Context operators carry their closing `::`. The parser does not require it
// (parseTags trims a trailing ": " run, so `:: x`, `:: x :` and `:: x ::` all
// parse identically, and no lint rule reports the difference) — but this file
// is on camera teaching N4L, and canonical N4L closes the delimiter.
const N4L = {
  // Memory research — kept from the original fixture, with (leads to) corrected
  // to its registry code (fwd). "leads to" is the LongName the column renders;
  // as a code it was unregistered, and drew the unregistered-arrow styling.
  'memory.n4l': `- Memory research

 :: _sequence_ ::

encoding (fwd) storage
storage (fwd) retrieval
`,

  // The file the columns tutorial is recorded against. Three chapters, so the
  // tree has something to expand; a context state machine that extends and
  // reduces, so the chips change as the caret moves; and one named sequence,
  // so the nodes column has a timeline to draw.
  'learning.n4l': `- how memory works

:: cognition ::

Encoding turns perception into a durable trace
"  (means) the first step of remembering
"  (e.g.) explaining an idea in your own words

+:: neuroscience ::

The hippocampus binds a new trace
"  (fwd) cortical consolidation
"  (note) most of this happens during sleep

Consolidation moves the trace to the cortex
"  (bfr) retrieval becomes cheap

-:: neuroscience ::

Retrieval strengthens the path back to a memory
"  (means) testing is practice, not measurement

:: pedagogy ::

Interleaving mixes topics within one session
"  (e.g.) alternating problem types
"  (contain) desirable difficulty

# @id enc-1
Elaboration produces a more durable trace than rote repetition

# @id enc-2
A quiz outperforms rereading even when no feedback is given

# @card
What does retrieval practice change?
"  (means) the memory itself, not just your estimate of it

- the study loop

:: workflow ::

+:: _sequence_, reading loop ::

Read the source and mark what matters
"  (next) extract the passage

Extract the passage into the collection
"  (next) turn it into a card

Turn the extract into a card
"  (next) recall it on schedule

Recall it just before you would forget
"  (fwd) a stronger trace

-:: _sequence_, reading loop ::

Nothing after the loop belongs to it
"  (note) the sequence ended with the context

- open questions

:: cognition ::

Does interleaving help motor skills as much as concepts?
"  (tbd) needs a proper trial

Why does massed practice feel more effective than it is?
"  (note) fluency is mistaken for learning
`,

  // The file the preview tutorial is recorded against. It carries the two
  // things the Note tab and the nodes column cannot show and the Preview panel
  // can: LaTeX, and referenced media.
  //
  // The math is written with `\\(…\\)` and `\\[…\\]`, which are the ONLY
  // delimiters N4L recognises. `$…$` is not math here — `$` introduces a
  // variable — so a fixture teaching the dollar form would teach a form that
  // corrupts every variable reference in a real collection.
  //
  // The media paths are arrow TARGETS, under registered codes: (pic) reads as
  // "has picture" and (vid) as "has video". Detection is by file extension, not
  // by the arrow's name — but a note that says "has picture" and shows one is
  // the form worth putting on camera.
  //
  // The last line is in capitals on purpose. NodeCard reads an all-caps node as
  // a reminder and gives it the amber card; column three shows the same line as
  // ordinary text. It is the cheapest difference between the two surfaces that
  // is neither media nor math.
  //
  // One plain line follows it, and its job is to be last. The preview feed ends
  // at the window's bottom edge, where the caption bar floats, so whatever is
  // final in the file is the one card a frame cannot show. The closing line
  // gives the reminder and the display expression somewhere above it to sit,
  // and it is long enough to wrap and carries an annotation because that tail
  // has to be about 90px tall to lift the amber card clear of the caption.
  'forgetting.n4l': `- the forgetting curve

:: ebbinghaus ::

Retention falls with the time since you learned something
"  (means) \\(R = e^{-t/S}\\), where \\(S\\) is the stability of the trace
"  (pic) media/forgetting-curve.svg

Ebbinghaus measured the decay on himself, with nonsense syllables
"  (note) steep at first, then flattening — the shape is the finding

A review does not reset the clock — it raises \\(S\\)
"  (vid) media/forgetting-curve.mp4

\\[ R(t) = e^{-t/S} \\qquad S_{n+1} > S_n \\]

SPACING RAISES S, IT DOES NOT RESET THE CLOCK

The useful question a curve like this answers is not how hard to study, but when the next review should happen
"  (note) which is exactly what the study schedule is for
`,

  // A second file, so the first column is a tree of files rather than a list of
  // chapters wearing a filename.
  'research-methods.n4l': `- evidence

:: method ::

A within-subject design controls for individual variation
"  (e.g.) each learner studies both ways

Effect sizes matter more than p-values
"  (note) significance is not magnitude

- limitations

:: method ::

Laboratory word lists are not coursework
"  (means) generalisation is the open question
`,
}

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

async function seedFixture() {
  rmSync(FIXTURE, { recursive: true, force: true });
  mkdirSync(join(FIXTURE, 'sources'), { recursive: true });
  mkdirSync(join(FIXTURE, 'reading'), { recursive: true });
  mkdirSync(join(FIXTURE, 'media'), { recursive: true });
  // Generated, not committed: seeding deletes this directory, so anything a
  // take needs must be reproducible from the repo. Both are the same figure —
  // see media.mjs. The video needs ffmpeg, which the recording already does.
  writeCurveSvg(join(FIXTURE, 'media', 'forgetting-curve.svg'));
  await writeCurveVideo(join(FIXTURE, 'media', 'forgetting-curve.mp4'));
  for (const [name, body] of Object.entries(N4L)) {
    writeFileSync(join(FIXTURE, name), body);
  }
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

await seedFixture();
const previous = pointConfigAtFixture();
console.log('fixture   :', FIXTURE);
console.log('config    :', CONFIG);
console.log('previous  :', previous || '(none)');
console.log('\nNow start the app:  make dev-demo');
console.log('Then run        :  npm run study-session');
console.log('Afterwards      :  npm run restore');
