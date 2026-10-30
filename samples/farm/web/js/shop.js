// The shop: Sir Oinksalot is an interdimensional prince trapped in a pig, and he sells the farm
// what it needs from beyond the veil. offers lists what is for sale, in sections, and why an item
// cannot be bought; buy spends the coins and delivers. Bulk items take a count of 1, 10 or 'max'.
// Imports no globals and touches no window or document.

import { CIG_BOOST, PORTAL_BOOST, isNight, hasKind, money, say, logEvent, puff } from './state.js';
import { SPECIES, KINDS, LAND } from './species.js';
import {
  room, maxBuy, buyAnimals, nextTier, upgradeHousing, nextLand, buyLand,
} from './herds.js';
import { LEASE, leasable, maxLease, leaseCows } from './aliens.js';
import { CARROT_UPGRADES, carrotUpgrades, buyCarrotUpgrade } from './carrots.js';
import { startTask } from './farmer.js';

// PORTAL is the price of the prince's way home: the game's goal.
export const PORTAL = 5e10;

// MARKET names each kind's female and male for sale, and the label the shop shows for the herd.
const MARKET = {
  cow: { female: { name: 'Cow', label: 'COWS' }, male: { name: 'Bull', label: 'BULLS' } },
  chicken: { female: { name: 'Hen', label: 'HENS' }, male: { name: 'Cock', label: 'COCKS' } },
  sheep: { female: { name: 'Ewe', label: 'EWES' }, male: { name: 'Ram', label: 'RAMS' } },
  goat: { female: { name: 'Nanny goat', label: 'NANNIES' }, male: { name: 'Billy goat', label: 'BILLIES' } },
};

// PLURAL is the plain name of a kind, for the sentences the shop writes.
const PLURAL = { cow: 'cows', chicken: 'chickens', sheep: 'sheep', goat: 'goats' };

// CARROT_BULK lists the field upgrades that can be bought in bulk.
const CARROT_BULK = ['gloves', 'combines'];

// BULK_LIMIT caps how many levels 'max' counts or buys at once.
const BULK_LIMIT = 1000;

// item builds one offer. ok is true when nothing blocks it; bulk items default to a count of 0 and
// otherwise maxCount is 0.
function item(section, id, fields) {
  const reason = fields.reason ?? '';
  return { section, id, bulk: false, maxCount: 0, ...fields, reason, ok: reason === '' };
}

// marketItems lists the animals for sale: a female and a male of each kind the farm has land for.
function marketItems(world) {
  const items = [];
  for (const kind of KINDS) {
    if (!hasKind(world, kind)) continue;
    const s = SPECIES[kind];
    for (const sex of ['female', 'male']) {
      const { name, label } = MARKET[kind][sex];
      const h = world.herds[kind];
      const count = sex === 'female' ? h.females : h.males;
      const cost = s.price[sex];
      const blurb = sex === 'female'
        ? `Makes ${s.product} worth ${s.value} coins every ${s.seconds} seconds, and has young with a ${s.male}. Her young are a random mix of the sexes.`
        : `Makes nothing itself, but covers up to ${s.matesPerMale} ${s.female}s, so the young come from it.`;
      let reason = '';
      if (room(world, kind) === 0) reason = 'NO ROOM: UPGRADE HOUSING';
      else if (world.coins < cost) reason = 'NOT ENOUGH COINS';
      items.push(item('MARKET', `${kind}:${sex}`, {
        name, blurb, owned: `${money(count)} ${label}`, cost, reason, bulk: true, maxCount: maxBuy(world, kind, sex),
      }));
    }
  }
  return items;
}

// housingItems lists the next housing tier for each kind the farm owns.
function housingItems(world) {
  return KINDS.filter((kind) => hasKind(world, kind)).map((kind) => {
    const current = SPECIES[kind].tiers[world.herds[kind].tier];
    const next = nextTier(world, kind);
    const blurb = next
      ? `Room for ${money(next.capacity)} animals, counting young and leased ones.`
      : 'This herd is in its biggest housing.';
    let reason = '';
    if (!next) reason = 'TOP TIER';
    else if (world.coins < next.cost) reason = 'NOT ENOUGH COINS';
    return item('HOUSING', `house:${kind}`, {
      name: (next ?? current).name,
      blurb,
      owned: `NOW: ${current.name.toUpperCase()} (${money(current.capacity)})`,
      cost: next ? next.cost : 0,
      reason,
    });
  });
}

// landItem lists the next parcel of land, and what lives there.
function landItem(world) {
  const next = nextLand(world);
  const kind = KINDS.find((k) => SPECIES[k].parcel === world.land);
  let reason = '';
  if (!next) reason = 'ALL LAND OWNED';
  else if (world.coins < next.cost) reason = 'NOT ENOUGH COINS';
  return item('LAND', 'land', {
    name: (next ?? LAND[LAND.length - 1]).name,
    blurb: next && kind ? `Buys ${next.name}, where ${PLURAL[kind]} live.` : 'Every parcel is owned.',
    owned: `${world.land}/${LAND.length} PARCELS`,
    cost: next ? next.cost : 0,
    reason,
  });
}

// maxLevels counts how many levels of a field upgrade 'max' would buy with the coins the farm has,
// without spending anything. It follows the same prices as buyCarrotUpgrade.
function maxLevels(world, id) {
  const def = CARROT_UPGRADES.find((u) => u.id === id);
  let level = world.carrots[id];
  let coins = world.coins;
  let count = 0;
  while (count < BULK_LIMIT && level < def.max) {
    const cost = Math.round(def.base * def.growth ** level);
    if (coins < cost) break;
    coins -= cost;
    level += 1;
    count += 1;
  }
  return count;
}

// fieldItems lists the carrot field's upgrades.
function fieldItems(world) {
  return carrotUpgrades(world).map((u) => item('FIELD', `carrot:${u.id}`, {
    name: u.name,
    blurb: u.blurb,
    owned: `LEVEL ${u.level}`,
    cost: u.cost,
    reason: u.reason,
    bulk: CARROT_BULK.includes(u.id),
    maxCount: CARROT_BULK.includes(u.id) ? maxLevels(world, u.id) : 0,
  }));
}

// offers returns the shop's items in order: {section, id, name, blurb, owned, cost, ok, reason,
// bulk, maxCount}. ok is true when the item can be bought now; for a bulk item, that at least one
// can be. reason says why not, the shortfall included, and is '' when ok. maxCount is how many a
// bulk item can take now, and 0 otherwise.
export function offers(world) {
  const u = world.upgrades;
  const cigCost = 20 * 3 ** u.cigs;
  const cigReason = world.coins < cigCost ? 'NOT ENOUGH COINS' : '';
  const leaseReason = leasable(world) === 0
    ? 'NO FREE COWS'
    : world.coins < LEASE.price ? 'NOT ENOUGH COINS' : '';
  const portalReason = u.portal
    ? 'HE IS HOME (SOMETIMES)'
    : world.coins < PORTAL ? 'NOT ENOUGH COINS' : '';

  return [
    ...marketItems(world),
    ...housingItems(world),
    landItem(world),
    ...fieldItems(world),
    item('GARY', 'cigs', {
      name: 'Royal cigarettes',
      blurb: `Gary works harder out of guilt: milk and eggs are worth x${CIG_BOOST} more. ` +
        'He will be right back. He is checking the barn.',
      owned: `${u.cigs} PACKS`,
      cost: cigCost,
      reason: cigReason,
    }),
    item('ALIENS', 'lease', {
      name: 'UFO lease',
      blurb: `My associates upstairs borrow one adult cow for ${LEASE.stay} seconds and pay ` +
        `${LEASE.rate} coins a second while they have her. She comes back. Mostly.`,
      owned: `${money(world.herds.cow.leased)} COWS IN SPACE`,
      cost: LEASE.price,
      reason: leaseReason,
      bulk: true,
      maxCount: maxLease(world),
    }),
    item('PORTAL', 'portal', {
      name: 'Fix the portal home',
      blurb: `Send me home and I put in a good word with the multiverse: x${PORTAL_BOOST} productivity, forever.`,
      owned: u.portal ? 'FIXED' : 'BROKEN',
      cost: PORTAL,
      reason: portalReason,
    }),
  ];
}

// pigSays puts a speech bubble over the prince.
function pigSays(world, text) {
  const p = world.animals.find((a) => a.kind === 'pig');
  if (p) say(world, p.x, p.y - p.lift - 9, text, 'speech', 4);
}

// deliver gives the farm what was bought and returns {count, text}: count is how many were
// delivered, 0 when none were. Spending is done here for cigarettes and the portal, and by the
// herd, land, aliens and carrot modules for the rest.
function deliver(world, offer, n, rng, line) {
  const u = world.upgrades;
  const amount = offer.bulk ? n : 1;
  switch (offer.section) {
    case 'MARKET': {
      const [kind, sex] = offer.id.split(':');
      const count = buyAnimals(world, kind, sex, amount);
      return { count, text: line('shop') };
    }
    case 'HOUSING': {
      const kind = offer.id.slice('house:'.length);
      return { count: upgradeHousing(world, kind) ? 1 : 0, text: line('tier') };
    }
    case 'LAND':
      return { count: buyLand(world) ? 1 : 0, text: line('land') };
    case 'FIELD': {
      const id = offer.id.slice('carrot:'.length);
      return { count: buyCarrotUpgrade(world, id, amount), text: line('shop') };
    }
    case 'ALIENS':
      return { count: leaseCows(world, amount), text: line('shop') };
    case 'GARY':
      world.coins -= offer.cost;
      u.cigs += 1;
      logEvent(world, `GARY GOT PACK NUMBER ${u.cigs}. PRODUCTIVITY UP.`);
      if (!isNight(world) && world.farmer.task !== 'smoke') startTask(world, 'smoke', rng, line);
      return { count: 1, text: line('shop') };
    case 'PORTAL': {
      world.coins -= offer.cost;
      u.portal = 1;
      const p = world.animals.find((a) => a.kind === 'pig');
      if (p) for (let i = 0; i < 12; i++) puff(world, 'portal', p.x + rng.range(-12, 12), p.y - rng.range(0, 16), 0, -10, 2);
      logEvent(world, 'THE PRINCE WENT HOME. HE CAME BACK FOR THE MUD.');
      return { count: 1, text: 'HOME! ...IT WAS WORSE. I AM BACK. KEEP THE MUD WARM.' };
    }
    default:
      return { count: 0, text: 'THE VOID SAYS NO.' };
  }
}

// buy buys the item with the given id, if offers says it can. n is 1, 10 or 'max' and is used
// only by bulk items. It returns {ok, text, count}: count is how many were bought, and text is
// what the prince says, which is also shown over his head on success.
export function buy(world, id, n, rng, line) {
  const offer = offers(world).find((o) => o.id === id);
  if (!offer) return { ok: false, count: 0, text: 'I DO NOT SELL THAT. YET.' };
  if (!offer.ok) return { ok: false, count: 0, text: offer.reason };
  const { count, text } = deliver(world, offer, n, rng, line);
  if (count === 0) return { ok: false, count: 0, text: offer.reason || 'THE VOID SAYS NO.' };
  pigSays(world, text);
  return { ok: true, text, count };
}
