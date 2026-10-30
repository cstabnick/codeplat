// The herd model: births, growing up, deaths, and buying animals, housing and land. Herds are
// counts, so every step costs the same for ten animals and for ten million. Imports only the
// species table and the world model, and touches no window or document on load.

import { SPECIES, KINDS, LAND } from './species.js';
import { herdTotal, housingCapacity, hasKind, logEvent } from './state.js';

// EVENT_LIMIT is how many herd events world.herdEvents keeps; the oldest are dropped first.
const EVENT_LIMIT = 50;
// EXACT_LIMIT is the count up to which split() tosses one coin per animal; above it a normal
// approximation is used so that large herds cost the same as small ones.
const EXACT_LIMIT = 32;

// pushEvent appends a herd event and keeps only the last EVENT_LIMIT entries.
function pushEvent(world, event) {
  world.herdEvents.push(event);
  if (world.herdEvents.length > EVENT_LIMIT) {
    world.herdEvents.splice(0, world.herdEvents.length - EVENT_LIMIT);
  }
}

// split returns a random binomial count: how many of n animals fall into a group that each has
// probability p. Small n is exact; large n uses a normal approximation (Box-Muller) clamped to
// [0, n].
function split(n, p, rng) {
  if (n <= EXACT_LIMIT) {
    let k = 0;
    for (let i = 0; i < n; i++) if (rng.chance(p)) k++;
    return k;
  }
  const u1 = 1 - rng.next();
  const u2 = rng.next();
  const z = Math.sqrt(-2 * Math.log(u1)) * Math.cos(2 * Math.PI * u2);
  const k = Math.round(n * p + z * Math.sqrt(n * p * (1 - p)));
  return Math.min(n, Math.max(0, k));
}

// room is how many more animals of a kind the housing has space for.
export function room(world, kind) {
  return Math.max(0, housingCapacity(world, kind) - herdTotal(world, kind));
}

// unitPrice is the market price of one female or male of a kind.
export function unitPrice(kind, sex) {
  return SPECIES[kind].price[sex];
}

// maxBuy is how many animals of a kind and sex the farm can buy now: what the coins pay for, up to
// the room in the housing. Zero when the farm does not own the species' parcel.
export function maxBuy(world, kind, sex) {
  if (!hasKind(world, kind)) return 0;
  return Math.min(Math.floor(world.coins / unitPrice(kind, sex)), room(world, kind));
}

// buyAnimals buys n animals of a kind and sex, or as many as it can when n is 'max'. It returns
// the number bought.
export function buyAnimals(world, kind, sex, n) {
  const count = Math.min(n === 'max' ? Infinity : n, maxBuy(world, kind, sex));
  if (!(count > 0)) return 0;
  const herd = world.herds[kind];
  world.coins -= count * unitPrice(kind, sex);
  if (sex === 'female') herd.females += count;
  else herd.males += count;
  pushEvent(world, { kind, type: 'buy', sex, count });
  return count;
}

// nextTier is the housing tier a herd would move up to, or null at the top.
export function nextTier(world, kind) {
  return SPECIES[kind].tiers[world.herds[kind].tier + 1] ?? null;
}

// upgradeHousing moves a herd up one housing tier when the farm owns the species and can pay.
// It returns true when it bought the upgrade.
export function upgradeHousing(world, kind) {
  if (!hasKind(world, kind)) return false;
  const tier = nextTier(world, kind);
  if (!tier || world.coins < tier.cost) return false;
  world.coins -= tier.cost;
  world.herds[kind].tier += 1;
  logEvent(world, `NEW HOUSING: ${tier.name.toUpperCase()}`);
  return true;
}

// nextLand is the next parcel the farm can buy, or null when all land is owned.
export function nextLand(world) {
  return LAND[world.land] ?? null;
}

// buyLand buys the next parcel when it is affordable. It returns true when it bought it.
export function buyLand(world) {
  const land = nextLand(world);
  if (!land || world.coins < land.cost) return false;
  world.coins -= land.cost;
  world.land += 1;
  logEvent(world, `BOUGHT ${land.name.toUpperCase()}`);
  return true;
}

// updateHerds advances every herd the farm owns by dt seconds: births, young growing up, and
// deaths. rng decides the sex of each animal that grows up and which adults die.
export function updateHerds(world, dt, rng) {
  for (const kind of KINDS) {
    if (!hasKind(world, kind)) continue;
    const s = SPECIES[kind];
    const h = world.herds[kind];

    // Births: breeding females, limited by males, add young; births with no room are lost.
    const breeders = Math.min(h.females, h.males * s.matesPerMale);
    h.acc.birth += breeders * s.birth * dt;
    let born = Math.floor(h.acc.birth);
    h.acc.birth -= born;
    born = Math.min(born, room(world, kind));
    if (born > 0) {
      h.young += born;
      world.stats.births += born;
      pushEvent(world, { kind, type: 'birth', count: born });
    }

    // Growing up: each young one takes s.grow seconds; half of those that grow up are females.
    if (h.young === 0) {
      h.acc.grow = 0;
    } else {
      h.acc.grow += h.young * dt / s.grow;
      const grown = Math.min(Math.floor(h.acc.grow), h.young);
      h.acc.grow -= grown;
      if (grown > 0) {
        const females = split(grown, 0.5, rng);
        h.young -= grown;
        h.females += females;
        h.males += grown - females;
        pushEvent(world, { kind, type: 'grow', count: grown });
      }
    }

    // Deaths: each adult dies at a rate of one over its mean life; the sex of the dead follows the
    // share of females in the herd, clamped so no sex goes negative.
    const adults = h.females + h.males;
    h.acc.death += adults * dt / s.life;
    const dead = Math.min(Math.floor(h.acc.death), adults);
    h.acc.death -= Math.floor(h.acc.death);
    if (dead > 0) {
      const lo = Math.max(0, dead - h.males);
      const hi = Math.min(dead, h.females);
      const deadFemales = Math.min(hi, Math.max(lo, split(dead, h.females / adults, rng)));
      h.females -= deadFemales;
      h.males -= dead - deadFemales;
      world.stats.deaths += dead;
      pushEvent(world, { kind, type: 'death', count: dead });
    }
  }
}
