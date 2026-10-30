// Economy: the coins the farm earns on its own. Each owned herd's females make product at a rate
// worth coins, counted as numbers rather than animals, so a herd of millions earns as a herd of
// three does. Whole units that build up in an accumulator are paid as coins, with at most one small
// effect per kind per update. The combines and the alien lease are paid by carrots.js and aliens.js,
// and this module only reports their rates so the HUD shows the whole income.

import { SPECIES, KINDS } from './species.js';
import { hasKind, productivity, puff } from './state.js';
import { combineRate } from './carrots.js';
import { alienRate } from './aliens.js';

// herdRate is the coins per second one owned kind earns from its females, productivity included.
function herdRate(world, kind) {
  const sp = SPECIES[kind];
  return world.herds[kind].females * sp.value / sp.seconds * productivity(world);
}

// productRate is the coins per second the owned herds earn from their products, productivity included.
function productRate(world) {
  return KINDS.filter((k) => hasKind(world, k)).reduce((sum, k) => sum + herdRate(world, k), 0);
}

// incomeRate is the coins per second the farm earns right now: herd products, carrot combines and
// alien lease pay.
export function incomeRate(world) {
  return productRate(world) + combineRate(world) + alienRate(world);
}

// puffSpot picks the animal a product effect floats over: a visible adult female of the kind, or
// null when there is none. Cows and chickens are in world.animals; sheep and goats are in world.flock.
// The pick is chosen by the kind's product count, so the same world stepped the same way shows the
// same effects.
function puffSpot(world, kind) {
  let candidates;
  if (kind === 'sheep' || kind === 'goat') {
    candidates = world.flock.filter((f) => f.kind === kind && f.sex === 'female' && !f.young);
  } else {
    candidates = world.animals.filter((a) => a.kind === kind && (a.sex ?? 'female') === 'female' &&
      !a.young && !a.abducted);
  }
  if (candidates.length === 0) return null;
  return candidates[world.stats[SPECIES[kind].product] % candidates.length];
}

// puffFor shows one product effect for a kind over a visible female of it, picked by the product
// count. Each kind has its own effect and height above its feet.
function puffFor(world, kind) {
  const a = puffSpot(world, kind);
  if (!a) return;
  if (kind === 'cow') puff(world, 'milk', a.x, a.y - 14, 0, -8, 1.2);
  else if (kind === 'chicken') puff(world, 'egg', a.x, a.y - 9, 0, -8, 1.2);
  else puff(world, 'coin', a.x, a.y - 12, 0, -8, 1.2);
}

// updateEconomy advances each owned herd's product accumulator by dt seconds and pays out each whole
// unit as coins. A kind shows at most one effect per update, however many units it made.
export function updateEconomy(world, dt) {
  for (const kind of KINDS) {
    if (!hasKind(world, kind)) continue;
    const sp = SPECIES[kind];
    const herd = world.herds[kind];
    herd.acc.product += herd.females * dt / sp.seconds;
    const n = Math.floor(herd.acc.product);
    if (n <= 0) continue;
    herd.acc.product -= n;
    world.coins += n * sp.value * productivity(world);
    world.stats[sp.product] = (world.stats[sp.product] ?? 0) + n;
    puffFor(world, kind);
  }
}

// creditAway pays the herd and combine income for a span of seconds the player was away and returns
// the coins credited. Aliens do not pay while the player is away. Negative spans credit nothing.
export function creditAway(world, seconds) {
  const amount = (productRate(world) + combineRate(world)) * Math.max(0, seconds);
  world.coins += amount;
  return amount;
}
