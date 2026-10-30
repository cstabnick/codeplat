import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, FIELD } from '../web/js/state.js';
import { updateCrops, drawField, cropDrawables } from '../web/js/crops.js';
import { fakeCtx } from './fakectx.js';

// A world with only the plots the test sets up; createWorld's plots are replaced.
function worldWith(plots, time = 0) {
  const w = createWorld(1);
  w.plots = plots;
  w.time = time;
  return w;
}

const plot = (over = {}) => ({
  col: 0, row: 0, crop: 'carrot', growth: 0, stage: 0, water: 1, ...over,
});

const near = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);

test('a watered carrot reaches stage 3 within 41 s and adds one sparkle', () => {
  const p = plot();
  const w = worldWith([p]);
  let t = 0;
  let ripeAt = null;
  for (let i = 0; i < 410; i++) {
    updateCrops(w, 0.1);
    t += 0.1;
    if (p.stage === 3 && ripeAt === null) ripeAt = t;
  }
  assert.notEqual(ripeAt, null, 'plot never reached stage 3');
  assert.ok(ripeAt <= 41 + 1e-9, `ripened at ${ripeAt}, expected within 41 s`);
  assert.equal(p.growth, 1);
  const sparkles = w.effects.filter((e) => e.kind === 'sparkle');
  assert.equal(sparkles.length, 1, 'exactly one sparkle when it ripens');
  const s = sparkles[0];
  assert.equal(s.x, FIELD.x + 0 * 16 + 8);
  assert.equal(s.y, FIELD.y + 0 * 16 + 8);
});

test('a plot already at stage 3 adds no sparkle', () => {
  const p = plot({ growth: 1, stage: 3, water: 1 });
  const w = worldWith([p]);
  updateCrops(w, 0.1);
  assert.equal(w.effects.filter((e) => e.kind === 'sparkle').length, 0);
  assert.equal(p.stage, 3);
});

test('a plot with water 0 does not grow', () => {
  const p = plot({ water: 0, growth: 0.2, stage: 0 });
  const w = worldWith([p]);
  for (let i = 0; i < 100; i++) updateCrops(w, 0.5);
  assert.equal(p.growth, 0.2);
  assert.equal(p.water, 0);
  assert.equal(p.stage, 0);
});

test('water falls by about dt/60 per step and never goes below 0', () => {
  const p = plot({ water: 1, growth: 0 });
  const w = worldWith([p]);
  updateCrops(w, 6);
  near(p.water, 0.9);

  const q = plot({ water: 0.01 });
  const w2 = worldWith([q]);
  updateCrops(w2, 1);
  assert.equal(q.water, 0);
});

test('stage follows growth: 0 to 0.33 is 0, 0.34 is 1, 0.67 is 2, 1 is 3', () => {
  const cases = [[0, 0], [0.32, 0], [0.34, 1], [0.66, 1], [0.67, 2], [0.99, 2], [1, 3]];
  for (const [growth, stage] of cases) {
    const p = plot({ growth: growth, water: 0, stage: 0 });
    const w = worldWith([p]);
    updateCrops(w, 0);
    assert.equal(p.stage, stage, `growth ${growth}`);
  }
});

test('an empty plot is unchanged and adds no effect', () => {
  const p = plot({ crop: null, growth: 0.3, stage: 1, water: 1 });
  const w = worldWith([p]);
  updateCrops(w, 10);
  assert.deepEqual(p, { col: 0, row: 0, crop: null, growth: 0.3, stage: 1, water: 1 });
  assert.equal(w.effects.length, 0);
});

test('drawField draws without throwing, for dry and wet soil and with a border', () => {
  const w = worldWith([
    plot({ col: 0, row: 0, water: 0 }),
    plot({ col: 1, row: 0, water: 1 }),
    plot({ col: 5, row: 2, crop: null, water: 0.3 }),
  ]);
  const ctx = fakeCtx();
  assert.doesNotThrow(() => drawField(ctx, w));
  assert.ok(ctx.count('fillRect') > 0);
  // Every plot draws its 16x16 soil tile at its grid position.
  const tiles = ctx.calls.filter((c) => c.op === 'fillRect' && c.args[2] === 16 && c.args[3] === 16);
  assert.equal(tiles.length, 3);
  assert.deepEqual(tiles[0].args, [FIELD.x, FIELD.y, 16, 16]);
  assert.deepEqual(tiles[1].args, [FIELD.x + 16, FIELD.y, 16, 16]);
  assert.deepEqual(tiles[2].args, [FIELD.x + 5 * 16, FIELD.y + 2 * 16, 16, 16]);
});

test('drawField uses the wet colour for water above 0.3 and the dry colour otherwise', () => {
  const dry = fakeCtx();
  drawField(dry, worldWith([plot({ water: 0.3 })]));
  const wet = fakeCtx();
  drawField(wet, worldWith([plot({ water: 0.31 })]));
  const firstFill = (ctx) => ctx.calls.find((c) => c.op === 'set' && c.name === 'fillStyle').value;
  assert.equal(firstFill(dry), '#ab5236');
  assert.equal(firstFill(wet), '#7e2553');
});

test('drawField draws the furrows at tile-local y 3, 7 and 11 across the full tile', () => {
  const ctx = fakeCtx();
  drawField(ctx, worldWith([plot({ col: 2, row: 1, water: 0 })]));
  const tx = FIELD.x + 2 * 16;
  const ty = FIELD.y + 1 * 16;
  for (const dy of [3, 7, 11]) {
    assert.ok(
      ctx.calls.some((c) => c.op === 'fillRect' && c.args[0] === tx && c.args[1] === ty + dy &&
        c.args[2] === 16 && c.args[3] === 1),
      `missing furrow at y ${dy}`,
    );
  }
});

test('drawField draws the 1-unit border just outside the grid', () => {
  const ctx = fakeCtx();
  drawField(ctx, worldWith([]));
  const left = FIELD.x - 1;
  const top = FIELD.y - 1;
  const width = FIELD.cols * 16 + 2;
  const height = FIELD.rows * 16 + 2;
  const has = (args) => ctx.calls.some((c) => c.op === 'fillRect' &&
    args.every((v, i) => c.args[i] === v));
  assert.ok(has([left, top, width, 1]), 'top edge');
  assert.ok(has([left, top + height - 1, width, 1]), 'bottom edge');
  assert.ok(has([left, top, 1, height]), 'left edge');
  assert.ok(has([left + width - 1, top, 1, height]), 'right edge');
});

test('cropDrawables returns one item per planted plot and none for an empty plot', () => {
  const w = worldWith([
    plot({ col: 0, row: 0, crop: 'carrot', stage: 0 }),
    plot({ col: 1, row: 0, crop: null }),
    plot({ col: 2, row: 0, crop: 'corn', stage: 2, growth: 0.7 }),
    plot({ col: 3, row: 0, crop: 'pumpkin', stage: 3, growth: 1 }),
    plot({ col: 4, row: 0, crop: null }),
  ]);
  const items = cropDrawables(w);
  assert.equal(items.length, 3);
  for (const item of items) {
    assert.equal(typeof item.draw, 'function');
    assert.equal(typeof item.y, 'number');
  }
  assert.equal(items[0].y, FIELD.y + 14);
});

test('every cropDrawables item draws without throwing at every stage and crop', () => {
  const plots = [];
  const crops = ['carrot', 'corn', 'pumpkin'];
  let col = 0;
  for (const crop of crops) {
    for (let stage = 0; stage <= 3; stage++) {
      plots.push(plot({ col: col % 6, row: Math.floor(col / 6), crop, stage, growth: stage / 3 }));
      col++;
    }
  }
  for (const time of [0, 0.25, 0.5, 1.2]) {
    const w = worldWith(plots, time);
    for (const item of cropDrawables(w)) {
      const ctx = fakeCtx();
      assert.doesNotThrow(() => item.draw(ctx));
      assert.ok(ctx.count('fillRect') > 0, 'item draws something');
    }
  }
});

test('a stage-3 crop draws its glint only on even half-second slots', () => {
  const p = plot({ col: 1, row: 1, crop: 'corn', stage: 3, growth: 1 });
  const tileX = FIELD.x + 16;
  const tileY = FIELD.y + 16;
  const glintCount = (time) => {
    const ctx = fakeCtx();
    cropDrawables(worldWith([p], time))[0].draw(ctx);
    return ctx.calls.filter((c) => c.op === 'fillRect' && c.args[0] === tileX + 13 &&
      c.args[1] === tileY + 2 && c.args[2] === 1 && c.args[3] === 1).length;
  };
  assert.equal(glintCount(0), 1);
  assert.equal(glintCount(0.4), 1);
  assert.equal(glintCount(0.5), 0);
  assert.equal(glintCount(0.9), 0);
  assert.equal(glintCount(1.0), 1);
});

test('a non-ripe crop never draws the glint', () => {
  const p = plot({ crop: 'carrot', stage: 2, growth: 0.7 });
  const ctx = fakeCtx();
  cropDrawables(worldWith([p], 0))[0].draw(ctx);
  assert.ok(!ctx.calls.some((c) => c.op === 'set' && c.name === 'fillStyle' && c.value === '#fff1e8'));
});

test('fertilizer 2 makes a watered plot grow 2.25x as fast', () => {
  const plain = plot({ growth: 0, water: 1 });
  const wPlain = worldWith([plain]);
  updateCrops(wPlain, 1);
  const fertile = plot({ growth: 0, water: 1 });
  const wFert = worldWith([fertile]);
  wFert.carrots.fertilizer = 2;
  updateCrops(wFert, 1);
  near(fertile.growth, plain.growth * 2.25);
  near(fertile.growth, 2.25 / 40);
});
