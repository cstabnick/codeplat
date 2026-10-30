// Tests for web/js/shop.js. The world comes from state.js and the rng from web/js/rng.js. The line
// stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour, productivity } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { offers, buy, PORTAL } from '../web/js/shop.js';
import { SPECIES } from '../web/js/species.js';
import { CARROT_UPGRADES } from '../web/js/carrots.js';

const line = (kind) => 'LINE ' + kind;
const offer = (w, id) => offers(w).find((o) => o.id === id);

test('a fresh farm offers the cow and chicken market and no sheep', () => {
  const w = createWorld(1);
  const market = offers(w).filter((o) => o.section === 'MARKET').map((o) => o.id);
  assert.deepEqual(market, ['cow:female', 'cow:male', 'chicken:female', 'chicken:male']);
  assert.equal(offer(w, 'cow:female').cost, 30);
  assert.equal(offer(w, 'cow:female').bulk, true);
  assert.equal(offer(w, 'cow:female').ok, false, 'a fresh farm has 10 coins and a cow costs 30');
  assert.equal(offer(w, 'cow:female').reason, 'NOT ENOUGH COINS');
});

test('buying land opens the sheep market', () => {
  const w = createWorld(1);
  assert.equal(offer(w, 'sheep:female'), undefined);
  w.coins = 3000;
  assert.equal(buy(w, 'land', 1, makeRng(1), line).ok, true);
  assert.equal(w.coins, 0);
  assert.equal(w.land, 2);
  assert.equal(offer(w, 'sheep:female').cost, 200);
  assert.equal(offer(w, 'sheep:female').owned, '0 EWES');
  assert.equal(offer(w, 'land').reason, 'NOT ENOUGH COINS');
});

test('a fresh farm with no coins says why each market item cannot be bought', () => {
  const w = createWorld(1);
  assert.equal(offer(w, 'cow:female').ok, false);
  assert.equal(offer(w, 'cow:female').reason, 'NOT ENOUGH COINS');
  assert.equal(offer(w, 'cow:female').maxCount, 0);
});

test('buying max of a market animal spends on what the coins and room allow', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  // a fresh pasture holds 10 and has 4 cows in it, so the room is 6
  const result = buy(w, 'cow:female', 'max', rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.count, Math.min(Math.floor(1e6 / 30), 6));
  assert.equal(w.herds.cow.females, 3 + 6);
  assert.equal(w.coins, 1e6 - 6 * 30);
  assert.equal(result.text, 'LINE shop');
  assert.ok(w.bubbles.some((b) => b.text === 'LINE shop'));
});

test('a full pasture refuses a cow until its housing is upgraded', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  buy(w, 'cow:female', 'max', rng, line);
  const refused = buy(w, 'cow:female', 1, rng, line);
  assert.equal(refused.ok, false);
  assert.equal(refused.count, 0);
  assert.equal(refused.text, 'NO ROOM: UPGRADE HOUSING');
  assert.equal(offer(w, 'cow:female').reason, 'NO ROOM: UPGRADE HOUSING');
  assert.equal(buy(w, 'house:cow', 1, rng, line).ok, true);
  assert.equal(offer(w, 'cow:female').ok, true);
});

test('buying a bull or a bulk hen counts only the animals that fit', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  // a coop of 12 holds the 4 hens and 1 cock, so the bull takes one place and 6 hens fit
  assert.equal(buy(w, 'chicken:male', 1, rng, line).count, 1);
  assert.equal(buy(w, 'chicken:female', 10, rng, line).count, 6);
  assert.equal(w.herds.chicken.females, 4 + 6);
  assert.equal(w.herds.chicken.males, 2);
  assert.equal(offer(w, 'chicken:female').reason, 'NO ROOM: UPGRADE HOUSING');
});

test('housing moves up one tier for the price of the next', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 400;
  assert.equal(offer(w, 'house:cow').name, 'Cow Stables');
  assert.equal(offer(w, 'house:cow').cost, 400);
  assert.equal(offer(w, 'house:cow').owned, 'NOW: PASTURE (10)');
  const result = buy(w, 'house:cow', 1, rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.count, 1);
  assert.equal(result.text, 'LINE tier');
  assert.equal(w.herds.cow.tier, 1);
  assert.equal(w.coins, 0);
  assert.equal(offer(w, 'house:cow').reason, 'NOT ENOUGH COINS');
});

test('the top housing tier says so', () => {
  const w = createWorld(1);
  w.herds.cow.tier = SPECIES.cow.tiers.length - 1;
  w.coins = 1e30;
  const top = offer(w, 'house:cow');
  assert.equal(top.name, 'Burj Moolifa');
  assert.equal(top.ok, false);
  assert.equal(top.reason, 'TOP TIER');
  assert.equal(buy(w, 'house:cow', 1, makeRng(1), line).text, 'TOP TIER');
});

test('field upgrades buy up to the count asked and pay each level', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  const start = w.coins;
  const result = buy(w, 'carrot:combines', 10, rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.count, 10);
  assert.equal(w.carrots.combines, 10);
  let spent = 0;
  for (let i = 0; i < 10; i++) spent += Math.round(400 * 1.35 ** i);
  assert.equal(w.coins, start - spent);
});

test('field upgrades that are not bulk buy one level', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1e6;
  const hoe = offer(w, 'carrot:hoe');
  assert.equal(hoe.bulk, false);
  assert.equal(hoe.maxCount, 0);
  assert.equal(buy(w, 'carrot:hoe', 10, rng, line).count, 1);
  assert.equal(w.carrots.hoe, 1);
});

test('the max of a field upgrade follows its price and the coins', () => {
  const w = createWorld(1);
  w.coins = 1000;
  // gloves cost 15, 23, 34, ... so 1000 coins buys the first levels only
  const want = offer(w, 'carrot:gloves').maxCount;
  let spent = 0;
  let count = 0;
  for (let level = 0; ; level++) {
    const cost = Math.round(CARROT_UPGRADES[0].base * CARROT_UPGRADES[0].growth ** level);
    if (spent + cost > 1000) break;
    spent += cost;
    count += 1;
  }
  assert.equal(want, count);
  assert.ok(want > 0);
  assert.equal(buy(w, 'carrot:gloves', 'max', makeRng(1), line).count, want);
});

test('a field upgrade with no coins says so', () => {
  const w = createWorld(1);
  assert.equal(offer(w, 'carrot:combines').reason, 'NOT ENOUGH COINS');
  assert.equal(offer(w, 'carrot:combines').maxCount, 0);
});

test('a UFO lease sends min(cow females, coins / 5) cows', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 12;
  const result = buy(w, 'lease', 'max', rng, line);
  assert.equal(result.count, Math.min(3, Math.floor(12 / 5)));
  assert.equal(result.count, 2);
  assert.equal(w.herds.cow.leased, 2);
  assert.equal(w.herds.cow.females, 1);
  assert.equal(w.coins, 2);
  assert.equal(offer(w, 'lease').owned, '2 COWS IN SPACE');
});

test('a UFO lease needs a free cow', () => {
  const w = createWorld(1);
  w.coins = 1e6;
  w.herds.cow.females = 0;
  assert.equal(offer(w, 'lease').reason, 'NO FREE COWS');
  assert.equal(buy(w, 'lease', 'max', makeRng(1), line).ok, false);
  assert.equal(w.coins, 1e6);
});

test('a lease of one cow takes the price up front', () => {
  const w = createWorld(1);
  w.coins = 5;
  assert.equal(buy(w, 'lease', 1, makeRng(1), line).count, 1);
  assert.equal(w.coins, 0);
});

test('a refused purchase costs nothing and an unknown id is refused', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 3;
  assert.equal(buy(w, 'cow:female', 1, rng, line).ok, false);
  assert.equal(w.coins, 3);
  const unknown = buy(w, 'nope', 1, rng, line);
  assert.equal(unknown.ok, false);
  assert.equal(unknown.count, 0);
  assert.equal(unknown.text, 'I DO NOT SELL THAT. YET.');
});

test('cigarettes raise productivity and send Gary to smoke by day only', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1000;
  assert.equal(buy(w, 'cigs', 1, rng, line).ok, true);
  assert.equal(productivity(w), 1.25);
  assert.equal(w.farmer.task, 'smoke');
  assert.equal(offer(w, 'cigs').cost, 60, 'each pack costs three times the last');

  const night = createWorld(1);
  night.coins = 1000;
  setHour(night, 23);
  buy(night, 'cigs', 1, rng, line);
  assert.notEqual(night.farmer.task, 'smoke');
});

test('the portal is bought once and doubles productivity, cigarettes included', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 20;
  buy(w, 'cigs', 1, rng, line);
  w.coins = PORTAL;
  const result = buy(w, 'portal', 1, rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.count, 1);
  assert.equal(w.coins, 0);
  assert.equal(productivity(w), 2.5);
  assert.ok(result.text.startsWith('HOME!'));
  w.coins = PORTAL;
  assert.equal(buy(w, 'portal', 1, rng, line).ok, false);
  assert.equal(offer(w, 'portal').reason, 'HE IS HOME (SOMETIMES)');
});

test('the portal shows the shortfall until it is affordable', () => {
  const w = createWorld(1);
  assert.equal(offer(w, 'portal').cost, PORTAL);
  assert.equal(offer(w, 'portal').reason, 'NOT ENOUGH COINS');
});

test('offers lists the sections in order', () => {
  const w = createWorld(1);
  const sections = [...new Set(offers(w).map((o) => o.section))];
  assert.deepEqual(sections, ['MARKET', 'HOUSING', 'LAND', 'FIELD', 'GARY', 'ALIENS', 'PORTAL']);
});

test('a fresh farm lists its herd in the owned text of each market item', () => {
  const w = createWorld(1);
  assert.equal(offer(w, 'cow:female').owned, '3 COWS');
  assert.equal(offer(w, 'cow:male').owned, '1 BULLS');
  assert.equal(offer(w, 'chicken:female').owned, '4 HENS');
  assert.equal(offer(w, 'chicken:male').owned, '1 COCKS');
});

test('buying a bulk item with too few coins for the count buys what the coins cover', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.coins = 1000;
  // combine levels cost 400, 540, 729, ...: 1000 coins covers two of them
  const result = buy(w, 'carrot:combines', 10, rng, line);
  assert.equal(result.ok, true);
  assert.equal(result.count, 2);
  assert.equal(w.carrots.combines, 2);
  assert.equal(w.coins, 1000 - 400 - Math.round(400 * 1.35));
});

test('a market animal bought max with few coins takes what the coins cover', () => {
  const w = createWorld(1);
  w.coins = 95;
  const result = buy(w, 'cow:female', 'max', makeRng(1), line);
  assert.equal(result.count, 3);
  assert.equal(w.herds.cow.females, 6);
  assert.equal(w.coins, 5);
});

test('a UFO lease with plenty of coins takes every cow female', () => {
  const w = createWorld(1);
  w.coins = 1e6;
  const result = buy(w, 'lease', 'max', makeRng(1), line);
  assert.equal(result.count, 3);
  assert.equal(w.herds.cow.females, 0);
  assert.equal(w.herds.cow.leased, 3);
  assert.equal(w.coins, 1e6 - 3 * 5);
  assert.equal(offer(w, 'lease').reason, 'NO FREE COWS');
});

test('buying the next parcel says so, and the last parcel is reported as owned', () => {
  const w = createWorld(1);
  w.coins = 1e6;
  const result = buy(w, 'land', 1, makeRng(1), line);
  assert.equal(result.ok, true);
  assert.equal(result.count, 1);
  assert.equal(result.text, 'LINE land');
  assert.equal(w.land, 2);
  assert.equal(buy(w, 'land', 1, makeRng(1), line).ok, true);
  assert.equal(w.land, 3);
  const done = offer(w, 'land');
  assert.equal(done.ok, false);
  assert.equal(done.reason, 'ALL LAND OWNED');
  assert.equal(done.owned, '3/3 PARCELS');
  assert.equal(buy(w, 'land', 1, makeRng(1), line).ok, false);
});
