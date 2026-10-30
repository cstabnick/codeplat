// The UFO: flying saucers drift in, beam a cow up into the sky, hold it, put it back and leave. A
// night visit is free and comes at most once a day by chance; a hired visit holds its cow for a
// contract and pays the aliens while it does. The animals module skips an animal whose `abducted` is
// true and draws it raised by `lift`; this module sets those two fields on the cow and nothing else
// on it. It never touches coins: the module that pays reads `rate` of saucers in state 'hold'.
// Imports only state.js and pixel.js. Nothing touches window or document on load.

import { WIDTH, isNight, isAdult, say, logEvent, puff } from './state.js';
import { drawSprite } from './pixel.js';

const ARRIVE_SPEED = 60;     // units per second while flying in
const LEAVE_SPEED_X = 80;    // units per second while leaving, rightwards
const LEAVE_SPEED_Y = 40;    // units per second while leaving, upwards
const LIFT = 58;             // how far the cow is raised
const BEAM_TIME = 3;
const RETURN_TIME = 3;
const SUMMON_RATE = 0.03;    // per second, at night, once per day
const FREE_STAY = 1.5;       // seconds a free night visit holds its cow

// CONTRACT is the terms of a hired visit: seconds the cow is held, and coins per second the aliens pay.
export const CONTRACT = { stay: 45, rate: 0.6 };

const SAUCER = [
  '.........cccccc.........',
  '........cc7ccccc........',
  '.......cccccccccc.......',
  '...666666666666666666...',
  '.6666666666666666666666.',
  '5L55L55L55L55L55L55L55L5',
  '.5555555555555555555555.',
  '....5555555555555555....',
];

// cowOf returns the animal a saucer is taking, or null if that animal no longer exists.
function cowOf(world, u) {
  return world.animals.find((a) => a.id === u.targetId) ?? null;
}

// freeCows lists the adult cows that are not abducted and are not already the target of a saucer.
export function freeCows(world) {
  const taken = new Set(world.ufos.map((u) => u.targetId));
  return world.animals.filter(
    (a) => a.kind === 'cow' && !a.abducted && isAdult(a) && !taken.has(a.id),
  );
}

// launch adds a saucer for a random free cow and returns true, or returns false if no cow is free.
function launch(world, rng, stay, rate) {
  const free = freeCows(world);
  if (free.length === 0) return false;
  world.ufos.push({
    state: 'arrive', x: -20, y: 18, targetId: rng.pick(free).id, timer: 0, stay, rate,
  });
  return true;
}

// summonUfo starts a free night visit. It returns false and changes nothing if a free visit is
// already in the sky or no cow is free; otherwise it returns true and records the day it came.
export function summonUfo(world, rng) {
  if (world.ufos.some((u) => u.rate === 0)) return false;
  if (!launch(world, rng, FREE_STAY, 0)) return false;
  world.ufoLastDay = world.day;
  return true;
}

// hireUfo sends a saucer for a paid hold of CONTRACT.stay seconds. It returns false if no cow is free.
export function hireUfo(world, rng) {
  return launch(world, rng, CONTRACT.stay, CONTRACT.rate);
}

// updateUfo advances the visits by dt seconds. At night, once per day, it may start a free visit by
// chance, after the saucers have moved.
export function updateUfo(world, dt, rng, line) {
  const alive = [];
  for (const u of world.ufos) {
    if (advance(world, u, dt, rng, line)) alive.push(u);
  }
  world.ufos.length = 0;
  world.ufos.push(...alive);

  if (
    isNight(world) &&
    world.ufoLastDay !== world.day &&
    !world.ufos.some((u) => u.rate === 0) &&
    rng.chance(SUMMON_RATE * dt)
  ) {
    summonUfo(world, rng);
  }
}

// advance moves one saucer by dt seconds and returns false when it should be removed from the sky.
function advance(world, u, dt, rng, line) {
  let cow = cowOf(world, u);
  if (!cow && u.state === 'arrive' && u.rate > 0) {
    const free = freeCows(world);
    if (free.length > 0) {
      u.targetId = rng.pick(free).id;
      cow = cowOf(world, u);
    }
  }
  if (!cow) return false;

  switch (u.state) {
    case 'arrive':
      arrive(u, cow, dt);
      break;
    case 'beam':
      beam(u, cow, dt);
      break;
    case 'hold':
      hold(u, cow, dt);
      break;
    case 'return':
      back(world, u, cow, dt, line);
      break;
    case 'leave':
      if (!leave(u, dt)) return false;
      break;
    default:
      break;
  }

  if (rng.chance(dt)) {
    puff(world, 'sparkle', u.x + rng.range(-10, 10), u.y + 4);
  }
  return true;
}

function holdCow(cow) {
  cow.abducted = true;
  cow.state = 'abducted';
}

function arrive(u, cow, dt) {
  const tx = cow.x;
  const ty = cow.y - 70;
  const dx = tx - u.x;
  const dy = ty - u.y;
  const dist = Math.hypot(dx, dy);
  const step = ARRIVE_SPEED * dt;
  if (dist <= step) {
    u.x = tx;
    u.y = ty;
    u.state = 'beam';
    u.timer = BEAM_TIME;
    holdCow(cow);
  } else {
    u.x += (dx / dist) * step;
    u.y += (dy / dist) * step;
  }
}

function beam(u, cow, dt) {
  holdCow(cow);
  u.timer -= dt;
  if (u.timer <= 0) {
    cow.lift = LIFT;
    u.state = 'hold';
    u.timer = u.stay;
    return;
  }
  cow.lift = LIFT * (1 - u.timer / BEAM_TIME);
}

function hold(u, cow, dt) {
  holdCow(cow);
  cow.lift = LIFT;
  u.timer -= dt;
  if (u.timer <= 0) {
    u.state = 'return';
    u.timer = RETURN_TIME;
  }
}

function back(world, u, cow, dt, line) {
  holdCow(cow);
  u.timer -= dt;
  if (u.timer > 0) {
    cow.lift = LIFT * (u.timer / RETURN_TIME);
    return;
  }
  cow.lift = 0;
  cow.abducted = false;
  cow.state = 'graze';
  cow.timer = 2;
  say(world, cow.x, cow.y - 14, line('ufo'), 'speech', 4);
  world.stats.abductions += 1;
  const name = cow.name.toUpperCase();
  if (u.rate > 0) {
    logEvent(world, `${name} IS BACK FROM HER ALIEN INTERNSHIP`);
  } else {
    logEvent(world, `${name} WAS ABDUCTED. SHE SEEMS FINE.`);
  }
  u.state = 'leave';
  u.timer = 0;
}

// leave flies a saucer off to the right and up. It returns false once it is past the edge of the screen.
function leave(u, dt) {
  u.x += LEAVE_SPEED_X * dt;
  u.y -= LEAVE_SPEED_Y * dt;
  return u.x <= WIDTH + 30;
}

// ufoDrawables returns each saucer, and the beam of each one that holds a cow, as {y, draw(ctx)}
// items for main.js to sort by y. It returns an empty array when there are no saucers.
export function ufoDrawables(world) {
  const items = [];
  for (const u of world.ufos) {
    // The saucer flies in front of everything on the ground, so it sorts after all of it.
    items.push({ y: Infinity, draw: (ctx) => drawSaucer(ctx, world, u) });
    if (u.state === 'beam' || u.state === 'hold' || u.state === 'return') {
      const cow = cowOf(world, u);
      if (cow) items.push({ y: cow.y - 1, draw: (ctx) => drawBeam(ctx, u, cow) });
    }
  }
  return items;
}

function drawSaucer(ctx, world, u) {
  const light = (Math.floor(world.time / 0.25) % 2) === 0 ? 'a' : '8';
  const rows = SAUCER.map((row) => row.replaceAll('L', light));
  drawSprite(ctx, rows, u.x - 12, u.y - 4);
}

function drawBeam(ctx, u, cow) {
  const top = Math.round(u.y + 4);
  const bottom = Math.round(cow.y - cow.lift - 1);
  const n = Math.max(1, bottom - top + 1);
  const rows = [];
  for (let i = 0; i < n; i++) {
    const t = n === 1 ? 1 : i / (n - 1);
    const w = Math.round(6 + 18 * t);
    rows.push({ left: Math.round(u.x - w / 2), y: top + i, w });
  }

  ctx.globalAlpha = 0.35;
  ctx.fillStyle = '#ffec27';
  for (const r of rows) ctx.fillRect(r.left, r.y, r.w, 1);

  ctx.globalAlpha = 0.5;
  ctx.fillStyle = '#fff1e8';
  for (const r of rows) {
    ctx.fillRect(r.left, r.y, 1, 1);
    ctx.fillRect(r.left + r.w - 1, r.y, 1, 1);
  }
  ctx.globalAlpha = 1;
}
