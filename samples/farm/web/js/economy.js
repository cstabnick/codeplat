// Economy: the coins the farm earns on its own. Adult cows make milk, adult chickens lay eggs and
// hovering saucers pay alien rent. Each stream runs through a fractional accumulator in
// world.economy, and every whole unit that builds up is paid out as coins with a small effect.

import { isAdult, productivity, puff } from './state.js';

// MILK: each adult cow gives a bottle every 10 s, sold for $3.
export const MILK = { seconds: 10, value: 3 };
// EGG: each adult chicken lays an egg every 12 s, sold for $1.
export const EGG = { seconds: 12, value: 1 };

// producers returns the animals and saucers that currently earn coins.
function producers(world) {
  const live = (kind) => world.animals.filter((a) => a.kind === kind && !a.abducted && isAdult(a));
  return {
    cows: live('cow'),
    hens: live('chicken'),
    hosts: world.ufos.filter((u) => u.state === 'hold' && u.rate > 0),
  };
}

// hostRate is the total alien pay rate of the saucers in hosts, in coins per second. Productivity
// does not touch it: the aliens pay a flat rate, which keeps UFOs the cheap early money.
function hostRate(hosts) {
  return hosts.reduce((sum, h) => sum + h.rate, 0);
}

// incomeRateWithoutAliens is the coins per second from milk and eggs alone, productivity included.
// It is what the farm earns while the player is away, since aliens do not pay then.
function incomeRateWithoutAliens(world) {
  const { cows, hens } = producers(world);
  const p = productivity(world);
  return (cows.length * MILK.value / MILK.seconds + hens.length * EGG.value / EGG.seconds) * p;
}

// incomeRate is the coins per second the farm earns right now, from milk, eggs and alien pay.
export function incomeRate(world) {
  const { hosts } = producers(world);
  return incomeRateWithoutAliens(world) + hostRate(hosts);
}

// updateEconomy advances the milk, egg and alien accumulators by dt seconds and pays out each whole
// unit as it completes. An accumulator does not grow while there is nothing to produce it.
export function updateEconomy(world, dt) {
  const { cows, hens, hosts } = producers(world);
  const p = productivity(world);
  const eco = world.economy;

  if (cows.length > 0) {
    eco.milk += cows.length * dt / MILK.seconds;
    while (eco.milk >= 1) {
      eco.milk -= 1;
      world.coins += MILK.value * p;
      world.stats.milk += 1;
      const c = cows[world.stats.milk % cows.length];
      puff(world, 'milk', c.x, c.y - 14, 0, -8, 1.2);
    }
  }

  if (hens.length > 0) {
    eco.eggs += hens.length * dt / EGG.seconds;
    while (eco.eggs >= 1) {
      eco.eggs -= 1;
      world.coins += EGG.value * p;
      world.stats.eggs += 1;
      const h = hens[world.stats.eggs % hens.length];
      puff(world, 'egg', h.x, h.y - 9, 0, -8, 1.2);
    }
  }

  if (hosts.length > 0) {
    eco.alien += hostRate(hosts) * dt;
    while (eco.alien >= 1) {
      eco.alien -= 1;
      world.coins += 1;
      const u = hosts[Math.floor(world.time) % hosts.length];
      puff(world, 'coin', u.x, u.y + 6, 0, 10, 1.2);
    }
  }
}

// creditAway pays the milk and egg income for a span of seconds the player was away and returns the
// coins credited. Aliens do not pay while the player is away. Negative spans credit nothing.
export function creditAway(world, seconds) {
  const amount = incomeRateWithoutAliens(world) * Math.max(0, seconds);
  world.coins += amount;
  return amount;
}
