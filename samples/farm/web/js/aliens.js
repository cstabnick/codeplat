// The aliens: the farm leases cows to UFOs by the thousand. A leased cow is gone for LEASE.stay
// seconds and the aliens pay LEASE.rate coins a second for each one; a few saucers hover over the
// pasture to show how many batches are out. The nightly visit lifts one visible cow, holds it and
// puts it back, as the old UFO module did. This module owns world.aliens, herds.cow.leased, the cows it moves
// between females and leased, the alien pay, stats.abductions, and a visible cow's abducted and
// lift during a visit. Imports only state.js and pixel.js. Nothing touches window or document on load.

import { PASTURE, isNight, say, logEvent, puff, money, landWidth } from './state.js';
import { drawSprite } from './pixel.js';

export const LEASE = { stay: 45, rate: 0.6, price: 5 };
export const MAX_SAUCERS = 6;

const ARRIVE_SPEED = 60;     // units per second while flying in
const LEAVE_SPEED_X = 80;    // units per second while leaving, rightwards
const LEAVE_SPEED_Y = 40;    // units per second while leaving, upwards
const LIFT = 58;             // how far the visit's cow is raised
const BEAM_TIME = 3;
const HOLD_TIME = 1.5;
const RETURN_TIME = 3;
const SUMMON_RATE = 0.03;    // per second, at night, once per day
const ENTRY = { x: -20, y: 18 }; // where saucers appear, off the left edge

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

// leasable is how many cows could be leased right now: the females at the farm.
export function leasable(world) {
  return world.herds.cow.females;
}

// maxLease is the most cows the farm can lease now, limited by the cows it has and the coins it can pay.
export function maxLease(world) {
  return Math.min(leasable(world), Math.floor(world.coins / LEASE.price));
}

// leaseCows sends n cows, or as many as it can when n is 'max', on an alien internship. It takes
// the price up front and returns how many cows went.
export function leaseCows(world, n) {
  const count = Math.max(0, Math.min(n === 'max' ? Infinity : n, maxLease(world)));
  if (!(count > 0)) return 0;
  world.coins -= count * LEASE.price;
  world.herds.cow.females -= count;
  world.herds.cow.leased += count;
  world.aliens.batches.push({ count, timer: LEASE.stay });
  logEvent(world, `${money(count)} COWS WENT ON AN ALIEN INTERNSHIP`);
  return count;
}

// alienRate is the coins per second the aliens pay for the leased cows. Productivity does not apply.
export function alienRate(world) {
  return world.herds.cow.leased * LEASE.rate;
}

// summonVisit starts the night visit on a free adult cow. It returns false and changes nothing if a
// visit is already under way or no visible cow is free; otherwise it returns true and records the day.
export function summonVisit(world, rng) {
  if (world.aliens.visit) return false;
  const free = world.animals.filter((a) => a.kind === 'cow' && !a.abducted && !a.young);
  if (free.length === 0) return false;
  const cow = rng.pick(free);
  world.aliens.visit = { state: 'arrive', x: ENTRY.x, y: ENTRY.y, targetId: cow.id, timer: 0 };
  world.aliens.lastDay = world.day;
  return true;
}

// updateAliens advances the leases, the saucers and the night visit by dt seconds. The visit may
// start by chance once the other parts have moved, as the old UFO module did.
export function updateAliens(world, dt, rng, line) {
  if (world.aliens.visit === undefined) world.aliens.visit = null;
  world.coins += alienRate(world) * dt;
  updateBatches(world, dt, line);
  updateSaucers(world, dt);
  if (world.aliens.visit) advanceVisit(world, dt, rng, line);

  if (
    isNight(world) &&
    world.aliens.lastDay !== world.day &&
    !world.aliens.visit &&
    rng.chance(SUMMON_RATE * dt)
  ) {
    summonVisit(world, rng);
  }
}

// updateBatches counts down each lease and brings back the cows whose time is up.
function updateBatches(world, dt, line) {
  const kept = [];
  for (const b of world.aliens.batches) {
    b.timer -= dt;
    if (b.timer > 0) {
      kept.push(b);
      continue;
    }
    world.herds.cow.leased -= b.count;
    world.herds.cow.females += b.count;
    world.stats.abductions += b.count;
    logEvent(world, `${money(b.count)} COWS ARE BACK FROM SPACE`);
    const cow = world.animals.find((a) => a.kind === 'cow' && !a.abducted);
    if (cow) say(world, cow.x, cow.y - 14, line('ufo'), 'speech', 4);
  }
  world.aliens.batches.length = 0;
  world.aliens.batches.push(...kept);
}

// updateSaucers gives each running batch a saucer in its own slot, sends the rest away, and moves
// every saucer. A slot holds at most one saucer, so there are never more than MAX_SAUCERS.
function updateSaucers(world, dt) {
  const wanted = Math.min(MAX_SAUCERS, world.aliens.batches.length);
  const saucers = world.aliens.saucers;
  for (let slot = 0; slot < wanted; slot++) {
    const s = saucers.find((u) => u.slot === slot);
    if (!s) saucers.push({ slot, x: ENTRY.x, y: ENTRY.y, state: 'arrive' });
    else if (s.state === 'leave') s.state = 'arrive';
  }

  for (const s of saucers) {
    if (s.slot >= wanted && s.state !== 'leave') s.state = 'leave';
    if (s.state === 'arrive' || s.state === 'hover') {
      const hx = PASTURE.x + 16 + s.slot * 20;
      const hy = 28 + 2 * Math.sin(world.time * 2 + s.slot);
      if (s.state === 'hover') {
        s.x = hx;
        s.y = hy;
      } else if (stepToward(s, hx, hy, ARRIVE_SPEED * dt)) {
        s.state = 'hover';
      }
    } else {
      s.x += LEAVE_SPEED_X * dt;
      s.y -= LEAVE_SPEED_Y * dt;
    }
  }

  const limit = landWidth(world) + 30;
  const alive = saucers.filter((s) => s.state !== 'leave' || s.x <= limit);
  saucers.length = 0;
  saucers.push(...alive);
}

// stepToward moves o by at most step units towards (tx, ty) and returns true once it is there.
function stepToward(o, tx, ty, step) {
  const dx = tx - o.x;
  const dy = ty - o.y;
  const dist = Math.hypot(dx, dy);
  if (dist <= step) {
    o.x = tx;
    o.y = ty;
    return true;
  }
  o.x += (dx / dist) * step;
  o.y += (dy / dist) * step;
  return false;
}

// advanceVisit moves the night visit by dt seconds: fly in, beam the cow up, hold it, put it back
// and leave. The visit ends early if its cow is gone.
function advanceVisit(world, dt, rng, line) {
  const v = world.aliens.visit;
  const cow = world.animals.find((a) => a.id === v.targetId);
  if (!cow) {
    world.aliens.visit = null;
    return;
  }

  switch (v.state) {
    case 'arrive':
      if (stepToward(v, cow.x, cow.y - 70, ARRIVE_SPEED * dt)) {
        holdCow(cow);
        v.state = 'beam';
        v.timer = BEAM_TIME;
      }
      break;
    case 'beam':
      holdCow(cow);
      v.timer -= dt;
      if (v.timer <= 0) {
        cow.lift = LIFT;
        v.state = 'hold';
        v.timer = HOLD_TIME;
      } else {
        cow.lift = LIFT * (1 - v.timer / BEAM_TIME);
      }
      break;
    case 'hold':
      holdCow(cow);
      cow.lift = LIFT;
      v.timer -= dt;
      if (v.timer <= 0) {
        v.state = 'return';
        v.timer = RETURN_TIME;
      }
      break;
    case 'return':
      holdCow(cow);
      v.timer -= dt;
      if (v.timer > 0) {
        cow.lift = LIFT * (v.timer / RETURN_TIME);
      } else {
        putBack(world, cow, line);
        v.state = 'leave';
      }
      break;
    case 'leave':
      v.x += LEAVE_SPEED_X * dt;
      v.y -= LEAVE_SPEED_Y * dt;
      if (v.x > landWidth(world) + 30) world.aliens.visit = null;
      break;
    default:
      break;
  }

  if (world.aliens.visit && rng.chance(dt)) {
    puff(world, 'sparkle', v.x + rng.range(-10, 10), v.y + 4);
  }
}

// holdCow marks a cow as taken by the saucer.
function holdCow(cow) {
  cow.abducted = true;
  cow.state = 'abducted';
}

// putBack sets a visit's cow down again, counts the abduction and writes the log line.
function putBack(world, cow, line) {
  cow.lift = 0;
  cow.abducted = false;
  cow.state = 'graze';
  cow.timer = 2;
  say(world, cow.x, cow.y - 14, line('ufo'), 'speech', 4);
  world.stats.abductions += 1;
  logEvent(world, `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`);
}

// alienDrawables returns each saucer, the beam of each hovering saucer, and the visit's saucer and
// beam, as {y, draw(ctx)} items for main.js to sort by y. It returns an empty array when there is nothing to show.
export function alienDrawables(world) {
  const items = [];
  for (const s of world.aliens.saucers) {
    items.push({ y: Infinity, draw: (ctx) => drawSaucer(ctx, world, s.x, s.y) });
    if (s.state === 'hover') {
      const bottom = PASTURE.y + 30;
      items.push({
        y: PASTURE.y + PASTURE.h - 1,
        draw: (ctx) => drawBeam(ctx, s.x, s.y + 4, bottom),
      });
    }
  }

  const v = world.aliens.visit;
  if (v) {
    items.push({ y: Infinity, draw: (ctx) => drawSaucer(ctx, world, v.x, v.y) });
    if (v.state === 'beam' || v.state === 'hold' || v.state === 'return') {
      const cow = world.animals.find((a) => a.id === v.targetId);
      if (cow) {
        items.push({
          y: cow.y - 1,
          draw: (ctx) => drawBeam(ctx, v.x, v.y + 4, cow.y - cow.lift - 1),
        });
      }
    }
  }
  return items;
}

// drawSaucer draws a saucer with its top-left at (x - 12, y - 4), its lights blinking.
function drawSaucer(ctx, world, x, y) {
  const light = (Math.floor(world.time / 0.25) % 2) === 0 ? 'a' : '8';
  const rows = SAUCER.map((row) => row.replaceAll('L', light));
  drawSprite(ctx, rows, x - 12, y - 4);
}

// drawBeam draws a light beam from y top down to y bottom, widening as it falls, and leaves globalAlpha at 1.
function drawBeam(ctx, x, top, bottom) {
  const t0 = Math.round(top);
  const n = Math.max(1, Math.round(bottom) - t0 + 1);
  const rows = [];
  for (let i = 0; i < n; i++) {
    const t = n === 1 ? 1 : i / (n - 1);
    const w = Math.round(6 + 18 * t);
    rows.push({ left: Math.round(x - w / 2), y: t0 + i, w });
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
