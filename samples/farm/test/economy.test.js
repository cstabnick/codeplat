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

// withoutAliens is incomeRate of w with its saucers removed. It is computed on a copy so w is untouched.
function incomeWithoutAliens(w) {
  return incomeRate({ ...w, ufos: [] });
}

test('incomeRate of a fresh world is 4 cows at $0.3 plus 5 hens at 1/12', () => {
  const w = createWorld(1);
  assert.ok(Math.abs(incomeRate(w) - (4 * 0.3 + 5 / 12)) < TOL);
});

test('incomeRate ignores abducted cows and calves', () => {
  const w = createWorld(1);
  const full = incomeRate(w);
  w.animals.find((a) => a.id === 'cow-1').abducted = true;
  assert.ok(Math.abs(incomeRate(w) - (full - 0.3)) < TOL, 'an abducted cow pays nothing');
  w.animals.find((a) => a.id === 'cow-2').age = 0;
  assert.ok(Math.abs(incomeRate(w) - (full - 0.6)) < TOL, 'a calf pays nothing');
});

test('incomeRate is multiplied by the cigarette boost', () => {
  const w = createWorld(1);
  const base = incomeRate(w);
  w.upgrades.cigs = 1;
  assert.ok(Math.abs(incomeRate(w) - base * 1.25) < TOL);
});

test('updateEconomy for 10 s pays 4 bottles and the eggs that are due', () => {
  const w = createWorld(1);
  const coins0 = w.coins;
  runFor(w, 10);
  assert.equal(w.stats.milk, 4, 'four cows, one bottle each in 10 s');
  // Five hens lay 50/12 = 4.17 eggs in 10 s, so four whole eggs are paid.
  assert.equal(w.stats.eggs, 4);
  assert.ok(Math.abs(w.coins - coins0 - (12 + 4)) < TOL, 'milk pays $3 each, eggs $1 each');
  const milkEffects = w.effects.filter((e) => e.kind === 'milk');
  assert.equal(milkEffects.length, 4, 'a milk effect for each bottle');
});

test('a hovering saucer pays about 0.6 coins per second', () => {
  const base = createWorld(1);
  const withSaucer = createWorld(1);
  withSaucer.ufos.push({ state: 'hold', x: 200, y: 40, rate: 0.6 });
  runFor(base, 10);
  runFor(withSaucer, 10);
  const extra = withSaucer.coins - base.coins;
  assert.ok(Math.abs(extra - 6) < 1, `alien pay over 10 s was ${extra}, expected about 6`);
  assert.ok(withSaucer.effects.some((e) => e.kind === 'coin'), 'alien pay shows a coin effect');
});

test('a saucer that is arriving pays nothing', () => {
  const base = createWorld(1);
  const arriving = createWorld(1);
  arriving.ufos.push({ state: 'arrive', x: 200, y: 40, rate: 0.6 });
  assert.ok(Math.abs(incomeRate(arriving) - incomeRate(base)) < TOL);
  runFor(base, 10);
  runFor(arriving, 10);
  assert.equal(arriving.coins, base.coins);
});

test('creditAway pays the milk and egg income for the time away, not alien pay', () => {
  const w = createWorld(1);
  w.ufos.push({ state: 'hold', x: 200, y: 40, rate: 0.6 });
  const expected = incomeWithoutAliens(w) * 100;
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

test('productivity multiplies milk and eggs but not alien pay', () => {
  const base = createWorld(1);
  const withSaucer = createWorld(1);
  for (const w of [base, withSaucer]) w.upgrades.cigs = 3;
  withSaucer.ufos.push({ state: 'hold', x: 200, y: 40, rate: 0.6 });
  assert.ok(Math.abs(incomeRate(withSaucer) - incomeRate(base) - 0.6) < TOL);
  runFor(base, 10);
  runFor(withSaucer, 10);
  const extra = withSaucer.coins - base.coins;
  assert.ok(Math.abs(extra - 6) < 1, `alien pay over 10 s was ${extra}, expected about 6`);
});
