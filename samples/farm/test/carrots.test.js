// Tests for carrots.js: the clicker rules (click power, pluck, upgrades, plot lookup) and the
// combine harvesters (their coins, their movement and their drawable).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, FIELD, CARROT_VALUE, RIPE_BONUS } from '../web/js/state.js';
import {
  CARROT_UPGRADES,
  clickPower,
  combineRate,
  upgradeCost,
  carrotUpgrades,
  buyCarrotUpgrade,
  plotAt,
  pluck,
  updateCarrots,
  carrotDrawables,
} from '../web/js/carrots.js';
import { fakeCtx } from './fakectx.js';

// A world with no coins and the 18 field plots, all planted with carrots and unwatered.
function fieldWorld() {
  const w = createWorld(1);
  w.coins = 0;
  w.plots = [];
  for (let row = 0; row < FIELD.rows; row++) {
    for (let col = 0; col < FIELD.cols; col++) {
      w.plots.push({ col, row, crop: 'carrot', growth: 0, stage: 0, water: 0 });
    }
  }
  return w;
}

const near = (a, b, eps = 1e-9) => assert.ok(Math.abs(a - b) < eps, `${a} != ${b}`);

test('CARROT_UPGRADES lists the four upgrades in shop order', () => {
  assert.deepEqual(CARROT_UPGRADES.map((u) => u.id), ['gloves', 'hoe', 'fertilizer', 'combines']);
});

test('clickPower is 1 on a fresh world, 3 with 2 gloves, 9 with 2 gloves and 2 hoes', () => {
  const w = fieldWorld();
  assert.equal(clickPower(w), 1);
  w.carrots.gloves = 2;
  assert.equal(clickPower(w), 3);
  w.carrots.hoe = 2;
  assert.equal(clickPower(w), 9);
});

test('combineRate is zero with no combines and scales with combines and productivity, not click power', () => {
  const w = fieldWorld();
  assert.equal(combineRate(w), 0);
  w.carrots.combines = 3;
  assert.equal(combineRate(w), 3 * 2 * CARROT_VALUE);
  w.carrots.gloves = 2;
  w.carrots.hoe = 5;
  assert.equal(combineRate(w), 3 * 2 * CARROT_VALUE, 'gloves and hoes are for clicks');
  w.upgrades.cigs = 1;
  near(combineRate(w), 3 * 2 * CARROT_VALUE * 1.25);
});

test('upgradeCost is round(base * growth ** level)', () => {
  const w = fieldWorld();
  assert.equal(upgradeCost(w, 'gloves'), 15);
  w.carrots.gloves = 2;
  assert.equal(upgradeCost(w, 'gloves'), Math.round(15 * 1.5 ** 2));
  assert.equal(upgradeCost(w, 'hoe'), 250);
  w.carrots.hoe = 3;
  assert.equal(upgradeCost(w, 'hoe'), Math.round(250 * 2.5 ** 3));
});

test('upgradeCost throws for an unknown upgrade', () => {
  assert.throws(() => upgradeCost(fieldWorld(), 'rake'));
});

test('carrotUpgrades lists one entry per upgrade, in order, with ok and reason', () => {
  const w = fieldWorld();
  w.coins = 20;
  const list = carrotUpgrades(w);
  assert.deepEqual(list.map((u) => u.id), CARROT_UPGRADES.map((u) => u.id));
  const gloves = list[0];
  assert.equal(gloves.level, 0);
  assert.equal(gloves.cost, 15);
  assert.equal(gloves.ok, true);
  assert.equal(gloves.reason, '');
  const hoe = list[1];
  assert.equal(hoe.ok, false);
  assert.equal(hoe.reason, 'NOT ENOUGH COINS');
});

test('carrotUpgrades reports MAXED at the level cap, before the coin check', () => {
  const w = fieldWorld();
  w.carrots.hoe = 30;
  w.coins = 0;
  const hoe = carrotUpgrades(w)[1];
  assert.equal(hoe.reason, 'MAXED');
  assert.equal(hoe.ok, false);
});

test('buyCarrotUpgrade buys gloves with 100 coins while affordable and leaves the remainder', () => {
  const w = fieldWorld();
  w.coins = 100;
  // Expected costs are 15, 23, 34 and 51 for levels 0 to 3: 15 + 23 + 34 = 72, leaving 28.
  let coins = 100;
  let level = 0;
  let expected = 0;
  while (true) {
    const cost = Math.round(15 * 1.5 ** level);
    if (cost > coins) break;
    coins -= cost;
    level += 1;
    expected += 1;
  }
  const bought = buyCarrotUpgrade(w, 'gloves', 'max');
  assert.equal(bought, expected);
  assert.equal(w.carrots.gloves, level);
  assert.equal(w.coins, coins);
  assert.equal(w.coins, 28);
  assert.equal(bought, 3);
});

test('buyCarrotUpgrade with a number buys at most that many levels', () => {
  const w = fieldWorld();
  w.coins = 1e6;
  assert.equal(buyCarrotUpgrade(w, 'gloves', 2), 2);
  assert.equal(w.carrots.gloves, 2);
  assert.equal(buyCarrotUpgrade(w, 'gloves'), 1);
  assert.equal(w.carrots.gloves, 3);
});

test('buyCarrotUpgrade buys nothing when the coins are short', () => {
  const w = fieldWorld();
  w.coins = 14;
  assert.equal(buyCarrotUpgrade(w, 'gloves', 'max'), 0);
  assert.equal(w.carrots.gloves, 0);
  assert.equal(w.coins, 14);
});

test('the hoe stops at level 30 with MAXED', () => {
  const w = fieldWorld();
  w.coins = 1e30;
  assert.equal(buyCarrotUpgrade(w, 'hoe', 'max'), 30);
  assert.equal(w.carrots.hoe, 30);
  assert.equal(carrotUpgrades(w)[1].reason, 'MAXED');
  assert.equal(buyCarrotUpgrade(w, 'hoe', 'max'), 0);
  assert.equal(w.carrots.hoe, 30);
});

test('plotAt finds plot 0 at (x+1, y+1), plot 7 at (x+17, y+17), and -1 outside the field', () => {
  assert.equal(plotAt(FIELD.x + 1, FIELD.y + 1), 0);
  assert.equal(plotAt(FIELD.x + 16 + 1, FIELD.y + 16 + 1), 7);
  assert.equal(plotAt(FIELD.x + 16, FIELD.y), 1);
  assert.equal(plotAt(FIELD.x - 1, FIELD.y + 1), -1);
  assert.equal(plotAt(FIELD.x + 1, FIELD.y - 1), -1);
  assert.equal(plotAt(FIELD.x + FIELD.cols * 16, FIELD.y + 1), -1);
  assert.equal(plotAt(FIELD.x + 1, FIELD.y + FIELD.rows * 16), -1);
  assert.equal(plotAt(0, 0), -1);
});

test('pluck on a ripe plot earns RIPE_BONUS * clickPower, and resets the plot', () => {
  const w = fieldWorld();
  w.carrots.gloves = 2;
  w.carrots.hoe = 2;
  const p = w.plots[7];
  p.growth = 1;
  p.stage = 3;
  p.water = 0.8;
  const earned = pluck(w, 7);
  assert.equal(earned, CARROT_VALUE * RIPE_BONUS * 9);
  assert.equal(w.coins, earned);
  assert.equal(p.growth, 0);
  assert.equal(p.stage, 0);
  assert.equal(p.water, 0.8, 'water above 0.5 is kept');
  assert.equal(w.stats.plucks, 1);
});

test('pluck on an unripe plot earns clickPower and raises water to at least 0.5', () => {
  const w = fieldWorld();
  w.carrots.gloves = 2;
  w.carrots.hoe = 2;
  const p = w.plots[3];
  p.growth = 0.5;
  p.stage = 1;
  p.water = 0;
  const earned = pluck(w, 3);
  assert.equal(earned, CARROT_VALUE * 9);
  assert.equal(p.growth, 0);
  assert.equal(p.stage, 0);
  assert.equal(p.water, 0.5);
  assert.equal(w.stats.plucks, 1);
});

test('pluck adds a carrot effect above the plot, and counts every pluck', () => {
  const w = fieldWorld();
  pluck(w, 7);
  pluck(w, 7);
  assert.equal(w.stats.plucks, 2);
  const carrots = w.effects.filter((e) => e.kind === 'carrot');
  assert.equal(carrots.length, 2);
  const p = w.plots[7];
  const tileX = FIELD.x + p.col * 16;
  const tileY = FIELD.y + p.row * 16;
  assert.equal(carrots[0].x, tileX + 8);
  assert.equal(carrots[0].y, tileY + 4);
  assert.equal(carrots[0].vy, -14);
  assert.equal(carrots[0].ttl, 0.8);
});

test('pluck(world, -1) picks the plot with the largest growth', () => {
  const w = fieldWorld();
  w.plots[2].growth = 0.4;
  w.plots[11].growth = 0.9;
  w.plots[11].stage = 2;
  w.plots[11].water = 1;
  pluck(w, -1);
  assert.equal(w.plots[11].growth, 0, 'the most grown plot is replanted');
  assert.equal(w.plots[2].growth, 0.4, 'other plots are untouched');
});

test('pluck(world, -1) with every plot at zero growth still plucks plot 0', () => {
  const w = fieldWorld();
  pluck(w, -1);
  assert.equal(w.stats.plucks, 1);
  assert.equal(w.coins, 1);
  assert.equal(w.plots[0].water, 0.5);
});

test('updateCarrots with no combines earns nothing and plucks nothing', () => {
  const w = fieldWorld();
  updateCarrots(w, 10);
  assert.equal(w.coins, 0);
  assert.equal(w.carrots.acc, 0);
  assert.equal(w.stats.plucks, 0);
  assert.ok(w.plots.every((p) => p.growth === 0));
});

test('3 combines for 10 s in 0.1 s steps earn about combineRate * 10 and stay inside the field', () => {
  const w = fieldWorld();
  w.coins = 0;
  w.carrots.combines = 3;
  const rate = combineRate(w);
  const left = FIELD.x;
  const right = FIELD.x + FIELD.cols * 16 - 16;
  for (let i = 0; i < 100; i++) {
    updateCarrots(w, 0.1);
    assert.ok(w.carrots.combineX >= left && w.carrots.combineX <= right,
      `combine at ${w.carrots.combineX} outside [${left}, ${right}]`);
  }
  const expected = rate * 10;
  assert.ok(Math.abs(w.coins - expected) <= CARROT_VALUE * clickPower(w) * 1 + 1e-9,
    `coins ${w.coins}, expected about ${expected}`);
});

test('a combine plucks the most grown plot, resetting it, while earning coins', () => {
  const w = fieldWorld();
  w.carrots.combines = 1;
  w.plots[5].growth = 0.8;
  w.plots[5].stage = 2;
  updateCarrots(w, 1);
  assert.equal(w.plots[5].growth, 0);
  assert.equal(w.plots[5].stage, 0);
  assert.ok(w.coins > 0);
  assert.ok(w.effects.some((e) => e.kind === 'carrot'));
});

test('the combine bounces: it reverses at the right edge and at the left edge', () => {
  const w = fieldWorld();
  w.carrots.combines = 1;
  w.carrots.combineX = FIELD.x + FIELD.cols * 16 - 16 - 1;
  w.carrots.dir = 1;
  updateCarrots(w, 0.1);
  assert.equal(w.carrots.dir, -1);
  assert.equal(w.carrots.combineX, FIELD.x + FIELD.cols * 16 - 16);
  for (let i = 0; i < 1000; i++) updateCarrots(w, 0.1);
  assert.ok(w.carrots.combineX >= FIELD.x);
  assert.ok(w.carrots.combineX <= FIELD.x + FIELD.cols * 16 - 16);
});

test('carrotDrawables is empty with no combines', () => {
  assert.deepEqual(carrotDrawables(fieldWorld()), []);
});

test('carrotDrawables draws one combine, and a count label with more than one', () => {
  const w = fieldWorld();
  w.carrots.combines = 1;
  const one = carrotDrawables(w);
  assert.equal(one.length, 1);
  assert.equal(typeof one[0].y, 'number');
  const ctx1 = fakeCtx();
  assert.doesNotThrow(() => one[0].draw(ctx1));
  assert.ok(ctx1.count('fillRect') > 0);

  w.carrots.combines = 3;
  w.carrots.dir = -1;
  const many = carrotDrawables(w);
  assert.equal(many.length, 1);
  const ctx2 = fakeCtx();
  assert.doesNotThrow(() => many[0].draw(ctx2));
  assert.ok(ctx2.count('fillRect') > ctx1.count('fillRect'), 'the count label adds pixels');
});
