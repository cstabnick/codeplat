// The animals: cows that gossip in whispers when Farmer Gary is not looking, chickens that peck and
// sometimes panic, and a pig that wallows in the mud. Imports only state.js and pixel.js; it
// touches no window or document on load.

import { PASTURE, YARD, MUD, farmerSees, isNight, say, logEvent, puff } from './state.js';
import { drawSprite, spriteSize } from './pixel.js';

const COW = [
  '..............6...6.',
  '..............77777.',
  '.777007777700777077.',
  '.770007777000777777.',
  '0777007777700777eeee',
  '0777777700777777eee0',
  '.77777770077777.....',
  '.777777777777777....',
  '..7ee77777777.......',
  '..77.77....77.77....',
  '..77.77....77.77....',
  '..00.00....00.00....',
];
const COW_LEGS_STEP = ['..77..77..77..77....', '..77..77..77..77....', '..00..00..00..00....'];
const CHICKEN = [
  '.....8..',
  '....777.',
  '....7079',
  '7...777.',
  '7777777.',
  '.777777.',
  '..7777..',
  '...9.9..',
];
const PIG = [
  '.........ee...',
  '..eeeeeeeeee..',
  '2eeeeeeeeee0e.',
  'eeeeeeeeeeeeff',
  'eeeeeeeeeeee2f',
  '.eeeeeeeeeee..',
  '..ee..ee.ee...',
  '..22..22.22...',
];

// Walk and peck frames, made by changing rows of the sprites above.
const COW_STEP = COW.slice(0, 9).concat(COW_LEGS_STEP);
const CHICKEN_PECK = [
  '........',
  '.....8..',
  '....777.',
  '7...7079',
  '7777777.',
  '.777777.',
  '..7777..',
  '...9.9..',
];
const CHICKEN_STEP = CHICKEN.slice(0, 7).concat(['..9..9..']);
// A pig in mud: the bottom two rows and a few splotches are mud-brown.
const MUDDY_PIG = PIG.map((row, r) =>
  Array.from(row)
    .map((ch, c) => {
      if (ch === '.') return '.';
      if (r >= 6) return '4';
      if ((r === 2 && (c === 4 || c === 5)) || (r === 4 && (c === 8 || c === 9))) return '4';
      return ch;
    })
    .join(''),
);
const SHADOW = ['..5555..', '.555555.', '..5555..'];

const HEIGHT = { cow: spriteSize(COW).h, chicken: spriteSize(CHICKEN).h, pig: spriteSize(PIG).h };

// Where each kind may stand, inclusive.
const COW_BOX = {
  x0: PASTURE.x + 10,
  x1: PASTURE.x + PASTURE.w - 10,
  y0: PASTURE.y + 10,
  y1: PASTURE.y + PASTURE.h - 10,
};
const CHICKEN_BOX = { x0: YARD.x, x1: YARD.x + YARD.w, y0: YARD.y, y1: YARD.y + YARD.h };
const PIG_BOX = {
  x0: MUD.x - 12,
  x1: MUD.x + MUD.w + 12,
  y0: MUD.y - 12,
  y1: MUD.y + MUD.h + 12,
};

const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
const dist = (ax, ay, bx, by) => Math.hypot(ax - bx, ay - by);
const feetY = (a) => a.y - a.lift;

function boxFor(a) {
  if (a.kind === 'cow') return COW_BOX;
  if (a.kind === 'chicken') return CHICKEN_BOX;
  return PIG_BOX;
}

function clampTo(a, box) {
  a.x = clamp(a.x, box.x0, box.x1);
  a.y = clamp(a.y, box.y0, box.y1);
}

// A bubble whose tail points just above the animal's head.
function bubble(world, a, text, kind, ttl) {
  say(world, a.x, feetY(a) - HEIGHT[a.kind] - 1, text, kind, ttl);
}

// stepToward moves a toward (tx, ty) at speed units per second, without going closer than stop
// units. It faces the direction of horizontal movement. arrived is true when a is within stop.
function stepToward(a, tx, ty, speed, dt, stop = 1) {
  const dx = tx - a.x;
  const dy = ty - a.y;
  const d = Math.hypot(dx, dy);
  if (d <= stop) return { moved: false, arrived: true };
  const step = Math.min(speed * dt, d - stop);
  a.x += (dx / d) * step;
  a.y += (dy / d) * step;
  if (dx > 0) a.facing = 'right';
  else if (dx < 0) a.facing = 'left';
  return { moved: true, arrived: Math.hypot(tx - a.x, ty - a.y) <= stop + 1e-9 };
}

// randomNear returns a random point within r units of (x, y), clamped to box.
function randomNear(rng, x, y, r, box) {
  const angle = rng.range(0, 2 * Math.PI);
  const radius = rng.range(0, r);
  return {
    x: clamp(x + Math.cos(angle) * radius, box.x0, box.x1),
    y: clamp(y + Math.sin(angle) * radius, box.y0, box.y1),
  };
}

function toGraze(a, rng) {
  a.state = 'graze';
  a.partner = null;
  a.target = null;
  a.timer = rng.range(2, 5);
}

function faceEachOther(a, b) {
  if (b.x > a.x) a.facing = 'right';
  else if (b.x < a.x) a.facing = 'left';
  if (a.x > b.x) b.facing = 'right';
  else if (a.x < b.x) b.facing = 'left';
}

// startGossip begins a gossip between a and another cow the farmer cannot see, if one is near.
// Both cows are added to done so that this update does not move either of them again.
function startGossip(world, a, dt, rng, line, done) {
  if (farmerSees(world, a.x, a.y)) return false;
  const b = world.animals.find(
    (o) =>
      o !== a &&
      o.kind === 'cow' &&
      !o.abducted &&
      (o.state === 'graze' || o.state === 'walk') &&
      !farmerSees(world, o.x, o.y) &&
      dist(a.x, a.y, o.x, o.y) <= 48,
  );
  if (!b || !rng.chance(0.35 * dt)) return false;

  const first = world.animals.indexOf(a) < world.animals.indexOf(b) ? a : b;
  const second = first === a ? b : a;
  a.state = 'gossip';
  b.state = 'gossip';
  a.partner = b.id;
  b.partner = a.id;
  a.target = null;
  b.target = null;
  a.timer = 7;
  b.timer = 7;
  faceEachOther(a, b);
  world.stats.gossip += 1;
  logEvent(world, `${first.name.toUpperCase()} AND ${second.name.toUpperCase()} ARE GOSSIPING`);

  const lower = a.id < b.id ? a : b;
  bubble(world, lower, line('gossip'), 'whisper', 2.2);
  done.add(a.id);
  done.add(b.id);
  return true;
}

// gossipStep runs one update of a gossip. a is the first of the pair in the animals array and
// walks toward b; the lower id speaks on even beats and the higher id on odd beats.
function gossipStep(world, a, b, dt, rng, line) {
  const lower = a.id < b.id ? a : b;
  const higher = lower === a ? b : a;

  if (farmerSees(world, a.x, a.y) || farmerSees(world, b.x, b.y)) {
    for (const c of [a, b]) {
      c.state = 'innocent';
      c.frame = 0;
      c.timer = 2.5;
      c.target = null;
      c.facing = world.farmer.x < c.x ? 'left' : 'right';
    }
    bubble(world, lower, line('innocent'), 'speech');
    puff(world, 'note', higher.x, feetY(higher) - HEIGHT[higher.kind]);
    return;
  }

  const walk = stepToward(a, b.x, b.y, 8, dt, 14);
  a.frame = walk.moved ? Math.floor(world.time * 6) % 2 : 0;
  b.frame = 0;

  const before = a.timer;
  const after = before - dt;
  const prevBeat = Math.floor(Math.max(0, 7 - before) / 1.6);
  const newBeat = Math.floor(Math.min(7, 7 - Math.max(after, 0)) / 1.6);
  for (let k = prevBeat + 1; k <= newBeat; k++) {
    bubble(world, k % 2 === 0 ? lower : higher, line('gossip'), 'whisper', 2.2);
  }

  a.timer = after;
  b.timer = after;
  if (after <= 0) {
    toGraze(a, rng);
    toGraze(b, rng);
    return;
  }
  faceEachOther(a, b);
}

function updateCow(world, a, dt, rng, line, done) {
  if (a.state === 'gossip') {
    const partner = world.animals.find((o) => o.id === a.partner);
    if (!partner || partner.abducted) {
      toGraze(a, rng);
      return;
    }
    done.add(partner.id);
    gossipStep(world, a, partner, dt, rng, line);
    return;
  }

  if (a.state === 'innocent') {
    a.frame = 0;
    a.timer -= dt;
    if (a.timer <= 0) toGraze(a, rng);
    return;
  }

  if (startGossip(world, a, dt, rng, line, done)) return;

  if (a.state === 'graze') {
    a.frame = Math.floor(world.time) % 2;
    a.timer -= dt;
    if (a.timer <= 0) {
      if (rng.chance(0.5)) {
        a.target = { x: rng.range(COW_BOX.x0, COW_BOX.x1), y: rng.range(COW_BOX.y0, COW_BOX.y1) };
        a.state = 'walk';
      } else {
        a.timer = rng.range(2, 5);
      }
    }
  } else if (a.state === 'walk') {
    a.frame = Math.floor(world.time * 6) % 2;
    if (!a.target || stepToward(a, a.target.x, a.target.y, 8, dt).arrived) {
      a.state = 'graze';
      a.target = null;
      a.timer = rng.range(2, 5);
    }
  }
}

function startPanic(world, a, rng, line) {
  a.state = 'panic';
  a.timer = 3;
  a.partner = null;
  a.target = randomNear(rng, a.x, a.y, 20, YARD);
  bubble(world, a, line('chicken'), 'speech');
}

// updatePanic moves a panicking chicken in zig-zags, one new target every 0.3 s, and panics
// awake chickens within 16 units. It calms down to peck after 3 s.
function updatePanic(world, a, dt, rng) {
  a.frame = Math.floor(world.time * 6) % 2;
  const before = a.timer;
  const after = before - dt;
  const prevBeat = Math.floor(Math.max(0, 3 - before) / 0.3);
  const newBeat = Math.floor(Math.min(3, 3 - Math.max(after, 0)) / 0.3);
  if (newBeat > prevBeat) a.target = randomNear(rng, a.x, a.y, 20, YARD);
  if (a.target) stepToward(a, a.target.x, a.target.y, 40, dt);

  for (const o of world.animals) {
    if (o === a || o.abducted || o.kind !== 'chicken') continue;
    if ((o.state === 'peck' || o.state === 'walk') && dist(a.x, a.y, o.x, o.y) <= 16) {
      o.state = 'panic';
      o.timer = 3;
      o.partner = null;
      o.target = randomNear(rng, o.x, o.y, 20, YARD);
    }
  }

  if (rng.chance(4 * dt)) {
    puff(world, 'feather', a.x, feetY(a) - 4, rng.range(-10, 10), -4, 1);
  }

  a.timer = after;
  if (after <= 0) {
    a.state = 'peck';
    a.target = null;
    a.timer = rng.range(2, 5);
  }
}

function updateChicken(world, a, dt, rng, line) {
  const night = isNight(world);
  if (a.state === 'sleep') {
    a.frame = 0;
    if (!night) {
      a.state = 'peck';
      a.timer = rng.range(2, 5);
    } else if (rng.chance(0.02 * dt)) {
      puff(world, 'zzz', a.x, feetY(a) - HEIGHT.chicken);
    }
    return;
  }

  if (a.state === 'panic') {
    updatePanic(world, a, dt, rng);
    return;
  }

  if (a.state === 'peck' || a.state === 'walk') {
    if (night) {
      a.state = 'sleep';
      a.target = null;
      a.frame = 0;
      return;
    }
    if (rng.chance(0.01 * dt)) {
      startPanic(world, a, rng, line);
      return;
    }
  }

  if (a.state === 'peck') {
    a.frame = Math.floor(world.time / 0.3) % 2;
    if (rng.chance(0.2 * dt)) {
      a.target = randomNear(rng, a.x, a.y, 20, CHICKEN_BOX);
      a.state = 'walk';
    }
  } else if (a.state === 'walk') {
    a.frame = Math.floor(world.time * 6) % 2;
    if (!a.target || stepToward(a, a.target.x, a.target.y, 12, dt).arrived) {
      a.state = 'peck';
      a.target = null;
      a.timer = rng.range(2, 5);
    }
  }
}

function updatePig(world, a, dt, rng, line) {
  const night = isNight(world);
  if (a.state === 'sleep') {
    a.frame = 0;
    if (!night) {
      a.state = 'wallow';
      a.timer = rng.range(2, 5);
    } else if (rng.chance(0.02 * dt)) {
      puff(world, 'zzz', a.x, feetY(a) - HEIGHT.pig);
    }
    return;
  }

  if (night) {
    a.state = 'sleep';
    a.target = null;
    a.frame = 0;
    return;
  }

  if (a.state === 'wallow') {
    a.frame = 0;
    if (rng.chance(0.1 * dt)) puff(world, 'mud', a.x, feetY(a));
    if (rng.chance(0.04 * dt)) bubble(world, a, line('pig'), 'thought', 4);
    if (rng.chance(0.05 * dt)) {
      a.target = randomNear(rng, a.x, a.y, 6, PIG_BOX);
      a.state = 'walk';
    }
  } else if (a.state === 'walk') {
    a.frame = Math.floor(world.time * 6) % 2;
    if (!a.target || stepToward(a, a.target.x, a.target.y, 4, dt).arrived) {
      a.state = 'wallow';
      a.target = null;
      a.timer = rng.range(2, 5);
    }
  }
}

// updateAnimals advances every animal whose abducted is false. line(kind) returns the text for a
// gag of that kind.
export function updateAnimals(world, dt, rng, line) {
  const animals = world.animals;

  // Smoke reaction: the first cow that has not yet noticed the latest cigarette says so. Every
  // cow records the current count, so each cigarette is noticed once.
  const cigarettes = world.stats.cigarettes;
  let smokeSaid = false;
  for (const a of animals) {
    if (a.abducted || a.kind !== 'cow') continue;
    if (world.farmer.state === 'smoke' && cigarettes > a.noticed && !smokeSaid) {
      smokeSaid = true;
      bubble(world, a, line('smoke'), 'whisper');
    }
    a.noticed = cigarettes;
  }

  const done = new Set();
  for (const a of animals) {
    if (a.abducted || done.has(a.id)) continue;
    if (a.kind === 'cow') updateCow(world, a, dt, rng, line, done);
    else if (a.kind === 'chicken') updateChicken(world, a, dt, rng, line);
    else if (a.kind === 'pig') updatePig(world, a, dt, rng, line);
    clampTo(a, boxFor(a));
  }
}

// spriteRows picks the rows to draw for an animal in its current state and frame.
function spriteRows(a) {
  if (a.kind === 'cow') {
    return a.frame === 1 && (a.state === 'walk' || a.state === 'gossip') ? COW_STEP : COW;
  }
  if (a.kind === 'chicken') {
    if (a.frame === 1 && a.state === 'peck') return CHICKEN_PECK;
    if (a.frame === 1 && (a.state === 'walk' || a.state === 'panic')) return CHICKEN_STEP;
    return CHICKEN;
  }
  return a.state === 'wallow' || a.state === 'sleep' ? MUDDY_PIG : PIG;
}

function drawAnimal(ctx, a) {
  const fy = feetY(a);
  if (a.kind === 'cow' && a.lift > 0) drawSprite(ctx, SHADOW, a.x - 4, a.y - 1);

  const rows = spriteRows(a);
  const base = HEIGHT[a.kind];
  const left = a.x - Math.floor(spriteSize(rows).w / 2);
  const flip = a.facing === 'left';
  // A sleeping animal loses its top row and sits one unit lower, with its feet where they were.
  if (a.state === 'sleep') drawSprite(ctx, rows.slice(1), left, fy - base + 1, flip);
  else drawSprite(ctx, rows, left, fy - base, flip);
}

// animalDrawables returns one {y, draw(ctx)} per animal, for the caller to sort by y.
export function animalDrawables(world) {
  return world.animals.map((a) => ({ y: a.y, draw: (ctx) => drawAnimal(ctx, a) }));
}

// animalAt returns the animal whose feet are nearest to (x, y) within 14 units, or null.
export function animalAt(world, x, y) {
  let best = null;
  let bestDistance = Infinity;
  for (const a of world.animals) {
    const d = dist(a.x, feetY(a), x, y);
    if (d <= 14 && d < bestDistance) {
      best = a;
      bestDistance = d;
    }
  }
  return best;
}
