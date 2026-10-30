// Saves and loads the Gossip Acres world as JSON in a storage object (like localStorage). The storage
// is always passed in, so this module touches no browser globals and can be tested with a fake.

import { createWorld } from './state.js';

export const SAVE_KEY = 'gossip-acres-save';
export const SAVE_VERSION = 2;

// The longest a farm can be away and still count as away time, in seconds.
const MAX_AWAY = 7200;

// saveWorld writes the world under SAVE_KEY, stamped with now (ms). Returns false if storage throws.
export function saveWorld(storage, world, now) {
  try {
    storage.setItem(SAVE_KEY, JSON.stringify({ version: SAVE_VERSION, savedAt: now, world }));
    return true;
  } catch {
    return false;
  }
}

// loadWorld returns { world, away } from the saved game, or null when there is no usable save.
// away is the seconds since the save, capped at 7200. Missing nested fields get their defaults.
export function loadWorld(storage, now) {
  let saved;
  try {
    const raw = storage.getItem(SAVE_KEY);
    if (raw == null) return null;
    saved = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!saved || saved.version !== SAVE_VERSION) return null;
  const w = saved.world;
  if (!w || !Array.isArray(w.animals) || typeof w.seed !== 'number') return null;

  const fresh = createWorld(w.seed);
  const world = {
    ...fresh,
    ...w,
    stats: { ...fresh.stats, ...w.stats },
    upgrades: { ...fresh.upgrades, ...w.upgrades },
    farmer: { ...fresh.farmer, ...w.farmer },
    carrots: { ...fresh.carrots, ...w.carrots },
    aliens: { ...fresh.aliens, ...w.aliens },
    camera: { ...fresh.camera, ...w.camera },
    herds: mergeHerds(fresh.herds, w.herds),
    herdEvents: [],
    paused: false,
  };
  const away = typeof saved.savedAt === 'number'
    ? Math.min(MAX_AWAY, Math.max(0, (now - saved.savedAt) / 1000))
    : 0;
  return { world, away };
}

// mergeHerds fills each kind's herd and its acc counters from the saved herds, so a missing kind or counter
// gets its default from the fresh world.
function mergeHerds(freshHerds, savedHerds) {
  const herds = {};
  for (const k of Object.keys(freshHerds)) {
    const saved = savedHerds?.[k];
    herds[k] = {
      ...freshHerds[k],
      ...saved,
      acc: { ...freshHerds[k].acc, ...saved?.acc },
    };
  }
  return herds;
}

// clearSave removes the saved game. It never throws.
export function clearSave(storage) {
  try {
    storage.removeItem(SAVE_KEY);
  } catch {
    // Storage is unavailable, so there is nothing to clear.
  }
}
