// The UFO: once a night a flying saucer drifts in, beams a cow up into the sky, holds it, puts it
// back and leaves. The animals module skips an animal whose `abducted` is true and draws it raised
// by `lift`; this module sets those two fields on the cow and nothing else on it.
// Imports only state.js and pixel.js. Nothing touches window or document on load.

import { WIDTH, isNight, say, logEvent, puff } from './state.js';
import { drawSprite } from './pixel.js';

const ARRIVE_SPEED = 60;     // units per second while flying in
const LEAVE_SPEED_X = 80;    // units per second while leaving, rightwards
const LEAVE_SPEED_Y = 40;    // units per second while leaving, upwards
const LIFT = 58;             // how far the cow is raised
const BEAM_TIME = 3;
const HOLD_TIME = 1.5;
const RETURN_TIME = 3;
const SUMMON_RATE = 0.03;    // per second, at night, once per day

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

const cowOf = (world) => world.animals.find((a) => a.id === world.ufo.targetId) ?? null;

// The cow is held by the beam from the start of 'beam' until the end of 'return'.
function holdCow(cow) {
  cow.abducted = true;
  cow.state = 'abducted';
}

// updateUfo advances the visit by dt seconds. It starts a visit by chance at night, once per day.
export function updateUfo(world, dt, rng, line) {
  const ufo = world.ufo;

  if (ufo.state === 'away') {
    if (isNight(world) && ufo.lastDay !== world.day && rng.chance(SUMMON_RATE * dt)) {
      summonUfo(world, rng);
    }
    return;
  }

  const cow = cowOf(world);
  if (!cow) {
    ufo.state = 'away';
    ufo.targetId = null;
    return;
  }

  switch (ufo.state) {
    case 'arrive':
      arrive(ufo, cow, dt);
      break;
    case 'beam':
      beam(ufo, cow, dt);
      break;
    case 'hold':
      hold(ufo, cow, dt);
      break;
    case 'return':
      back(world, ufo, cow, dt, line);
      break;
    case 'leave':
      leave(world, ufo, dt);
      break;
    default:
      break;
  }

  if (ufo.state !== 'away' && rng.chance(dt)) {
    puff(world, 'sparkle', ufo.x + rng.range(-10, 10), ufo.y + 4);
  }
}

function arrive(ufo, cow, dt) {
  const tx = cow.x;
  const ty = cow.y - 70;
  const dx = tx - ufo.x;
  const dy = ty - ufo.y;
  const dist = Math.hypot(dx, dy);
  const step = ARRIVE_SPEED * dt;
  if (dist <= step) {
    ufo.x = tx;
    ufo.y = ty;
    ufo.state = 'beam';
    ufo.timer = BEAM_TIME;
    holdCow(cow);
  } else {
    ufo.x += (dx / dist) * step;
    ufo.y += (dy / dist) * step;
  }
}

function beam(ufo, cow, dt) {
  holdCow(cow);
  ufo.timer -= dt;
  if (ufo.timer <= 0) {
    cow.lift = LIFT;
    ufo.state = 'hold';
    ufo.timer = HOLD_TIME;
    return;
  }
  cow.lift = LIFT * (1 - ufo.timer / BEAM_TIME);
}

function hold(ufo, cow, dt) {
  holdCow(cow);
  cow.lift = LIFT;
  ufo.timer -= dt;
  if (ufo.timer <= 0) {
    ufo.state = 'return';
    ufo.timer = RETURN_TIME;
  }
}

function back(world, ufo, cow, dt, line) {
  holdCow(cow);
  ufo.timer -= dt;
  if (ufo.timer > 0) {
    cow.lift = LIFT * (ufo.timer / RETURN_TIME);
    return;
  }
  cow.lift = 0;
  cow.abducted = false;
  cow.state = 'graze';
  cow.timer = 2;
  say(world, cow.x, cow.y - 14, line('ufo'), 'speech', 4);
  world.stats.abductions += 1;
  logEvent(world, `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`);
  ufo.state = 'leave';
  ufo.timer = 0;
}

function leave(world, ufo, dt) {
  ufo.x += LEAVE_SPEED_X * dt;
  ufo.y -= LEAVE_SPEED_Y * dt;
  if (ufo.x > WIDTH + 30) {
    ufo.state = 'away';
    ufo.targetId = null;
  }
}

// summonUfo starts a visit now and picks a random cow that is not abducted. It returns false and
// does nothing if a visit is under way or no cow is free.
export function summonUfo(world, rng) {
  const ufo = world.ufo;
  if (ufo.state !== 'away') return false;
  const free = world.animals.filter((a) => a.kind === 'cow' && !a.abducted);
  if (free.length === 0) return false;
  const cow = rng.pick(free);
  ufo.state = 'arrive';
  ufo.x = -20;
  ufo.y = 18;
  ufo.targetId = cow.id;
  ufo.timer = 0;
  ufo.lastDay = world.day;
  return true;
}

// ufoDrawables returns the saucer, and the beam while the cow is being lifted, as {y, draw(ctx)}
// items for main.js to sort by y.
export function ufoDrawables(world) {
  const ufo = world.ufo;
  if (ufo.state === 'away') return [];
  // The saucer flies in front of everything on the ground, so it sorts after all of it.
  const items = [{ y: Infinity, draw: (ctx) => drawSaucer(ctx, world) }];
  if (ufo.state === 'beam' || ufo.state === 'hold' || ufo.state === 'return') {
    const cow = cowOf(world);
    if (cow) items.push({ y: cow.y - 1, draw: (ctx) => drawBeam(ctx, world, cow) });
  }
  return items;
}

function drawSaucer(ctx, world) {
  const light = (Math.floor(world.time / 0.25) % 2) === 0 ? 'a' : '8';
  const rows = SAUCER.map((row) => row.replaceAll('L', light));
  drawSprite(ctx, rows, world.ufo.x - 12, world.ufo.y - 4);
}

function drawBeam(ctx, world, cow) {
  const ufo = world.ufo;
  const top = Math.round(ufo.y + 4);
  const bottom = Math.round(cow.y - cow.lift - 1);
  const n = Math.max(1, bottom - top + 1);
  const rows = [];
  for (let i = 0; i < n; i++) {
    const t = n === 1 ? 1 : i / (n - 1);
    const w = Math.round(6 + 18 * t);
    rows.push({ left: Math.round(ufo.x - w / 2), y: top + i, w });
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
