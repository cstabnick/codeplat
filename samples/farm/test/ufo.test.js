// Tests for web/js/ufo.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { updateUfo, summonUfo, ufoDrawables } from '../web/js/ufo.js';
import { fakeCtx } from './fakectx.js';

const line = (kind) => 'LINE ' + kind;
const DT = 0.1;

// step advances the UFO by one 0.1 s update.
const step = (w, rng) => updateUfo(w, DT, rng, line);

test('by day the UFO never leaves away', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 7);
  for (let i = 0; i < 1000; i++) {
    step(w, rng);
    assert.equal(w.ufo.state, 'away', `left away at update ${i}`);
  }
});

test('a night visit beams a cow up, returns it and leaves', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonUfo(w, rng), true);
  const cow = w.animals.find((a) => a.id === w.ufo.targetId);
  assert.ok(cow, 'the visit targets a cow');

  const states = new Set();
  let sawLift = false;
  let ended = false;
  for (let i = 0; i < 250; i++) {
    step(w, rng);
    states.add(w.ufo.state);
    if (['beam', 'hold', 'return'].includes(w.ufo.state)) {
      assert.equal(cow.abducted, true, `cow not abducted in ${w.ufo.state}`);
      assert.equal(cow.state, 'abducted');
    }
    if (cow.lift > 0) sawLift = true;
    if (w.ufo.state === 'away') {
      ended = true;
      break;
    }
  }

  assert.ok(ended, 'the visit ends within 25 s');
  for (const s of ['arrive', 'beam', 'hold', 'return', 'leave']) {
    assert.ok(states.has(s), `visit passed through ${s}`);
  }
  assert.ok(sawLift, 'the cow was lifted');
  assert.equal(cow.lift, 0);
  assert.equal(cow.abducted, false);
  assert.equal(cow.state, 'graze');
  assert.equal(w.stats.abductions, 1);
  assert.equal(w.ufo.targetId, null);

  const speech = w.bubbles.filter((b) => b.kind === 'speech');
  assert.equal(speech.length, 1, 'one speech bubble');
  assert.equal(speech[0].text, 'LINE ufo');
  assert.equal(w.log[0].text, `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`);
});

test('summonUfo returns true when it starts a visit and false during one', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonUfo(w, rng), true);
  assert.equal(w.ufo.state, 'arrive');
  assert.equal(summonUfo(w, rng), false);
  assert.equal(w.ufo.state, 'arrive', 'a refused summon changes nothing');
});

test('a second visit does not start the same night by itself', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  // Positive control: an unattended night does summon a visit.
  let summoned = false;
  for (let i = 0; i < 5000 && !summoned; i++) {
    step(w, rng);
    summoned = w.ufo.state !== 'away';
  }
  assert.ok(summoned, 'updates at night eventually summon a visit');

  // Let that visit finish.
  for (let i = 0; i < 300 && w.ufo.state !== 'away'; i++) step(w, rng);
  assert.equal(w.ufo.state, 'away');
  assert.equal(w.stats.abductions, 1);
  assert.equal(w.ufo.lastDay, w.day);

  // The rest of the night, at the same odds, produces no further visit.
  for (let i = 0; i < 2000; i++) {
    step(w, rng);
    assert.equal(w.ufo.state, 'away', `second visit started at update ${i}`);
  }
  assert.equal(w.stats.abductions, 1);
});

test('ufoDrawables is empty when away and draws the saucer and beam without throwing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.deepEqual(ufoDrawables(w), []);

  summonUfo(w, rng);
  let items = ufoDrawables(w);
  assert.equal(items.length, 1, 'one item while arriving');
  assert.equal(items[0].y, 0);

  for (let i = 0; i < 200 && w.ufo.state !== 'beam'; i++) step(w, rng);
  assert.equal(w.ufo.state, 'beam');

  items = ufoDrawables(w);
  assert.equal(items.length, 2, 'saucer and beam while beaming');
  const cow = w.animals.find((a) => a.id === w.ufo.targetId);
  assert.equal(items[0].y, 0, 'the saucer sorts first');
  assert.equal(items[1].y, cow.y - 1, 'the beam sits just behind the cow');

  const ctx = fakeCtx();
  for (const item of items) {
    assert.doesNotThrow(() => item.draw(ctx));
  }
  assert.ok(ctx.count('fillRect') > 0, 'something was drawn');
  assert.ok(
    ctx.calls.some((c) => c.op === 'set' && c.name === 'globalAlpha' && c.value === 0.35),
    'the beam is drawn at alpha 0.35',
  );
  assert.equal(ctx.globalAlpha, 1, 'globalAlpha is back to 1');
});
