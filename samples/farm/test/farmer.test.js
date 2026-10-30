import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, advanceTime, setHour, hourOf, farmerSees, DOOR, CROPS } from '../web/js/state.js';
import { updateFarmer, startTask, farmerDrawables, SPEED } from '../web/js/farmer.js';
import { makeRng } from '../web/js/rng.js';
import { fakeCtx } from './fakectx.js';

const DT = 0.1;
const line = (kind) => 'LINE ' + kind;

// step advances the world by one 0.1 s tick, as the game loop does: time first, then Gary.
function step(world, rng) {
  advanceTime(world, DT);
  updateFarmer(world, DT, rng, line);
}

// runFor steps for up to `seconds` of simulated time and returns true as soon as `done(world)` holds.
function runUntil(world, rng, seconds, done) {
  const n = Math.round(seconds / DT);
  for (let i = 0; i < n; i++) {
    step(world, rng);
    if (done(world)) return true;
  }
  return done(world);
}

test('at 23:00 Gary walks to the door within 30 s, goes to sleep hidden, and has no drawables', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 23);
  const reached = runUntil(world, rng, 30, (w) => w.farmer.state === 'sleep');
  assert.ok(reached, `Gary is ${world.farmer.state}, not asleep, after 30 s`);
  assert.equal(world.farmer.hidden, true);
  assert.equal(world.farmer.x, DOOR.x);
  assert.equal(world.farmer.y, DOOR.y);
  assert.deepEqual(farmerDrawables(world), []);
});

test('asleep, Gary puffs zzz above the door every 2 s', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 23);
  runUntil(world, rng, 30, (w) => w.farmer.state === 'sleep');
  world.effects = [];
  for (let i = 0; i < 50; i++) step(world, rng); // 5 s asleep
  const zzz = world.effects.filter((e) => e.kind === 'zzz');
  assert.ok(zzz.length >= 2 && zzz.length <= 3, `expected 2 or 3 zzz in 5 s, got ${zzz.length}`);
});

test('at 06:30 the next day Gary is awake and not hidden', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 23);
  runUntil(world, rng, 30, (w) => w.farmer.state === 'sleep');
  setHour(world, 6.5);
  assert.equal(world.day, 2);
  for (let i = 0; i < 10; i++) step(world, rng); // 1 s: the wake rule runs on the first step
  assert.ok(hourOf(world) >= 6.5 && hourOf(world) < 7, `clock is at ${hourOf(world)}`);
  assert.notEqual(world.farmer.state, 'sleep', 'Gary is still asleep at 06:30');
  assert.equal(world.farmer.hidden, false);
});

test('startTask smoke at 10:00 hides Gary behind the barn within 30 s, then he comes out', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 10);
  startTask(world, 'smoke', rng, line);

  const hidden = runUntil(world, rng, 30, (w) => w.farmer.state === 'smoke');
  assert.ok(hidden, `Gary is ${world.farmer.state}, not smoking, after 30 s`);
  assert.equal(world.farmer.hidden, true);
  assert.equal(world.farmer.cigsToday, 1);
  assert.equal(world.stats.cigarettes, 1);
  assert.equal(farmerSees(world, 200, 120), false, 'the cows can not see Gary while he smokes');

  const cigStart = world.effects.length;
  for (let i = 0; i < 80 && world.farmer.state === 'smoke'; i++) {
    assert.equal(farmerSees(world, 200, 120), false, 'the cows can not see Gary while he smokes');
    step(world, rng);
  }
  const smokes = world.effects.slice(cigStart).filter((e) => e.kind === 'smoke').length;
  assert.ok(smokes >= 1, 'expected at least one smoke puff while smoking');

  const done = runUntil(world, rng, 15, (w) => w.farmer.state !== 'smoke');
  assert.ok(done, 'smoking never ended');
  assert.equal(world.farmer.hidden, false);
  assert.equal(world.farmer.state, 'idle');
});

test('harvesting a ripe plot adds its crop value to coins and replants it at stage 0', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  world.plots.forEach((p) => { p.stage = 0; });
  const plot = world.plots[7];
  plot.stage = 3;
  plot.crop = 'carrot';
  const before = world.coins;
  startTask(world, 'harvest', rng, line);

  const harvested = runUntil(world, rng, 30, () => world.coins !== before);
  assert.ok(harvested, 'no coins added within 30 s');
  assert.equal(world.coins - before, CROPS.carrot.value);
  assert.equal(plot.stage, 0, 'replanted at stage 0');
  assert.equal(plot.growth, 0);
  assert.equal(plot.water, 0);
  assert.equal(plot.crop, 'carrot', `replanted as ${plot.crop}`);
});

test('startTask harvest with no ripe plot does nothing', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  world.plots.forEach((p) => { p.stage = 1; });
  startTask(world, 'harvest', rng, line);
  assert.equal(world.farmer.task, null);
  assert.equal(world.farmer.state, 'idle');
});

test('startTask water sets the driest plot to water 1', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  world.plots.forEach((p) => { p.water = 0.5; });
  const driest = world.plots[10];
  driest.water = 0;
  startTask(world, 'water', rng, line);

  const watered = runUntil(world, rng, 30, () => driest.water === 1);
  assert.ok(watered, `driest plot has water ${driest.water} after 30 s`);
  assert.equal(driest.water, 1);
});

test('cigsToday never exceeds 3 from 06:00 to 21:00 in 0.1 s steps', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 6);
  let steps = 0;
  while (hourOf(world) < 21 && steps < 100000) {
    step(world, rng);
    steps += 1;
    assert.ok(world.farmer.cigsToday <= 3, `cigsToday is ${world.farmer.cigsToday} at hour ${hourOf(world)}`);
  }
  assert.ok(steps < 100000, 'did not reach 21:00');
  // The cap must be tested while it binds: this seed smokes up to the limit during the day.
  assert.equal(world.stats.cigarettes, 3);
});

test('Gary faces left while he walks left', () => {
  const world = createWorld(1);
  const rng = makeRng(1);
  setHour(world, 10);
  world.farmer.x = 200;
  world.farmer.y = 84;
  world.farmer.facing = 'right';
  startTask(world, 'feed', rng, line); // feed yard is to the left of x = 200
  step(world, rng);
  assert.equal(world.farmer.state, 'walk');
  assert.equal(world.farmer.facing, 'left');
  assert.ok(world.farmer.x < 200, 'Gary moved left');
  assert.ok(world.farmer.x >= 200 - SPEED * DT - 1e-9, 'Gary moved no more than SPEED * dt');
  for (let i = 0; i < 5; i++) step(world, rng);
  assert.equal(world.farmer.facing, 'left', 'still facing left while walking left');
});

test('farmerDrawables draws one sprite while Gary is awake', () => {
  const world = createWorld(1);
  const drawables = farmerDrawables(world);
  assert.equal(drawables.length, 1);
  assert.equal(drawables[0].y, world.farmer.y);
  const ctx = fakeCtx();
  drawables[0].draw(ctx);
  assert.ok(ctx.count('fillRect') > 0, 'the sprite draws at least one rectangle');
});
