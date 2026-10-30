// Tests for web/js/flock.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. flock.js imports only state.js and pixel.js, so no stub is needed.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, advanceTime, MEADOW, HILL } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { updateFlock, flockDrawables, FLOCK_MAX } from '../web/js/flock.js';
import { fakeCtx } from './fakectx.js';

const DT = 0.1;

// inside reports whether the anchor of e lies in area, edges included.
function inside(e, area) {
  return e.x >= area.x && e.x <= area.x + area.w && e.y >= area.y && e.y <= area.y + area.h;
}

// countOf counts the entries of a kind that match the predicate.
function countOf(w, kind, pred = () => true) {
  return w.flock.filter((e) => e.kind === kind && pred(e)).length;
}

// stocked returns a world at land 3 with sheep and goats (20 females, 5 males, 5 young each).
function stocked() {
  const w = createWorld(1);
  w.land = 3;
  for (const kind of ['sheep', 'goat']) {
    Object.assign(w.herds[kind], { females: 20, males: 5, young: 5 });
  }
  return w;
}

test('a fresh world (land 1) has no flock after an update', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  updateFlock(w, DT, rng);
  assert.equal(w.flock.length, 0);
});

test('a stocked world shows FLOCK_MAX sheep and goats with the group counts', () => {
  const w = stocked();
  const rng = makeRng(2);
  updateFlock(w, DT, rng);

  assert.equal(FLOCK_MAX, 10);
  assert.equal(countOf(w, 'sheep'), 10, 'ten sheep for a herd of 30');
  assert.equal(countOf(w, 'goat'), 10, 'ten goats for a herd of 30');

  // Herd 20 females, 5 males, 5 young: 2 young, 2 rams, 6 ewes.
  assert.equal(countOf(w, 'sheep', (e) => e.young), 2, 'two lambs');
  assert.equal(countOf(w, 'sheep', (e) => !e.young && e.sex === 'male'), 2, 'two rams');
  assert.equal(countOf(w, 'sheep', (e) => !e.young && e.sex === 'female'), 6, 'six ewes');

  // The same split for goats: 2 kids, 2 billies, 6 nannies.
  assert.equal(countOf(w, 'goat', (e) => e.young), 2, 'two kids');
  assert.equal(countOf(w, 'goat', (e) => !e.young && e.sex === 'male'), 2, 'two billies');
  assert.equal(countOf(w, 'goat', (e) => !e.young && e.sex === 'female'), 6, 'six nannies');
});

test('entries have the documented shape and start grazing in their area', () => {
  const w = stocked();
  updateFlock(w, DT, makeRng(3));
  for (const e of w.flock) {
    assert.ok(e.kind === 'sheep' || e.kind === 'goat');
    assert.ok(e.sex === 'female' || e.sex === 'male');
    assert.equal(typeof e.young, 'boolean');
    if (e.young) assert.equal(e.sex, 'female', 'a young one is given sex female');
    assert.ok(e.facing === 'left' || e.facing === 'right');
    assert.equal(e.state, 'graze', 'a new entry starts grazing');
    assert.equal(e.target, null);
  }
  for (const e of w.flock.filter((x) => x.kind === 'sheep')) {
    assert.ok(inside(e, MEADOW), 'a sheep spawns inside the Sheep Meadow');
  }
  for (const e of w.flock.filter((x) => x.kind === 'goat')) {
    assert.ok(inside(e, HILL), 'a goat spawns inside Goat Hill');
  }
});

test('lowering a herd removes entries, and so does losing the parcel', () => {
  const w = stocked();
  const rng = makeRng(4);
  updateFlock(w, DT, rng);
  assert.equal(countOf(w, 'sheep'), 10);

  // Total 4: target 4, so 1 lamb, 1 ram and 2 ewes.
  Object.assign(w.herds.sheep, { females: 2, males: 1, young: 1 });
  updateFlock(w, DT, rng);
  assert.equal(countOf(w, 'sheep'), 4, 'the sheep shown fall to the herd size');
  assert.equal(countOf(w, 'sheep', (e) => e.young), 1);
  assert.equal(countOf(w, 'sheep', (e) => !e.young && e.sex === 'male'), 1);
  assert.equal(countOf(w, 'sheep', (e) => !e.young && e.sex === 'female'), 2);
  assert.equal(countOf(w, 'goat'), 10, 'the goats are unchanged');

  // Herd of zero: no sheep at all.
  Object.assign(w.herds.sheep, { females: 0, males: 0, young: 0 });
  updateFlock(w, DT, rng);
  assert.equal(countOf(w, 'sheep'), 0, 'an empty herd shows no sheep');

  // Losing the Goat Hill parcel removes the goats.
  w.land = 2;
  updateFlock(w, DT, rng);
  assert.equal(countOf(w, 'goat'), 0, 'goats go when their parcel is not owned');
});

test('over 300 s of updates every entry stays inside its area', () => {
  const w = stocked();
  const rng = makeRng(5);
  let sawWalk = false;
  for (let i = 0; i < 3000; i++) {
    advanceTime(w, DT);
    updateFlock(w, DT, rng);
    for (const e of w.flock) {
      if (e.state === 'walk') sawWalk = true;
      const area = e.kind === 'sheep' ? MEADOW : HILL;
      assert.ok(inside(e, area), `${e.kind} at (${e.x}, ${e.y}) at t=${w.time.toFixed(1)} is inside its area`);
    }
  }
  assert.equal(w.flock.length, 20, 'the flock keeps its size');
  assert.ok(sawWalk, 'some entry walked during the run');
});

test('every sprite draws at least one fillRect', () => {
  // One entry per sprite: sheep ewe, ram, lamb; goat nanny, billy, kid. Each is drawn grazing,
  // walking on frame 0 and walking on frame 1 (the stance row), facing both ways.
  const w = createWorld(1);
  const base = { x: 100, y: 120, timer: 3, target: null, facing: 'right' };
  const kinds = [
    { kind: 'sheep', sex: 'female', young: false },
    { kind: 'sheep', sex: 'male', young: false },
    { kind: 'sheep', sex: 'female', young: true },
    { kind: 'goat', sex: 'female', young: false },
    { kind: 'goat', sex: 'male', young: false },
    { kind: 'goat', sex: 'female', young: true },
  ];
  const states = [
    { state: 'graze', frame: 0 },
    { state: 'walk', frame: 0 },
    { state: 'walk', frame: 1 },
  ];
  for (const k of kinds) {
    for (const s of states) {
      for (const facing of ['left', 'right']) {
        w.flock.push({ ...base, ...k, ...s, facing });
      }
    }
  }

  const drawables = flockDrawables(w);
  assert.equal(drawables.length, w.flock.length, 'one drawable per entry');
  for (let i = 0; i < w.flock.length; i++) {
    const ctx = fakeCtx();
    assert.equal(drawables[i].y, w.flock[i].y);
    assert.doesNotThrow(() => drawables[i].draw(ctx));
    assert.ok(ctx.count('fillRect') >= 1, `entry ${i} (${w.flock[i].kind}, young ${w.flock[i].young}) draws a fillRect`);
  }
});
