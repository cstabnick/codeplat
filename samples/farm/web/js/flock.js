// The flock: a few sheep and goats drawn on the Sheep Meadow and Goat Hill to stand for their herds.
// Males have horns and young ones are small. Each one grazes, then walks to a new point in its
// area. Imports only state.js and pixel.js; it touches no window or document on load.

import { MEADOW, HILL, herdTotal, hasKind } from './state.js';
import { drawSprite, spriteSize } from './pixel.js';

// FLOCK_MAX is the most entries shown per kind, however large the herd.
export const FLOCK_MAX = 10;

const AREA = { sheep: MEADOW, goat: HILL };
const SPEED = { sheep: 6, goat: 10 };
const GROUPS = ['young', 'male', 'female'];

// Sprites face right. An adult has two leg rows at the bottom; on frame 1 of a walk the first is
// replaced by a second stance, made by stepRows.
const SHEEP = [
  '....77777.....',
  '..777777777...',
  '.77777777777..',
  '.7766666677770',
  '.7766666677700',
  '..766666666770',
  '..77777777....',
  '..77777777....',
  '..0.0.....0.0.',
  '..0.0.....0.0.',
];
const RAM = [
  '....77777.99..',
  '..777777777.9.',
].concat(SHEEP.slice(2));
const LAMB = [
  '...777...',
  '..77777..',
  '.7777777.',
  '.77777770',
  '..777777.',
  '..0.0.0..',
  '..0...0..',
];
const GOAT = [
  '........5.5..',
  '.66666666655.',
  '.666666666666',
  '.66666666666.',
  '.66666666666.',
  '.66666666655.',
  '..5555555....',
  '.66666666666.',
  '.6.6.....6.6.',
  '.0.0.....0.0.',
];
const BILLY = [
  '.......5.5.5.',
  '.66666666655.',
  '.666666666666',
  '.66666666666.',
  '.66666666666.',
  '.66666666655.',
  '..55555555555',
  '.66666666666.',
  '.6.6.....6.6.',
  '.0.0.....0.0.',
];
const KID = [
  '..77....',
  '.777777.',
  '.7777770',
  '.666666.',
  '..666...',
  '..0...0.',
  '..0...0.',
];

// stepRows returns an adult's rows with its first leg row replaced by the stance given.
function stepRows(rows, stance) {
  return rows.slice(0, -2).concat([stance, rows[rows.length - 1]]);
}

const SHEEP_STEP = stepRows(SHEEP, '.0..0....0..0.');
const RAM_STEP = stepRows(RAM, '.0..0....0..0.');
const GOAT_STEP = stepRows(GOAT, '.6..6....6.6.');
const BILLY_STEP = stepRows(BILLY, '.6..6....6.6.');

// rowsFor picks the sprite for an entry: a walking stance on frame 1 while walking.
function rowsFor(e) {
  const walking = e.state === 'walk' && e.frame === 1;
  if (e.kind === 'sheep') {
    if (e.young) return LAMB;
    if (e.sex === 'male') return walking ? RAM_STEP : RAM;
    return walking ? SHEEP_STEP : SHEEP;
  }
  if (e.young) return KID;
  if (e.sex === 'male') return walking ? BILLY_STEP : BILLY;
  return walking ? GOAT_STEP : GOAT;
}

// clamp keeps v within [lo, hi].
function clamp(v, lo, hi) {
  return Math.min(hi, Math.max(lo, v));
}

// randomPoint returns a random point inside area, edges included.
function randomPoint(rng, area) {
  return { x: rng.range(area.x, area.x + area.w), y: rng.range(area.y, area.y + area.h) };
}

// groupOf names the group an entry belongs to: 'young', 'male' or 'female'.
function groupOf(e) {
  if (e.young) return 'young';
  return e.sex === 'male' ? 'male' : 'female';
}

// groupCounts splits the shown entries of a herd into young, male and adult female counts.
function groupCounts(h, total) {
  if (total === 0) return { young: 0, male: 0, female: 0 };
  const target = Math.min(FLOCK_MAX, total);
  const young = Math.round(target * h.young / total);
  const male = Math.round((target - young) * h.males / Math.max(1, h.females + h.males));
  return { young, male, female: target - young - male };
}

// newEntry makes a graze-state entry of a group at a random spot in its area.
function newEntry(kind, group, rng) {
  const p = randomPoint(rng, AREA[kind]);
  return {
    kind,
    sex: group === 'male' ? 'male' : 'female',
    young: group === 'young',
    x: p.x,
    y: p.y,
    facing: rng.chance(0.5) ? 'left' : 'right',
    state: 'graze',
    target: null,
    timer: rng.range(2, 5),
    frame: 0,
  };
}

// syncKind makes the entries of one kind match its herd. Surplus entries of a group are dropped
// from the end; missing ones are added at random points.
function syncKind(world, kind, rng) {
  const want = groupCounts(world.herds[kind], herdTotal(world, kind));
  const have = { young: 0, male: 0, female: 0 };
  const next = [];
  for (const e of world.flock) {
    if (e.kind !== kind) {
      next.push(e);
      continue;
    }
    const g = groupOf(e);
    if (have[g] < want[g]) {
      have[g] += 1;
      next.push(e);
    }
  }
  for (const g of GROUPS) {
    for (; have[g] < want[g]; have[g] += 1) next.push(newEntry(kind, g, rng));
  }
  world.flock.splice(0, world.flock.length, ...next);
}

// walkStep moves an entry toward its target by at most dist units. On arrival it stops exactly on
// the target and goes back to grazing for a random time.
function walkStep(world, e, dist, rng) {
  const dx = e.target.x - e.x;
  const dy = e.target.y - e.y;
  const d = Math.hypot(dx, dy);
  e.facing = dx < 0 ? 'left' : 'right';
  e.frame = Math.floor(world.time * 6) % 2;
  if (d <= dist) {
    e.x = e.target.x;
    e.y = e.target.y;
    e.timer = rng.range(2, 5);
    e.target = null;
    e.state = 'graze';
    return;
  }
  const area = AREA[e.kind];
  e.x = clamp(e.x + (dx / d) * dist, area.x, area.x + area.w);
  e.y = clamp(e.y + (dy / d) * dist, area.y, area.y + area.h);
}

// updateEntry grazes an entry until its timer runs out, then walks it to a new point in its area.
function updateEntry(world, e, dt, rng) {
  if (e.state === 'graze') {
    e.timer -= dt;
    if (e.timer <= 0) {
      e.target = randomPoint(rng, AREA[e.kind]);
      e.state = 'walk';
    }
    return;
  }
  walkStep(world, e, SPEED[e.kind] * dt, rng);
}

// updateFlock keeps world.flock in step with the sheep and goat herds, and moves each entry.
export function updateFlock(world, dt, rng) {
  for (const kind of ['sheep', 'goat']) {
    if (hasKind(world, kind)) {
      syncKind(world, kind, rng);
    } else {
      world.flock.splice(0, world.flock.length, ...world.flock.filter((e) => e.kind !== kind));
    }
  }
  for (const e of world.flock) updateEntry(world, e, dt, rng);
}

// flockDrawables returns one {y, draw(ctx)} per entry of world.flock. The sprite's feet are at
// (x, y), and it is mirrored when facing left.
export function flockDrawables(world) {
  return world.flock.map((e) => ({
    y: e.y,
    draw(ctx) {
      const rows = rowsFor(e);
      const { w, h } = spriteSize(rows);
      drawSprite(ctx, rows, e.x - Math.floor(w / 2), e.y - h, e.facing === 'left');
    },
  }));
}
