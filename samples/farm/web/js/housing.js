// Draws each herd's housing tier as a building at its site, so that housing levels up on the farm:
// from a shed, through barns, hotels and towers, to the Burj Moolifa and the Burj Al Hen. It imports
// only state.js, species.js and pixel.js, and touches no window or document on load. Nothing here
// uses Math.random, so the same arguments always draw the same picture.

import { hasKind, SITES } from './state.js';
import { SPECIES, KINDS } from './species.js';
import { PALETTE } from './pixel.js';

// cell is a deterministic value in 0..n-1 for a row and column. It stands in for randomness, so
// the cow spots on the Burj Moolifa stay put from frame to frame.
function cell(row, col, n) {
  let h = Math.imul(row, 374761393) ^ Math.imul(col, 668265263);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) % n;
}

// drawShed draws a sloped-roof shed: the roof on the top third, narrowing one unit per row to the
// right, then a wall with a door and one window.
function drawShed(fill, w, h, look) {
  const roofH = Math.round(h / 3);
  for (let r = 0; r < roofH; r++) fill(look.roof, 0, r, w - r, 1);
  fill(look.wall, 0, roofH, w, h - roofH);
  fill('0', Math.round(w / 2) - 4, h - 12, 8, 12);
  fill('1', 6, roofH + 3, 6, 5);
}

// drawBarn draws a barn with a gambrel roof on the top third (narrowing two units a row near its
// top), a wall with white trim on both sides, and a white double door with an X brace.
function drawBarn(fill, w, h, look) {
  const roofH = Math.round(h / 3);
  for (let r = 0; r < roofH; r++) {
    const inset = r < 4 ? 2 * (4 - r) : 0;
    fill(look.roof, inset, r, w - 2 * inset, 1);
  }
  const bodyH = h - roofH;
  fill(look.wall, 0, roofH, w, bodyH);
  fill('7', 0, roofH, 1, bodyH);
  fill('7', w - 1, roofH, 1, bodyH);

  const doorW = Math.round(w / 3);
  const doorH = Math.round(bodyH * 0.7);
  const doorL = Math.round((w - doorW) / 2);
  const doorT = h - doorH;
  fill('7', doorL, doorT, doorW, doorH);
  const span = Math.max(1, doorH - 1);
  for (let r = 0; r < doorH; r++) {
    const col = Math.round((r * Math.max(0, doorW - 1)) / span);
    fill('5', doorL + col, doorT + r, 1, 1);
    fill('5', doorL + doorW - 1 - col, doorT + r, 1, 1);
  }
}

// drawBlock draws a hotel or factory: a wall under a three-row roof stripe, a grid of 2x2 windows
// every four units (one column lights up in turn, so the block looks lived in), and an entrance at
// the bottom centre.
function drawBlock(fill, w, h, look, time) {
  fill(look.roof, 0, 0, w, 3);
  fill(look.wall, 0, 3, w, h - 3);
  const doorW = 6;
  const doorH = 8;
  const doorL = Math.round((w - doorW) / 2);
  const doorT = h - doorH;
  const lit = Math.floor(time);
  let col = 0;
  for (let lx = 2; lx + 2 <= w - 2; lx += 4, col++) {
    for (let ly = 5; ly + 2 < doorT; ly += 4) {
      fill(lit % 7 === col % 7 ? 'a' : '1', lx, ly, 2, 2);
    }
  }
  fill('0', doorL, doorT, doorW, doorH);
}

// drawTower draws a tall block: a full-width base for the lower 60% with windows, a middle block
// half as wide for the next 30%, and a two-wide spire in the roof colour for the top 10%.
function drawTower(fill, w, h, look) {
  const spireH = Math.round(h * 0.1);
  const baseTop = Math.round(h * 0.4);
  const midW = Math.round(w / 2);
  const midL = Math.round((w - midW) / 2);
  const spireL = Math.round((w - 2) / 2);
  fill(look.roof, spireL, 0, 2, spireH);
  fill(look.wall, midL, spireH, midW, baseTop - spireH);
  fill(look.wall, 0, baseTop, w, h - baseTop);
  for (let ly = baseTop + 2; ly + 2 <= h - 2; ly += 4) {
    for (let lx = 2; lx + 2 <= w - 2; lx += 4) fill('1', lx, ly, 2, 2);
  }
}

// drawBurj draws the Burj Moolifa: a two-wide spire in the wall colour on the top 8 rows (its light
// blinks pink at the tip), and under it a stepped needle of five equal segments, the bottom one
// full width and each one above it 0.2 narrower. The body is spotted with cow blotches.
function drawBurj(fill, w, h, look, time) {
  const spireH = 8;
  const spireL = Math.round((w - 2) / 2);
  fill(look.wall, spireL, 0, 2, spireH);
  if (Math.floor(time * 2) % 2 === 0) fill('e', spireL, 0, 2, 1);

  const body = h - spireH;
  const fractions = [0.2, 0.4, 0.6, 0.8, 1];
  const spans = [];
  for (let k = 0; k < fractions.length; k++) {
    const top = spireH + Math.round((k * body) / fractions.length);
    const bottom = spireH + Math.round(((k + 1) * body) / fractions.length);
    const sw = Math.round(w * fractions[k]);
    const left = Math.round((w - sw) / 2);
    fill(look.wall, left, top, sw, bottom - top);
    for (let r = top; r < bottom; r++) spans[r] = [left, left + sw - 1];
  }

  // A 3x2 blotch goes where the hash of its row and column says, if it fits inside the body.
  for (let r = spireH + 2; r + 1 < h; r += 5) {
    for (let c = 0; c + 2 < w; c += 5) {
      if (cell(r, c, 3) !== 0) continue;
      const a = spans[r];
      const b = spans[r + 1];
      if (!a || !b) continue;
      if (c >= a[0] && c + 2 <= a[1] && c >= b[0] && c + 2 <= b[1]) fill('0', c, r, 3, 2);
    }
  }
}

// drawSail draws the Burj Al Hen: a two-wide mast in '6' on the right edge, from the bottom row up
// to 3 rows below the top, with a rooster comb of three one-unit bumps in the roof colour above it,
// and a sail in the wall colour to its left, rounder in the middle and ribbed every 6 rows.
function drawSail(fill, w, h, look) {
  const right = w - 3;
  fill('6', w - 2, 3, 2, h - 3);
  fill(look.roof, w - 5, 1, 1, 2);
  fill(look.roof, w - 3, 0, 1, 3);
  fill(look.roof, w - 1, 0, 1, 2);
  for (let ly = 0; ly < h; ly++) {
    const r = h - 1 - ly;
    const sw = Math.min(w - 2, Math.round(w * Math.sin((Math.PI * (r + 1)) / (h + 1)) * 0.9));
    if (sw <= 0) continue;
    fill(look.wall, right - sw + 1, ly, sw, 1);
    if (r > 0 && r % 6 === 0) fill('6', right - sw + 1, ly, sw, 1);
  }
}

// drawMountain draws a triangle in the wall colour with its top 20% in the roof colour like snow,
// and a half-disc cave mouth in '0' at the bottom centre, of radius w / 8.
function drawMountain(fill, w, h, look) {
  const snowH = Math.round(h * 0.2);
  for (let ly = 0; ly < h; ly++) {
    const width = Math.max(1, Math.round((w * (ly + 1)) / h));
    const left = Math.round((w - width) / 2);
    fill(ly < snowH ? look.roof : look.wall, left, ly, width, 1);
  }
  const radius = Math.round(w / 8);
  const cx = Math.round((w - 1) / 2);
  for (let dy = 0; dy <= radius; dy++) {
    const hw = Math.round(Math.sqrt(radius * radius - dy * dy));
    fill('0', cx - hw, h - 1 - dy, 2 * hw + 1, 1);
  }
}

// drawBuilding draws one building of a housing tier. x is its left edge and baseY the ground line
// under it, so its bottom row is baseY - 1. Everything is painted inside its box of look.w by
// look.h, and nothing above row 0. time drives the blinking lights.
export function drawBuilding(ctx, look, x, baseY, time) {
  if (look.shape === 'none') return;
  const w = look.w;
  const h = look.h;
  const top = baseY - h;

  // fill paints a box given in building coordinates (column and row from the top-left corner),
  // clipped to the building's box and to row 0, and does nothing if nothing is left.
  const fill = (key, lx, ly, lw, lh) => {
    const L = Math.round(lx);
    const T = Math.round(ly);
    const c0 = Math.max(0, L);
    const c1 = Math.min(w, L + Math.round(lw));
    const r0 = Math.max(0, T);
    const r1 = Math.min(h, T + Math.round(lh));
    const ay0 = Math.max(0, top + r0);
    const ay1 = top + r1;
    if (c1 <= c0 || ay1 <= ay0) return;
    ctx.fillStyle = PALETTE[key];
    ctx.fillRect(x + c0, ay0, c1 - c0, ay1 - ay0);
  };

  switch (look.shape) {
    case 'shed': drawShed(fill, w, h, look); break;
    case 'barn': drawBarn(fill, w, h, look); break;
    case 'block': drawBlock(fill, w, h, look, time); break;
    case 'tower': drawTower(fill, w, h, look); break;
    case 'burj': drawBurj(fill, w, h, look, time); break;
    case 'sail': drawSail(fill, w, h, look); break;
    case 'mountain': drawMountain(fill, w, h, look); break;
    default: break;
  }
}

// housingDrawables returns one drawable per owned herd whose housing tier is drawn, each standing
// at its site. A tier of shape 'none' (the pasture, the coop) has no drawable.
export function housingDrawables(world) {
  const out = [];
  for (const kind of KINDS) {
    if (!hasKind(world, kind)) continue;
    const look = SPECIES[kind].tiers[world.herds[kind].tier].look;
    if (look.shape === 'none') continue;
    const site = SITES[kind];
    out.push({
      y: site.y,
      draw: (ctx) => drawBuilding(ctx, look, site.x, site.y, world.time),
    });
  }
  return out;
}
