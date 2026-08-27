// Put the collectionDir the demo replaced back, and remove the fixture.
import { readFileSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { FIXTURE, CONFIG, SAVED } from './fixture.mjs';

if (!existsSync(SAVED)) {
  console.error(`nothing to restore: ${SAVED} not found`);
  process.exit(1);
}
const previous = readFileSync(SAVED, 'utf8').trim();
if (!previous) {
  console.error('saved collectionDir was empty; leaving the config alone');
  process.exit(1);
}
if (!existsSync(previous)) {
  // Never repoint the app at a directory that is gone — that is exactly the
  // state this harness used to leave behind, and the app cannot recover from
  // it without the user picking a collection by hand.
  console.error(`saved collectionDir no longer exists: ${previous}`);
  process.exit(1);
}
const cfg = JSON.parse(readFileSync(CONFIG, 'utf8'));
cfg.collectionDir = previous;
writeFileSync(CONFIG, JSON.stringify(cfg, null, 2));
rmSync(FIXTURE, { recursive: true, force: true });
console.log('collectionDir restored ->', previous);
console.log('fixture removed        ->', FIXTURE);
