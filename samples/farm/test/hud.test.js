// Tests for web/js/hud.js. They use the recording context from test/fakectx.js and the world
// model from web/js/state.js, and check the contract in the FARM-11 issue.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fakeCtx } from './fakectx.js';
import { createWorld, say, puff, logEvent, setHour } from '../web/js/state.js';
import {
  updateOverlay,
  formatClock,
  drawEffects,
  drawBubbles,
  drawHud,
} from '../web/js/hud.js';
import { drawBubble, drawText, textWidth } from '../web/js/pixel.js';

// fills returns every fillRect recorded on ctx as {x, y, w, h, fill, alpha}, where fill is the
// fillStyle and alpha the globalAlpha in effect at that call.
function fills(ctx) {
  let fill;
  let alpha;
  const out = [];
  for (const c of ctx.calls) {
    if (c.op === 'set' && c.name === 'fillStyle') fill = c.value;
    if (c.op === 'set' && c.name === 'globalAlpha') alpha = c.value;
    if (c.op === 'fillRect') {
      const [x, y, w, h] = c.args;
      out.push({ x, y, w, h, fill, alpha });
    }
  }
  return out;
}

// bbox is the bounding box of rects as {x, y, w, h}; it fails on an empty list.
function bbox(rects) {
  assert.ok(rects.length > 0, 'expected some rectangles');
  const x0 = Math.min(...rects.map((r) => r.x));
  const y0 = Math.min(...rects.map((r) => r.y));
  const x1 = Math.max(...rects.map((r) => r.x + r.w));
  const y1 = Math.max(...rects.map((r) => r.y + r.h));
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

// area is the number of unit pixels covered by rects.
function area(rects) {
  return rects.reduce((sum, r) => sum + r.w * r.h, 0);
}

// effectRects draws one effect of the given kind at (x, y) with the given age and ttl, and
// returns the rectangles it produced.
function effectRects(kind, age, ttl = 1.5, x = 50, y = 60) {
  const world = createWorld(1);
  puff(world, kind, x, y, 0, 0, ttl);
  world.effects[0].age = age;
  const ctx = fakeCtx();
  drawEffects(ctx, world);
  return fills(ctx);
}

test('updateOverlay removes a bubble once its ttl has run out', () => {
  const world = createWorld(1);
  say(world, 100, 50, 'HI', 'speech', 1);
  updateOverlay(world, 0.5);
  assert.equal(world.bubbles.length, 1);
  assert.equal(world.bubbles[0].ttl, 0.5);
  updateOverlay(world, 0.5);
  assert.equal(world.bubbles.length, 0);
});

test('updateOverlay moves, ages and removes effects', () => {
  const world = createWorld(1);
  puff(world, 'drop', 10, 10, 4, -2, 1.5);
  updateOverlay(world, 0.5);
  assert.equal(world.effects.length, 1);
  const e = world.effects[0];
  assert.equal(e.x, 12);
  assert.equal(e.y, 9);
  assert.equal(e.age, 0.5);
  assert.equal(e.ttl, 1);
  updateOverlay(world, 1);
  assert.equal(world.effects.length, 0);
});

test('updateOverlay makes smoke drift faster: vx grows by 2*dt', () => {
  const world = createWorld(1);
  puff(world, 'smoke', 0, 0, 1, 0, 2);
  updateOverlay(world, 0.25);
  assert.equal(world.effects[0].vx, 1 + 2 * 0.25);
});

test('formatClock shows 06:00 at the start of the day', () => {
  const world = createWorld(1);
  assert.equal(formatClock(world), '06:00');
});

test('formatClock rounds minutes down to a multiple of 5', () => {
  const world = createWorld(1);
  setHour(world, 13 + 27 / 60);
  assert.equal(formatClock(world), '13:25');
});

test('formatClock shows 00:05 and not 00:00 when hourOf lands a hair under 00:05', () => {
  const world = createWorld(1);
  setHour(world, 7 / 60);
  assert.equal(formatClock(world), '00:05');
  setHour(world, 5 / 60);
  assert.equal(formatClock(world), '00:05');
});

test('drawEffects draws smoke as a growing puff of palette 6', () => {
  const young = bbox(effectRects('smoke', 0));
  assert.deepEqual(young, { x: 49, y: 59, w: 2, h: 2 });
  const old = effectRects('smoke', 1.5);
  assert.equal(bbox(old).w, 5);
  assert.equal(bbox(old).h, 5);
  assert.ok(old.every((r) => r.fill === '#c2c3c7'));
});

test('drawEffects draws zzz as Z, and ZZ once age is 1 or more', () => {
  const one = effectRects('zzz', 0.2);
  assert.equal(bbox(one).w, 3);
  assert.ok(one.every((r) => r.fill === '#fff1e8'));
  const two = effectRects('zzz', 1.2);
  assert.equal(bbox(two).w, 7);
  assert.ok(two.every((r) => r.fill === '#fff1e8'));
});

test('drawEffects draws note as a 3x5 gold sprite', () => {
  const rects = effectRects('note', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 3, h: 5 });
  assert.ok(rects.every((r) => r.fill === '#ffec27'));
});

test('drawEffects draws a full 3x3 coin with a gold centre and orange edge, narrow on odd tenths', () => {
  const full = effectRects('coin', 0);
  assert.deepEqual(bbox(full), { x: 50, y: 60, w: 3, h: 3 });
  const fills1 = new Set(full.map((r) => r.fill));
  assert.deepEqual([...fills1].sort(), ['#ffa300', '#ffec27']);

  const narrow = effectRects('coin', 0.15);
  assert.equal(bbox(narrow).w, 1);
  assert.equal(bbox(narrow).h, 3);
  assert.equal(bbox(narrow).x, 51);
  assert.ok(narrow.every((r) => r.fill === '#ffec27' || r.fill === '#ffa300'));
});

test('drawEffects draws a drop as a 1x2 blue sprite', () => {
  const rects = effectRects('drop', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 1, h: 2 });
  assert.equal(area(rects), 2);
  assert.ok(rects.every((r) => r.fill === '#29adff'));
});

test('drawEffects draws a feather as a 2x1 cream sprite', () => {
  const rects = effectRects('feather', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 2, h: 1 });
  assert.ok(rects.every((r) => r.fill === '#fff1e8'));
});

test('drawEffects draws sparkle as a 3x3 plus that blinks', () => {
  const on = effectRects('sparkle', 0.2);
  assert.deepEqual(bbox(on), { x: 50, y: 60, w: 3, h: 3 });
  assert.ok(on.every((r) => r.fill === '#fff1e8'));
  const off = effectRects('sparkle', 0.3);
  assert.equal(off.length, 0);
});

test('drawEffects draws heart as a 5x4 red sprite', () => {
  const rects = effectRects('heart', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 5, h: 4 });
  assert.ok(rects.every((r) => r.fill === '#ff004d'));
});

test('drawEffects draws mud as a 2x2 brown blob', () => {
  const rects = effectRects('mud', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 2, h: 2 });
  assert.equal(area(rects), 4);
  assert.ok(rects.every((r) => r.fill === '#ab5236'));
});

test('drawEffects draws milk as a 3x3 bottle in palette 6 and 7', () => {
  const rects = effectRects('milk', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 3, h: 3 });
  assert.ok(rects.every((r) => r.fill === '#c2c3c7' || r.fill === '#fff1e8'));
});

test('drawEffects draws an egg as a 3x4 cream sprite', () => {
  const rects = effectRects('egg', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 3, h: 4 });
  assert.ok(rects.every((r) => r.fill === '#fff1e8'));
});

test('drawEffects draws a ghost as a 5x4 sprite', () => {
  const rects = effectRects('ghost', 0);
  assert.deepEqual(bbox(rects), { x: 50, y: 60, w: 5, h: 4 });
  assert.ok(rects.length > 0);
});

test('drawEffects draws the portal as a 3x3 sprite that flickers between two frames', () => {
  const frameA = effectRects('portal', 0);
  assert.deepEqual(bbox(frameA), { x: 50, y: 60, w: 3, h: 3 });
  const frameB = effectRects('portal', 0.2);
  assert.deepEqual(bbox(frameB), { x: 50, y: 60, w: 3, h: 3 });
  assert.notDeepEqual(frameA, frameB);
});

test('drawEffects skips an unknown kind without throwing and leaves globalAlpha at 1', () => {
  const world = createWorld(1);
  puff(world, 'confetti', 20, 20);
  const ctx = fakeCtx();
  assert.doesNotThrow(() => drawEffects(ctx, world));
  assert.equal(ctx.count('fillRect'), 0);
  assert.equal(ctx.globalAlpha, 1);
});

test('drawEffects fades an effect over its last half second and restores globalAlpha', () => {
  const world = createWorld(1);
  puff(world, 'drop', 20, 20, 0, 0, 0.25);
  const ctx = fakeCtx();
  drawEffects(ctx, world);
  assert.equal(fills(ctx)[0].alpha, 0.5);
  assert.equal(ctx.globalAlpha, 1);

  const full = createWorld(1);
  puff(full, 'drop', 20, 20, 0, 0, 3);
  const ctx2 = fakeCtx();
  drawEffects(ctx2, full);
  assert.equal(fills(ctx2)[0].alpha, 1);
  assert.equal(ctx2.globalAlpha, 1);
});

test('drawBubbles calls fillRect once there is a bubble', () => {
  const world = createWorld(1);
  say(world, 100, 60, 'HELLO');
  const ctx = fakeCtx();
  drawBubbles(ctx, world);
  assert.ok(ctx.count('fillRect') >= 1);
});

test('drawBubbles draws oldest first, each with drawBubble', () => {
  const world = createWorld(1);
  say(world, 40, 60, 'FIRST', 'speech');
  say(world, 200, 100, 'SECOND', 'thought');
  const ctx = fakeCtx();
  drawBubbles(ctx, world);

  const expected = fakeCtx();
  drawBubble(expected, 40, 60, 'FIRST', 'speech');
  drawBubble(expected, 200, 100, 'SECOND', 'thought');
  assert.deepEqual(ctx.calls, expected.calls);
});

test('drawHud draws with an empty log and leaves globalAlpha at 1', () => {
  const world = createWorld(1);
  world.log = [];
  const ctx = fakeCtx();
  assert.doesNotThrow(() => drawHud(ctx, world));
  assert.ok(ctx.count('fillRect') > 0);
  assert.equal(ctx.globalAlpha, 1);
});

test('drawHud keeps a long log entry clear of the statistics text', () => {
  const world = createWorld(1);
  world.stats = { cigarettes: 12, gossip: 34, harvests: 5, abductions: 6 };
  logEvent(world, 'Gary told everyone that Daisy the cow had been abducted by aliens again');
  const ctx = fakeCtx();
  drawHud(ctx, world);

  const statsText = 'COWS 4/6 HENS 5/8 X1.00';
  const statsX = 318 - textWidth(statsText);
  const logPixels = fills(ctx).filter((r) => r.fill === '#fff1e8' && r.y >= 171);
  assert.ok(logPixels.length > 0, 'the log entry should be drawn');
  const rightmost = Math.max(...logPixels.map((r) => r.x + r.w - 1));
  assert.ok(rightmost <= statsX - 5, `log ends at ${rightmost}, stats start at ${statsX}`);
  assert.equal(ctx.globalAlpha, 1);
});

test('drawHud draws the coins with money formatting, ending at the right edge', () => {
  const world = createWorld(1);
  world.coins = 12345.6;
  const ctx = fakeCtx();
  drawHud(ctx, world);

  const text = '$12.3K';
  const coinsX = 318 - textWidth(text);
  const expected = fakeCtx();
  drawText(expected, text, coinsX, 2, '#ffec27');
  const want = fills(expected).map((r) => ({ x: r.x, y: r.y, w: r.w, h: r.h }));
  const got = fills(ctx)
    .filter((r) => r.fill === '#ffec27' && r.y >= 2 && r.y <= 6 && r.x >= coinsX)
    .map((r) => ({ x: r.x, y: r.y, w: r.w, h: r.h }));
  assert.deepEqual(got, want);
  assert.equal(Math.max(...got.map((r) => r.x + r.w)), 318);
});

test('drawHud shows the income rate in dim text left of the coins', () => {
  const world = createWorld(1);
  const ctx = fakeCtx();
  drawHud(ctx, world, 1.64);

  const coinsX = 318 - textWidth('$10');
  const coinSpriteX = coinsX - 5;
  const dim = fills(ctx).filter((r) => r.fill === '#c2c3c7' && r.y >= 2 && r.y <= 6);
  assert.ok(dim.length > 0, 'expected the dim rate text');
  // '+1.6/S' ends a 4-unit gap before the coin sprite.
  assert.ok(Math.max(...dim.map((r) => r.x + r.w)) <= coinSpriteX - 4);
  assert.equal(ctx.globalAlpha, 1);
});

test('drawHud shows PAUSED in a centred black box and leaves globalAlpha at 1', () => {
  const world = createWorld(1);
  world.paused = true;
  const ctx = fakeCtx();
  drawHud(ctx, world);
  const boxW = textWidth('PAUSED') + 6;
  const boxX = Math.floor((320 - boxW) / 2);
  const boxY = Math.floor((180 - 11) / 2);
  const box = fills(ctx).find(
    (r) => r.fill === '#000000' && r.x === boxX && r.y === boxY && r.w === boxW && r.h === 11,
  );
  assert.ok(box, 'expected the centred PAUSED box');
  assert.ok(fills(ctx).some((r) => r.fill === '#ffec27' && r.x >= boxX && r.x < boxX + boxW));
  assert.equal(ctx.globalAlpha, 1);
});

test('drawHud shows the speed badge under the top bar at speed 4', () => {
  const world = createWorld(1);
  world.speed = 4;
  const ctx = fakeCtx();
  drawHud(ctx, world);
  const badge = fills(ctx).filter((r) => r.y >= 11 && r.y <= 15 && r.fill === '#fff1e8');
  assert.ok(badge.length > 0, 'expected the >> 4X badge');
  assert.ok(Math.max(...badge.map((r) => r.x + r.w - 1)) <= 318);
  assert.equal(ctx.globalAlpha, 1);
});

test('drawHud shows no speed badge at speed 1', () => {
  const world = createWorld(1);
  const ctx = fakeCtx();
  drawHud(ctx, world);
  assert.equal(fills(ctx).filter((r) => r.y >= 11 && r.y <= 15 && r.fill === '#fff1e8').length, 0);
});
