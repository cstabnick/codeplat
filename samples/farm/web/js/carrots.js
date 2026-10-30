// Carrots: the field is a clicker. A click plucks a carrot for coins, upgrades raise what a click
// earns and how fast crops grow, and combine harvesters pluck on their own. This module owns
// world.carrots, stats.plucks, and a plucked plot's growth, stage and water, and the coins from
// plucking and combines. It draws the combine. Imports only state.js and pixel.js, and touches no
// window or document on load.

import { FIELD, CARROT_VALUE, RIPE_BONUS, productivity, puff, money } from './state.js';
import { drawSprite, drawText } from './pixel.js';

const TILE = 16;
const COMBINE_SPEED = 30;   // units per second the combine travels across the field

// CARROT_UPGRADES lists the clicker upgrades in the order the shop shows them. Each level costs
// Math.round(base * growth ** level) coins; max is the level cap.
export const CARROT_UPGRADES = [
  { id: 'gloves', name: 'Gardening gloves', blurb: 'Each pair adds 1 carrot to every click.', base: 15, growth: 1.5, max: Infinity },
  { id: 'hoe', name: 'Golden hoe', blurb: 'Each one adds your gloves\' worth again to every click.', base: 250, growth: 2.5, max: 30 },
  { id: 'fertilizer', name: 'Fertilizer', blurb: 'Each level makes every crop grow 1.5x faster, so more of them are ripe.', base: 60, growth: 2, max: 8 },
  { id: 'combines', name: 'Combine harvester', blurb: 'Plucks 2 carrots a second on its own, times productivity.', base: 400, growth: 1.35, max: Infinity },
];

// clickPower is what one click plucks: each pair of gloves adds one, and each hoe adds the
// gloves' total again. Both grow linearly so that clicking never outruns the herds.
export function clickPower(world) {
  return (1 + world.carrots.gloves) * (1 + world.carrots.hoe);
}

// combineRate is the coins per second the combine harvesters earn, before any click is made. A
// combine picks plain carrots: click power is for clicks, so it does not apply here.
export function combineRate(world) {
  return world.carrots.combines * 2 * CARROT_VALUE * productivity(world);
}

// upgradeDef finds an upgrade's entry in CARROT_UPGRADES, or throws for an unknown id.
function upgradeDef(id) {
  const def = CARROT_UPGRADES.find((u) => u.id === id);
  if (!def) throw new Error(`unknown carrot upgrade: ${id}`);
  return def;
}

// upgradeCost is the price of the next level of an upgrade.
export function upgradeCost(world, id) {
  const def = upgradeDef(id);
  return Math.round(def.base * def.growth ** world.carrots[id]);
}

// upgradeView describes one upgrade's state: its level, price, and why it cannot be bought.
function upgradeView(world, def) {
  const level = world.carrots[def.id];
  const cost = upgradeCost(world, def.id);
  let reason = '';
  if (level >= def.max) reason = 'MAXED';
  else if (world.coins < cost) reason = 'NOT ENOUGH COINS';
  return { id: def.id, name: def.name, blurb: def.blurb, level, cost, ok: reason === '', reason };
}

// carrotUpgrades lists every clicker upgrade, in CARROT_UPGRADES order, with whether it can be bought.
export function carrotUpgrades(world) {
  return CARROT_UPGRADES.map((def) => upgradeView(world, def));
}

// buyCarrotUpgrade buys up to n levels of an upgrade while each is ok. n is a positive integer or
// 'max' (at most 1000). It returns the number of levels bought.
export function buyCarrotUpgrade(world, id, n = 1) {
  const limit = n === 'max' ? 1000 : n;
  let bought = 0;
  while (bought < limit) {
    const view = upgradeView(world, upgradeDef(id));
    if (!view.ok) break;
    world.coins -= view.cost;
    world.carrots[id] += 1;
    bought += 1;
  }
  return bought;
}

// plotAt returns the index into world.plots of the plot whose tile holds the point (x, y), or -1
// when the point is outside the field. Plots are row-major, so index = row * FIELD.cols + col.
export function plotAt(x, y) {
  const col = Math.floor((x - FIELD.x) / TILE);
  const row = Math.floor((y - FIELD.y) / TILE);
  if (col < 0 || col >= FIELD.cols || row < 0 || row >= FIELD.rows) return -1;
  return row * FIELD.cols + col;
}

// mostGrown returns the index of the plot with the largest growth, or 0 when there are no plots.
function mostGrown(world) {
  let best = 0;
  for (let i = 1; i < world.plots.length; i++) {
    if (world.plots[i].growth > world.plots[best].growth) best = i;
  }
  return best;
}

// pluck harvests the plot at index for coins, and the most grown plot when index is -1. A ripe
// carrot is worth RIPE_BONUS carrots. The plot is replanted to growth 0 and keeps at least half
// its water. It returns the coins earned.
export function pluck(world, index) {
  const i = index === -1 ? mostGrown(world) : index;
  const plot = world.plots[i];
  const earned = CARROT_VALUE * clickPower(world) * (plot.stage === 3 ? RIPE_BONUS : 1);
  plot.growth = 0;
  plot.stage = 0;
  plot.water = Math.max(plot.water, 0.5);
  world.coins += earned;
  world.stats.plucks += 1;
  const tileX = FIELD.x + plot.col * TILE;
  const tileY = FIELD.y + plot.row * TILE;
  puff(world, 'carrot', tileX + 8, tileY + 4, 0, -14, 0.8);
  return earned;
}

// updateCarrots advances the combine harvesters by dt seconds. Each combine plucks two carrots a
// second; every whole carrot earns coins and resets the most grown plot once, however many there
// are. The combine bounces back and forth across the field.
export function updateCarrots(world, dt) {
  const c = world.carrots;
  c.acc += c.combines * 2 * dt;
  const n = Math.floor(c.acc);
  c.acc -= n;
  if (n > 0) {
    world.coins += n * CARROT_VALUE * productivity(world);
    const plot = world.plots[mostGrown(world)];
    plot.growth = 0;
    plot.stage = 0;
    const tileX = FIELD.x + plot.col * TILE;
    const tileY = FIELD.y + plot.row * TILE;
    puff(world, 'carrot', tileX + 8, tileY + 4, 0, -14, 0.8);
  }

  const left = FIELD.x;
  const right = FIELD.x + FIELD.cols * TILE - TILE;
  let dir = c.dir ?? 1;
  c.combineX += COMBINE_SPEED * dt * dir;
  if (c.combineX > right) {
    c.combineX = right;
    dir = -1;
  } else if (c.combineX < left) {
    c.combineX = left;
    dir = 1;
  }
  c.dir = dir;
}

// The combine is a 16x10 sprite. Its front is on the right when it faces right.
const COMBINE = [
  '.....3333.......',
  '....33aa3.....9.',
  '....3333333...9.',
  '3333333333333333',
  '3333333333333333',
  '3333333333333333',
  '.333333333333333',
  '.5.5.5.5.5.5.5..',
  '.5.5.5.5.5.5.5..',
  '................',
];

// carrotDrawables returns nothing with no combines, and otherwise one drawable for the combine
// (and its count when there is more than one).
export function carrotDrawables(world) {
  const c = world.carrots;
  if (c.combines <= 0) return [];
  const flip = (c.dir ?? 1) === -1;
  const x = c.combineX;
  const baseY = FIELD.y + FIELD.rows * TILE;
  return [{
    y: baseY + 1,
    draw(ctx) {
      drawSprite(ctx, COMBINE, x, baseY - 10, flip);
      if (c.combines > 1) {
        drawText(ctx, `X${money(c.combines)}`, x, baseY - 16, '#ffec27');
      }
    },
  }];
}
