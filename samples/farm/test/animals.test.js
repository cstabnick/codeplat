// Tests for web/js/animals.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  createWorld,
  advanceTime,
  setHour,
  PASTURE,
  YARD,
  MUD,
} from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { updateAnimals, animalDrawables, animalAt } from '../web/js/animals.js';
import { fakeCtx } from './fakectx.js';

const line = (kind) => 'LINE ' + kind;
const DT = 0.1;

// step advances time and the animals by one 0.1 s update.
function step(w, rng) {
  advanceTime(w, DT);
  updateAnimals(w, DT, rng, line);
}

const byId = (w, id) => w.animals.find((a) => a.id === id);
const cows = (w) => w.animals.filter((a) => a.kind === 'cow');

// bubblesAdded returns the bubbles in w.bubbles that are not in seen, and adds them to seen.
function newBubbles(w, seen) {
  const fresh = w.bubbles.filter((b) => !seen.has(b));
  for (const b of fresh) seen.add(b);
  return fresh;
}

test('gossip: cows whisper while hidden, then go innocent when Gary looks', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.farmer.hidden = true;
  byId(w, 'cow-1').x = 200;
  byId(w, 'cow-1').y = 122;
  byId(w, 'cow-2').x = 220;
  byId(w, 'cow-2').y = 122;

  const seen = new Set();
  let sawWhisper = false;
  let gossiping = null;
  for (let i = 0; i < 200 && !gossiping; i++) {
    step(w, rng);
    for (const b of newBubbles(w, seen)) {
      if (b.kind === 'whisper') sawWhisper = true;
    }
    const pair = cows(w).filter((c) => c.state === 'gossip');
    if (pair.length > 0) gossiping = pair;
  }

  assert.ok(gossiping, 'a cow starts gossiping within 20 s');
  assert.equal(gossiping.length, 2, 'both cows of the pair are gossiping');
  assert.ok(w.stats.gossip >= 1, 'stats.gossip counts the gossip');
  assert.ok(sawWhisper, 'a whisper bubble was shown');

  w.farmer.hidden = false;
  w.farmer.x = 170;
  w.farmer.y = 130;
  w.farmer.facing = 'right';
  w.farmer.state = 'idle';
  step(w, rng);

  for (const c of gossiping) {
    assert.equal(c.state, 'innocent', `${c.id} is innocent once Gary looks`);
  }
});

test('smoke: exactly one smoke line per cigarette, and every cow notices', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.farmer.state = 'smoke';
  w.stats.cigarettes = 1;
  // Hold the cows still in 'innocent' so no gossip starts: a gossip bubble at the same spot
  // would replace the smoke bubble in say(), and the test counts the smoke lines as bubbles.
  for (const c of cows(w)) {
    c.state = 'innocent';
    c.timer = 1000;
  }

  const seen = new Set();
  let smokeLines = 0;
  const countSmoke = () => {
    for (const b of newBubbles(w, seen)) {
      if (b.text === 'LINE smoke') smokeLines += 1;
    }
  };

  step(w, rng);
  countSmoke();
  assert.equal(smokeLines, 1, 'one smoke line on the first update');
  for (const c of cows(w)) {
    assert.equal(c.noticed, 1, `${c.id} noticed the cigarette`);
  }

  for (let i = 0; i < 20; i++) {
    step(w, rng);
    countSmoke();
  }
  assert.equal(smokeLines, 1, 'no further smoke lines for the same cigarette');
});

test('bounds: over 300 s every animal stays inside its area', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const pigBox = {
    x0: MUD.x - 12,
    x1: MUD.x + MUD.w + 12,
    y0: MUD.y - 12,
    y1: MUD.y + MUD.h + 12,
  };

  for (let i = 0; i < 3000; i++) {
    step(w, rng);
    for (const a of w.animals) {
      if (a.kind === 'cow') {
        assert.ok(a.x >= PASTURE.x && a.x <= PASTURE.x + PASTURE.w, `${a.id} x ${a.x} in pasture at ${i}`);
        assert.ok(a.y >= PASTURE.y && a.y <= PASTURE.y + PASTURE.h, `${a.id} y ${a.y} in pasture at ${i}`);
      } else if (a.kind === 'chicken') {
        assert.ok(a.x >= YARD.x && a.x <= YARD.x + YARD.w, `${a.id} x ${a.x} in yard at ${i}`);
        assert.ok(a.y >= YARD.y && a.y <= YARD.y + YARD.h, `${a.id} y ${a.y} in yard at ${i}`);
      } else {
        assert.ok(a.x >= pigBox.x0 && a.x <= pigBox.x1, `${a.id} x ${a.x} in mud at ${i}`);
        assert.ok(a.y >= pigBox.y0 && a.y <= pigBox.y1, `${a.id} y ${a.y} in mud at ${i}`);
      }
    }
  }
});

test('abduction: an abducted animal does not move or change state', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-3');
  cow.abducted = true;
  cow.state = 'abducted';
  cow.lift = 30;
  const { x, y } = cow;

  for (let i = 0; i < 100; i++) step(w, rng);

  assert.equal(cow.x, x);
  assert.equal(cow.y, y);
  assert.equal(cow.state, 'abducted');
  assert.equal(cow.abducted, true);
});

test('clicks: animalAt finds a cow near its feet and nothing far away', () => {
  const w = createWorld(1);
  const cow = byId(w, 'cow-1');

  assert.equal(animalAt(w, cow.x + 5, cow.y - cow.lift), cow);
  assert.equal(animalAt(w, 300, 40), null);
});

test('drawing: every drawable draws without throwing, day and night', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  byId(w, 'cow-2').lift = 10;

  for (const hour of [12, 23]) {
    setHour(w, hour);
    for (let i = 0; i < 30; i++) step(w, rng);
    for (const d of animalDrawables(w)) {
      const ctx = fakeCtx();
      assert.doesNotThrow(() => d.draw(ctx));
      assert.ok(ctx.count('fillRect') > 0, 'the drawable paints something');
    }
  }
});
