// Tests for web/js/save.js: round trips, away time, defaults for old saves, and bad or missing data.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createWorld } from '../web/js/state.js';
import { SAVE_KEY, SAVE_VERSION, saveWorld, loadWorld, clearSave } from '../web/js/save.js';

// memoryStorage is a Map-backed stand-in for localStorage.
function memoryStorage() {
  const m = new Map();
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, String(v)),
    removeItem: (k) => m.delete(k),
    _map: m,
  };
}

// throwingStorage is a storage whose every method throws, like a blocked or full localStorage.
function throwingStorage() {
  const boom = () => {
    throw new Error('storage unavailable');
  };
  return { getItem: boom, setItem: boom, removeItem: boom };
}

test('round trip keeps changed fields and reports away time', () => {
  const storage = memoryStorage();
  const world = createWorld(42);
  world.coins = 555;
  world.upgrades.cigs = 3;
  world.log.unshift({ day: 1, hour: 9, text: 'Daisy gave milk' });

  assert.equal(saveWorld(storage, world, 1000), true);
  const loaded = loadWorld(storage, 61000);

  assert.ok(loaded);
  assert.equal(loaded.away, 60);
  assert.equal(loaded.world.coins, 555);
  assert.equal(loaded.world.upgrades.cigs, 3);
  assert.deepEqual(loaded.world.log, world.log);
  assert.deepEqual(loaded.world, world);
});

test('away is capped at 7200 seconds and is never negative', () => {
  const storage = memoryStorage();
  saveWorld(storage, createWorld(1), 0);
  assert.equal(loadWorld(storage, 100 * 60 * 60 * 1000).away, 7200);
  assert.equal(loadWorld(storage, -5000).away, 0);
});

test('away is 0 when savedAt is not a number', () => {
  const storage = memoryStorage();
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, world: createWorld(7) }));
  assert.equal(loadWorld(storage, 999999).away, 0);
});

test('a save missing stats.births and upgrades.cigs loads them as 0', () => {
  const storage = memoryStorage();
  const world = createWorld(5);
  delete world.stats.births;
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: 0, world }));

  const loaded = loadWorld(storage, 0);
  assert.ok(loaded);
  assert.equal(loaded.world.stats.births, 0);
  assert.equal(loaded.world.upgrades.cigs, 0);
});

test('wrong version, garbage JSON and a missing key all return null', () => {
  const storage = memoryStorage();
  assert.equal(loadWorld(storage, 0), null);

  storage.setItem(SAVE_KEY, '{not json');
  assert.equal(loadWorld(storage, 0), null);

  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION + 1, savedAt: 0, world: createWorld(1) }));
  assert.equal(loadWorld(storage, 0), null);
});

test('a world without an animals array or a numeric seed is rejected', () => {
  const storage = memoryStorage();
  const noAnimals = createWorld(1);
  delete noAnimals.animals;
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: 0, world: noAnimals }));
  assert.equal(loadWorld(storage, 0), null);

  const noSeed = createWorld(1);
  delete noSeed.seed;
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: 0, world: noSeed }));
  assert.equal(loadWorld(storage, 0), null);
});

test('throwing storage: save returns false, load returns null, clear does not throw', () => {
  const storage = throwingStorage();
  assert.equal(saveWorld(storage, createWorld(1), 0), false);
  assert.equal(loadWorld(storage, 0), null);
  assert.doesNotThrow(() => clearSave(storage));
});

test('clearSave removes the save', () => {
  const storage = memoryStorage();
  saveWorld(storage, createWorld(9), 0);
  assert.ok(storage._map.has(SAVE_KEY));
  clearSave(storage);
  assert.equal(storage._map.has(SAVE_KEY), false);
  assert.equal(loadWorld(storage, 0), null);
});

test('a loaded world that was paused is unpaused', () => {
  const storage = memoryStorage();
  const world = createWorld(3);
  world.paused = true;
  saveWorld(storage, world, 0);
  assert.equal(loadWorld(storage, 0).world.paused, false);
});

test('version 2 round trip keeps herds, carrots, aliens and land', () => {
  const storage = memoryStorage();
  const world = createWorld(11);
  world.herds.cow.females = 2e6;
  world.herds.cow.tier = 6;
  world.carrots.combines = 12;
  world.land = 3;
  world.aliens.batches.push({ x: 40, y: 30, count: 5, ttl: 9 });

  assert.equal(saveWorld(storage, world, 0), true);
  const loaded = loadWorld(storage, 0);

  assert.ok(loaded);
  assert.equal(loaded.world.herds.cow.females, 2e6);
  assert.equal(loaded.world.herds.cow.tier, 6);
  assert.equal(loaded.world.carrots.combines, 12);
  assert.equal(loaded.world.land, 3);
  assert.deepEqual(loaded.world.aliens.batches, [{ x: 40, y: 30, count: 5, ttl: 9 }]);
  assert.deepEqual(loaded.world, world);
});

test('a version 2 save missing herds.goat loads goat with the defaults', () => {
  const storage = memoryStorage();
  const world = createWorld(12);
  delete world.herds.goat;
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: 0, world }));

  const loaded = loadWorld(storage, 0);
  assert.ok(loaded);
  assert.deepEqual(loaded.world.herds.goat, createWorld(12).herds.goat);
  assert.equal(loaded.world.herds.goat.acc.birth, 0);
  assert.equal(loaded.world.herds.goat.females, 0);
});

test('a version 2 save whose herds.cow.acc lacks product loads product as 0', () => {
  const storage = memoryStorage();
  const world = createWorld(13);
  world.herds.cow.acc = { birth: 0.5, grow: 0.25, death: 0 };
  storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: 0, world }));

  const loaded = loadWorld(storage, 0);
  assert.ok(loaded);
  assert.equal(loaded.world.herds.cow.acc.product, 0);
  assert.equal(loaded.world.herds.cow.acc.birth, 0.5);
  assert.equal(loaded.world.herds.cow.acc.grow, 0.25);
  assert.equal(loaded.world.herds.cow.acc.death, 0);
});

test('herdEvents is [] after loading even if the save had some', () => {
  const storage = memoryStorage();
  const world = createWorld(14);
  world.herdEvents = [{ kind: 'cow', type: 'birth', count: 2, sex: 'female' }];
  saveWorld(storage, world, 0);

  const loaded = loadWorld(storage, 0);
  assert.ok(loaded);
  assert.deepEqual(loaded.world.herdEvents, []);
});

test('a version 1 save returns null', () => {
  const storage = memoryStorage();
  storage.setItem(SAVE_KEY, JSON.stringify({ version: 1, savedAt: 0, world: createWorld(15) }));
  assert.equal(loadWorld(storage, 0), null);
});
