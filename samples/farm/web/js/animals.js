// The animals: cows that gossip in whispers when Farmer Gary is not looking, chickens that peck and
// sometimes panic, and a pig that wallows in the mud and is secretly a prince from another
// dimension. Cows and chickens are born small, grow up, have young while there is room, and die
// of old age. Imports only state.js and pixel.js; it touches no window or document on load.

import {
  PASTURE, YARD, MUD, ADULT_AGE, farmerSees, isNight, isAdult, herdSize, capacity, say, logEvent,
  puff,
} from './state.js';
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
const CALF = [
  '.........6..',
  '........7770',
  '.770077777ee',
  '077007777ee.',
  '.77777777...',
  '.7.7...7.7..',
  '.7.7...7.7..',
  '.0.0...0.0..',
];
const CHICK = [
  '..aa.',
  '..a09',
  'aaaa.',
  '.9.9.',
];
const CROWN = ['a.a.a', 'aaaaa'];

// How long each kind lives and how often an adult has young, in seconds: [min, max).
// Gossip: a hidden pair of cows starts one at GOSSIP_RATE per second, it lasts GOSSIP_SECONDS, and
// the pair take turns to whisper every GOSSIP_BEAT seconds.
const GOSSIP_RATE = 0.15;
const GOSSIP_SECONDS = 7;
const GOSSIP_BEAT = 2.4;

const LIFE = { cow: [900, 1260], chicken: [540, 720] };
const BREED = { cow: [150, 240], chicken: [80, 140] };
const YOUNG = { cow: 'CALF', chicken: 'CHICK' };
const HEAVEN = { cow: 'PASTURE', chicken: 'COOP' };
const NAMES = {
  cow: [
    'Daisy', 'Bessie', 'Moolinda', 'Clarabelle', 'Buttercup', 'Marigold', 'Dolly', 'Penelope',
    'Mabel', 'Gertrude', 'Maude', 'Hazel', 'Rosie', 'Butterscotch', 'Milkshake', 'Oreo', 'Pudding',
    'Biscuit', 'Nutmeg', 'Waffles', 'Moonpie', 'Doris', 'Agnes', 'Moozart', 'Cowlette',
  ],
  chicken: [
    'Henrietta', 'Cluck Norris', 'Eggatha', 'Nugget', 'Pecky', 'Omelette', 'Drumstick', 'Peep',
    'Yolko Ono', 'Hen Solo', 'Clucky', 'Sunny', 'Scramble', 'Popcorn', 'Goldie', 'Penny', 'Waddles',
    'Beaky', 'Eggbert', 'Feathers', 'Mother Clucker', 'Pip', 'Beakonce', 'Henny', 'Chickpea',
    'Dumpling', 'Mrs Peck', 'Sir Cluckington',
  ],
};

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

// baseRows is the standing sprite for an animal's kind and age.
function baseRows(a) {
  if (a.kind === 'cow') return isAdult(a) ? COW : CALF;
  if (a.kind === 'chicken') return isAdult(a) ? CHICKEN : CHICK;
  return PIG;
}

const heightOf = (a) => spriteSize(baseRows(a)).h;

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
  say(world, a.x, feetY(a) - heightOf(a) - 1, text, kind, ttl);
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
  if (!b || !rng.chance(GOSSIP_RATE * dt)) return false;

  const first = world.animals.indexOf(a) < world.animals.indexOf(b) ? a : b;
  const second = first === a ? b : a;
  a.state = 'gossip';
  b.state = 'gossip';
  a.partner = b.id;
  b.partner = a.id;
  a.target = null;
  b.target = null;
  a.timer = GOSSIP_SECONDS;
  b.timer = GOSSIP_SECONDS;
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
    puff(world, 'note', higher.x, feetY(higher) - heightOf(higher));
    return;
  }

  const walk = stepToward(a, b.x, b.y, 8, dt, 14);
  a.frame = walk.moved ? Math.floor(world.time * 6) % 2 : 0;
  b.frame = 0;

  const before = a.timer;
  const after = before - dt;
  const prevBeat = Math.floor(Math.max(0, GOSSIP_SECONDS - before) / GOSSIP_BEAT);
  const newBeat = Math.floor(Math.min(GOSSIP_SECONDS, GOSSIP_SECONDS - Math.max(after, 0)) / GOSSIP_BEAT);
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
  a.target = randomNear(rng, a.x, a.y, 20, CHICKEN_BOX);
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
  if (newBeat > prevBeat) a.target = randomNear(rng, a.x, a.y, 20, CHICKEN_BOX);
  if (a.target) stepToward(a, a.target.x, a.target.y, 40, dt);

  for (const o of world.animals) {
    if (o === a || o.abducted || o.kind !== 'chicken') continue;
    if ((o.state === 'peck' || o.state === 'walk') && dist(a.x, a.y, o.x, o.y) <= 16) {
      o.state = 'panic';
      o.timer = 3;
      o.partner = null;
      o.target = randomNear(rng, o.x, o.y, 20, CHICKEN_BOX);
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
      puff(world, 'zzz', a.x, feetY(a) - heightOf(a));
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
      puff(world, 'zzz', a.x, feetY(a) - heightOf(a));
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
    if (rng.chance(0.3 * dt)) {
      puff(world, 'portal', a.x + rng.range(-8, 8), feetY(a) - rng.range(0, 10), 0, -4, 1.2);
    }
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

// pickName returns a name from NAMES that no living animal has, or a numbered one when all are taken.
function pickName(world, kind, rng) {
  const taken = new Set(world.animals.map((a) => a.name));
  const free = NAMES[kind].filter((n) => !taken.has(n));
  if (free.length > 0) return rng.pick(free);
  const base = rng.pick(NAMES[kind]);
  let n = 2;
  while (taken.has(`${base} ${n}`)) n += 1;
  return `${base} ${n}`;
}

// addAnimal puts a new cow or chicken on the farm and returns it. Without opts.x and opts.y it
// stands at a random spot in its area; opts.age is its age in seconds, an adult's by default.
export function addAnimal(world, kind, rng, opts = {}) {
  const box = kind === 'cow' ? COW_BOX : CHICKEN_BOX;
  const a = {
    id: `${kind}-${world.nextAnimal}`,
    kind,
    name: pickName(world, kind, rng),
    x: clamp(opts.x ?? rng.range(box.x0, box.x1), box.x0, box.x1),
    y: clamp(opts.y ?? rng.range(box.y0, box.y1), box.y0, box.y1),
    facing: rng.chance(0.5) ? 'right' : 'left',
    state: kind === 'cow' ? 'graze' : 'peck',
    target: null,
    timer: 1,
    frame: 0,
    partner: null,
    noticed: world.stats.cigarettes,
    lift: 0,
    abducted: false,
    age: opts.age ?? ADULT_AGE[kind],
    life: rng.range(...LIFE[kind]),
    breed: rng.range(...BREED[kind]),
  };
  world.nextAnimal += 1;
  world.animals.push(a);
  return a;
}

// age ages a cow or chicken by dt. It returns true when the animal has died, for the caller to
// remove.
function age(world, a, dt, line) {
  a.age += dt;
  if (a.age < a.life) return false;
  world.stats.deaths += 1;
  bubble(world, a, line('death'), 'speech', 4);
  puff(world, 'ghost', a.x - 2, feetY(a) - heightOf(a), 0, -8, 3);
  logEvent(world, `${a.name.toUpperCase()} WENT TO THE BIG ${HEAVEN[a.kind]} IN THE SKY`);
  return true;
}

// breed counts down an adult's breeding timer by dt. When it runs out, the adult has young if
// another adult of its kind is on the ground and the farm has room. The dead of this update are
// already gone, so they are neither mates nor counted in the herd.
function breed(world, a, dt, rng, line) {
  if (!isAdult(a)) return;
  a.breed -= dt;
  if (a.breed > 0) return;
  a.breed = rng.range(...BREED[a.kind]);
  if (herdSize(world, a.kind) >= capacity(world, a.kind)) return;
  const mate = world.animals.some((o) => o !== a && o.kind === a.kind && !o.abducted && isAdult(o));
  if (!mate) return;

  const baby = addAnimal(world, a.kind, rng, { x: a.x + rng.range(-10, 10), y: a.y + rng.range(-4, 4), age: 0 });
  world.stats.births += 1;
  puff(world, 'heart', a.x - 2, feetY(a) - heightOf(a) - 4, 0, -6, 1.5);
  bubble(world, a, line('birth'), 'speech');
  logEvent(world, `${a.name.toUpperCase()} HAD A ${YOUNG[a.kind]}: ${baby.name.toUpperCase()}`);
}

// updateAnimals advances every animal whose abducted is false. line(kind) returns the text for a
// gag of that kind. Cows and chickens age first, those that die this update are removed, and then
// the adults breed.
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

  const living = (a) => !a.abducted && a.kind !== 'pig';
  const dead = new Set(animals.filter((a) => living(a) && age(world, a, dt, line)));
  if (dead.size > 0) world.animals = world.animals.filter((a) => !dead.has(a));
  for (const a of world.animals.filter(living)) breed(world, a, dt, rng, line);

  const done = new Set();
  for (const a of world.animals) {
    if (a.abducted || done.has(a.id)) continue;
    if (a.kind === 'cow') updateCow(world, a, dt, rng, line, done);
    else if (a.kind === 'chicken') updateChicken(world, a, dt, rng, line);
    else if (a.kind === 'pig') updatePig(world, a, dt, rng, line);
    clampTo(a, boxFor(a));
  }
}

// spriteRows picks the rows to draw for an animal in its current state and frame.
function spriteRows(a) {
  if (!isAdult(a)) return baseRows(a);
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
  const base = heightOf(a);
  const w = spriteSize(rows).w;
  const left = a.x - Math.floor(w / 2);
  const flip = a.facing === 'left';
  // A sleeping animal loses its top row and sits one unit lower, with its feet where they were.
  const top = a.state === 'sleep' ? fy - base + 1 : fy - base;
  if (a.state === 'sleep') drawSprite(ctx, rows.slice(1), left, top, flip);
  else drawSprite(ctx, rows, left, top, flip);
  // The prince's crown sits between his ears.
  if (a.kind === 'pig') drawSprite(ctx, CROWN, flip ? left + 1 : left + w - 6, top - 2);
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
