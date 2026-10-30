// Tests for web/js/shop.js. The world comes from state.js and the rng from web/js/rng.js. The line
// stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour, herdSize, productivity } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { offers, buy, price, PORTAL } from '../web/js/shop.js';

const line = (kind) => 'LINE ' + kind;
const offer = (w, id) => offers(w).find((o) => o.id === id);

test('offers lists every item, and a fresh farm can afford only the cheap ones', () => {
  const w = createWorld(1);
  assert.deepEqual(offers(w).map((o) => o.id), ['cow', 'chicken', 'ufo', 'cigs', 'pasture', 'coop', 'portal']);
  assert.equal(offer(w, 'ufo').ok, true, 'a UFO contract costs 5');
  assert.equal(offer(w, 'cow').ok, false);
  assert.equal(offer(w, 'cow').reason, 'NOT ENOUGH COINS');
});

test('buying a chicken spends its price, adds an adult and the prince speaks', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 100;
  const cost = price(w, 'chicken');
  const result = buy(w, 'chicken', rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.text, 'LINE shop');
  assert.equal(w.coins, 100 - cost);
  assert.equal(herdSize(w, 'chicken'), 6);
  assert.ok(price(w, 'chicken') > cost, 'the next chicken costs more');
  assert.ok(w.bubbles.some((b) => b.text === 'LINE shop'));
});

test('a full pasture refuses a cow until it is made bigger', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  assert.equal(buy(w, 'cow', rng, line).ok, true);
  assert.equal(buy(w, 'cow', rng, line).ok, true);
  const refused = buy(w, 'cow', rng, line);
  assert.equal(refused.ok, false);
  assert.equal(refused.text, 'PASTURE FULL');
  assert.equal(buy(w, 'pasture', rng, line).ok, true);
  assert.equal(offer(w, 'cow').ok, true);
});

test('a refused purchase costs nothing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 3;
  assert.equal(buy(w, 'ufo', rng, line).ok, false);
  assert.equal(w.coins, 3);
  assert.equal(buy(w, 'nope', rng, line).ok, false);
});

test('UFO contracts: one per free adult cow, each 5 dearer than the last', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1000;
  const costs = [];
  for (let i = 0; i < 4; i++) {
    costs.push(price(w, 'ufo'));
    assert.equal(buy(w, 'ufo', rng, line).ok, true);
  }
  assert.deepEqual(costs, [5, 10, 15, 20]);
  assert.equal(w.ufos.length, 4);
  assert.equal(new Set(w.ufos.map((u) => u.targetId)).size, 4);
  assert.equal(offer(w, 'ufo').reason, 'NO FREE ADULT COW');
});

test('cigarettes raise productivity and send Gary to smoke by day only', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1000;
  buy(w, 'cigs', rng, line);
  assert.equal(productivity(w), 1.25);
  assert.equal(w.farmer.task, 'smoke');
  assert.equal(price(w, 'cigs'), 40);

  const night = createWorld(1);
  night.coins = 1000;
  setHour(night, 23);
  buy(night, 'cigs', rng, line);
  assert.notEqual(night.farmer.task, 'smoke');
});

test('the portal is bought once and doubles productivity', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = PORTAL;
  assert.equal(buy(w, 'portal', rng, line).ok, true);
  assert.equal(w.coins, 0);
  assert.equal(productivity(w), 2);
  w.coins = PORTAL;
  assert.equal(buy(w, 'portal', rng, line).ok, false);
});

test('upgrades stop at the largest herd', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e9;
  let bought = 0;
  while (buy(w, 'coop', rng, line).ok) bought += 1;
  assert.equal(bought, 5, 'from room for 8 to room for 28');
  assert.equal(offer(w, 'coop').reason, 'AS BIG AS IT GETS');
});
