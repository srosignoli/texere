import { chromium } from 'playwright';
import { existsSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { FIXTURE, PHRASE } from './fixture.mjs';

// A full study session — document -> extract -> card -> review — driven end to
// end against the real Go backend. See README.md.
//
// Run `npm run seed` and restart the app first: the fixture has to be in the
// config before the app boots, because App.svelte decides whether a collection
// is open from a startup snapshot.

const OUT = process.env.TEXERE_DEMO_OUT ?? mkdtempSync(join(tmpdir(), 'texere-demo-'));
const URL = process.env.TEXERE_DEMO_URL ?? 'http://localhost:34115';

if (!existsSync(join(FIXTURE, 'study-doc.md'))) {
  console.error(`fixture missing at ${FIXTURE}\nRun: npm run seed   (then restart the app)`);
  process.exit(1);
}
console.log('fixture:', FIXTURE);
console.log('output :', OUT);

const b = await chromium.launch({
  executablePath:'/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome',
  args:['--no-sandbox','--disable-dev-shm-usage'] });
const ctx = await b.newContext({ viewport:{width:1440,height:900},
  recordVideo:{ dir:`${OUT}/study`, size:{width:1440,height:900} } });
const p = await ctx.newPage();
p.on('pageerror', e => console.log('  PAGEERROR:', String(e).slice(0,140)));

// cursor + caption overlays
// Painted at document-start, before any app CSS: recordVideo captures from
// context creation, so without this the take opens on the browser's white
// default background.
await p.addInitScript(() => {
  // Guarded, and attempted twice: at document-start neither <head> nor
  // documentElement is guaranteed to exist, and an uncaught throw here aborts
  // the whole init script — taking the listener below with it and silently
  // removing every overlay from the take.
  const paintBackground = () => {
    if (document.getElementById('__bg')) return;
    const style = document.createElement('style');
    style.id = '__bg';
    style.textContent = 'html,body{background:#0d1520!important}';
    const host = document.head || document.documentElement;
    if (host) host.appendChild(style);
  };
  try { paintBackground(); } catch (e) { /* retried on DOMContentLoaded */ }
  addEventListener('DOMContentLoaded', () => {
    paintBackground();
    const card = document.createElement('div');
    card.id = '__title';
    card.style.cssText = 'position:fixed;inset:0;background:#0d1520;z-index:2147483645;'+
      'display:flex;flex-direction:column;align-items:center;justify-content:center;gap:14px;'+
      'font-family:Nunito,system-ui,sans-serif;transition:opacity .7s';
    card.innerHTML = '<div style="font-size:46px;font-weight:800;color:#e8eef8;letter-spacing:-.5px">Texere</div>'+
      '<div style="font-size:20px;color:#7fa8e8;font-weight:600">A study session, start to finish</div>'+
      '<div style="font-size:14px;color:#5a6b85;margin-top:6px">every action below is scripted</div>';
    document.body.appendChild(card);
    const c=document.createElement('div'); c.id='__cur';
    c.style.cssText='position:fixed;width:22px;height:22px;border-radius:50%;background:rgba(255,255,255,.92);box-shadow:0 0 0 3px rgba(60,140,255,.85),0 2px 10px rgba(0,0,0,.5);z-index:2147483647;pointer-events:none;left:-60px;top:-60px;transition:transform .08s';
    document.body.appendChild(c);
    const cap=document.createElement('div'); cap.id='__cap';
    cap.style.cssText='position:fixed;left:50%;transform:translateX(-50%);bottom:34px;font:600 21px/1.45 Nunito,system-ui,sans-serif;color:#fff;background:rgba(12,18,30,.94);padding:13px 26px;border-radius:10px;z-index:2147483646;pointer-events:none;opacity:0;transition:opacity .3s;border:1px solid rgba(90,150,255,.45);max-width:82vw;text-align:center';
    document.body.appendChild(cap);
    addEventListener('mousemove',e=>{c.style.left=(e.clientX-11)+'px';c.style.top=(e.clientY-11)+'px'});
    addEventListener('mousedown',()=>c.style.transform='scale(.65)');
    addEventListener('mouseup',()=>c.style.transform='scale(1)');
  });
});

const say   = async (t,h=2200) => { await p.evaluate(s=>{const c=document.getElementById('__cap');c.textContent=s;c.style.opacity='1'},t); await p.waitForTimeout(h); };
const clear = () => p.evaluate(()=>{document.getElementById('__cap').style.opacity='0'});
const call  = (fn,...a) => p.evaluate(([f,args])=>window.go.main.App[f](...args), [fn,a]);
// Playwright's {steps} interpolates linearly, which reads as machine motion.
// Ease in and out, and remember where the pointer is so travel starts there.
let cursor = { x: 720, y: 450 };
const easeInOut = (t) => (t < 0.5 ? 4*t*t*t : 1 - Math.pow(-2*t + 2, 3) / 2);
const glide = async (x, y, steps = 26) => {
  const from = { ...cursor };
  for (let i = 1; i <= steps; i++) {
    const t = easeInOut(i / steps);
    await p.mouse.move(from.x + (x - from.x)*t, from.y + (y - from.y)*t);
    await p.waitForTimeout(9);
  }
  cursor = { x, y };
};
const click = async (loc) => {
  const el=loc.first(); await el.scrollIntoViewIfNeeded().catch(()=>{});
  const bx=await el.boundingBox(); if(!bx) throw new Error('no box for '+loc);
  await glide(bx.x+bx.width/2, bx.y+bx.height/2);
  await p.waitForTimeout(260);
  await p.mouse.down(); await p.waitForTimeout(80); await p.mouse.up();
};

await p.goto(URL,{waitUntil:'domcontentloaded'});
await p.waitForFunction(()=>window.go?.main?.App,null,{timeout:25000});

// Fail loudly and specifically rather than timing out on a selector: an app
// that booted without the fixture renders only the "Open collection" button.
const openDir = await p.evaluate(()=>window.go.main.App.GetInitialCollectionDir());
if (openDir !== FIXTURE) {
  console.error(`app is on ${openDir}, not the fixture ${FIXTURE}`);
  console.error('Run: npm run seed, then restart the app (make dev-demo).');
  await b.close();
  process.exit(1);
}
await p.waitForSelector('button[title^="Study"]',{timeout:20000});

// Hold on the card while the app finishes painting, then dissolve into it.
await p.waitForTimeout(2100);
await p.evaluate(() => { const c=document.getElementById('__title'); if(c) c.style.opacity='0'; });
await p.waitForTimeout(900);
await p.evaluate(() => document.getElementById('__title')?.remove());
await p.waitForTimeout(600);

// ---- BEAT 1: import the document ------------------------------------------
await say('Opening a document. The file dialog is bypassed by the demo seam.',2400);
const rv = await call('DemoImportTextSource', join(FIXTURE, 'study-doc.md'));
console.log('imported:', rv.id, rv.refPath);
await p.waitForFunction(()=>window.go.main.App.GetStudyCounts().then(c=>c.read>0),null,{timeout:15000});
await say('Imported and queued for reading.',1900); await clear();

// ---- BEAT 2: into the Study surface, Read tab ------------------------------
await say('Study — the Read · Process · Recall queue.',2000);
await click(p.locator('button[title^="Study"]'));
await p.waitForSelector('[role="dialog"][aria-label="Study surface"]',{timeout:10000});
await p.waitForTimeout(900);
await click(p.locator('.ss-tab[aria-label="Read"]'));
await p.waitForTimeout(1200); await clear();

// ---- BEAT 3: open the document in the reader -------------------------------
await say('Open the document.',1600);
await click(p.locator('.sb-name').first());
await p.waitForSelector('.tsp',{timeout:12000});
await p.waitForFunction(()=>document.querySelector('.tsp')?.innerText?.length>50,null,{timeout:12000});
await p.waitForTimeout(1200); await clear();

// ---- BEAT 4: select a passage and extract it -------------------------------
await say('Select a passage worth keeping.',1800);
const sel = await p.evaluate((phrase) => {
  const el=document.querySelector('.tsp');
  const walker=document.createTreeWalker(el,NodeFilter.SHOW_TEXT);
  let node; while((node=walker.nextNode())){
    const i=node.textContent.indexOf(phrase);
    if(i>=0){
      const r=document.createRange();
      r.setStart(node,i); r.setEnd(node,i+phrase.length);
      const s=window.getSelection(); s.removeAllRanges(); s.addRange(r);
      const b=r.getBoundingClientRect();
      return {ok:true,text:s.toString().slice(0,60),top:b.top};
    }
  }
  return {ok:false, sample: el?.innerText?.slice(0,120)};
}, PHRASE);
console.log('selection:', JSON.stringify(sel).slice(0,180));
await p.waitForTimeout(1600);

await say('Extract it.',1500);
await click(p.locator('button[aria-label="Extract selection"]'));
await p.waitForFunction(()=>window.go.main.App.GetStudyCounts().then(c=>c.process>0),null,{timeout:15000})
  .then(()=>console.log('extract registered: process queue > 0'))
  .catch(()=>console.log('WARN: process queue did not grow'));
await p.waitForTimeout(1200); await clear();

// ---- BEAT 5: process the extract into a card -------------------------------
await say('Process turns the extract into knowledge.',2100);
await click(p.locator('.ss-tab[aria-label="Process"]'));
await p.waitForTimeout(2000); await clear();

const hasMake = await p.locator('button[aria-label="Make card"]').count();
console.log('Make card button present:', hasMake);
if (hasMake) {
  await say('Make card seeds the front and waits for the answer.',2200);
  await click(p.locator('button[aria-label="Make card"]'));
  // makeCard() places the cursor at the end of the seeded "... >> " line so the
  // author types the back. Clicking Create card before typing materialises
  // nothing, because a card with an empty back is not a card.
  await p.waitForTimeout(1600); await clear();

  await say('Type the back of the card.',1500);
  await p.keyboard.type(' it beats passive rereading', { delay: 85 });
  await p.waitForTimeout(1400); await clear();

  await say('Commit it to the deck.',1600);
  await click(p.locator('button[aria-label="Create card"]'));
  await p.waitForFunction(()=>window.go.main.App.GetStudyCounts().then(c=>c.recall>0),null,{timeout:15000})
    .then(()=>console.log('card materialised: recall queue > 0'))
    .catch(()=>console.log('WARN: no card became due'));
  await p.waitForTimeout(1800); await clear();
}

// ---- BEAT 6: recall ---------------------------------------------------------
const counts = await call('GetStudyCounts');
console.log('counts after processing:', JSON.stringify(counts));
await say('Recall — the card comes back on schedule.',2100);
await click(p.locator('.ss-tab[aria-label="Recall"]'));
await p.waitForTimeout(2200);
const reveal = await p.locator('button[aria-label="Show answer"]').count();
console.log('recall card present:', reveal);
if (reveal) {
  await click(p.locator('button[aria-label="Show answer"]'));
  await p.waitForTimeout(1800);
  await say('Grade it, and FSRS schedules the next review.',2100);
  await click(p.locator('button[aria-label="Good"]'));
  await p.waitForTimeout(1800);
}
await clear();

await say('Document → extract → card → review. One script, no hands.',3000);
await clear(); await p.waitForTimeout(800);

await p.screenshot({path:`${OUT}/study-final.png`});
await ctx.close(); await b.close();
console.log('--- study session recorded ---');
console.log('video:', `${OUT}/study`);
// The fixture stays: the app is pointed at it, and deleting it here is what
// used to leave the app booting into a directory that no longer existed.
// `npm run restore` removes it and puts the previous collection back.
