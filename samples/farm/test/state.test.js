import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  createWorld,
  advanceTime,
  hourOf,
  isNight,
  setHour,
  farmerSees,
  say,
  logEvent,
  puff,
  DAY_LENGTH,
  productivity,
  money,
  herdTotal,
  housingCapacity,
  hasKind,
  landWidth,
} from '../web/js/state.js';
import { SPECIES, KINDS, LAND } from '../web/js/species.js';
import { fakeCtx } from './fakectx.js';

const near = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);

test('createWorld is deterministic for a seed', () => {
  assert.deepEqual(createWorld(42), createWorld(42));
});

test('createWorld has the animals, plots and clock from the contract', () => {
  const w = createWorld(1);
  assert.equal(w.animals.filter((a) => a.kind === 'cow').length, 4);
  assert.equal(w.animals.filter((a) => a.kind === 'chicken').length, 5);
  assert.equal(w.animals.filter((a) => a.kind === 'pig').length, 1);
  assert.equal(w.animals.length, 10);

  const daisy = w.animals.find((a) => a.id === 'cow-1');
  assert.deepEqual(
    { name: daisy.name, x: daisy.x, y: daisy.y, facing: daisy.facing, state: daisy.state },
    { name: 'Daisy', x: 200, y: 122, facing: 'right', state: 'graze' },
  );
  for (const a of w.animals) {
    assert.equal(a.target, null);
    assert.equal(a.timer, 1);
    assert.equal(a.frame, 0);
    assert.equal(a.partner, null);
    assert.equal(a.noticed, 0);
    assert.equal(a.lift, 0);
    assert.equal(a.abducted, false);
  }

  assert.equal(w.plots.length, 18);
  for (const p of w.plots) {
    assert.equal(p.crop, 'carrot');
    assert.equal(p.water, 0.5);
    const growth = ((p.col * 7 + p.row * 3) % 10) / 10;
    assert.equal(p.growth, growth);
    assert.equal(p.stage, growth >= 1 ? 3 : Math.floor(growth * 3));
  }
  // row-major: first plot is (0,0); (1,0) has growth 0.7, stage 2.
  assert.deepEqual(w.plots[0], { col: 0, row: 0, crop: 'carrot', growth: 0, stage: 0, water: 0.5 });
  assert.deepEqual(w.plots[1], { col: 1, row: 0, crop: 'carrot', growth: 0.7, stage: 2, water: 0.5 });
  assert.equal(w.plots[6].row, 1);
  assert.equal(w.plots[6].col, 0);

  assert.equal(w.time, 0);
  assert.equal(w.day, 1);
  assert.equal(w.clock, 0);
  assert.deepEqual(w.stats, {
    cigarettes: 0, gossip: 0, harvests: 0, abductions: 0, milk: 0, eggs: 0, births: 0, deaths: 0,
    plucks: 0,
  });
  assert.deepEqual(w.log, []);
  assert.deepEqual(w.bubbles, []);
  assert.deepEqual(w.effects, []);
  assert.equal(w.coins, 10);
  assert.deepEqual(w.upgrades, { cigs: 0, portal: 0 });
});

test('productivity is 1.25 per pack of cigs, doubled by the portal', () => {
  const w = createWorld(1);
  near(productivity(w), 1);
  w.upgrades.cigs = 2;
  near(productivity(w), 1.5625);
  w.upgrades.portal = 1;
  near(productivity(w), 3.125);
});

test('money: whole coins, then K, M and B with one decimal, rounded down', () => {
  assert.equal(money(0), '0');
  assert.equal(money(12.9), '12');
  assert.equal(money(9999), '9999');
  assert.equal(money(12345), '12.3K');
  assert.equal(money(4560000), '4.5M');
  assert.equal(money(7.89e9), '7.8B');
  assert.equal(money(8.95e12), '8.9T');
  assert.equal(money(1.25e15), '1.2QA');
  assert.equal(money(999999), '999.9K');
  assert.equal(money(-5), '0');
});

test('advanceTime and hourOf: clock 0 is hour 6, and day and hour roll over', () => {
  const w = createWorld(1);
  near(hourOf(w), 6);
  advanceTime(w, DAY_LENGTH * 1.5);
  assert.equal(w.day, 2);
  near(hourOf(w), 18);
});

test('isNight is true at hours 22, 3 and false at 12', () => {
  const w = createWorld(1);
  setHour(w, 22);
  assert.equal(isNight(w), true);
  setHour(w, 3);
  assert.equal(isNight(w), true);
  setHour(w, 12);
  assert.equal(isNight(w), false);
});

test('setHour(21) from hour 10 keeps the day', () => {
  const w = createWorld(1);
  advanceTime(w, 4 * (DAY_LENGTH / 24)); // hour 10
  near(hourOf(w), 10);
  setHour(w, 21);
  assert.equal(w.day, 1);
  near(hourOf(w), 21);
});

test('setHour(6) from hour 22 advances the day', () => {
  const w = createWorld(1);
  advanceTime(w, 16 * (DAY_LENGTH / 24)); // hour 22
  near(hourOf(w), 22);
  setHour(w, 6);
  assert.equal(w.day, 2);
  near(hourOf(w), 6);
});

test('farmerSees respects facing, range, hidden and smoking', () => {
  const w = createWorld(1);
  // farmer at (38, 84), facing right
  assert.equal(w.farmer.x, 38);
  assert.equal(w.farmer.y, 84);
  assert.equal(farmerSees(w, 50, 84), true, 'ahead, in range');
  assert.equal(farmerSees(w, 20, 84), false, 'behind him, facing right');
  assert.equal(farmerSees(w, 38 + 141, 84), false, 'out of range');
  assert.equal(farmerSees(w, 38 + 140, 84), true, 'range is inclusive');

  w.farmer.facing = 'left';
  assert.equal(farmerSees(w, 30, 84), true, 'ahead when facing left');
  assert.equal(farmerSees(w, 50, 84), false, 'behind him when facing left');

  w.farmer.facing = 'right';
  w.farmer.hidden = true;
  assert.equal(farmerSees(w, 50, 84), false, 'hidden');
  w.farmer.hidden = false;
  w.farmer.state = 'smoke';
  assert.equal(farmerSees(w, 50, 84), false, 'smoking');
  w.farmer.state = 'sleep';
  assert.equal(farmerSees(w, 50, 84), false, 'sleeping');
  w.farmer.state = 'idle';
  assert.equal(farmerSees(w, 50, 84), true, 'idle again');
});

test('say replaces a nearby bubble and caps at 8', () => {
  const w = createWorld(1);
  say(w, 10, 10, 'a');
  say(w, 13, 12, 'b');
  assert.equal(w.bubbles.length, 1);
  assert.equal(w.bubbles[0].text, 'b');
  assert.deepEqual(w.bubbles[0], { x: 13, y: 12, text: 'b', kind: 'speech', ttl: 3 });

  say(w, 40, 10, 'far');
  assert.equal(w.bubbles.length, 2, 'a bubble 27 px away is not replaced');

  const v = createWorld(2);
  for (let i = 0; i < 10; i++) say(v, i * 20, 10, `t${i}`);
  assert.equal(v.bubbles.length, 8);
  assert.equal(v.bubbles[0].text, 't2', 'oldest dropped first');
  assert.equal(v.bubbles[7].text, 't9');

  say(v, 500, 500, 'th', 'thought', 2);
  assert.equal(v.bubbles.at(-1).kind, 'thought');
  assert.equal(v.bubbles.at(-1).ttl, 2);
});

test('logEvent keeps the newest 5 first', () => {
  const w = createWorld(1);
  for (let i = 1; i <= 7; i++) logEvent(w, `e${i}`);
  assert.equal(w.log.length, 5);
  assert.equal(w.log[0].text, 'e7');
  assert.equal(w.log[4].text, 'e3');
  assert.deepEqual(w.log[0], { day: 1, hour: 6, text: 'e7' });
});

test('puff adds an effect with defaults and caps at 120', () => {
  const w = createWorld(1);
  puff(w, 'smoke', 1, 2);
  assert.deepEqual(w.effects[0], { kind: 'smoke', x: 1, y: 2, vx: 0, vy: -6, ttl: 1.5, age: 0 });

  const v = createWorld(1);
  for (let i = 0; i < 125; i++) puff(v, 'coin', i, 0);
  assert.equal(v.effects.length, 120);
  assert.equal(v.effects[0].x, 5, 'oldest five dropped');
  assert.equal(v.effects[119].x, 124);
});

test('fakeCtx records method calls and property sets and counts them', () => {
  const ctx = fakeCtx();
  ctx.fillRect(1, 2, 3, 4);
  assert.deepEqual(ctx.calls[0], { op: 'fillRect', args: [1, 2, 3, 4] });
  assert.equal(ctx.count('fillRect'), 1);

  ctx.fillStyle = 'red';
  assert.equal(ctx.fillStyle, 'red');
  assert.deepEqual(ctx.calls[1], { op: 'set', name: 'fillStyle', value: 'red' });
  assert.equal(ctx.count('set'), 1);

  ctx.fillStyle = 'blue';
  assert.equal(ctx.fillStyle, 'blue', 'last value wins');
  assert.equal(ctx.count('set'), 2);

  assert.deepEqual(ctx.canvas, { width: 320, height: 180 });
  assert.deepEqual(ctx.measureText('hi'), { width: 0 });
  assert.doesNotThrow(() => ctx.createLinearGradient(0, 0, 1, 1).addColorStop(0, 'red'));
  assert.doesNotThrow(() => ctx.createRadialGradient(0, 0, 1, 0, 0, 2).addColorStop(0, 'red'));

  const before = ctx.calls.length;
  ctx.count('fillRect');
  assert.equal(ctx.calls.length, before, 'count itself is not recorded');
  assert.equal(ctx.count('nothing'), 0);
});

test('herds start with 3 cows and a bull, 4 hens and a cock, and nothing on unbought land', () => {
  const w = createWorld(1);
  assert.deepEqual(Object.keys(w.herds), KINDS);
  assert.deepEqual(
    { f: w.herds.cow.females, m: w.herds.cow.males, y: w.herds.cow.young, l: w.herds.cow.leased, t: w.herds.cow.tier },
    { f: 3, m: 1, y: 0, l: 0, t: 0 },
  );
  assert.equal(herdTotal(w, 'chicken'), 5);
  assert.equal(herdTotal(w, 'sheep'), 0);
  assert.equal(housingCapacity(w, 'cow'), 10);
  w.herds.cow.tier = SPECIES.cow.tiers.length - 1;
  assert.equal(SPECIES.cow.tiers[w.herds.cow.tier].name, 'Burj Moolifa');
  assert.ok(housingCapacity(w, 'cow') >= 1e6, 'the top tier holds millions');
  assert.equal(hasKind(w, 'cow'), true);
  assert.equal(hasKind(w, 'sheep'), false);
  w.land = 2;
  assert.equal(hasKind(w, 'sheep'), true);
  assert.equal(landWidth(w), 640);
  assert.deepEqual(w.camera, { x: 0 });
  assert.deepEqual(w.herdEvents, []);
});

test('species data: tiers grow in capacity and cost, and every species has a parcel', () => {
  for (const kind of KINDS) {
    const s = SPECIES[kind];
    assert.equal(s.tiers[0].cost, 0, `${kind} starts with free housing`);
    for (let i = 1; i < s.tiers.length; i++) {
      assert.ok(s.tiers[i].capacity > s.tiers[i - 1].capacity, `${kind} tier ${i} capacity`);
      assert.ok(s.tiers[i].cost > s.tiers[i - 1].cost, `${kind} tier ${i} cost`);
    }
    assert.ok(s.parcel < LAND.length, `${kind} lives on a parcel that exists`);
  }
  assert.equal(SPECIES.chicken.tiers.at(-1).name, 'Burj Al Hen');
});
