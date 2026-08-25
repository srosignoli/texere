import { chromium } from 'playwright';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// A full study session — document -> extract -> card -> review — driven end to
// end against the real Go backend. See README.md.
//
// The scenario seeds its own throwaway collection. It must never be pointed at
// a real one: it imports documents and materialises cards.

const OUT = process.env.TEXERE_DEMO_OUT ?? mkdtempSync(join(tmpdir(), 'texere-demo-'));
const URL = process.env.TEXERE_DEMO_URL ?? 'http://localhost:34115';
const PHRASE = 'Active recall means retrieving an answer from memory rather than rereading it';

// ---- fixture collection ----------------------------------------------------
const FX = mkdtempSync(join(tmpdir(), 'texere-fixture-'));
mkdirSync(join(FX, 'sources'), { recursive: true });
mkdirSync(join(FX, 'reading'), { recursive: true });
writeFileSync(join(FX, 'memory.n4l'), `- Memory research

 :: _sequence_ ::

encoding (leads to) storage
storage (leads to) retrieval
`);
writeFileSync(join(FX, 'study-doc.md'), `# The Forgetting Curve

Hermann Ebbinghaus discovered that memory retention declines sharply in the
hours after learning, then levels off. He called this the forgetting curve.

Spaced repetition counters the curve by scheduling reviews just before the
point of forgetting, which strengthens the memory trace each time.

${PHRASE},
and it produces markedly stronger retention than passive review.
`);
console.log('fixture collection:', FX);
console.log('output:', OUT);

const b = await chromium.launch({
  executablePath:'/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome',
  args:['--no-sandbox','--disable-dev-shm-usage'] });
const ctx = await b.newContext({ viewport:{width:1440,height:900},
  recordVideo:{ dir:`${OUT}/study`, size:{width:1440,height:900} } });
const p = await ctx.newPage();
p.on('pageerror', e => console.log('  PAGEERROR:', String(e).slice(0,140)));

// cursor + caption overlays
await p.addInitScript(() => {
  addEventListener('DOMContentLoaded', () => {
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
const click = async (loc) => {
  const el=loc.first(); await el.scrollIntoViewIfNeeded().catch(()=>{});
  const bx=await el.boundingBox(); if(!bx) throw new Error('no box for '+loc);
  await p.mouse.move(bx.x+bx.width/2, bx.y+bx.height/2, {steps:28});
  await p.waitForTimeout(280);
  await p.mouse.down(); await p.waitForTimeout(80); await p.mouse.up();
};

await p.goto(URL,{waitUntil:'domcontentloaded'});
await p.waitForFunction(()=>window.go?.main?.App,null,{timeout:25000});
await p.waitForTimeout(1500);

await say('A study session, start to finish — every action scripted.',2800); await clear();

// ---- BEAT 1: open the fixture collection + import the document -------------
await say('Opening a document. The file dialog is bypassed by the demo seam.',2400);
await call('OpenCollection', FX);
await p.waitForTimeout(1800);
const rv = await call('DemoImportTextSource', join(FX, 'study-doc.md'));
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
if (!process.env.TEXERE_KEEP_FIXTURE) rmSync(FX, { recursive: true, force: true });
