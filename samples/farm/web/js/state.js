// The world model. Every other module reads and writes this state; see the field
// ownership notes in the issue for which module changes which field.

export const WIDTH = 320;
export const HEIGHT = 180;
export const HORIZON = 64;        // sky above, ground below
export const DAY_LENGTH = 180;    // seconds of simulated time per day
export const HOUSE = { x: 8, y: 22, w: 60, h: 56 };
export const DOOR = { x: 38, y: 78 };                 // feet position at the front door
export const WINDOWS = [ { x: 16, y: 50, w: 8, h: 8 }, { x: 52, y: 50, w: 8, h: 8 } ];
export const FIELD = { x: 14, y: 96, cols: 6, rows: 3, tile: 16 };  // plot (col,row) top-left = (x + col*tile, y + row*tile)
export const COOP = { x: 120, y: 58, w: 32, h: 26 };
export const YARD = { x: 112, y: 90, w: 56, h: 40 };  // where chickens roam
export const BARN = { x: 230, y: 24, w: 72, h: 60 };
export const HIDEOUT = { x: 310, y: 80 };             // feet position behind the barn, where Gary smokes
export const PASTURE = { x: 176, y: 96, w: 138, h: 72 };  // fenced; cows stay inside
export const MUD = { x: 60, y: 150, w: 44, h: 16 };   // the pig's puddle
export const CROPS = {
  carrot:  { grow: 40, value: 3 },   // grow: seconds of watered time from planted to ripe
  corn:    { grow: 60, value: 5 },
  pumpkin: { grow: 90, value: 12 },
};

const BUBBLE_LIMIT = 8;
const LOG_LIMIT = 5;
const EFFECT_LIMIT = 120;

// createWorld returns a new world. Same seed, same world.
export function createWorld(seed) {
  const animals = [
    { id: 'cow-1', kind: 'cow', name: 'Daisy',      x: 200, y: 122, facing: 'right', state: 'graze' },
    { id: 'cow-2', kind: 'cow', name: 'Bessie',     x: 236, y: 144, facing: 'left',  state: 'graze' },
    { id: 'cow-3', kind: 'cow', name: 'Moolinda',   x: 280, y: 118, facing: 'left',  state: 'graze' },
    { id: 'cow-4', kind: 'cow', name: 'Clarabelle', x: 262, y: 160, facing: 'right', state: 'graze' },
    { id: 'chicken-1', kind: 'chicken', name: 'Henrietta',    x: 118, y: 102, facing: 'right', state: 'peck' },
    { id: 'chicken-2', kind: 'chicken', name: 'Cluck Norris', x: 130, y: 118, facing: 'left',  state: 'peck' },
    { id: 'chicken-3', kind: 'chicken', name: 'Eggatha',      x: 146, y: 106, facing: 'right', state: 'peck' },
    { id: 'chicken-4', kind: 'chicken', name: 'Nugget',       x: 156, y: 124, facing: 'left',  state: 'peck' },
    { id: 'chicken-5', kind: 'chicken', name: 'Pecky',        x: 124, y: 126, facing: 'right', state: 'peck' },
    { id: 'pig-1', kind: 'pig', name: 'Sir Oinksalot', x: 82, y: 160, facing: 'right', state: 'wallow' },
  ].map((a) => ({
    ...a,
    target: null,
    timer: 1,
    frame: 0,
    partner: null,
    noticed: 0,
    lift: 0,
    abducted: false,
  }));

  const plots = [];
  for (let row = 0; row < FIELD.rows; row++) {
    for (let col = 0; col < FIELD.cols; col++) {
      const growth = ((col * 7 + row * 3) % 10) / 10;
      plots.push({
        col,
        row,
        crop: ['carrot', 'corn', 'pumpkin'][col % 3],
        growth,
        stage: growth >= 1 ? 3 : Math.floor(growth * 3),
        water: 0.5,
      });
    }
  }

  return {
    seed,
    time: 0, day: 1, clock: 0, speed: 1, paused: false,
    coins: 0,
    stats: { cigarettes: 0, gossip: 0, harvests: 0, abductions: 0 },
    farmer: { x: 38, y: 84, facing: 'right', state: 'idle', task: null, target: null,
              timer: 2, frame: 0, hidden: false, cigsToday: 0, day: 1 },
    animals,
    plots,
    bubbles: [],
    effects: [],
    log: [],
    ufo: { state: 'away', x: -30, y: 20, targetId: null, timer: 0, lastDay: 0 },
  };
}

// setClock derives day and clock from world.time.
function syncClock(world) {
  world.day = 1 + Math.floor(world.time / DAY_LENGTH);
  world.clock = (world.time % DAY_LENGTH) / DAY_LENGTH;
}

// advanceTime adds dt seconds: time += dt; day = 1 + floor(time / DAY_LENGTH);
// clock = (time % DAY_LENGTH) / DAY_LENGTH. Clock 0 is 06:00.
export function advanceTime(world, dt) {
  world.time += dt;
  syncClock(world);
}

// hourOf returns the hour of day as a float in [0, 24): (6 + clock * 24) % 24.
export function hourOf(world) {
  return (6 + world.clock * 24) % 24;
}

// isNight is true when hourOf(world) >= 21 or < 6.
export function isNight(world) {
  const h = hourOf(world);
  return h >= 21 || h < 6;
}

// setHour moves time forward (never back) to the next moment whose hour is h, keeping the day if
// that moment is later today. Used by the "skip" button and by tests.
export function setHour(world, h) {
  const clock = ((h - 6 + 24) % 24) / 24;
  const dayStart = (world.day - 1) * DAY_LENGTH;
  let target = dayStart + clock * DAY_LENGTH;
  if (target < world.time) target += DAY_LENGTH;
  world.time = target;
  syncClock(world);
}

// farmerSees reports whether Gary can see the point (x, y): false when farmer.hidden is true or
// farmer.state is 'smoke' or 'sleep'; otherwise true when the point is on the side he faces
// (facing 'right': x >= farmer.x - 8; facing 'left': x <= farmer.x + 8) and within 140 px. "Within
// 140 px" means the Euclidean distance from (farmer.x, farmer.y) to (x, y) is at most 140.
export function farmerSees(world, x, y) {
  const f = world.farmer;
  if (f.hidden || f.state === 'smoke' || f.state === 'sleep') return false;
  if (f.facing === 'right') {
    if (!(x >= f.x - 8)) return false;
  } else if (f.facing === 'left') {
    if (!(x <= f.x + 8)) return false;
  } else {
    return false;
  }
  const dx = x - f.x;
  const dy = y - f.y;
  return Math.sqrt(dx * dx + dy * dy) <= 140;
}

// say adds a bubble {x, y, text, kind, ttl}. (x, y) is where its tail points, normally just above a
// head. kind is 'speech', 'thought' or 'whisper'. It first removes any bubble whose x and y are both
// within 6 px of the new one (|dx| <= 6 and |dy| <= 6), and afterwards drops the oldest bubbles so
// at most 8 remain.
export function say(world, x, y, text, kind = 'speech', ttl = 3) {
  world.bubbles = world.bubbles.filter(
    (b) => !(Math.abs(b.x - x) <= 6 && Math.abs(b.y - y) <= 6),
  );
  world.bubbles.push({ x, y, text, kind, ttl });
  while (world.bubbles.length > BUBBLE_LIMIT) world.bubbles.shift();
}

// logEvent puts {day, hour: Math.floor(hourOf(world)), text} at the front of world.log and keeps
// at most 5 entries.
export function logEvent(world, text) {
  world.log.unshift({ day: world.day, hour: Math.floor(hourOf(world)), text });
  if (world.log.length > LOG_LIMIT) world.log.length = LOG_LIMIT;
}

// puff adds an effect {kind, x, y, vx, vy, ttl, age: 0} and keeps at most 120 effects (drops oldest).
// kinds: 'smoke', 'zzz', 'note', 'coin', 'drop', 'feather', 'sparkle', 'heart', 'mud'.
export function puff(world, kind, x, y, vx = 0, vy = -6, ttl = 1.5) {
  world.effects.push({ kind, x, y, vx, vy, ttl, age: 0 });
  while (world.effects.length > EFFECT_LIMIT) world.effects.shift();
}
