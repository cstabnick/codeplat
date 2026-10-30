// Entry point: creates or loads the world, runs every module in a fixed-step loop, draws each frame
// in layer order, scales the canvas to a whole multiple of its size, and handles the page controls
// and the pig's shop. URL parameters: seed (integer), hour (0-23), speed (1, 2 or 4). Invalid values
// are ignored. Without a seed the farm is saved in localStorage and loaded on the next visit; a
// seeded farm is a sandbox that is neither loaded nor saved.

import {
  WIDTH, HEIGHT, createWorld, advanceTime, setHour, isNight, say, logEvent, money,
} from './state.js';
import { makeRng } from './rng.js';
import { drawSky, drawNightShade } from './sky.js';
import { drawGround, sceneDrawables } from './buildings.js';
import { updateCrops, drawField, cropDrawables } from './crops.js';
import { updateFarmer, farmerDrawables, startTask } from './farmer.js';
import { updateAnimals, animalDrawables, animalAt } from './animals.js';
import { updateUfo, summonUfo, ufoDrawables } from './ufo.js';
import { updateEconomy, incomeRate, creditAway } from './economy.js';
import { offers, buy } from './shop.js';
import { saveWorld, loadWorld, clearSave } from './save.js';
import { pickLine } from './gags.js';
import { updateOverlay, drawEffects, drawBubbles, drawHud } from './hud.js';

const STEP = 1 / 30;
const SPEEDS = [1, 2, 4];
const SAVE_SECONDS = 5;      // real seconds between autosaves
const SHOP_SECONDS = 0.25;   // real seconds between shop refreshes

// intParam returns the URL parameter as an integer, or null when it is missing or not an integer.
function intParam(params, name) {
  const raw = params.get(name);
  if (raw === null || !/^-?\d+$/.test(raw.trim())) return null;
  const n = Number(raw);
  return Number.isSafeInteger(n) ? n : null;
}

const params = new URLSearchParams(window.location.search);
const seedParam = intParam(params, 'seed');
const sandbox = seedParam !== null;
const hourParam = intParam(params, 'hour');
const speedParam = intParam(params, 'speed');

// storage is localStorage, or null when the browser refuses it or the farm is a sandbox.
function openStorage() {
  if (sandbox) return null;
  try {
    return window.localStorage;
  } catch (_) {
    return null;
  }
}

const storage = openStorage();
const saved = storage ? loadWorld(storage, Date.now()) : null;
const world = saved ? saved.world : createWorld(sandbox ? seedParam : Date.now() % 100000);
const rng = makeRng(saved ? (world.seed + Math.floor(world.time)) >>> 0 : world.seed);
const line = (kind) => pickLine(kind, rng);
if (saved && saved.away >= 60) {
  const earned = creditAway(world, saved.away);
  logEvent(world, `WHILE YOU WERE AWAY: +$${money(earned)}`);
}

if (hourParam !== null && hourParam >= 0 && hourParam <= 23) setHour(world, hourParam);
if (speedParam !== null && SPEEDS.includes(speedParam)) world.speed = speedParam;

const canvas = document.getElementById('farm');
const ctx = canvas.getContext('2d');
const pauseButton = document.getElementById('pause');
const speedButtons = new Map(SPEEDS.map((n) => [n, document.getElementById(`speed-${n}`)]));
const skipButton = document.getElementById('skip');
const shopButton = document.getElementById('shop-open');
const resetButton = document.getElementById('reset');
const shopPanel = document.getElementById('shop');
const shopSay = document.getElementById('shop-say');
const shopList = document.getElementById('shop-items');
const shopClose = document.getElementById('shop-close');

// syncControls makes the pause label and the active speed badge match the world.
function syncControls() {
  pauseButton.textContent = world.paused ? 'Play' : 'Pause';
  for (const [n, button] of speedButtons) button.classList.toggle('on', world.speed === n);
}

function togglePause() {
  world.paused = !world.paused;
  syncControls();
}

function setSpeed(n) {
  world.speed = n;
  syncControls();
}

function skipToNightOrMorning() {
  setHour(world, isNight(world) ? 6 : 21);
}

// step advances the world by one fixed step, in the order the modules depend on.
function step(dt) {
  advanceTime(world, dt);
  updateFarmer(world, dt, rng, line);
  updateAnimals(world, dt, rng, line);
  updateCrops(world, dt);
  updateUfo(world, dt, rng, line);
  updateEconomy(world, dt);
  updateOverlay(world, dt);
}

// draw paints one frame: background layers, then the sorted sprites, then the overlays.
function draw() {
  ctx.imageSmoothingEnabled = false;
  drawSky(ctx, world);
  drawGround(ctx, world);
  drawField(ctx, world);

  const items = [
    ...sceneDrawables(world),
    ...cropDrawables(world),
    ...farmerDrawables(world),
    ...animalDrawables(world),
    ...ufoDrawables(world),
  ];
  items.sort((a, b) => a.y - b.y);
  for (const item of items) item.draw(ctx);

  drawEffects(ctx, world);
  drawNightShade(ctx, world);
  drawBubbles(ctx, world);
  drawHud(ctx, world, incomeRate(world));
}

// The shop panel: one row per offer, built once and refreshed in place so focus survives.
const shopRows = new Map();

function buildShop() {
  for (const o of offers(world)) {
    const li = document.createElement('li');
    const text = document.createElement('div');
    text.className = 'item-text';
    const name = document.createElement('strong');
    name.textContent = o.name;
    const have = document.createElement('span');
    have.className = 'owned';
    const blurb = document.createElement('p');
    blurb.textContent = o.blurb;
    text.append(name, ' ', have, blurb);
    const button = document.createElement('button');
    button.type = 'button';
    const note = document.createElement('span');
    note.className = 'note';
    const side = document.createElement('div');
    side.className = 'item-buy';
    side.append(button, note);
    li.append(text, side);
    shopList.append(li);
    button.addEventListener('click', () => {
      const result = buy(world, o.id, rng, line);
      shopSay.textContent = result.text;
      refreshShop();
    });
    shopRows.set(o.id, { have, button, note });
  }
}

function refreshShop() {
  for (const o of offers(world)) {
    const row = shopRows.get(o.id);
    row.have.textContent = o.owned;
    row.button.textContent = `BUY $${money(o.cost)}`;
    row.button.disabled = !o.ok;
    row.note.textContent = o.ok ? '' : o.reason;
  }
}

function openShop() {
  shopPanel.hidden = false;
  shopButton.classList.add('on');
  shopSay.textContent = line('shop');
  refreshShop();
  shopPanel.scrollIntoView({ block: 'nearest' });
}

function closeShop() {
  shopPanel.hidden = true;
  shopButton.classList.remove('on');
}

function toggleShop() {
  if (shopPanel.hidden) openShop();
  else closeShop();
}

function save() {
  if (storage) saveWorld(storage, world, Date.now());
}

function newFarm() {
  if (!window.confirm('Start a new farm? The prince keeps no records of your old one.')) return;
  if (storage) clearSave(storage);
  window.removeEventListener('pagehide', save);
  window.location.search = '';
}

let scale = 1;

// fit picks the largest whole scale k (at least 1) at which the canvas, its 4-px border and the
// page furniture still fit the window, and sets the canvas CSS size to 320k by 180k.
function fit() {
  const k = Math.max(
    1,
    Math.min(
      Math.floor((window.innerWidth - 40) / WIDTH),
      Math.floor((window.innerHeight - 168) / HEIGHT),
    ),
  );
  scale = k;
  canvas.style.width = `${WIDTH * k}px`;
  canvas.style.height = `${HEIGHT * k}px`;
}

canvas.addEventListener('click', (e) => {
  const r = canvas.getBoundingClientRect();
  const x = (e.clientX - r.left - canvas.clientLeft) / scale;
  const y = (e.clientY - r.top - canvas.clientTop) / scale;

  const animal = animalAt(world, x, y);
  if (animal && animal.kind === 'pig') {
    openShop();
    say(world, animal.x, animal.y - 9, shopSay.textContent, 'speech', 4);
    return;
  }
  if (animal) {
    const H = animal.kind === 'cow' ? 12 : 8;
    const text = `${animal.name.toUpperCase()}: ${line('click')}`;
    say(world, animal.x, animal.y - animal.lift - H - 1, text, 'speech');
    return;
  }

  const f = world.farmer;
  if (!f.hidden && Math.hypot(x - f.x, y - f.y) <= 10) {
    say(world, f.x, f.y - 20, line('farmer'), 'thought');
  }
});

pauseButton.addEventListener('click', togglePause);
for (const [n, button] of speedButtons) button.addEventListener('click', () => setSpeed(n));
skipButton.addEventListener('click', skipToNightOrMorning);
shopButton.addEventListener('click', toggleShop);
shopClose.addEventListener('click', closeShop);
resetButton.addEventListener('click', newFarm);
window.addEventListener('pagehide', save);

window.addEventListener('keydown', (e) => {
  if (e.key === ' ') {
    e.preventDefault();
    if (!e.repeat) togglePause();
    return;
  }
  if (e.repeat || e.ctrlKey || e.metaKey || e.altKey) return;
  const key = e.key.toLowerCase();
  if (key === '1' || key === '2' || key === '4') setSpeed(Number(key));
  else if (key === 'n') skipToNightOrMorning();
  else if (key === 's') toggleShop();
  else if (key === 'escape') closeShop();
  else if (key === 'u') {
    if (isNight(world)) summonUfo(world, rng);
  } else if (key === 'f') {
    startTask(world, 'smoke', rng, line);
  }
});

window.addEventListener('resize', fit);
fit();
buildShop();
syncControls();
window.farm = { world };

let acc = 0;
let last = performance.now();
let sinceSave = 0;
let sinceShop = 0;

// frame runs once per animation frame: it advances the fixed steps the elapsed time has earned,
// then draws.
function frame(now) {
  const realDt = Math.min((now - last) / 1000, 0.1);
  last = now;
  if (!world.paused) {
    acc += realDt * world.speed;
    while (acc >= STEP) {
      step(STEP);
      acc -= STEP;
    }
  }
  draw();
  sinceSave += realDt;
  if (sinceSave >= SAVE_SECONDS) {
    sinceSave = 0;
    save();
  }
  sinceShop += realDt;
  if (!shopPanel.hidden && sinceShop >= SHOP_SECONDS) {
    sinceShop = 0;
    refreshShop();
  }
  requestAnimationFrame(frame);
}

requestAnimationFrame(frame);
