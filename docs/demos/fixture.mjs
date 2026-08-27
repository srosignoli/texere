// Shared constants for the demo harness. Deliberately side-effect free: both
// seed.mjs and study-session.mjs import this, and importing the seeder itself
// would re-seed the fixture and rewrite the app config as a side effect.
import { tmpdir, homedir } from 'node:os';
import { join } from 'node:path';

export const FIXTURE = process.env.TEXERE_DEMO_FIXTURE ?? join(tmpdir(), 'texere-demo-fixture');
export const CONFIG = process.env.TEXERE_CONFIG ?? join(homedir(), '.config', 'texere', 'config.json');
export const SAVED = join(FIXTURE, '.previous-collection-dir');
export const PHRASE =
  'Active recall means retrieving an answer from memory rather than rereading it';
