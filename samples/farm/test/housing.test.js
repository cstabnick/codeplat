// Tests for web/js/housing.js. Worlds come from state.js, species from species.js and drawing goes to a fakeCtx; housing.js is the only thing under test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld } from '../web/js/state.js';
import { SPECIES, KINDS } from '../web/js/species.js';
import { PALETTE } from '../web/js/pixel.js';
import { drawBuilding, housingDrawables } from '../web/js/housing.js';
import { fakeCtx } from './fakectx.js';

// The fillRect calls recorded on a fakeCtx, as [x, y, w, h].
const rects = (ctx) => ctx.calls.filter((c) => c.op === 'fillRect').map((c) => c.args);

// The fillStyle in effect at each fillRect call, as [fillStyle, x, y, w, h].
function paints(ctx) {
  const out = [];
  let style = undefined;
  for (const c of ctx.calls) {
    if (c.op === 'set' && c.name === 'fillStyle') style = c.value;
    if (c.op === 'fillRect') out.push([style, ...c.args]);
  }
  return out;
}

// pixelsOf expands each fillRect into the individual pixels it paints, as 'x,y' -> fillStyle.
function pixelsOf(ctx) {
  const px = new Map();
  for (const [style, x, y, w, h] of paints(ctx)) {
    for (let dx = 0; dx < w; dx++) {
      for (let dy = 0; dy < h; dy++) px.set(`${x + dx},${y + dy}`, style);
    }
  }
  return px;
}

test('every tier of every species draws inside its box and above row 0', () => {
  for (const kind of KINDS) {
    SPECIES[kind].tiers.forEach((tier, i) => {
      const { look } = tier;
      const label = `${kind} tier ${i} (${tier.name}, ${look.shape})`;
      const ctx = fakeCtx();
      const x = 100;
      const baseY = 90;
      drawBuilding(ctx, look, x, baseY, 0);
      const rs = rects(ctx);
      if (look.shape === 'none') {
        assert.equal(rs.length, 0, `${label} should draw nothing`);
        return;
      }
      assert.ok(rs.length > 0, `${label} drew nothing`);
      const top = Math.max(0, baseY - look.h);
      for (const [rx, ry, rw, rh] of rs) {
        assert.ok(Number.isInteger(rx) && Number.isInteger(ry) && Number.isInteger(rw) && Number.isInteger(rh),
          `${label} has a non-integer fillRect ${JSON.stringify([rx, ry, rw, rh])}`);
        assert.ok(ry >= 0, `${label} paints above row 0: ${JSON.stringify([rx, ry, rw, rh])}`);
        assert.ok(rx >= x && rx + rw <= x + look.w,
          `${label} paints outside its width: ${JSON.stringify([rx, ry, rw, rh])}`);
        assert.ok(ry >= top && ry + rh <= baseY,
          `${label} paints outside its height: ${JSON.stringify([rx, ry, rw, rh])}`);
        assert.ok(rw > 0 && rh > 0, `${label} has an empty fillRect ${JSON.stringify([rx, ry, rw, rh])}`);
      }
    });
  }
});

test('the Burj Moolifa paints cow spots and its wall, and its top pixel is at y >= 0 when baseY is 84', () => {
  const look = SPECIES.cow.tiers.at(-1).look;
  assert.equal(look.shape, 'burj');
  const ctx = fakeCtx();
  drawBuilding(ctx, look, 0, 84, 0);
  const px = pixelsOf(ctx);
  const colours = new Set(px.values());
  assert.ok(colours.has(PALETTE['0']), 'no black (0) cow spots were painted');
  assert.ok(colours.has(PALETTE['7']), 'no white (7) wall was painted');

  let minY = Infinity;
  for (const key of px.keys()) minY = Math.min(minY, Number(key.split(',')[1]));
  assert.ok(minY >= 0, `topmost pixel is at y ${minY}, above row 0`);
});

test('housingDrawables is empty for a fresh world, whose two tier 0 looks are none', () => {
  assert.deepEqual(housingDrawables(createWorld(1)), []);
});

test('housingDrawables has one item at y 84 when the cow herd reaches tier 2', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 2;
  const items = housingDrawables(w);
  assert.equal(items.length, 1);
  assert.equal(items[0].y, 84);
});

test('housingDrawables includes the sheep at y 92 once the second parcel is owned', () => {
  const w = createWorld(1);
  w.land = 2;
  const items = housingDrawables(w);
  assert.ok(items.some((it) => it.y === 92), `no item at y 92: ${JSON.stringify(items.map((it) => it.y))}`);
});

test('each housingDrawables item draws the building through drawBuilding', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 2;
  w.land = 2;
  for (const item of housingDrawables(w)) {
    const ctx = fakeCtx();
    item.draw(ctx);
    assert.ok(ctx.count('fillRect') > 0, `item at y ${item.y} drew nothing`);
  }
});

test('drawing the same building twice records identical calls', () => {
  const look = SPECIES.cow.tiers.at(-1).look;
  const a = fakeCtx();
  const b = fakeCtx();
  drawBuilding(a, look, 230, 84, 3.5);
  drawBuilding(b, look, 230, 84, 3.5);
  assert.deepEqual(a.calls, b.calls);
});
