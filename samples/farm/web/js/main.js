// Entry point: creates or loads the world, runs every module in a fixed-step loop, draws each frame
// in layer order, scales the canvas to a whole multiple of its size, and handles the page controls,
// panning across the land the farm owns, plucking carrots and the pig's shop. URL parameters: seed
// (integer), hour (0-23), speed (1, 2 or 4). Invalid values are ignored. Without a seed the farm is
// saved in localStorage and loaded on the next visit; a seeded farm is a sandbox that is neither
// loaded nor saved.

import {
  WIDTH, HEIGHT, createWorld, advanceTime, setHour, isNight, say, logEvent, money, landWidth,
} from './state.js';
import { makeRng } from './rng.js';
import { drawSky, drawNightShade } from './sky.js';
import { drawGround, sceneDrawables } from './buildings.js';
import { updateCrops, drawField, cropDrawables } from './crops.js';
import { updateFarmer, farmerDrawables, startTask } from './farmer.js';
import { updateAnimals, animalDrawables, animalAt } from './animals.js';
import { updateHerds } from './herds.js';
import { updateAliens, alienDrawables, summonVisit } from './aliens.js';
import { updateCarrots, carrotDrawables, pluck, plotAt, CARROT_UPGRADES } from './carrots.js';
import { housingDrawables } from './housing.js';
import { updateFlock, flockDrawables } from './flock.js';
import { updateEconomy, incomeRate, creditAway } from './economy.js';
import { offers, buy } from './shop.js';
import { saveWorld, loadWorld, clearSave } from './save.js';
import { pickLine } from './gags.js';
import { updateOverlay, drawEffects, drawBubbles, drawHud } from './hud.js';

const STEP = 1 / 30;
const SPEEDS = [1, 2, 4];
const SAVE_SECONDS = 5;      // real seconds between autosaves
const SHOP_SECONDS = 0.25;   // real seconds between shop refreshes
const PAN_SPEED = 160;       // units per real second while an arrow key or A/D is held
const PAN_STEP = 160;        // units moved by each Pan button press
const DRAG_PX = 4;           // CSS pixels a press must move before it counts as a drag
const PLUCK_THOUGHT = 0.15;  // chance that Gary says something after a pluck
const PAN_KEYS = { arrowleft: -1, a: -1, arrowright: 1, d: 1 };

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
const panLeftButton = document.getElementById('pan-left');
const panRightButton = document.getElementById('pan-right');
const shopPanel = document.getElementById('shop');
const shopSay = document.getElementById('shop-say');
const shopList = document.getElementById('shop-items');
const shopClose = document.getElementById('shop-close');

// held says which pan keys are down: left for the arrow or A, right for the arrow or D.
const held = { left: false, right: false };
// camX is the camera's left edge as the last frame drew it, in world units.
let camX = 0;
// press is the mouse press on the canvas being tracked, or null: where it began, where the
// pointer was last, and whether it has become a drag.
let press = null;
// ignoreClick is set when a drag ends, so the click event that follows it does nothing.
let ignoreClick = false;

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
  updateHerds(world, dt, rng);
  updateAnimals(world, dt, rng, line);
  updateFlock(world, dt, rng);
  updateCrops(world, dt);
  updateCarrots(world, dt);
  updateAliens(world, dt, rng, line);
  updateEconomy(world, dt);
  updateOverlay(world, dt);
}

// draw paints one frame: the sky in screen space, then the land and sprites shifted by the camera,
// then the night shade, the speech bubbles and the HUD in screen space again.
function draw() {
  ctx.imageSmoothingEnabled = false;
  camX = Math.round(Math.min(Math.max(world.camera.x, 0), landWidth(world) - WIDTH));
  world.camera.x = camX;

  drawSky(ctx, world);
  ctx.save();
  ctx.translate(-camX, 0);
  drawGround(ctx, world);
  drawField(ctx, world);

  const items = [
    ...sceneDrawables(world),
    ...housingDrawables(world),
    ...cropDrawables(world),
    ...carrotDrawables(world),
    ...farmerDrawables(world),
    ...animalDrawables(world),
    ...flockDrawables(world),
    ...alienDrawables(world),
  ];
  items.sort((a, b) => a.y - b.y);
  for (const item of items) item.draw(ctx);

  drawEffects(ctx, world);
  ctx.restore();
  drawNightShade(ctx, world);
  drawBubbles(ctx, world, camX);
  drawHud(ctx, world, incomeRate(world));
}

// The shop panel: rows are built once per list of offers and refreshed in place so focus survives.
// shopRows maps an offer id to its row's elements; builtKey is the offer ids the rows were built for.
const shopRows = new Map();
let builtKey = '';

// offerKey joins the offer ids; when it changes (new land adds rows) the shop is rebuilt.
function offerKey(list) {
  return list.map((o) => o.id).join('|');
}

// makeButton creates a shop button that runs onClick. Its label is set by refreshRow.
function makeButton(onClick) {
  const button = document.createElement('button');
  button.type = 'button';
  button.addEventListener('click', onClick);
  return button;
}

// buyClick returns the click handler that buys n of an offer and shows what the prince said.
function buyClick(id, n) {
  return () => {
    const result = buy(world, id, n, rng, line);
    shopSay.textContent = result.text;
    refreshShop();
  };
}

// tenPrice is what ten more of a bulk offer cost. Carrot levels cost more each time, so those sum
// the next ten levels; everything else has a fixed unit price.
function tenPrice(o) {
  if (!o.id.startsWith('carrot:')) return o.cost * 10;
  const key = o.id.slice('carrot:'.length);
  const def = CARROT_UPGRADES.find((u) => u.id === key);
  const level = world.carrots[key];
  let total = 0;
  for (let k = 0; k < 10; k++) total += Math.round(def.base * def.growth ** (level + k));
  return total;
}

// buildRow creates the list item for one offer and records its elements in shopRows.
function buildRow(o) {
  const li = document.createElement('li');
  li.dataset.id = o.id;
  const text = document.createElement('div');
  text.className = 'item-text';
  const name = document.createElement('strong');
  name.textContent = o.name;
  const have = document.createElement('span');
  have.className = 'owned';
  const blurb = document.createElement('p');
  blurb.textContent = o.blurb;
  text.append(name, ' ', have, blurb);

  const note = document.createElement('span');
  note.className = 'note';
  const side = document.createElement('div');
  side.className = 'item-buy';
  const row = { have, note };
  if (o.bulk) {
    row.one = makeButton(buyClick(o.id, 1));
    row.ten = makeButton(buyClick(o.id, 10));
    row.max = makeButton(buyClick(o.id, 'max'));
    const group = document.createElement('div');
    group.className = 'buy-group';
    group.append(row.one, row.ten, row.max);
    side.append(group, note);
  } else {
    row.one = makeButton(buyClick(o.id, 1));
    side.append(row.one, note);
  }
  li.append(text, side);
  shopRows.set(o.id, row);
  return li;
}

// buildShop writes one heading and one list per section, in the order the offers give them.
function buildShop(list) {
  shopList.replaceChildren();
  shopRows.clear();
  const sections = new Map();
  for (const o of list) {
    if (!sections.has(o.section)) sections.set(o.section, []);
    sections.get(o.section).push(o);
  }
  for (const [title, rows] of sections) {
    const heading = document.createElement('h3');
    heading.textContent = title;
    const ul = document.createElement('ul');
    for (const o of rows) ul.append(buildRow(o));
    shopList.append(heading, ul);
  }
  builtKey = offerKey(list);
}

// refreshRow sets a row's text and disabled state from its offer.
function refreshRow(o, row) {
  row.have.textContent = o.owned;
  row.note.textContent = o.ok ? '' : o.reason;
  if (o.bulk) {
    row.one.textContent = `BUY 1 $${money(o.cost)}`;
    row.one.disabled = !o.ok;
    row.ten.textContent = `10 $${money(tenPrice(o))}`;
    row.ten.disabled = o.maxCount < 10;
    row.max.textContent = `MAX (${money(o.maxCount)})`;
    row.max.disabled = !o.ok;
  } else {
    row.one.textContent = `BUY $${money(o.cost)}`;
    row.one.disabled = !o.ok;
  }
}

// refreshShop rebuilds the shop when its offers changed, then refreshes every row.
function refreshShop() {
  const list = offers(world);
  if (offerKey(list) !== builtKey) buildShop(list);
  for (const o of list) refreshRow(o, shopRows.get(o.id));
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

// panDirection is -1 for a key that pans left, 1 for one that pans right, and 0 for any other.
function panDirection(e) {
  return PAN_KEYS[e.key.toLowerCase()] ?? 0;
}

// clickScene handles a click on the canvas: a carrot plot is plucked, otherwise the pig opens the
// shop and the other animals and Gary talk.
function clickScene(e) {
  if (ignoreClick) {
    ignoreClick = false;
    return;
  }
  const r = canvas.getBoundingClientRect();
  const x = (e.clientX - r.left - canvas.clientLeft) / scale + camX;
  const y = (e.clientY - r.top - canvas.clientTop) / scale;
  const f = world.farmer;

  const plot = plotAt(x, y);
  if (plot >= 0) {
    pluck(world, plot);
    if (!f.hidden && rng.chance(PLUCK_THOUGHT)) say(world, f.x, f.y - 20, line('pluck'), 'thought');
    return;
  }

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

  if (!f.hidden && Math.hypot(x - f.x, y - f.y) <= 10) {
    say(world, f.x, f.y - 20, line('farmer'), 'thought');
  }
}

canvas.addEventListener('click', clickScene);

// A press on the canvas becomes a drag once it has moved DRAG_PX; from then on the scene follows
// the pointer. A press that never becomes a drag is left to the click handler.
canvas.addEventListener('mousedown', (e) => {
  if (e.button !== 0) return;
  press = { startX: e.clientX, startY: e.clientY, lastX: e.clientX, dragging: false };
  ignoreClick = false;
});

window.addEventListener('mousemove', (e) => {
  if (!press) return;
  if (!press.dragging) {
    if (Math.hypot(e.clientX - press.startX, e.clientY - press.startY) < DRAG_PX) return;
    press.dragging = true;
  }
  // Dragging right moves the scene right, so the camera moves left.
  world.camera.x -= (e.clientX - press.lastX) / scale;
  press.lastX = e.clientX;
});

window.addEventListener('mouseup', () => {
  if (!press) return;
  ignoreClick = press.dragging;
  press = null;
});

pauseButton.addEventListener('click', togglePause);
for (const [n, button] of speedButtons) button.addEventListener('click', () => setSpeed(n));
skipButton.addEventListener('click', skipToNightOrMorning);
shopButton.addEventListener('click', toggleShop);
shopClose.addEventListener('click', closeShop);
resetButton.addEventListener('click', newFarm);
panLeftButton.addEventListener('click', () => { world.camera.x -= PAN_STEP; });
panRightButton.addEventListener('click', () => { world.camera.x += PAN_STEP; });
window.addEventListener('pagehide', save);

window.addEventListener('keydown', (e) => {
  const dir = panDirection(e);
  if (dir !== 0 && !e.ctrlKey && !e.metaKey && !e.altKey) {
    if (dir < 0) held.left = true;
    else held.right = true;
    e.preventDefault();
    return;
  }
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
    if (isNight(world)) summonVisit(world, rng);
  } else if (key === 'f') {
    startTask(world, 'smoke', rng, line);
  }
});

window.addEventListener('keyup', (e) => {
  const dir = panDirection(e);
  if (dir < 0) held.left = false;
  else if (dir > 0) held.right = false;
});

// Losing focus can swallow the keyup, so forget any held key when the window blurs.
window.addEventListener('blur', () => {
  held.left = false;
  held.right = false;
});

window.addEventListener('resize', fit);
fit();
refreshShop();
syncControls();
window.farm = { world };

let acc = 0;
let last = performance.now();
let sinceSave = 0;
let sinceShop = 0;

// frame runs once per animation frame: it pans while a pan key is held, advances the fixed steps
// the elapsed time has earned, then draws.
function frame(now) {
  const realDt = Math.min((now - last) / 1000, 0.1);
  last = now;
  const pan = (held.right ? 1 : 0) - (held.left ? 1 : 0);
  if (pan !== 0) world.camera.x += pan * PAN_SPEED * realDt;
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
