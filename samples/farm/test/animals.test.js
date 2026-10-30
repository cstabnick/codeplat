// Tests for web/js/animals.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  createWorld,
  advanceTime,
  setHour,
  PASTURE,
  YARD,
  MUD,
} from '../web/js/state.js';
import { makeRng } from '../web/js/rng.js';
import { updateAnimals, animalDrawables, animalAt, addAnimal } from '../web/js/animals.js';
import { fakeCtx } from './fakectx.js';

const line = (kind) => 'LINE ' + kind;
const DT = 0.1;

// step advances time and the animals by one 0.1 s update.
function step(w, rng) {
  advanceTime(w, DT);
  updateAnimals(w, DT, rng, line);
}

const byId = (w, id) => w.animals.find((a) => a.id === id);
const cows = (w) => w.animals.filter((a) => a.kind === 'cow');

// countWhere counts the animals of a kind that satisfy pred.
function countWhere(w, kind, pred) {
  return w.animals.filter((a) => a.kind === kind && pred(a)).length;
}
// adultMales, adultFemales and young count the scene's animals of a kind in each group. An
// animal with no sex counts as a female.
const adultMales = (w, kind) => countWhere(w, kind, (a) => !a.young && a.sex === 'male');
const adultFemales = (w, kind) => countWhere(w, kind, (a) => !a.young && a.sex !== 'male');
const young = (w, kind) => countWhere(w, kind, (a) => a.young);

// bubblesAdded returns the bubbles in w.bubbles that are not in seen, and adds them to seen.
function newBubbles(w, seen) {
  const fresh = w.bubbles.filter((b) => !seen.has(b));
  for (const b of fresh) seen.add(b);
  return fresh;
}

// holdCows keeps every cow in 'innocent' so no gossip starts and logs its own entry during a test.
function holdCows(w) {
  for (const c of cows(w)) {
    c.state = 'innocent';
    c.timer = 1000;
  }
}

test('gossip: cows whisper while hidden, then go innocent when Gary looks', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.farmer.hidden = true;
  byId(w, 'cow-1').x = 200;
  byId(w, 'cow-1').y = 122;
  byId(w, 'cow-2').x = 220;
  byId(w, 'cow-2').y = 122;

  const seen = new Set();
  let sawWhisper = false;
  let gossiping = null;
  for (let i = 0; i < 200 && !gossiping; i++) {
    step(w, rng);
    for (const b of newBubbles(w, seen)) {
      if (b.kind === 'whisper') sawWhisper = true;
    }
    const pair = cows(w).filter((c) => c.state === 'gossip');
    if (pair.length > 0) gossiping = pair;
  }

  assert.ok(gossiping, 'a cow starts gossiping within 20 s');
  assert.equal(gossiping.length, 2, 'both cows of the pair are gossiping');
  assert.ok(w.stats.gossip >= 1, 'stats.gossip counts the gossip');
  assert.ok(sawWhisper, 'a whisper bubble was shown');

  w.farmer.hidden = false;
  w.farmer.x = 170;
  w.farmer.y = 130;
  w.farmer.facing = 'right';
  w.farmer.state = 'idle';
  step(w, rng);

  for (const c of gossiping) {
    assert.equal(c.state, 'innocent', `${c.id} is innocent once Gary looks`);
  }
});

test('smoke: exactly one smoke line per cigarette, and every cow notices', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.farmer.state = 'smoke';
  w.stats.cigarettes = 1;
  // Hold the cows still in 'innocent' so no gossip starts: a gossip bubble at the same spot
  // would replace the smoke bubble in say(), and the test counts the smoke lines as bubbles.
  for (const c of cows(w)) {
    c.state = 'innocent';
    c.timer = 1000;
  }

  const seen = new Set();
  let smokeLines = 0;
  const countSmoke = () => {
    for (const b of newBubbles(w, seen)) {
      if (b.text === 'LINE smoke') smokeLines += 1;
    }
  };

  step(w, rng);
  countSmoke();
  assert.equal(smokeLines, 1, 'one smoke line on the first update');
  for (const c of cows(w)) {
    assert.equal(c.noticed, 1, `${c.id} noticed the cigarette`);
  }

  for (let i = 0; i < 20; i++) {
    step(w, rng);
    countSmoke();
  }
  assert.equal(smokeLines, 1, 'no further smoke lines for the same cigarette');
});

test('bounds: over 300 s every animal stays inside its area', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const pigBox = {
    x0: MUD.x - 12,
    x1: MUD.x + MUD.w + 12,
    y0: MUD.y - 12,
    y1: MUD.y + MUD.h + 12,
  };

  for (let i = 0; i < 3000; i++) {
    step(w, rng);
    for (const a of w.animals) {
      if (a.kind === 'cow') {
        assert.ok(a.x >= PASTURE.x && a.x <= PASTURE.x + PASTURE.w, `${a.id} x ${a.x} in pasture at ${i}`);
        assert.ok(a.y >= PASTURE.y && a.y <= PASTURE.y + PASTURE.h, `${a.id} y ${a.y} in pasture at ${i}`);
      } else if (a.kind === 'chicken') {
        assert.ok(a.x >= YARD.x && a.x <= YARD.x + YARD.w, `${a.id} x ${a.x} in yard at ${i}`);
        assert.ok(a.y >= YARD.y && a.y <= YARD.y + YARD.h, `${a.id} y ${a.y} in yard at ${i}`);
      } else {
        assert.ok(a.x >= pigBox.x0 && a.x <= pigBox.x1, `${a.id} x ${a.x} in mud at ${i}`);
        assert.ok(a.y >= pigBox.y0 && a.y <= pigBox.y1, `${a.id} y ${a.y} in mud at ${i}`);
      }
    }
  }
});

test('abduction: an abducted animal does not move or change state', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-3');
  cow.abducted = true;
  cow.state = 'abducted';
  cow.lift = 30;
  const { x, y } = cow;

  for (let i = 0; i < 100; i++) step(w, rng);

  assert.equal(cow.x, x);
  assert.equal(cow.y, y);
  assert.equal(cow.state, 'abducted');
  assert.equal(cow.abducted, true);
});

test('clicks: animalAt finds a cow near its feet and nothing far away', () => {
  const w = createWorld(1);
  const cow = byId(w, 'cow-1');

  assert.equal(animalAt(w, cow.x + 5, cow.y - cow.lift), cow);
  assert.equal(animalAt(w, 300, 40), null);
});

test('drawing: every drawable draws without throwing, day and night', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  byId(w, 'cow-2').lift = 10;

  for (const hour of [12, 23]) {
    setHour(w, hour);
    for (let i = 0; i < 30; i++) step(w, rng);
    for (const d of animalDrawables(w)) {
      const ctx = fakeCtx();
      assert.doesNotThrow(() => d.draw(ctx));
      assert.ok(ctx.count('fillRect') > 0, 'the drawable paints something');
    }
  }
});

// Herd: the scene's cows and chickens are a sample of world.herds (herds.js owns the counts).

test('herd: a fresh world shows 4 cows of which 1 is a bull, and 5 chickens of which 1 is a cock', () => {
  const w = createWorld(1);
  const rng = makeRng(1);

  step(w, rng);

  assert.equal(cows(w).length, 4, 'four cows');
  assert.equal(adultMales(w, 'cow'), 1, 'one of them a bull');
  assert.equal(w.animals.filter((a) => a.kind === 'chicken').length, 5, 'five chickens');
  assert.equal(adultMales(w, 'chicken'), 1, 'one of them a cock');
});

test('herd: a big herd shows 12 cows (2 calves, no bull, 10 cows) and 14 chickens', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  Object.assign(w.herds.cow, { females: 2e6, males: 1e5, young: 3e5, leased: 0, tier: 6 });
  Object.assign(w.herds.chicken, { females: 1e6, males: 1e5, young: 3e5, leased: 0 });

  step(w, rng);

  assert.equal(cows(w).length, 12, 'twelve cows');
  assert.equal(young(w, 'cow'), 2, 'two calves');
  assert.equal(adultMales(w, 'cow'), 0, 'no bull');
  assert.equal(adultFemales(w, 'cow'), 10, 'ten cows');
  assert.equal(w.animals.filter((a) => a.kind === 'chicken').length, 14, 'fourteen chickens');
});

test('herd: over 50 updates no update shows more than 12 cows or 14 chickens', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  Object.assign(w.herds.cow, { females: 2e6, males: 1e5, young: 3e5, leased: 0, tier: 6 });
  Object.assign(w.herds.chicken, { females: 1e6, males: 1e5, young: 3e5, leased: 0 });

  for (let i = 0; i < 50; i++) {
    step(w, rng);
    assert.ok(cows(w).length <= 12, `${cows(w).length} cows at update ${i}`);
    assert.ok(
      w.animals.filter((a) => a.kind === 'chicken').length <= 14,
      `chickens over 14 at update ${i}`,
    );
  }
});

test('herd: lowering the herd removes visible animals but never an abducted cow', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-1');
  cow.abducted = true;
  cow.state = 'abducted';
  cow.lift = 30;
  w.herds.cow.females = 0;
  w.herds.cow.males = 0;

  step(w, rng);

  assert.equal(byId(w, 'cow-1'), cow, 'the abducted cow is still in the scene');
  assert.equal(cow.abducted, true);
  assert.equal(cows(w).length, 1, 'only the abducted cow is left');
});

// Herd events: deaths, births and purchases show as ghosts, hearts and portals.

test('death: a cow death shows a ghost, a death bubble and the log line', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  holdCows(w);
  const names = cows(w).map((c) => c.name.toUpperCase());
  w.herdEvents.push({ kind: 'cow', type: 'death', count: 1 });

  step(w, rng);

  assert.ok(w.effects.some((e) => e.kind === 'ghost'), 'a ghost was puffed');
  assert.ok(w.bubbles.some((b) => b.text === 'LINE death'), 'a death bubble was shown');
  assert.equal(w.log.length, 1, 'one log line');
  assert.ok(
    names.some((n) => w.log[0].text === `${n} WENT TO THE BIG PASTURE IN THE SKY`),
    `log line ${w.log[0].text}`,
  );
  assert.deepEqual(w.herdEvents, [], 'the events were cleared');
});

test('death: a chicken death goes to the big coop in the sky', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  holdCows(w);
  const names = w.animals.filter((a) => a.kind === 'chicken').map((c) => c.name.toUpperCase());
  w.herdEvents.push({ kind: 'chicken', type: 'death', count: 1 });

  step(w, rng);

  assert.ok(
    names.some((n) => w.log[0].text === `${n} WENT TO THE BIG COOP IN THE SKY`),
    `log line ${w.log[0].text}`,
  );
});

test('death: a group of deaths in one update logs AND n MORE once', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  holdCows(w);
  w.herdEvents.push({ kind: 'cow', type: 'death', count: 2 });
  w.herdEvents.push({ kind: 'cow', type: 'death', count: 1 });

  step(w, rng);

  assert.equal(w.log.length, 1, 'one log line for the group');
  assert.match(w.log[0].text, / WENT TO THE BIG PASTURE IN THE SKY AND 2 MORE$/);
  assert.equal(w.bubbles.filter((b) => b.text === 'LINE death').length, 1, 'one death bubble');
});

test('death: a second death within 6 s gives no second bubble, ghost or log line', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  holdCows(w);
  w.herdEvents.push({ kind: 'cow', type: 'death', count: 1 });
  step(w, rng);
  const logs = w.log.length;

  w.herdEvents.push({ kind: 'cow', type: 'death', count: 1 });
  step(w, rng);

  assert.equal(w.log.length, logs, 'no second log line');
  assert.equal(w.bubbles.filter((b) => b.text === 'LINE death').length, 1, 'one death bubble');
  assert.equal(w.effects.filter((e) => e.kind === 'ghost').length, 1, 'one ghost');

  for (let i = 0; i < 60; i++) step(w, rng);
  w.herdEvents.push({ kind: 'cow', type: 'death', count: 1 });
  step(w, rng);
  assert.equal(w.log.length, Math.min(5, logs + 1), 'a death after 6 s is logged');
});

test('birth: a birth shows a heart and the birth line', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.herdEvents.push({ kind: 'chicken', type: 'birth', count: 1 });

  step(w, rng);

  assert.ok(w.effects.some((e) => e.kind === 'heart'), 'a heart was puffed');
  assert.ok(w.bubbles.some((b) => b.text === 'LINE birth'), 'a birth bubble was shown');
});

test('buy: a purchase shows a portal puff over an animal of the kind', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  // Without the pig, the only portal puffs are the purchase's.
  w.animals = w.animals.filter((a) => a.kind !== 'pig');
  w.herdEvents.push({ kind: 'cow', type: 'buy', count: 1, sex: 'female' });

  step(w, rng);

  assert.ok(w.effects.some((e) => e.kind === 'portal'), 'a portal was puffed');
});

test('drawing: bulls, cocks, calves and chicks draw without throwing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  // No step here: the herd reconcile would remove the animals added beside the existing ones.
  const bull = addAnimal(w, 'cow', rng, { x: 200, y: 122, sex: 'male' });
  const calf = addAnimal(w, 'cow', rng, { x: 230, y: 130, young: true });
  const cock = addAnimal(w, 'chicken', rng, { x: 130, y: 110, sex: 'male' });
  const chick = addAnimal(w, 'chicken', rng, { x: 140, y: 112, young: true });
  bull.state = 'walk';
  bull.frame = 1;
  cock.state = 'peck';
  cock.frame = 1;
  calf.state = 'walk';
  calf.frame = 1;
  chick.state = 'peck';
  chick.frame = 1;

  for (const d of animalDrawables(w)) {
    const ctx = fakeCtx();
    assert.doesNotThrow(() => d.draw(ctx));
    assert.ok(ctx.count('fillRect') > 0, 'the drawable paints something');
  }
});
