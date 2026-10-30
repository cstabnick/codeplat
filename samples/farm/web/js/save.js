// Saves and loads the Gossip Acres world as JSON in a storage object (like localStorage). The storage
// is always passed in, so this module touches no browser globals and can be tested with a fake.

import { createWorld } from './state.js';

export const SAVE_KEY = 'gossip-acres-save';
export const SAVE_VERSION = 1;

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
    economy: { ...fresh.economy, ...w.economy },
    farmer: { ...fresh.farmer, ...w.farmer },
    paused: false,
  };
  const away = typeof saved.savedAt === 'number'
    ? Math.min(MAX_AWAY, Math.max(0, (now - saved.savedAt) / 1000))
    : 0;
  return { world, away };
}

// clearSave removes the saved game. It never throws.
export function clearSave(storage) {
  try {
    storage.removeItem(SAVE_KEY);
  } catch {
    // Storage is unavailable, so there is nothing to clear.
  }
}
