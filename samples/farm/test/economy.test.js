// Tests for web/js/economy.js. Worlds come from state.js; the economy module is the only thing under test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld } from '../web/js/state.js';
import { incomeRate, updateEconomy, creditAway } from '../web/js/economy.js';

const DT = 0.1;
const TOL = 1e-6;

// runFor advances the economy by the given number of seconds in 0.1 s steps.
function runFor(w, seconds) {
  const steps = Math.round(seconds / DT);
  for (let i = 0; i < steps; i++) updateEconomy(w, DT);
}

test('incomeRate of a fresh world is 3 cows at $3/10 s plus 4 hens at $1/12 s', () => {
  const w = createWorld(1);
  assert.ok(Math.abs(incomeRate(w) - (3 * 3 / 10 + 4 * 1 / 12)) < TOL, 'bulls and cocks make nothing');
});

test('incomeRate is multiplied by the cigarette boost', () => {
  const w = createWorld(1);
  const base = incomeRate(w);
  w.upgrades.cigs = 1;
  assert.ok(Math.abs(incomeRate(w) - base * 1.25) < TOL);
});

test('incomeRate includes the combine rate and the alien lease rate', () => {
  const w = createWorld(1);
  const base = incomeRate(w);
  w.carrots.combines = 2;
  const withCombines = incomeRate(w);
  assert.ok(withCombines > base, 'combines add to the income');
  w.herds.cow.leased = 10;
  assert.ok(Math.abs(incomeRate(w) - (withCombines + 10 * 0.6)) < TOL, 'ten leased cows add 0.6 each');
});

test('a huge cow herd earns by count, and adds few effects', () => {
  const w = createWorld(1);
  w.herds.cow.females = 2e6;
  w.herds.cow.tier = 6;
  const coins0 = w.coins;
  runFor(w, 10);
  const earned = w.coins - coins0;
  const expected = 2e6 * 0.3 * 10;
  // Chickens (4 hens) also earn, so compare with their share removed.
  const hens = 4 * 1 / 12 * 10;
  assert.ok(Math.abs(earned - expected - hens) / expected < 0.001, `earned ${earned}, expected about ${expected + hens}`);
  assert.ok(w.stats.milk > 1e6, 'the milk count is in the millions');
  // One effect per kind per update: at most 100 milk effects for 100 updates, plus a few egg effects.
  const milk = w.effects.filter((e) => e.kind === 'milk').length;
  assert.ok(milk <= 100, `milk effects ${milk}, expected at most 100`);
  assert.ok(w.effects.length <= 110, `effects ${w.effects.length}, expected at most 110`);
});

test('updateEconomy for 10 s pays about 3 bottles per cow and the eggs that are due', () => {
  const w = createWorld(1);
  const coins0 = w.coins;
  runFor(w, 10);
  // Three cows make 3 bottles in 10 s and four hens lay 40/12 = 3.33 eggs, so whole units are paid.
  assert.ok(Math.abs(w.stats.milk - 3) <= 1, `milk ${w.stats.milk}`);
  assert.ok(Math.abs(w.stats.eggs - 3) <= 1, `eggs ${w.stats.eggs}`);
  const expected = w.stats.milk * 3 + w.stats.eggs * 1;
  assert.ok(Math.abs(w.coins - coins0 - expected) < TOL, 'milk pays $3 each, eggs $1 each');
  assert.ok(w.effects.some((e) => e.kind === 'milk'), 'a milk effect for the cows');
  assert.ok(w.effects.some((e) => e.kind === 'egg'), 'an egg effect for the hens');
});

test('updateEconomy pays no alien lease money', () => {
  const w = createWorld(1);
  w.herds.cow.leased = 10;
  const base = createWorld(1);
  runFor(w, 10);
  runFor(base, 10);
  assert.equal(w.coins, base.coins, 'leased cows pay through aliens.js, not here');
});

test('sheep earn nothing until the farm owns the Sheep Meadow, then females * 12 / 20 a second', () => {
  const w = createWorld(1);
  // Only sheep, so the cows and hens do not muddy the sums.
  w.herds.cow.females = 0;
  w.herds.chicken.females = 0;
  w.herds.sheep.females = 20;
  assert.equal(incomeRate(w), 0, 'no sheep income before the meadow');
  runFor(w, 10);
  assert.equal(w.coins, 10, 'no coins before the meadow');
  w.land = 2;
  assert.ok(Math.abs(incomeRate(w) - 20 * 12 / 20) < TOL, 'sheep add 12 per second');
  const coins0 = w.coins;
  runFor(w, 10);
  // Sheep pay whole 12-coin units, so 10 s may fall one unit short of 120.
  assert.ok(Math.abs(w.coins - coins0 - 120) <= 12, `sheep paid ${w.coins - coins0} in 10 s, expected about 120`);
  assert.ok(w.stats.wool >= 9 && w.stats.wool <= 11, `wool ${w.stats.wool}, expected about 10`);
});

test('creditAway pays the herds and combines for the time away, not alien pay', () => {
  const w = createWorld(1);
  w.carrots.combines = 2;
  w.herds.cow.leased = 10;
  const expected = (incomeRate(w) - 10 * 0.6) * 100;
  const coins0 = w.coins;
  const credited = creditAway(w, 100);
  assert.ok(Math.abs(credited - expected) < TOL, `credited ${credited}, expected ${expected}`);
  assert.ok(Math.abs(w.coins - coins0 - expected) < TOL, 'the credit is added to coins');
});

test('creditAway with negative seconds credits nothing', () => {
  const w = createWorld(1);
  const coins0 = w.coins;
  assert.equal(creditAway(w, -50), 0);
  assert.equal(w.coins, coins0);
});

test('productivity multiplies herd products and combines alike', () => {
  const base = createWorld(1);
  const boosted = createWorld(1);
  boosted.upgrades.cigs = 3;
  assert.ok(Math.abs(incomeRate(boosted) - incomeRate(base) * 1.25 ** 3) < TOL);
});

test('two identical worlds stepped the same way end with identical effects and coins', () => {
  const a = createWorld(7);
  const b = createWorld(7);
  a.herds.cow.females = 40;
  b.herds.cow.females = 40;
  a.land = 2;
  b.land = 2;
  a.herds.sheep.females = 9;
  b.herds.sheep.females = 9;
  runFor(a, 30);
  runFor(b, 30);
  assert.equal(a.coins, b.coins, 'the same coins');
  assert.deepEqual(a.effects, b.effects, 'the same effects in the same places');
  assert.ok(a.effects.length > 0, 'the herds made some effects');
});
