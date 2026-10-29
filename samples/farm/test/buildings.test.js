import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, HORIZON, WIDTH, HEIGHT, HOUSE, BARN, COOP, PASTURE } from '../web/js/state.js';
import { drawGround, sceneDrawables } from '../web/js/buildings.js';
import { fakeCtx } from './fakectx.js';

// The fillRect calls recorded on a fakeCtx, as [x, y, w, h].
const rects = (ctx) => ctx.calls.filter((c) => c.op === 'fillRect').map((c) => c.args);

const inCanvas = ([x, y, w, h]) => x >= 0 && y >= 0 && x + w <= WIDTH && y + h <= HEIGHT;

test('drawGround draws only at y >= HORIZON', () => {
  const ctx = fakeCtx();
  drawGround(ctx, createWorld(1));
  const rs = rects(ctx);
  assert.ok(rs.length > 0, 'drawGround drew nothing');
  for (const r of rs) {
    assert.ok(r[1] >= HORIZON, `fillRect at y ${r[1]} is above HORIZON ${HORIZON}: ${JSON.stringify(r)}`);
  }
});

test('drawGround draws nothing outside the canvas', () => {
  const ctx = fakeCtx();
  drawGround(ctx, createWorld(1));
  for (const r of rects(ctx)) {
    assert.ok(inCanvas(r), `fillRect outside the canvas: ${JSON.stringify(r)}`);
  }
});

test('drawGround fills the whole ground with grass first', () => {
  const ctx = fakeCtx();
  drawGround(ctx, createWorld(1));
  const first = ctx.calls.find((c) => c.op === 'fillRect');
  assert.deepEqual(first.args, [0, HORIZON, WIDTH, HEIGHT - HORIZON]);
});

test('drawGround gives the same calls for the same world on every call', () => {
  const world = createWorld(7);
  const a = fakeCtx();
  const b = fakeCtx();
  drawGround(a, world);
  drawGround(b, world);
  assert.equal(JSON.stringify(a.calls), JSON.stringify(b.calls));
});

test('sceneDrawables returns at least 9 items, each with a numeric y and a draw function', () => {
  const items = sceneDrawables(createWorld(1));
  assert.ok(Array.isArray(items));
  assert.ok(items.length >= 9, `got ${items.length} items`);
  for (const item of items) {
    assert.equal(typeof item.y, 'number', `y is not a number: ${item.y}`);
    assert.ok(Number.isFinite(item.y), `y is not finite: ${item.y}`);
    assert.equal(typeof item.draw, 'function');
  }
});

test('every scenery item draws into a fakeCtx without throwing, calling fillRect, inside the canvas', () => {
  const items = sceneDrawables(createWorld(1));
  for (const [i, item] of items.entries()) {
    const ctx = fakeCtx();
    assert.doesNotThrow(() => item.draw(ctx), `item ${i} threw`);
    assert.ok(ctx.count('fillRect') > 0, `item ${i} drew no fillRect`);
    for (const r of rects(ctx)) {
      assert.ok(inCanvas(r), `item ${i} fillRect outside the canvas: ${JSON.stringify(r)}`);
    }
  }
});

test('the house item sits at HOUSE.y + HOUSE.h', () => {
  const items = sceneDrawables(createWorld(1));
  assert.ok(items.some((it) => it.y === HOUSE.y + HOUSE.h), 'no item at the house base line');
  // The house is the item whose draw paints the house's cream walls, so check its base line directly.
  const house = items.find((it) => it.y === HOUSE.y + HOUSE.h);
  assert.equal(house.y, 78);
});

test('the barn item sits at BARN.y + BARN.h', () => {
  const items = sceneDrawables(createWorld(1));
  assert.ok(items.some((it) => it.y === BARN.y + BARN.h), 'no item at the barn base line');
  assert.equal(BARN.y + BARN.h, 84);
});

test('the coop item sits at COOP.y + COOP.h', () => {
  const items = sceneDrawables(createWorld(1));
  assert.ok(items.some((it) => it.y === COOP.y + COOP.h), 'no item at the coop base line');
});

test('the pasture fence front and sides sit at PASTURE.y + PASTURE.h', () => {
  const items = sceneDrawables(createWorld(1));
  const base = PASTURE.y + PASTURE.h;
  assert.ok(items.filter((it) => it.y === base).length >= 2, 'no pasture fence items at the base line');
});

test('the scarecrow item sits at y 140 and the mailbox at y 90', () => {
  const items = sceneDrawables(createWorld(1));
  assert.ok(items.some((it) => it.y === 140), 'no scarecrow item at y 140');
  assert.ok(items.some((it) => it.y === 90), 'no mailbox item at y 90');
});

test('the three trees sit at their feet y values 66, 62 and 70', () => {
  const ys = sceneDrawables(createWorld(1)).map((it) => it.y);
  for (const y of [66, 62, 70]) {
    assert.ok(ys.includes(y), `no tree item at y ${y}`);
  }
});

test('sceneDrawables is deterministic: the same world draws the same calls', () => {
  const world = createWorld(3);
  const run = () => {
    const ctx = fakeCtx();
    for (const item of sceneDrawables(world)) item.draw(ctx);
    return JSON.stringify(ctx.calls);
  };
  assert.equal(run(), run());
});
