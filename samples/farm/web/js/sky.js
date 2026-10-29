// The sky behind the ground: the gradient, stars, sun or moon, clouds and distant hills, plus the
// translucent night shade with the farmhouse windows, and fireflies after dark. Imports no
// globals and touches no window or document on load.

import { WIDTH, HEIGHT, HORIZON, WINDOWS, hourOf, isNight } from './state.js';
import { PALETTE, drawSprite } from './pixel.js';
import { makeRng } from './rng.js';

// Gradient keyframes by hour of day; each channel is interpolated linearly between them.
const SKY_KEYS = [
  { hour: 0,  top: '#0b0f2a', bottom: '#1d2b53' },
  { hour: 5,  top: '#1d2b53', bottom: '#7e2553' },
  { hour: 7,  top: '#29adff', bottom: '#ffccaa' },
  { hour: 10, top: '#29adff', bottom: '#bdebff' },
  { hour: 17, top: '#29adff', bottom: '#bdebff' },
  { hour: 19, top: '#7e2553', bottom: '#ffa300' },
  { hour: 21, top: '#1d2b53', bottom: '#7e2553' },
  { hour: 24, top: '#0b0f2a', bottom: '#1d2b53' },
];

const BANDS = 8;
const STAR_COUNT = 40;
const FIREFLY_COUNT = 6;

const SUN = [
  '..9999..',
  '.9aaaa9.',
  '9aaaaaa9',
  '9aaaaaa9',
  '9aaaaaa9',
  '9aaaaaa9',
  '.9aaaa9.',
  '..9999..',
];

const MOON = [
  '.77777.',
  '7777777',
  '7767777',
  '7777777',
  '7777677',
  '7777777',
  '.77777.',
];

// Each cloud is a set of [dx, dy, w, h] rectangles relative to its top-left corner, with a
// one-unit underside row under the whole cloud.
const CLOUD_PARTS = [[0, 4, 10, 4], [6, 0, 10, 6], [14, 3, 10, 5], [22, 5, 8, 3]];
const CLOUD_SPAN = WIDTH + 80;
const CLOUDS = [
  { x: 10,  y: 10, speed: 3 },
  { x: 100, y: 22, speed: 4 },
  { x: 190, y: 6,  speed: 5 },
  { x: 260, y: 26, speed: 6 },
];

const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

function toRgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function toHex(rgb) {
  return `#${rgb.map((v) => v.toString(16).padStart(2, '0')).join('')}`;
}

// mix blends two '#rrggbb' colours by t in [0, 1], rounding each channel.
function mix(a, b, t) {
  const pa = toRgb(a);
  const pb = toRgb(b);
  return toHex(pa.map((v, i) => Math.round(v + (pb[i] - v) * t)));
}

// skyColors returns the sky gradient for an hour in [0, 24) as {top, bottom}.
export function skyColors(hour) {
  for (let i = 0; i < SKY_KEYS.length - 1; i++) {
    const a = SKY_KEYS[i];
    const b = SKY_KEYS[i + 1];
    if (hour <= b.hour) {
      const t = (hour - a.hour) / (b.hour - a.hour);
      return { top: mix(a.top, b.top, t), bottom: mix(a.bottom, b.bottom, t) };
    }
  }
  const last = SKY_KEYS[SKY_KEYS.length - 1];
  return { top: last.top, bottom: last.bottom };
}

// darkness returns how dark the scene is, 0 to 0.55.
export function darkness(hour) {
  if (hour >= 7 && hour < 19) return 0;
  if (hour >= 21 || hour < 5) return 0.55;
  if (hour >= 19 && hour < 21) return (0.55 * (hour - 19)) / 2;
  return (0.55 * (7 - hour)) / 2;
}

// drawAboveHorizon draws a sprite, cutting off any rows that would fall at or below HORIZON.
function drawAboveHorizon(ctx, rows, x, y) {
  const top = Math.round(y);
  const visible = Math.min(rows.length, HORIZON - top);
  if (visible > 0) drawSprite(ctx, rows.slice(0, visible), x, top);
}

function drawGradient(ctx, hour) {
  const colours = skyColors(hour);
  const bandHeight = HORIZON / BANDS;
  for (let i = 0; i < BANDS; i++) {
    ctx.fillStyle = mix(colours.top, colours.bottom, i / (BANDS - 1));
    ctx.fillRect(0, i * bandHeight, WIDTH, bandHeight);
  }
}

function drawStars(ctx, world) {
  const rng = makeRng(7);
  ctx.fillStyle = PALETTE['7'];
  for (let i = 0; i < STAR_COUNT; i++) {
    const x = rng.int(WIDTH);
    const y = 2 + rng.int(HORIZON - 21);
    // Every third star blinks.
    if (i % 3 === 0 && Math.floor(world.time * 2 + i) % 2 !== 0) continue;
    ctx.fillRect(x, y, 1, 1);
  }
}

function drawCloud(ctx, cloud, world, night) {
  const travelled = cloud.x + 40 + cloud.speed * world.time;
  const left = Math.round((((travelled % CLOUD_SPAN) + CLOUD_SPAN) % CLOUD_SPAN) - 40);
  ctx.fillStyle = night ? PALETTE['5'] : PALETTE['7'];
  for (const [dx, dy, w, h] of CLOUD_PARTS) {
    ctx.fillRect(left + dx, cloud.y + dy, w, h);
  }
  ctx.fillStyle = PALETTE['6'];
  ctx.fillRect(left + 2, cloud.y + 8, 26, 1);
}

function drawHills(ctx) {
  ctx.fillStyle = PALETTE['3'];
  for (let x = 0; x < WIDTH; x++) {
    const height = Math.round(10 + 3 * Math.sin(x / 23) + Math.sin(x / 7));
    ctx.fillRect(x, HORIZON - height, 1, height);
  }
  ctx.fillStyle = PALETTE['b'];
  for (let x = 0; x < WIDTH; x++) {
    const height = Math.round(4 + 2 * Math.sin(x / 17 + 2) + Math.sin(x / 5));
    ctx.fillRect(x, HORIZON - height, 1, height);
  }
}

// drawSky fills y 0..HORIZON-1 with the sky, the stars, the sun or moon, the clouds and the hills.
export function drawSky(ctx, world) {
  const hour = hourOf(world);

  drawGradient(ctx, hour);

  if (darkness(hour) > 0.1) drawStars(ctx, world);

  if (hour >= 6 && hour < 20) {
    const x = (WIDTH * (hour - 6)) / 14;
    const y = HORIZON - 6 - Math.sin((Math.PI * (hour - 6)) / 14) * 48;
    drawAboveHorizon(ctx, SUN, x, y);
  }
  if (hour >= 20 || hour < 6) {
    const h = hour < 6 ? hour + 24 : hour;
    const x = (WIDTH * (h - 20)) / 10;
    const y = HORIZON - 6 - Math.sin((Math.PI * (h - 20)) / 10) * 48;
    drawAboveHorizon(ctx, MOON, x, y);
  }

  const night = isNight(world);
  for (const cloud of CLOUDS) drawCloud(ctx, cloud, world, night);

  drawHills(ctx);
}

// drawFireflies draws six wandering, blinking fireflies over the field and yard.
function drawFireflies(ctx, world) {
  const rng = makeRng(23);
  const t = world.time;
  ctx.fillStyle = PALETTE['a'];
  for (let i = 0; i < FIREFLY_COUNT; i++) {
    const baseX = 20 + rng.int(151);
    const baseY = 90 + rng.int(51);
    const x = clamp(baseX + Math.round(6 * Math.sin(t * 0.7 + i)), 20, 170);
    const y = clamp(baseY + Math.round(4 * Math.cos(t * 0.9 + i * 1.7)), 90, 140);
    if (Math.sin(t * 3 + i * 2) > -0.4) ctx.fillRect(x, y, 1, 1);
  }
}

// drawNightShade, when darkness(hour) > 0, darkens the whole canvas and draws the lit farmhouse
// windows on top. From 18:00 to 22:00 it also draws the fireflies.
export function drawNightShade(ctx, world) {
  const hour = hourOf(world);
  const shade = darkness(hour);

  if (shade > 0) {
    ctx.globalAlpha = shade;
    ctx.fillStyle = PALETTE['1'];
    ctx.fillRect(0, 0, WIDTH, HEIGHT);

    ctx.globalAlpha = Math.min(1, shade * 2);
    for (const w of WINDOWS) {
      ctx.fillStyle = PALETTE['9'];
      ctx.fillRect(w.x - 1, w.y - 1, w.w + 2, w.h + 2);
      ctx.fillStyle = PALETTE['a'];
      ctx.fillRect(w.x, w.y, w.w, w.h);
    }
    ctx.globalAlpha = 1;
  }

  if (hour >= 18 && hour < 22) drawFireflies(ctx, world);
}
