import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, HORIZON, WIDTH, HEIGHT, HOUSE, BARN, COOP, PASTURE, MEADOW, HILL, landWidth } from '../web/js/state.js';
import { PALETTE } from '../web/js/pixel.js';
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

// The fillRect calls recorded on a fakeCtx with the fillStyle in force for each, as {fill, args}.
const paintedRects = (ctx) => {
  let fill;
  const out = [];
  for (const c of ctx.calls) {
    if (c.op === 'set' && c.name === 'fillStyle') fill = c.value;
    else if (c.op === 'fillRect') out.push({ fill, args: c.args });
  }
  return out;
};

// The furthest right edge of any fillRect drawn by drawGround for a world.
const rightEdge = (world) => {
  const ctx = fakeCtx();
  drawGround(ctx, world);
  return Math.max(...rects(ctx).map(([x, , w]) => x + w));
};

test('drawGround without a world draws land 1 only, to the canvas width', () => {
  assert.equal(rightEdge(undefined), WIDTH);
});

test('drawGround with land 3 paints the grass out to x 960, past the canvas', () => {
  const world = createWorld(1);
  world.land = 3;
  const ctx = fakeCtx();
  drawGround(ctx, world);
  assert.deepEqual(rects(ctx)[0], [0, HORIZON, 960, HEIGHT - HORIZON]);
  assert.equal(rightEdge(world), 960);
  assert.ok(rects(ctx).some(([x]) => x >= 900), 'nothing drawn at x >= 900');
});

test('drawGround with land 2 adds a dirt path of 4 from x 320 to the land edge', () => {
  const world = createWorld(1);
  world.land = 2;
  const ctx = fakeCtx();
  drawGround(ctx, world);
  assert.ok(rects(ctx).some((r) => r[0] === 320 && r[1] === 92 && r[2] === 320 && r[3] === 4), 'no dirt path');
});

test('drawGround with land 3 paints a visible b crest on the mound above HILL', () => {
  const world = createWorld(1);
  world.land = 3;
  const ctx = fakeCtx();
  drawGround(ctx, world);
  const crest = paintedRects(ctx).filter(
    ({ fill, args: [x, y, w] }) =>
      fill === PALETTE.b && w >= 5 && x >= HILL.x && x + w <= HILL.x + HILL.w && y >= HILL.y - 12 && y < HILL.y,
  );
  assert.ok(crest.length > 0, 'no b crest drawn across the mound on HILL');
});

test('drawGround with land 1 draws no mound crest', () => {
  const ctx = fakeCtx();
  drawGround(ctx, createWorld(1));
  const wide = paintedRects(ctx).filter(({ fill, args: [x, y, w] }) => fill === PALETTE.b && w >= 5 && y < HILL.y);
  assert.equal(wide.length, 0);
});

// drewRect reports whether drawing every scenery item of a world paints the given rect in the given
// palette colour. The barn's body is a '8' rect at (BARN.x, BARN.y + 16); the coop's is a '4' rect
// at (COOP.x, COOP.y + 8). The barn and coop share a base line, so they are told apart by these.
const drewRect = (world, key, [x, y, w, h]) => {
  const ctx = fakeCtx();
  for (const item of sceneDrawables(world)) item.draw(ctx);
  return paintedRects(ctx).some((p) => p.fill === PALETTE[key] && p.args.join() === [x, y, w, h].join());
};

test('sceneDrawables has the barn on a fresh world and not once herds.cow.tier is 1', () => {
  const body = [BARN.x, BARN.y + 16, BARN.w, BARN.h - 16];
  const world = createWorld(1);
  assert.equal(drewRect(world, '8', body), true, 'no barn on a fresh world');
  world.herds.cow.tier = 1;
  assert.equal(drewRect(world, '8', body), false, 'barn still drawn at cow tier 1');
  assert.equal(sceneDrawables(world).length, sceneDrawables(createWorld(1)).length - 1);
});

test('sceneDrawables has the coop on a fresh world and not once herds.chicken.tier is 1', () => {
  const body = [COOP.x, COOP.y + 8, COOP.w, COOP.h - 8];
  const world = createWorld(1);
  assert.equal(drewRect(world, '4', body), true, 'no coop on a fresh world');
  world.herds.chicken.tier = 1;
  assert.equal(drewRect(world, '4', body), false, 'coop still drawn at chicken tier 1');
  assert.equal(sceneDrawables(world).length, sceneDrawables(createWorld(1)).length - 1);
});

test('the barn and coop drop out of the scene one at a time', () => {
  const world = createWorld(1);
  const before = sceneDrawables(world).length;
  world.herds.cow.tier = 1;
  assert.equal(sceneDrawables(world).length, before - 1);
  world.herds.chicken.tier = 1;
  assert.equal(sceneDrawables(world).length, before - 2);
});

// The FOR SALE sign's post is at x landWidth - 40 + 16, y 116 to 127.
const signPost = (world) => {
  const x = landWidth(world) - 40 + 16;
  return (r) => r[0] === x && r[1] === 116 && r[2] === 2 && r[3] === 12;
};

const sceneHasSign = (world) => {
  const ctx = fakeCtx();
  for (const item of sceneDrawables(world)) item.draw(ctx);
  return rects(ctx).some(signPost(world));
};

test('the FOR SALE sign is in the scene for land 1 and land 2', () => {
  for (const land of [1, 2]) {
    const world = createWorld(1);
    world.land = land;
    assert.ok(sceneHasSign(world), `no FOR SALE sign for land ${land}`);
  }
});

test('the FOR SALE sign is not in the scene for land 3', () => {
  const world = createWorld(1);
  world.land = 3;
  assert.equal(sceneHasSign(world), false);
});

test('the Sheep Meadow fence comes in at land 2 and the Goat Hill fence at land 3', () => {
  const railAt = (world, x) => {
    const ctx = fakeCtx();
    for (const item of sceneDrawables(world)) item.draw(ctx);
    return rects(ctx).some((r) => r[0] === x && r[1] === 163 && r[3] === 1);
  };
  const world = createWorld(1);
  assert.equal(railAt(world, MEADOW.x), false);
  world.land = 2;
  assert.equal(railAt(world, MEADOW.x), true);
  assert.equal(railAt(world, HILL.x), false);
  world.land = 3;
  assert.equal(railAt(world, HILL.x), true);
});
