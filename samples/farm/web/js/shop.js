// The shop: Sir Oinksalot is an interdimensional prince trapped in a pig, and he sells the farm
// what it needs from beyond the veil. offers lists what is for sale and why an item cannot be
// bought; buy spends the coins and delivers. Imports no globals and touches no window or document.

import {
  CIG_BOOST, PORTAL_BOOST, HERD, herdSize, capacity, isNight, say, logEvent, puff,
} from './state.js';
import { addAnimal } from './animals.js';
import { CONTRACT, freeCows, hireUfo } from './ufo.js';
import { startTask } from './farmer.js';

// PORTAL is the price of the prince's way home: the game's goal.
export const PORTAL = 25000;

const ITEMS = ['cow', 'chicken', 'ufo', 'cigs', 'pasture', 'coop', 'portal'];

const contracts = (world) => world.ufos.filter((u) => u.rate > 0).length;
const pig = (world) => world.animals.find((a) => a.kind === 'pig');

// price returns the cost of an item now. Animals cost more the bigger the herd, upgrades double
// each time, and a UFO contract costs 5 more for each one already flying.
export function price(world, id) {
  const u = world.upgrades;
  switch (id) {
    case 'cow': return Math.round(30 * 1.12 ** herdSize(world, 'cow'));
    case 'chicken': return Math.round(8 * 1.12 ** herdSize(world, 'chicken'));
    case 'ufo': return 5 + 5 * contracts(world);
    case 'cigs': return 20 * 2 ** u.cigs;
    case 'pasture': return 75 * 2 ** u.pasture;
    case 'coop': return 40 * 2 ** u.coop;
    case 'portal': return PORTAL;
    default: return Infinity;
  }
}

// blocked returns why an item cannot be bought apart from its price, or '' when it can.
function blocked(world, id) {
  switch (id) {
    case 'cow':
    case 'chicken':
      return herdSize(world, id) >= capacity(world, id) ? (id === 'cow' ? 'PASTURE FULL' : 'COOP FULL') : '';
    case 'ufo':
      return freeCows(world).length === 0 ? 'NO FREE ADULT COW' : '';
    case 'pasture':
    case 'coop': {
      const kind = id === 'pasture' ? 'cow' : 'chicken';
      return capacity(world, kind) >= HERD[kind].max ? 'AS BIG AS IT GETS' : '';
    }
    case 'portal':
      return world.upgrades.portal ? 'HE IS HOME (SOMETIMES)' : '';
    default:
      return '';
  }
}

// owned describes what the farm has of an item, for the shop's row.
function owned(world, id) {
  const u = world.upgrades;
  switch (id) {
    case 'cow': return `${herdSize(world, 'cow')}/${capacity(world, 'cow')} COWS`;
    case 'chicken': return `${herdSize(world, 'chicken')}/${capacity(world, 'chicken')} HENS`;
    case 'ufo': return `${contracts(world)} FLYING`;
    case 'cigs': return `${u.cigs} PACKS`;
    case 'pasture': return `ROOM FOR ${capacity(world, 'cow')}`;
    case 'coop': return `ROOM FOR ${capacity(world, 'chicken')}`;
    case 'portal': return u.portal ? 'FIXED' : 'BROKEN';
    default: return '';
  }
}

const NAMES = {
  cow: 'Cow',
  chicken: 'Chicken',
  ufo: 'UFO contract',
  cigs: 'Royal cigarettes',
  pasture: 'Bigger pasture',
  coop: 'Bigger coop',
  portal: 'Fix the portal home',
};

const BLURBS = {
  cow: 'From a dimension where cows are slightly more cow. Gives milk, has calves, and one day does not.',
  chicken: 'Lays eggs, panics, lays more eggs. Hatches chicks while the coop has room.',
  ufo: `My associates upstairs borrow one adult cow for ${CONTRACT.stay} seconds and pay ` +
    `$${CONTRACT.rate} a second while they have her. One saucer per cow. She comes back. Mostly.`,
  cigs: `Gary works harder out of guilt: milk and eggs are worth x${CIG_BOOST} more. ` +
    'He will be right back. He is checking the barn.',
  pasture: `I fold a little space into the fence. Room for ${HERD.cow.step} more cows.`,
  coop: `The coop becomes bigger on the inside. Room for ${HERD.chicken.step} more chickens.`,
  portal: `Send me home and I put in a good word with the multiverse: x${PORTAL_BOOST} productivity, forever.`,
};

// offers returns one entry per item: {id, name, blurb, owned, cost, ok, reason}. ok is true when it
// can be bought now; otherwise reason says why, the shortfall included.
export function offers(world) {
  return ITEMS.map((id) => {
    const cost = price(world, id);
    let reason = blocked(world, id);
    if (!reason && world.coins < cost) reason = 'NOT ENOUGH COINS';
    return { id, name: NAMES[id], blurb: BLURBS[id], owned: owned(world, id), cost, ok: !reason, reason };
  });
}

// pigSays puts a speech bubble over the prince.
function pigSays(world, text) {
  const p = pig(world);
  if (p) say(world, p.x, p.y - p.lift - 9, text, 'speech', 4);
}

// deliver gives the farm what was bought. It returns false when the delivery failed and nothing
// should be charged.
function deliver(world, id, rng, line) {
  const u = world.upgrades;
  switch (id) {
    case 'cow':
    case 'chicken': {
      const a = addAnimal(world, id, rng);
      puff(world, 'portal', a.x, a.y - 6, 0, -4, 1.2);
      puff(world, 'sparkle', a.x - 1, a.y - 10);
      logEvent(world, `${a.name.toUpperCase()} ARRIVED THROUGH THE PORTAL`);
      return true;
    }
    case 'ufo':
      if (!hireUfo(world, rng)) return false;
      logEvent(world, 'THE ALIENS ARE ON THEIR WAY. CASH UP FRONT.');
      return true;
    case 'cigs':
      u.cigs += 1;
      logEvent(world, `GARY GOT PACK NUMBER ${u.cigs}. PRODUCTIVITY UP.`);
      if (!isNight(world) && world.farmer.task !== 'smoke') startTask(world, 'smoke', rng, line);
      return true;
    case 'pasture':
      u.pasture += 1;
      logEvent(world, 'THE PASTURE IS NOW BIGGER ON THE INSIDE');
      return true;
    case 'coop':
      u.coop += 1;
      logEvent(world, 'THE COOP IS NOW BIGGER ON THE INSIDE');
      return true;
    case 'portal': {
      u.portal = 1;
      const p = pig(world);
      if (p) for (let i = 0; i < 12; i++) puff(world, 'portal', p.x + rng.range(-12, 12), p.y - rng.range(0, 16), 0, -10, 2);
      logEvent(world, 'THE PRINCE WENT HOME. HE CAME BACK FOR THE MUD.');
      return true;
    }
    default:
      return false;
  }
}

// buy buys an item if offers says it can, and returns {ok, text}: text is what the prince says,
// which is also shown over his head.
export function buy(world, id, rng, line) {
  const offer = offers(world).find((o) => o.id === id);
  if (!offer) return { ok: false, text: 'I DO NOT SELL THAT. YET.' };
  if (!offer.ok) return { ok: false, text: offer.reason };
  if (!deliver(world, id, rng, line)) return { ok: false, text: 'THE VOID SAYS NO.' };
  world.coins -= offer.cost;
  const text = id === 'portal' ? 'HOME! ...IT WAS WORSE. I AM BACK. KEEP THE MUD WARM.' : line('shop');
  pigSays(world, text);
  return { ok: true, text };
}
