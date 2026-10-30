// The animals: cows that gossip in whispers when Farmer Gary is not looking, chickens that peck and
// sometimes panic, and a pig that wallows in the mud and is secretly a prince from another
// dimension. The cows and chickens drawn here are a sample of their herds (herds.js owns the
// counts): bulls and cocks stand among them, and births, deaths and purchases show as hearts,
// ghosts and portals. Imports only state.js and pixel.js; it touches no window or document on load.

import {
  PASTURE, YARD, MUD, farmerSees, isNight, hourOf, herdTotal, money, say, logEvent, puff,
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

// A bull is a cow with a brown body, no udder and white horns on the head row. Its legs are brown
// too, so its walk frame is made from its own rows.
const BULL = COW.map((row, r) => {
  let s = row.replace(/7/g, '4');
  if (r === 8) s = s.replace(/e/g, '4');
  if (r === 0) s = s.replace(/6/g, '7');
  return s;
});
const BULL_LEGS_STEP = COW_LEGS_STEP.map((row) => row.replace(/7/g, '4'));
const BULL_STEP = BULL.slice(0, 9).concat(BULL_LEGS_STEP);

// A cock is a hen with a two-row red comb and tail feathers on the left, one row taller than a hen.
const COCK = [
  '.....8..',
  '....88..',
  '....777.',
  '....7079',
  'c...777.',
  '9777777.',
  '.777777.',
  '..7777..',
  '...9.9..',
];
const COCK_PECK = [
  '........',
  '.....8..',
  '....88..',
  '....777.',
  'c...7079',
  '9777777.',
  '.777777.',
  '..7777..',
  '...9.9..',
];
const COCK_STEP = COCK.slice(0, 8).concat(['..9..9..']);

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

// Gossip: a hidden pair of cows starts one at GOSSIP_RATE per second, it lasts GOSSIP_SECONDS, and
// the pair take turns to whisper every GOSSIP_BEAT seconds.
const GOSSIP_RATE = 0.15;
const GOSSIP_SECONDS = 7;
const GOSSIP_BEAT = 2.4;

// VISIBLE is how many of each kind the scene draws at most, the rest of the herd being off-stage.
export const VISIBLE = { cow: 12, chicken: 14 };

// LIFE_GAP is how many seconds of world time must pass between two gated bubbles about life and
// death (births and the death group).
const LIFE_GAP = 6;

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

// isMale is true for an adult male. A missing sex counts as female.
function isMale(a) {
  return !a.young && a.sex === 'male';
}

// baseRows is the standing sprite for an animal's kind, sex and youth.
function baseRows(a) {
  if (a.kind === 'cow') {
    if (a.young) return CALF;
    return isMale(a) ? BULL : COW;
  }
  if (a.kind === 'chicken') {
    if (a.young) return CHICK;
    return isMale(a) ? COCK : CHICKEN;
  }
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
// Both cows are added to done so that this update does not move either of them again. The first
// whisper is by a bull a third of the time, when the lower id is a bull.
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
  const bullSpeaks = isMale(lower) && rng.chance(0.3);
  bubble(world, lower, line(bullSpeaks ? 'bull' : 'gossip'), 'whisper', 2.2);
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

  // A cock crows once in a while between 6 and 6:30 in the morning while it is awake.
  const hour = hourOf(world);
  if (isMale(a) && hour >= 6 && hour < 6.5 && rng.chance(0.2 * dt)) {
    bubble(world, a, line('rooster'), 'speech');
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
// stands at a random spot in its area. opts.sex is 'female' or 'male' and opts.young says whether
// it is a calf or chick; they default to a female adult.
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
    sex: opts.sex ?? 'female',
    young: opts.young ?? false,
  };
  world.nextAnimal += 1;
  world.animals.push(a);
  return a;
}

// gateOpen is true when a gated bubble or log line may show now.
function gateOpen(world) {
  return world.time >= (world.lifeQuietUntil ?? 0);
}

// closeGate starts the quiet time after a gated line has shown.
function closeGate(world) {
  world.lifeQuietUntil = world.time + LIFE_GAP;
}

// visibleOf lists the animals of a kind in the scene.
function visibleOf(world, kind) {
  return world.animals.filter((a) => a.kind === kind);
}

// birthEvent puts a heart over a visible female of the kind and, if the gate is open, says the
// birth line over her.
function birthEvent(world, kind, rng, line) {
  const mothers = visibleOf(world, kind).filter((a) => !isMale(a));
  if (mothers.length === 0) return;
  const mother = rng.pick(mothers);
  puff(world, 'heart', mother.x - 2, feetY(mother) - heightOf(mother) - 4, 0, -6, 1.5);
  if (!gateOpen(world)) return;
  bubble(world, mother, line('birth'), 'speech');
  closeGate(world);
}

// deathGroup puts a ghost over a visible adult of the kind that is not abducted, says the death
// line over it and logs the deaths of this update. A closed gate or no such adult drops it all.
function deathGroup(world, kind, count, rng, line) {
  if (!gateOpen(world)) return;
  const adults = visibleOf(world, kind).filter((a) => !a.young && !a.abducted);
  if (adults.length === 0) return;
  const a = rng.pick(adults);
  puff(world, 'ghost', a.x - 2, feetY(a) - heightOf(a), 0, -8, 3);
  bubble(world, a, line('death'), 'speech', 4);
  const more = count > 1 ? ` AND ${money(count - 1)} MORE` : '';
  logEvent(world, `${a.name.toUpperCase()} WENT TO THE BIG ${HEAVEN[kind]} IN THE SKY${more}`);
  closeGate(world);
}

// herdEventsStep reads the herd events of this update, then clears them. Grow events are ignored.
// Each death group is handled once per kind, with the sum of its deaths.
function herdEventsStep(world, rng, line) {
  const events = world.herdEvents;
  world.herdEvents = [];
  const handled = new Set();
  for (const ev of events) {
    if (ev.kind !== 'cow' && ev.kind !== 'chicken') continue;
    if (ev.type === 'birth') {
      birthEvent(world, ev.kind, rng, line);
    } else if (ev.type === 'buy') {
      const list = visibleOf(world, ev.kind);
      if (list.length === 0) continue;
      const a = rng.pick(list);
      puff(world, 'portal', a.x, feetY(a) - heightOf(a) / 2, 0, -4, 1.5);
    } else if (ev.type === 'death' && !handled.has(ev.kind)) {
      handled.add(ev.kind);
      const count = events
        .filter((e) => e.type === 'death' && e.kind === ev.kind)
        .reduce((sum, e) => sum + e.count, 0);
      deathGroup(world, ev.kind, count, rng, line);
    }
  }
}

// dropAnimals removes up to n animals of a group, newest first. It never removes an abducted one,
// and it removes a gossiping one only when no other in the group can go.
function dropAnimals(world, group, n) {
  const newest = group.slice().reverse();
  const victims = new Set();
  // First pass spares gossiping animals; the second pass allows them.
  const passes = [(a) => a.state !== 'gossip', () => true];
  for (const ok of passes) {
    for (const a of newest) {
      if (victims.size >= n) break;
      if (a.abducted || victims.has(a) || !ok(a)) continue;
      victims.add(a);
    }
  }
  world.animals = world.animals.filter((a) => !victims.has(a));
}

// reconcile makes the scene's animals of one kind match the herd: the young, the adult males and
// the adult females, in the herd's proportions and at most VISIBLE[kind] of them in all.
function reconcile(world, kind, rng) {
  const h = world.herds[kind];
  const total = herdTotal(world, kind) - h.leased;
  const target = Math.min(VISIBLE[kind], total);
  const youngN = total ? Math.round((target * h.young) / total) : 0;
  const maleN = Math.round(((target - youngN) * h.males) / Math.max(1, h.females + h.males));
  const femaleN = target - youngN - maleN;

  const ofKind = () => world.animals.filter((a) => a.kind === kind);
  const isYoung = (a) => a.young;
  const isAdultMale = (a) => !a.young && isMale(a);
  const isAdultFemale = (a) => !a.young && !isMale(a);

  // Young that are too many while adults are too few grow up in place: a male where a bull is
  // short, otherwise a female.
  let excess = ofKind().filter(isYoung).length - youngN;
  let maleShort = maleN - ofKind().filter(isAdultMale).length;
  let femaleShort = femaleN - ofKind().filter(isAdultFemale).length;
  for (const a of ofKind().filter(isYoung).reverse()) {
    if (excess <= 0 || (maleShort <= 0 && femaleShort <= 0)) break;
    a.young = false;
    if (maleShort > 0) {
      a.sex = 'male';
      maleShort -= 1;
    } else {
      a.sex = 'female';
      femaleShort -= 1;
    }
    excess -= 1;
  }

  // Remove the surplus of each group, then add the missing ones.
  const groups = [
    [isYoung, youngN, () => ({ sex: 'female', young: true })],
    [isAdultMale, maleN, () => ({ sex: 'male' })],
    [isAdultFemale, femaleN, () => ({ sex: 'female' })],
  ];
  for (const [pred, want] of groups) {
    const group = ofKind().filter(pred);
    if (group.length > want) dropAnimals(world, group, group.length - want);
  }
  for (const [pred, want, opts] of groups) {
    for (let n = ofKind().filter(pred).length; n < want; n++) {
      addAnimal(world, kind, rng, opts());
    }
  }
}

// updateAnimals advances every animal whose abducted is false. line(kind) returns the text for a
// gag of that kind. The smoke reaction comes first, then the herd events, then the scene is made
// to match the herds, and then each animal moves.
export function updateAnimals(world, dt, rng, line) {
  // Smoke reaction: the first cow that has not yet noticed the latest cigarette says so. Every
  // cow records the current count, so each cigarette is noticed once.
  const cigarettes = world.stats.cigarettes;
  let smokeSaid = false;
  for (const a of world.animals) {
    if (a.abducted || a.kind !== 'cow') continue;
    if (world.farmer.state === 'smoke' && cigarettes > a.noticed && !smokeSaid) {
      smokeSaid = true;
      bubble(world, a, line('smoke'), 'whisper');
    }
    a.noticed = cigarettes;
  }

  herdEventsStep(world, rng, line);
  reconcile(world, 'cow', rng);
  reconcile(world, 'chicken', rng);

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
  if (a.young) return baseRows(a);
  if (a.kind === 'cow') {
    const walking = a.frame === 1 && (a.state === 'walk' || a.state === 'gossip');
    if (isMale(a)) return walking ? BULL_STEP : BULL;
    return walking ? COW_STEP : COW;
  }
  if (a.kind === 'chicken') {
    const cock = isMale(a);
    if (a.frame === 1 && a.state === 'peck') return cock ? COCK_PECK : CHICKEN_PECK;
    if (a.frame === 1 && (a.state === 'walk' || a.state === 'panic')) {
      return cock ? COCK_STEP : CHICKEN_STEP;
    }
    return cock ? COCK : CHICKEN;
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
