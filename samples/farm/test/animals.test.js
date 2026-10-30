// Tests for web/js/animals.js. The world comes from state.js, the rng from web/js/rng.js, and the
// canvas is test/fakectx.js. The line stub stands in for gags.js, which this file never imports.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  createWorld,
  advanceTime,
  setHour,
  isAdult,
  herdSize,
  capacity,
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

// bubblesAdded returns the bubbles in w.bubbles that are not in seen, and adds them to seen.
function newBubbles(w, seen) {
  const fresh = w.bubbles.filter((b) => !seen.has(b));
  for (const b of fresh) seen.add(b);
  return fresh;
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

// Life cycle: aging, births and deaths.

test('aging: a cow and a chicken gain dt of age on every update', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-1');
  const chicken = byId(w, 'chicken-1');
  cow.age = 10;
  chicken.age = 10;

  for (let i = 0; i < 10; i++) step(w, rng);

  assert.ok(Math.abs(cow.age - 11) < 1e-9, `cow age ${cow.age}`);
  assert.ok(Math.abs(chicken.age - 11) < 1e-9, `chicken age ${chicken.age}`);
});

// holdCows keeps every cow in 'innocent' so no gossip starts and logs its own entry during a test.
function holdCows(w) {
  for (const c of cows(w)) {
    c.state = 'innocent';
    c.timer = 1000;
  }
}

test('death: a cow whose age reaches its life is removed and the farm records it', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-1');
  cow.age = 100;
  cow.life = cow.age + 0.05;
  holdCows(w);

  step(w, rng);

  assert.equal(byId(w, 'cow-1'), undefined, 'the cow is gone');
  assert.equal(w.stats.deaths, 1);
  assert.ok(w.bubbles.some((b) => b.text === 'LINE death'), 'a death bubble was shown');
  assert.ok(w.effects.some((e) => e.kind === 'ghost'), 'a ghost was puffed');
  assert.equal(w.log[0].text, 'DAISY WENT TO THE BIG PASTURE IN THE SKY');
});

test('death: a chicken goes to the big coop in the sky', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const chicken = byId(w, 'chicken-2');
  chicken.age = 100;
  chicken.life = chicken.age + 0.05;
  holdCows(w);

  step(w, rng);

  assert.equal(byId(w, 'chicken-2'), undefined, 'the chicken is gone');
  assert.equal(w.stats.deaths, 1);
  assert.equal(w.log[0].text, 'CLUCK NORRIS WENT TO THE BIG COOP IN THE SKY');
});

test('death: an animal whose life is not yet over stays', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-1');
  cow.age = 100;
  cow.life = cow.age + 1;

  step(w, rng);

  assert.equal(byId(w, 'cow-1'), cow);
  assert.equal(w.stats.deaths, 0);
});

test('birth: an adult cow whose breed timer runs out has a calf beside it', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const mother = byId(w, 'cow-1');
  mother.breed = 0.05;
  const before = cows(w).length;

  step(w, rng);

  const calf = byId(w, 'cow-11');
  assert.ok(calf, 'the calf is cow-11');
  assert.equal(w.nextAnimal, 12, 'the id counter moved on once');
  assert.equal(cows(w).length, before + 1);
  assert.equal(calf.age, 0);
  assert.ok(!isAdult(calf), 'the calf is not an adult');
  assert.ok(Math.abs(calf.x - mother.x) <= 12, 'the calf is near its mother');
  assert.equal(w.stats.births, 1);
  assert.ok(w.effects.some((e) => e.kind === 'heart'), 'a heart was puffed');
  assert.ok(w.bubbles.some((b) => b.text === 'LINE birth'), 'a birth bubble was shown');
  assert.equal(w.log[0].text, `DAISY HAD A CALF: ${calf.name.toUpperCase()}`);
  assert.ok(mother.breed >= 150 && mother.breed < 240, 'the mother breed timer is reset');
  assert.ok(
    !cows(w).some((c) => c !== calf && c.name === calf.name),
    'the calf has a name no other cow has',
  );
});

test('birth: an adult chicken has a chick and the log says CHICK', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const hen = byId(w, 'chicken-1');
  hen.breed = 0.05;

  step(w, rng);

  const chick = byId(w, 'chicken-11');
  assert.ok(chick, 'the chick is chicken-11');
  assert.equal(chick.kind, 'chicken');
  assert.equal(chick.age, 0);
  assert.equal(w.stats.births, 1);
  assert.equal(w.log[0].text, `HENRIETTA HAD A CHICK: ${chick.name.toUpperCase()}`);
});

test('birth: a calf does not count down its breed timer', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const calf = addAnimal(w, 'cow', rng, { x: 200, y: 122, age: 0 });
  calf.breed = 5;

  for (let i = 0; i < 10; i++) step(w, rng);

  assert.equal(calf.breed, 5);
  assert.equal(w.stats.births, 0);
});

test('capacity: a full herd of cows has no calf, but the breed timer still resets', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  while (herdSize(w, 'cow') < capacity(w, 'cow')) addAnimal(w, 'cow', rng);
  const full = cows(w).length;
  const mother = byId(w, 'cow-1');
  mother.breed = 0.05;

  step(w, rng);

  assert.equal(cows(w).length, full, 'no calf was added');
  assert.equal(w.stats.births, 0);
  assert.ok(!w.bubbles.some((b) => b.text === 'LINE birth'));
  assert.ok(mother.breed > 0.05, 'the breed timer was reset');
});

test('capacity: a full coop has no chick', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  while (herdSize(w, 'chicken') < capacity(w, 'chicken')) addAnimal(w, 'chicken', rng);
  const full = herdSize(w, 'chicken');
  byId(w, 'chicken-1').breed = 0.05;

  step(w, rng);

  assert.equal(herdSize(w, 'chicken'), full, 'no chick was added');
  assert.equal(w.stats.births, 0);
});

test('birth: a lone adult cow has no calf, but its breed timer resets', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.animals = w.animals.filter((a) => !(a.kind === 'cow' && a.id !== 'cow-1'));
  const mother = byId(w, 'cow-1');
  mother.breed = 0.05;

  step(w, rng);

  assert.equal(cows(w).length, 1);
  assert.equal(w.stats.births, 0);
  assert.ok(mother.breed > 0.05, 'the breed timer was reset');
});

test('birth: calves are no mate for an adult cow', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  for (const c of cows(w)) if (c.id !== 'cow-1') c.age = 0;
  byId(w, 'cow-1').breed = 0.05;

  step(w, rng);

  assert.equal(w.stats.births, 0);
  assert.equal(cows(w).length, 4);
});

test('abduction: an abducted cow does not age, die or have young', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const cow = byId(w, 'cow-1');
  cow.abducted = true;
  cow.state = 'abducted';
  cow.lift = 30;
  cow.age = 100;
  cow.life = 100.05;
  cow.breed = 0.05;

  for (let i = 0; i < 5; i++) step(w, rng);

  assert.equal(cow.age, 100);
  assert.equal(cow.breed, 0.05);
  assert.equal(byId(w, 'cow-1'), cow, 'the cow is still on the farm');
  assert.equal(w.stats.deaths, 0);
  assert.equal(w.stats.births, 0);
});

test('the pig never ages or dies', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const pig = byId(w, 'pig-1');
  // Even with a life that has already run out, the pig is not touched by the life cycle.
  pig.age = 0;
  pig.life = 0.05;

  for (let i = 0; i < 50; i++) step(w, rng);

  assert.equal(byId(w, 'pig-1'), pig, 'the pig is still on the farm');
  assert.equal(pig.age, 0);
  assert.equal(w.stats.deaths, 0);
});

test('growing up: a calf becomes an adult after 60 s and draws while young', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  const calf = addAnimal(w, 'cow', rng, { x: 200, y: 122, age: 0 });

  for (let i = 0; i < 500; i++) step(w, rng);
  assert.equal(isAdult(calf), false, 'not an adult at 50 s');

  for (let i = 0; i < 110; i++) step(w, rng);
  assert.equal(isAdult(calf), true, 'an adult at 61 s');
});

test('drawing: a calf and a chick draw without throwing', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  addAnimal(w, 'cow', rng, { x: 200, y: 122, age: 0 });
  addAnimal(w, 'chicken', rng, { x: 130, y: 110, age: 0 });
  step(w, rng);

  for (const d of animalDrawables(w)) {
    const ctx = fakeCtx();
    assert.doesNotThrow(() => d.draw(ctx));
    assert.ok(ctx.count('fillRect') > 0, 'the drawable paints something');
  }
});

test('life: a cow that dies this update is not a mate for a birth in the same update', () => {
  const w = createWorld(1);
  const rng = makeRng(1);
  w.animals = w.animals.filter((a) => a.id === 'cow-1' || a.id === 'cow-2');
  const daisy = byId(w, 'cow-1');
  daisy.age = 100;
  daisy.life = 100.05;
  byId(w, 'cow-2').breed = 0.05;
  step(w, rng);
  assert.equal(w.stats.deaths, 1);
  assert.equal(w.stats.births, 0);
  assert.deepEqual(w.animals.map((a) => a.id), ['cow-2']);
});
