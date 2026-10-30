// Tests for web/js/herds.js. Worlds come from state.js and random numbers from rng.js; herds.js is the only thing under test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, herdTotal, housingCapacity } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import {
  updateHerds, room, unitPrice, maxBuy, buyAnimals, nextTier, upgradeHousing, nextLand, buyLand,
} from '../web/js/herds.js';

const DT = 0.1;

// onlyCows empties the chicken herd so that the births and deaths counted in stats are the cows'.
function onlyCows(w) {
  w.herds.chicken.females = 0;
  w.herds.chicken.males = 0;
  w.herds.chicken.young = 0;
}

// runFor advances the herds by the given number of seconds in steps of dt, and returns the largest
// young count seen along the way.
function runFor(w, seconds, rng, dt = DT) {
  const steps = Math.round(seconds / dt);
  let maxYoung = 0;
  for (let i = 0; i < steps; i++) {
    updateHerds(w, dt, rng);
    maxYoung = Math.max(maxYoung, w.herds.cow.young);
  }
  return maxYoung;
}

test('a fresh world\'s cows have young within 300 s and the young grow up into both sexes', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  let sawFemaleUp = false;
  let sawMaleUp = false;
  let sawYoung = false;
  for (let i = 0; i < 3000; i++) {
    const before = { f: w.herds.cow.females, m: w.herds.cow.males };
    updateHerds(w, DT, rng);
    if (w.herds.cow.young > 0) sawYoung = true;
    if (w.herds.cow.females > before.f) sawFemaleUp = true;
    if (w.herds.cow.males > before.m) sawMaleUp = true;
  }
  assert.ok(sawYoung, 'the cows have young within 300 s');
  assert.ok(sawFemaleUp, 'some young grew up into females');
  assert.ok(sawMaleUp, 'some young grew up into males');
});

test('a herd with no males has no births', () => {
  const w = createWorld(1);
  onlyCows(w);
  w.herds.cow.males = 0;
  w.herds.cow.females = 3;
  w.herds.cow.young = 0;
  runFor(w, 100, makeRng(2));
  assert.equal(w.stats.births, 0);
  assert.equal(w.herds.cow.young, 0);
});

test('a herd with no room has no births', () => {
  const w = createWorld(1);
  onlyCows(w);
  w.herds.cow.tier = 0;
  w.herds.cow.females = 8;
  w.herds.cow.males = 2;
  w.herds.cow.young = 0;
  assert.equal(herdTotal(w, 'cow'), housingCapacity(w, 'cow'));
  assert.equal(room(w, 'cow'), 0);
  runFor(w, 20, makeRng(3));
  assert.equal(w.stats.births, 0);
  assert.equal(w.herds.cow.young, 0);
});

test('a million young grow up with about half of them female', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 6;
  w.herds.cow.females = 0;
  w.herds.cow.males = 0;
  w.herds.cow.young = 1e6;
  updateHerds(w, 1, makeRng(4));
  const grown = 1e6 - w.herds.cow.young;
  assert.ok(grown > 0, 'some young grew up');
  const share = w.herds.cow.females / (w.herds.cow.females + w.herds.cow.males);
  assert.ok(share > 0.45 && share < 0.55, `female share ${share} is between 0.45 and 0.55`);
});

test('deaths follow the mean adult life and are counted in stats.deaths', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 6;
  w.herds.cow.females = 1e6;
  w.herds.cow.males = 1e6;
  const expected = 2e6 * 10 / 1080;
  runFor(w, 10, makeRng(5));
  const deaths = w.stats.deaths;
  assert.ok(Math.abs(deaths - expected) / expected < 0.05, `${deaths} deaths is within 5% of ${expected}`);
  assert.ok(w.herds.cow.females >= 0 && w.herds.cow.males >= 0, 'no sex goes negative');
});

test('maxBuy and buyAnimals buy as many bulls as the coins and the room allow', () => {
  const w = createWorld(1);
  assert.equal(unitPrice('cow', 'male'), 60);
  w.coins = 1000;
  assert.equal(maxBuy(w, 'cow', 'male'), Math.min(Math.floor(1000 / 60), room(w, 'cow')));
  const bullsBefore = w.herds.cow.males;
  const bought = buyAnimals(w, 'cow', 'male', 'max');
  assert.equal(bought, Math.min(Math.floor(1000 / 60), 6));
  assert.equal(w.herds.cow.males, bullsBefore + bought);
  assert.equal(w.coins, 1000 - bought * 60);
  assert.deepEqual(w.herdEvents.at(-1), { kind: 'cow', type: 'buy', sex: 'male', count: bought });
});

test('buyAnimals is limited by the coins when they run short', () => {
  const w = createWorld(1);
  w.coins = 100;
  assert.equal(buyAnimals(w, 'cow', 'male', 'max'), 1);
  assert.equal(w.coins, 40);
});

test('buyAnimals on sheep buys nothing while the farm does not own the sheep meadow', () => {
  const w = createWorld(1);
  w.land = 1;
  w.coins = 1e6;
  assert.equal(maxBuy(w, 'sheep', 'female'), 0);
  assert.equal(buyAnimals(w, 'sheep', 'female', 'max'), 0);
  assert.equal(w.coins, 1e6);
  assert.equal(w.herds.sheep.females, 0);
});

test('upgradeHousing refuses when poor, then moves the cows up a tier', () => {
  const w = createWorld(1);
  assert.equal(nextTier(w, 'cow').name, 'Cow Stables');
  w.coins = 10;
  assert.equal(upgradeHousing(w, 'cow'), false);
  assert.equal(w.herds.cow.tier, 0);
  assert.equal(w.coins, 10);
  w.coins = 400;
  const before = housingCapacity(w, 'cow');
  assert.equal(upgradeHousing(w, 'cow'), true);
  assert.equal(w.herds.cow.tier, 1);
  assert.equal(housingCapacity(w, 'cow'), 60);
  assert.ok(housingCapacity(w, 'cow') > before);
  assert.equal(w.coins, 0);
  assert.equal(w.log[0].text, 'NEW HOUSING: COW STABLES');
});

test('upgradeHousing refuses at the top tier', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 6;
  w.coins = 1e12;
  assert.equal(nextTier(w, 'cow'), null);
  assert.equal(upgradeHousing(w, 'cow'), false);
  assert.equal(w.herds.cow.tier, 6);
  assert.equal(w.coins, 1e12);
});

test('buyLand buys the sheep meadow for 3000 coins and refuses when all land is owned', () => {
  const w = createWorld(1);
  assert.equal(nextLand(w).name, 'Sheep Meadow');
  w.coins = 2999;
  assert.equal(buyLand(w), false);
  assert.equal(w.land, 1);
  w.coins = 3000;
  assert.equal(buyLand(w), true);
  assert.equal(w.land, 2);
  assert.equal(w.coins, 0);
  assert.equal(w.log[0].text, 'BOUGHT SHEEP MEADOW');
  w.coins = 1e12;
  w.land = 3;
  assert.equal(nextLand(w), null);
  assert.equal(buyLand(w), false);
  assert.equal(w.land, 3);
});

test('herdEvents never holds more than 50 entries', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 6;
  w.herds.cow.females = 1e6;
  w.herds.cow.males = 1e6;
  runFor(w, 20, makeRng(6));
  assert.ok(w.herdEvents.length > 0);
  assert.ok(w.herdEvents.length <= 50, `${w.herdEvents.length} events`);
});

test('10,000 updates of a herd of 2e6 cows finish in under 1 s', () => {
  const w = createWorld(1);
  w.herds.cow.tier = 6;
  w.herds.cow.females = 1e6;
  w.herds.cow.males = 1e6;
  const rng = makeRng(7);
  const start = performance.now();
  for (let i = 0; i < 10000; i++) updateHerds(w, DT, rng);
  const elapsed = performance.now() - start;
  assert.ok(elapsed < 1000, `took ${elapsed.toFixed(1)} ms`);
});
