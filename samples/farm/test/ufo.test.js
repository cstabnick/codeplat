// Tests for web/js/ufo.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import {
  updateUfo, summonUfo, hireUfo, freeCows, ufoDrawables, CONTRACT,
} from '../web/js/ufo.js';
import { fakeCtx } from './fakectx.js';

const line = (kind) => 'LINE ' + kind;
const DT = 0.1;

// step advances the UFOs by one 0.1 s update.
const step = (w, rng) => updateUfo(w, DT, rng, line);

test('by day no saucer ever appears', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 7);
  for (let i = 0; i < 1000; i++) {
    step(w, rng);
    assert.equal(w.ufos.length, 0, `a saucer appeared at update ${i}`);
  }
});

test('a night visit beams a cow up, returns it and leaves', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonUfo(w, rng), true);
  assert.equal(w.ufos.length, 1);
  const cow = w.animals.find((a) => a.id === w.ufos[0].targetId);
  assert.ok(cow, 'the visit targets a cow');
  assert.equal(w.ufos[0].rate, 0, 'a night visit is free');

  const states = new Set();
  let sawLift = false;
  let ended = false;
  for (let i = 0; i < 250; i++) {
    step(w, rng);
    if (w.ufos.length === 0) {
      ended = true;
      break;
    }
    const s = w.ufos[0].state;
    states.add(s);
    if (['beam', 'hold', 'return'].includes(s)) {
      assert.equal(cow.abducted, true, `cow not abducted in ${s}`);
      assert.equal(cow.state, 'abducted');
    }
    if (cow.lift > 0) sawLift = true;
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

  const speech = w.bubbles.filter((b) => b.kind === 'speech');
  assert.equal(speech.length, 1, 'one speech bubble');
  assert.equal(speech[0].text, 'LINE ufo');
  assert.equal(w.log[0].text, `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`);
});

test('summonUfo returns true when it starts a visit and false while one is under way', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonUfo(w, rng), true);
  assert.equal(w.ufos.length, 1);
  assert.equal(w.ufos[0].state, 'arrive');
  assert.equal(summonUfo(w, rng), false);
  assert.equal(w.ufos.length, 1, 'a refused summon changes nothing');
  assert.equal(w.ufos[0].state, 'arrive', 'a refused summon changes nothing');
});

test('a second visit does not start the same night by itself', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  // Positive control: an unattended night does summon a visit.
  let summoned = false;
  for (let i = 0; i < 5000 && !summoned; i++) {
    step(w, rng);
    summoned = w.ufos.length > 0;
  }
  assert.ok(summoned, 'updates at night eventually summon a visit');

  // Let that visit finish.
  for (let i = 0; i < 300 && w.ufos.length > 0; i++) step(w, rng);
  assert.equal(w.ufos.length, 0);
  assert.equal(w.stats.abductions, 1);
  assert.equal(w.ufoLastDay, w.day);

  // The rest of the night, at the same odds, produces no further visit.
  for (let i = 0; i < 2000; i++) {
    step(w, rng);
    assert.equal(w.ufos.length, 0, `second visit started at update ${i}`);
  }
  assert.equal(w.stats.abductions, 1);
});

test('ufoDrawables is empty with no saucers and draws the saucer and beam without throwing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.deepEqual(ufoDrawables(w), []);

  summonUfo(w, rng);
  let items = ufoDrawables(w);
  assert.equal(items.length, 1, 'one item while arriving');
  assert.equal(items[0].y, Infinity);

  for (let i = 0; i < 200 && w.ufos[0].state !== 'beam'; i++) step(w, rng);
  assert.equal(w.ufos[0].state, 'beam');

  items = ufoDrawables(w);
  assert.equal(items.length, 2, 'saucer and beam while beaming');
  const cow = w.animals.find((a) => a.id === w.ufos[0].targetId);
  assert.equal(items[0].y, Infinity, 'the saucer sorts last, in front of the ground');
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

test('a hired visit works by day, holds the cow for about CONTRACT.stay and returns her with the internship line', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.equal(hireUfo(w, rng), true);
  assert.equal(w.ufos.length, 1);
  const u = w.ufos[0];
  assert.equal(u.rate, CONTRACT.rate);
  assert.equal(u.stay, CONTRACT.stay);
  assert.equal(w.ufoLastDay, 0, 'a hire leaves ufoLastDay alone');
  const cow = w.animals.find((a) => a.id === u.targetId);
  assert.ok(cow, 'the visit targets a cow');

  let holdSteps = 0;
  let ended = false;
  for (let i = 0; i < 1000; i++) {
    step(w, rng);
    if (w.ufos.length === 0) {
      ended = true;
      break;
    }
    const s = w.ufos[0].state;
    if (['beam', 'hold', 'return'].includes(s)) {
      assert.equal(cow.abducted, true, `cow not abducted in ${s}`);
    }
    if (s === 'hold') holdSteps++;
  }

  assert.ok(ended, 'the hired visit ends within 100 s');
  assert.ok(holdSteps > 0, 'the cow was held');
  assert.ok(
    Math.abs(holdSteps * DT - CONTRACT.stay) <= 0.2,
    `held for ${holdSteps * DT} s, expected about ${CONTRACT.stay}`,
  );
  assert.equal(cow.abducted, false);
  assert.equal(cow.state, 'graze');
  assert.equal(w.stats.abductions, 1);
  assert.equal(w.coins, 10, 'the UFO module never touches coins');
  assert.equal(w.log[0].text, `${cow.name.toUpperCase()} IS BACK FROM HER ALIEN INTERNSHIP`);
});

test('two hired saucers at once take two different cows', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.equal(hireUfo(w, rng), true);
  assert.equal(hireUfo(w, rng), true);
  assert.equal(w.ufos.length, 2);
  assert.notEqual(w.ufos[0].targetId, w.ufos[1].targetId);
  assert.equal(freeCows(w).length, 2, 'two adult cows are still free');

  for (let i = 0; i < 100; i++) step(w, rng);
  assert.equal(w.ufos.length, 2, 'both saucers are still in the sky');
  assert.notEqual(w.ufos[0].targetId, w.ufos[1].targetId);
});

test('hireUfo refuses once every adult cow is taken', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  // createWorld(1) has four adult cows.
  for (let i = 0; i < 4; i++) {
    assert.equal(hireUfo(w, rng), true, `hire ${i} succeeds`);
  }
  assert.equal(w.ufos.length, 4);
  assert.equal(hireUfo(w, rng), false);
  assert.equal(w.ufos.length, 4, 'a refused hire changes nothing');
});

test('freeCows leaves out a calf', () => {
  const w = createWorld(1);
  const calf = w.animals.find((a) => a.id === 'cow-1');
  calf.age = 0;

  const free = freeCows(w);
  assert.equal(free.length, 3);
  assert.ok(!free.includes(calf), 'the calf is not free');
});

test('freeCows leaves out a cow a saucer is already taking', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.equal(hireUfo(w, rng), true);
  const taken = w.animals.find((a) => a.id === w.ufos[0].targetId);
  const free = freeCows(w);
  assert.equal(free.length, 3);
  assert.ok(!free.includes(taken), 'the targeted cow is not free');
});

test('a hired saucer whose cow vanishes while arriving retargets to another free cow', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.equal(hireUfo(w, rng), true);
  const u = w.ufos[0];
  const gone = u.targetId;
  w.animals = w.animals.filter((a) => a.id !== gone);

  step(w, rng);
  assert.equal(w.ufos.length, 1, 'the saucer stays in the sky');
  assert.equal(u.state, 'arrive');
  assert.notEqual(u.targetId, gone, 'the saucer has a new target');
  const cow = w.animals.find((a) => a.id === u.targetId);
  assert.ok(cow, 'the new target is in the world');
  assert.equal(cow.kind, 'cow');
});
