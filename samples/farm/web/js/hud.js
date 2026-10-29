// Overlay and HUD: ages the speech bubbles and particle effects that other modules add with say
// and puff, draws them, and draws the heads-up display (title, day and clock, coins, statistics,
// the latest log entry, and the pause and speed badges). Nothing here runs on import.
//
// Sprite letters are PALETTE keys from pixel.js: '4' #ab5236, '6' #c2c3c7, '7' #fff1e8,
// '8' #ff004d, '9' #ffa300, 'a' #ffec27, 'c' #29adff.

import { WIDTH, HEIGHT, hourOf } from './state.js';
import { drawSprite, drawText, textWidth, drawBubble } from './pixel.js';

const HUD_TEXT = '#fff1e8';
const GOLD = '#ffec27';
const DIM = '#c2c3c7';
const BLACK = '#000000';
const RIGHT = 318;   // right-hand text ends here
const BAR_H = 9;

const KNOWN_EFFECTS = new Set([
  'smoke', 'zzz', 'note', 'coin', 'drop', 'feather', 'sparkle', 'heart', 'mud',
]);

const SPRITES = {
  note: ['.aa', '.a.', '.a.', 'aa.', 'aa.'],
  coinFull: ['999', '9a9', '999'],
  coinNarrow: ['9', 'a', '9'],
  drop: ['c', 'c'],
  feather: ['77'],
  sparkle: ['.7.', '777', '.7.'],
  heart: ['88.88', '88888', '88888', '.888.'],
  mud: ['44', '44'],
};

const pad2 = (n) => String(n).padStart(2, '0');

// smokeRows is a solid square of '6' that grows from 2x2 to 5x5 as the puff ages.
function smokeRows(age) {
  const size = Math.min(5, 2 + Math.floor(age * 2));
  return Array.from({ length: size }, () => '6'.repeat(size));
}

// updateOverlay ages bubbles and effects by dt seconds and removes the ones whose ttl has run out.
// Smoke also drifts and spreads: its vx grows by 2 per second.
export function updateOverlay(world, dt) {
  for (const b of world.bubbles) b.ttl -= dt;
  world.bubbles = world.bubbles.filter((b) => b.ttl > 0);

  for (const e of world.effects) {
    e.x += e.vx * dt;
    e.y += e.vy * dt;
    e.age += dt;
    e.ttl -= dt;
    if (e.kind === 'smoke') e.vx += 2 * dt;
  }
  world.effects = world.effects.filter((e) => e.ttl > 0);
}

// formatClock returns the hour of day as 'HH:MM', with the minutes rounded down to a multiple of 5.
// The 1e-6 guards against hourOf landing a hair under a whole minute (4.9999999 for 00:05).
export function formatClock(world) {
  const minutes = Math.floor(hourOf(world) * 60 + 1e-6);
  const rounded = minutes - (minutes % 5);
  return `${pad2(Math.floor(rounded / 60))}:${pad2(rounded % 60)}`;
}

// drawOne draws a single effect at its own alpha; the caller has already set globalAlpha.
function drawOne(ctx, e) {
  switch (e.kind) {
    case 'smoke': {
      const rows = smokeRows(e.age);
      drawSprite(ctx, rows, e.x - rows.length / 2, e.y - rows.length / 2);
      break;
    }
    case 'zzz':
      drawText(ctx, e.age >= 1 ? 'ZZ' : 'Z', e.x, e.y, HUD_TEXT);
      break;
    case 'note':
      drawSprite(ctx, SPRITES.note, e.x, e.y);
      break;
    case 'coin': {
      // Spinning: narrow on every other 0.1 s.
      const narrow = Math.floor(e.age * 10 + 1e-6) % 2 === 1;
      if (narrow) drawSprite(ctx, SPRITES.coinNarrow, e.x + 1, e.y);
      else drawSprite(ctx, SPRITES.coinFull, e.x, e.y);
      break;
    }
    case 'drop':
      drawSprite(ctx, SPRITES.drop, e.x, e.y);
      break;
    case 'feather':
      drawSprite(ctx, SPRITES.feather, e.x, e.y);
      break;
    case 'sparkle':
      if (Math.floor(e.age * 4) % 2 === 0) drawSprite(ctx, SPRITES.sparkle, e.x, e.y);
      break;
    case 'heart':
      drawSprite(ctx, SPRITES.heart, e.x, e.y);
      break;
    case 'mud':
      drawSprite(ctx, SPRITES.mud, e.x, e.y);
      break;
    default:
      break;
  }
}

// drawEffects draws every known effect, fading it out over its last half second. Unknown kinds
// are skipped. globalAlpha is back to 1 afterwards.
export function drawEffects(ctx, world) {
  for (const e of world.effects) {
    if (!KNOWN_EFFECTS.has(e.kind)) continue;
    ctx.globalAlpha = Math.max(0, Math.min(1, e.ttl / 0.5));
    drawOne(ctx, e);
  }
  ctx.globalAlpha = 1;
}

// drawBubbles draws each bubble, oldest first.
export function drawBubbles(ctx, world) {
  for (const b of world.bubbles) drawBubble(ctx, b.x, b.y, b.text, b.kind);
}

// bar paints a translucent black strip of the given height from row y.
function bar(ctx, y) {
  ctx.globalAlpha = 0.55;
  ctx.fillStyle = BLACK;
  ctx.fillRect(0, y, WIDTH, BAR_H);
  ctx.globalAlpha = 1;
}

// drawHud draws the top bar, the bottom bar, and the pause and speed badges.
export function drawHud(ctx, world) {
  bar(ctx, 0);
  bar(ctx, HEIGHT - BAR_H);

  // Top bar: title, day and clock on the left; coins on the right.
  const title = 'GOSSIP ACRES';
  drawText(ctx, title, 2, 2, GOLD);
  drawText(ctx, `DAY ${world.day}  ${formatClock(world)}`, 2 + textWidth(title) + 8, 2, HUD_TEXT);

  const coinsText = `$${world.coins}`;
  const coinsX = RIGHT - textWidth(coinsText);
  drawText(ctx, coinsText, coinsX, 2, GOLD);
  drawSprite(ctx, SPRITES.coinFull, coinsX - 2 - 3, 2);

  // Bottom bar: the newest log entry on the left, cut to fit; the statistics on the right.
  const stats = world.stats || {};
  const statsText = `CIGS ${stats.cigarettes || 0} GOSSIP ${stats.gossip || 0} UFO ${stats.abductions || 0}`;
  const statsX = RIGHT - textWidth(statsText);
  const barY = HEIGHT - BAR_H + 2;
  drawText(ctx, statsText, statsX, barY, DIM);

  const entry = world.log && world.log[0];
  if (entry) {
    const chars = Array.from(String(entry.text));
    while (chars.length > 0 && 2 + textWidth(chars.join('')) > statsX - 4) chars.pop();
    if (chars.length > 0) drawText(ctx, chars.join(''), 2, barY, HUD_TEXT);
  }

  if (world.paused) {
    const label = 'PAUSED';
    const boxW = textWidth(label) + 6;
    const boxH = 11;
    const boxX = Math.floor((WIDTH - boxW) / 2);
    const boxY = Math.floor((HEIGHT - boxH) / 2);
    ctx.fillStyle = BLACK;
    ctx.fillRect(boxX, boxY, boxW, boxH);
    drawText(ctx, label, boxX + 3, boxY + 3, GOLD);
  } else if (world.speed > 1) {
    const label = `>> ${world.speed}X`;
    drawText(ctx, label, RIGHT - textWidth(label), BAR_H + 2, HUD_TEXT);
  }

  ctx.globalAlpha = 1;
}
