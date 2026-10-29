import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour, HORIZON, WIDTH } from '../web/js/state.js';
import { skyColors, darkness, drawSky, drawNightShade } from '../web/js/sky.js';
import { fakeCtx } from './fakectx.js';

// A world set to the given hour of day, at the given world time in seconds.
function worldAt(hour, time = 0) {
  const w = createWorld(1);
  setHour(w, hour);
  w.time = time;
  return w;
}

const near = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);

// Each channel of a '#rrggbb' colour as a number.
const channels = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));

test('skyColors at hour 0 is the midnight keyframe', () => {
  assert.deepEqual(skyColors(0), { top: '#0b0f2a', bottom: '#1d2b53' });
});

test('skyColors at hour 7 is the morning keyframe', () => {
  assert.deepEqual(skyColors(7), { top: '#29adff', bottom: '#ffccaa' });
});

test('skyColors at hour 12 is the midday colours', () => {
  assert.deepEqual(skyColors(12), { top: '#29adff', bottom: '#bdebff' });
});

test('skyColors at hour 19 is the dusk keyframe', () => {
  assert.deepEqual(skyColors(19), { top: '#7e2553', bottom: '#ffa300' });
});

test('skyColors at hour 6 lies between the hour 5 and hour 7 colours', () => {
  const a = skyColors(5);
  const b = skyColors(7);
  const mid = skyColors(6);
  for (const key of ['top', 'bottom']) {
    const lo = channels(a[key]).map((v, i) => Math.min(v, channels(b[key])[i]));
    const hi = channels(a[key]).map((v, i) => Math.max(v, channels(b[key])[i]));
    channels(mid[key]).forEach((v, i) => {
      assert.ok(v >= lo[i] && v <= hi[i], `${key} channel ${i}: ${v} not in [${lo[i]}, ${hi[i]}]`);
    });
  }
});

test('skyColors at hour 20 lies between the hour 19 and hour 21 colours', () => {
  const a = skyColors(19);
  const b = skyColors(21);
  const mid = skyColors(20);
  for (const key of ['top', 'bottom']) {
    const lo = channels(a[key]).map((v, i) => Math.min(v, channels(b[key])[i]));
    const hi = channels(a[key]).map((v, i) => Math.max(v, channels(b[key])[i]));
    channels(mid[key]).forEach((v, i) => {
      assert.ok(v >= lo[i] && v <= hi[i], `${key} channel ${i}: ${v} not in [${lo[i]}, ${hi[i]}]`);
    });
  }
});

test('darkness is 0.55 at 3, 0.275 at 6, 0 at 12, 0.275 at 20 and 0.55 at 22', () => {
  near(darkness(3), 0.55);
  near(darkness(6), 0.275);
  near(darkness(12), 0);
  near(darkness(20), 0.275);
  near(darkness(22), 0.55);
});

test('drawSky calls fillRect at hours 3, 6.5, 12 and 19.5', () => {
  for (const hour of [3, 6.5, 12, 19.5]) {
    const ctx = fakeCtx();
    drawSky(ctx, worldAt(hour, 10));
    assert.ok(ctx.count('fillRect') > 0, `no fillRect at hour ${hour}`);
  }
});

test('drawSky never paints at or below HORIZON at hours 3, 6.5, 12 and 19.5', () => {
  for (const hour of [3, 6.5, 12, 19.5]) {
    for (const time of [0, 37.5, 91]) {
      const ctx = fakeCtx();
      drawSky(ctx, worldAt(hour, time));
      for (const c of ctx.calls.filter((call) => call.op === 'fillRect')) {
        const [x, y, w, h] = c.args;
        assert.ok(
          y + h <= HORIZON,
          `hour ${hour} time ${time}: fillRect(${x}, ${y}, ${w}, ${h}) reaches y ${y + h} > ${HORIZON}`,
        );
      }
    }
  }
});

test('drawSky stays within the canvas width for the far-right sun at hour 19.99', () => {
  const ctx = fakeCtx();
  drawSky(ctx, worldAt(19.99, 0));
  for (const c of ctx.calls.filter((call) => call.op === 'fillRect')) {
    const [y, h] = [c.args[1], c.args[3]];
    assert.ok(y + h <= HORIZON, `fillRect reaches y ${y + h} > ${HORIZON}`);
  }
  assert.ok(WIDTH > 0);
});

test('drawNightShade at noon makes no fillRect', () => {
  const ctx = fakeCtx();
  drawNightShade(ctx, worldAt(12));
  assert.equal(ctx.count('fillRect'), 0);
});

test('drawNightShade at 23:00 draws the shade and leaves globalAlpha at 1', () => {
  const ctx = fakeCtx();
  drawNightShade(ctx, worldAt(23));
  assert.ok(ctx.count('fillRect') > 0, 'no fillRect at 23:00');
  assert.equal(ctx.globalAlpha, 1);
});
