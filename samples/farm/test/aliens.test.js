// Tests for web/js/aliens.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld, setHour, money } from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import {
  LEASE, MAX_SAUCERS, leasable, maxLease, leaseCows, alienRate, summonVisit, updateAliens,
  alienDrawables,
} from '../web/js/aliens.js';
import { fakeCtx } from './fakectx.js';

const line = (kind) => 'LINE ' + kind;
const DT = 0.1;

// step advances the aliens by one 0.1 s update.
const step = (w, rng) => updateAliens(w, DT, rng, line);

test('leaseCows with max on a fresh world leases 2 cows, spends 10 coins and moves them to leased', () => {
  const w = createWorld(1);
  assert.equal(leasable(w), 3);
  assert.equal(maxLease(w), 2);

  const n = leaseCows(w, 'max');
  assert.equal(n, 2);
  assert.equal(w.coins, 0, 'two leases cost 10 coins');
  assert.equal(w.herds.cow.females, 1, 'two cows left the females');
  assert.equal(w.herds.cow.leased, 2, 'two cows are leased');
  assert.equal(w.aliens.batches.length, 1);
  assert.deepEqual(w.aliens.batches[0], { count: 2, timer: LEASE.stay });
  assert.equal(w.log[0].text, `${money(2)} COWS WENT ON AN ALIEN INTERNSHIP`);
});

test('leaseCows leases nothing with no cows or no coins, and changes nothing', () => {
  const noCows = createWorld(1);
  noCows.herds.cow.females = 0;
  assert.equal(leaseCows(noCows, 'max'), 0);
  assert.equal(noCows.herds.cow.leased, 0);
  assert.equal(noCows.coins, 10);
  assert.equal(noCows.aliens.batches.length, 0);

  const poor = createWorld(1);
  poor.coins = 4;
  assert.equal(maxLease(poor), 0);
  assert.equal(leaseCows(poor, 'max'), 0);
  assert.equal(poor.herds.cow.females, 3);
  assert.equal(poor.herds.cow.leased, 0);
  assert.equal(poor.coins, 4);
  assert.equal(poor.aliens.batches.length, 0);
});

test('leaseCows with a number leases at most that many and at most what the coins allow', () => {
  const w = createWorld(1);
  w.coins = 1000;
  assert.equal(leaseCows(w, 1), 1);
  assert.equal(w.herds.cow.leased, 1);
  assert.equal(w.coins, 995);

  // Only 2 cows are left to lease, so asking for 10 leases 2.
  assert.equal(leaseCows(w, 10), 2);
  assert.equal(w.herds.cow.females, 0);
  assert.equal(w.herds.cow.leased, 3);
  assert.equal(w.coins, 985);
});

test('alienRate is 0.6 per leased cow and does not depend on the cigarette upgrade', () => {
  const w = createWorld(1);
  assert.equal(alienRate(w), 0);

  w.herds.cow.leased = 1000;
  assert.ok(Math.abs(alienRate(w) - 600) < 1e-9, `rate ${alienRate(w)}`);

  w.upgrades.cigs = 5;
  assert.ok(Math.abs(alienRate(w) - 600) < 1e-9, 'cigs do not change the alien rate');
});

test('ten seconds with 1000 leased cows pay about 6000 coins', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);
  w.coins = 0;
  w.herds.cow.females = 0;
  w.herds.cow.leased = 1000;

  for (let i = 0; i < 100; i++) step(w, rng);
  assert.ok(Math.abs(w.coins - 6000) < 1e-6, `paid ${w.coins}, expected 6000`);
});

test('a batch comes back after 45 s: females restored, leased 0, abductions counted and logged', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.equal(leaseCows(w, 'max'), 2);
  assert.equal(w.herds.cow.leased, 2);

  for (let i = 0; i < 440; i++) step(w, rng); // 44 s
  assert.equal(w.herds.cow.leased, 2, 'still away at 44 s');
  assert.equal(w.stats.abductions, 0);

  for (let i = 0; i < 20; i++) step(w, rng); // 46 s
  assert.equal(w.herds.cow.leased, 0, 'back home');
  assert.equal(w.herds.cow.females, 3, 'females restored');
  assert.equal(w.aliens.batches.length, 0);
  assert.equal(w.stats.abductions, 2, 'the two cows count as abductions');
  assert.equal(w.log[0].text, `${money(2)} COWS ARE BACK FROM SPACE`);
});

test('with 10 batches at most 6 saucers exist, and they leave and are removed when the batches end', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  // Set the batches up directly so that coins do not limit the number of leases.
  for (let i = 0; i < 10; i++) w.aliens.batches.push({ count: 1, timer: LEASE.stay });
  w.herds.cow.females = 0;
  w.herds.cow.leased = 10;

  let most = 0;
  for (let i = 0; i < 100; i++) { // 10 s: all saucers arrive
    step(w, rng);
    most = Math.max(most, w.aliens.saucers.length);
    assert.ok(w.aliens.saucers.length <= MAX_SAUCERS, `${w.aliens.saucers.length} saucers at update ${i}`);
  }
  assert.equal(most, MAX_SAUCERS, 'six saucers show the batches');
  assert.equal(w.aliens.saucers.filter((s) => s.state === 'hover').length, MAX_SAUCERS);

  // Run past the 45 s the batches last, then until every saucer has flown off.
  for (let i = 0; i < 700 && w.aliens.saucers.length > 0; i++) step(w, rng);
  assert.equal(w.aliens.batches.length, 0);
  assert.equal(w.herds.cow.leased, 0);
  assert.equal(w.aliens.saucers.length, 0, 'every saucer left and was removed');
});

test('a saucer that is leaving is removed once it is past the right edge of the land', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  w.aliens.batches.push({ count: 1, timer: 1 });
  w.herds.cow.females = 0;
  w.herds.cow.leased = 1;
  for (let i = 0; i < 100 && w.aliens.saucers.length === 0; i++) step(w, rng);
  assert.equal(w.aliens.saucers.length, 1, 'one saucer for one batch');

  for (let i = 0; i < 300 && w.aliens.saucers.length > 0; i++) step(w, rng);
  assert.equal(w.aliens.saucers.length, 0);
});

test('the night visit lifts a visible cow, puts it back and leaves, as the old ufo test did', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonVisit(w, rng), true);
  assert.ok(w.aliens.visit, 'a visit is under way');
  const cow = w.animals.find((a) => a.id === w.aliens.visit.targetId);
  assert.ok(cow, 'the visit targets a cow');
  assert.equal(w.aliens.lastDay, w.day, 'the visit records the day');

  const states = new Set();
  let sawLift = false;
  let ended = false;
  for (let i = 0; i < 400; i++) {
    step(w, rng);
    if (w.aliens.visit === null) {
      ended = true;
      break;
    }
    const s = w.aliens.visit.state;
    states.add(s);
    if (['beam', 'hold', 'return'].includes(s)) {
      assert.equal(cow.abducted, true, `cow not abducted in ${s}`);
      assert.equal(cow.state, 'abducted');
    }
    if (cow.lift > 0) sawLift = true;
  }

  assert.ok(ended, 'the visit ends within 40 s');
  for (const s of ['arrive', 'beam', 'hold', 'return', 'leave']) {
    assert.ok(states.has(s), `visit passed through ${s}`);
  }
  assert.ok(sawLift, 'the cow was lifted');
  assert.equal(cow.lift, 0);
  assert.equal(cow.abducted, false);
  assert.equal(cow.state, 'graze');
  assert.equal(w.stats.abductions, 1);

  const speech = w.bubbles.filter((b) => b.kind === 'speech');
  assert.equal(speech.length, 1, 'one speech bubble');
  assert.equal(speech[0].text, 'LINE ufo');
  assert.equal(w.log[0].text, `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`);
});

test('summonVisit returns false while a visit is under way, and with no free adult cow', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 23);

  assert.equal(summonVisit(w, rng), true);
  const first = w.aliens.visit;
  assert.equal(summonVisit(w, rng), false);
  assert.equal(w.aliens.visit, first, 'a refused summon changes nothing');

  const calves = createWorld(1);
  for (const a of calves.animals) if (a.kind === 'cow') a.young = true;
  assert.equal(summonVisit(calves, makeRng(1)), false);
  assert.equal(calves.aliens.visit, undefined, 'no visit starts without a free cow');
});

test('summonVisit picks a cow that has no age field, since only young cows are skipped', () => {
  const w = createWorld(1);
  for (const a of w.animals) if (a.kind === 'cow') delete a.age;
  assert.equal(summonVisit(w, makeRng(1)), true);
  const target = w.animals.find((a) => a.id === w.aliens.visit.targetId);
  assert.equal(target.kind, 'cow');
  assert.ok(!target.young);
});

test('a night visit starts by chance at night and never by day', () => {
  const day = createWorld(1);
  const dayRng = makeRng(1);
  setHour(day, 12);
  for (let i = 0; i < 2000; i++) step(day, dayRng);
  assert.equal(day.aliens.visit ?? null, null, 'no visit by day');
  assert.equal(day.stats.abductions, 0);

  const night = createWorld(1);
  const rng = makeRng(1);
  setHour(night, 23);
  let summoned = false;
  for (let i = 0; i < 5000 && !summoned; i++) {
    step(night, rng);
    summoned = Boolean(night.aliens.visit);
  }
  assert.ok(summoned, 'updates at night eventually summon a visit');

  for (let i = 0; i < 1000 && night.aliens.visit; i++) step(night, rng);
  assert.equal(night.aliens.visit, null);
  assert.equal(night.stats.abductions, 1);
  assert.equal(night.aliens.lastDay, night.day);

  // The rest of the night, at the same odds, produces no further visit.
  for (let i = 0; i < 2000; i++) {
    step(night, rng);
    assert.equal(night.aliens.visit, null, `second visit started at update ${i}`);
  }
  assert.equal(night.stats.abductions, 1);
});

test('alienDrawables is empty with nothing out, and draws saucers and beams without throwing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  setHour(w, 12);

  assert.deepEqual(alienDrawables(w), []);

  w.aliens.batches.push({ count: 1, timer: LEASE.stay });
  w.herds.cow.females = 0;
  w.herds.cow.leased = 1;
  for (let i = 0; i < 100; i++) step(w, rng); // the saucer arrives and hovers
  let items = alienDrawables(w);
  assert.equal(items.length, 2, 'a hovering saucer and its beam');
  assert.equal(items[0].y, Infinity, 'the saucer sorts last, in front of the ground');

  const ctx = fakeCtx();
  for (const item of items) assert.doesNotThrow(() => item.draw(ctx));
  assert.ok(ctx.count('fillRect') > 0, 'something was drawn');
  assert.ok(
    ctx.calls.some((c) => c.op === 'set' && c.name === 'globalAlpha' && c.value === 0.35),
    'the beam is drawn at alpha 0.35',
  );
  assert.equal(ctx.globalAlpha, 1, 'globalAlpha is back to 1');

  // A night visit adds its own saucer and, while beaming, its beam.
  const v = createWorld(1);
  const vrng = makeRng(1);
  setHour(v, 23);
  assert.equal(summonVisit(v, vrng), true);
  items = alienDrawables(v);
  assert.equal(items.length, 1, 'one item while the visit arrives');
  for (let i = 0; i < 300 && v.aliens.visit.state !== 'beam'; i++) step(v, vrng);
  assert.equal(v.aliens.visit.state, 'beam');

  items = alienDrawables(v);
  assert.equal(items.length, 2, 'saucer and beam while beaming');
  const cow = v.animals.find((a) => a.id === v.aliens.visit.targetId);
  assert.equal(items[1].y, cow.y - 1, 'the beam sits just behind the cow');

  const vctx = fakeCtx();
  for (const item of items) assert.doesNotThrow(() => item.draw(vctx));
  assert.equal(vctx.globalAlpha, 1, 'globalAlpha is back to 1');
});
