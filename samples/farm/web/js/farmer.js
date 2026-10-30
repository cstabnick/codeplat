// Farmer Gary: his chores, his sneaky cigarette breaks, his bedtime, and his sprite.
// This module touches no globals on load; drawing happens only inside the returned draw functions.

import {
  DOOR, FIELD, YARD, PASTURE, HIDEOUT, CROPS, isNight, say, logEvent, puff,
} from './state.js';
import { drawSprite } from './pixel.js';

export const SPEED = 28; // units per second when walking

const WORK_SECONDS = { water: 1.5, harvest: 1.5, feed: 2 };
const SMOKE_SECONDS = 8;
const FACE_SECONDS = 0.7;
const CROP_NAMES = ['carrot'];

const HEAD = [
  '....aaaa....',
  '...aaaaaa...',
  '.9999999999.',
  '...ffff0f...',
  '...fffffff..',
  '...f44fff...',
  '....ffff....',
];
const BODY = [
  '..88c88c88..',
  '.888cccc888.',
  '.f88cccc88f.',
  '.f.cccccc.f.',
  '...cccccc...',
];
const LEGS_STAND = ['...cc..cc...', '...cc..cc...', '...44..44...', '..444..444..'];
const LEGS_STEP  = ['..cc....cc..', '..cc....cc..', '..44....44..', '.444....444.'];
// smoking: mouth row with a cigarette, and his right arm raised to it
const HEAD_SMOKE_MOUTH = '...f44ff779.';
const BODY_SMOKE = [
  '..88c88c8f..',
  '.888cccc88..',
  '.f88cccc8...',
  '.f.cccccc...',
  '...cccccc...',
];

// speak puts a bubble above Gary's head.
function speak(world, text, kind = 'speech') {
  const f = world.farmer;
  say(world, f.x, f.y - 20, text, kind);
}

// plotBottom is the feet position for working the plot at (col, row): the bottom middle of its tile.
function plotBottom(col, row) {
  return {
    x: FIELD.x + col * FIELD.tile + 8,
    y: FIELD.y + row * FIELD.tile + FIELD.tile,
  };
}

// plotUnderFarmer finds the plot whose feet position Gary is standing on.
function plotUnderFarmer(world) {
  const f = world.farmer;
  const col = Math.round((f.x - FIELD.x - 8) / FIELD.tile);
  const row = Math.round((f.y - FIELD.y - FIELD.tile) / FIELD.tile);
  return world.plots.find((p) => p.col === col && p.row === row);
}

// goTo starts a walk. The walk is 'sneak' during the smoke walk to HIDEOUT, 'walk' otherwise.
function goTo(f, x, y) {
  f.target = { x, y };
  f.state = f.task === 'smoke' ? 'sneak' : 'walk';
}

// finishTask ends the current task (the after-every-task rule).
function finishTask(f, timer) {
  f.task = null;
  f.state = 'idle';
  f.target = null;
  f.timer = timer;
}

// startTask starts a task right now, dropping the current one. name is one of 'water', 'harvest',
// 'feed', 'check', 'smoke', 'bed'. 'harvest' with no ripe plot does nothing.
export function startTask(world, name, rng, line) {
  const f = world.farmer;
  if (f.state === 'sleep' && name === 'bed') return;

  let target = null;
  switch (name) {
    case 'water': {
      const plot = world.plots.reduce((low, p) => (p.water < low.water ? p : low));
      target = plotBottom(plot.col, plot.row);
      break;
    }
    case 'harvest': {
      const plot = world.plots.find((p) => p.stage === 3);
      if (!plot) return;
      target = plotBottom(plot.col, plot.row);
      break;
    }
    case 'feed':
      target = { x: YARD.x + 28, y: YARD.y + 20 };
      break;
    case 'check':
      target = { x: PASTURE.x - 6, y: PASTURE.y + 38 };
      break;
    case 'smoke':
      break;
    case 'bed':
      target = { x: DOOR.x, y: DOOR.y };
      break;
    default:
      return;
  }

  // Dropping whatever he was doing: he comes out from hiding, and wakes if he was asleep.
  f.task = name;
  f.hidden = false;
  f.target = null;
  f.timer = 0;
  if (name === 'smoke') {
    f.state = 'sneak';
    f.facing = 'left';
    f.timer = FACE_SECONDS;
  } else {
    goTo(f, target.x, target.y);
  }
}

// arrive runs the next step of the current task when Gary reaches his target.
function arrive(world, rng, line) {
  const f = world.farmer;
  f.target = null;
  switch (f.task) {
    case 'water':
    case 'harvest':
    case 'feed':
      f.state = 'work';
      f.timer = WORK_SECONDS[f.task];
      return;
    case 'check':
      f.state = 'idle';
      f.facing = 'right';
      f.timer = 3;
      return;
    case 'smoke':
      f.state = 'smoke';
      f.hidden = true;
      f.facing = 'left';
      f.cigsToday += 1;
      world.stats.cigarettes += 1;
      logEvent(world, "GARY IS 'CHECKING THE BARN' AGAIN");
      f.timer = SMOKE_SECONDS;
      f.frame = rng.range(0.5, SMOKE_SECONDS - 0.5); // seconds into the smoke when he coughs; -1 once he has
      return;
    case 'bed':
      speak(world, line('night'));
      f.state = 'sleep';
      f.hidden = true;
      f.timer = 0; // counts up to the next 'zzz'
      return;
    default:
      finishTask(f, 1);
  }
}

// completeWork ends a 'work' step: the chore's effect, then the after-every-task rule.
function completeWork(world, rng, line) {
  const f = world.farmer;
  if (f.task === 'water') {
    const plot = plotUnderFarmer(world);
    if (plot) plot.water = 1;
  } else if (f.task === 'harvest') {
    const plot = plotUnderFarmer(world);
    if (plot) {
      const crop = plot.crop;
      world.coins += CROPS[crop].value;
      world.stats.harvests += 1;
      puff(world, 'coin', f.x, f.y - 20, 0, -8, 1.2);
      speak(world, line('harvest'));
      logEvent(world, `GARY HARVESTED A ${crop.toUpperCase()}`);
      plot.crop = rng.pick(CROP_NAMES);
      plot.growth = 0;
      plot.stage = 0;
      plot.water = 0;
    }
  } else if (f.task === 'feed') {
    speak(world, line('farmer'));
  }
  finishTask(f, rng.range(0.5, 2));
}

// chooseTask picks Gary's next task by day, when he is idle.
function chooseTask(world, rng, line) {
  const f = world.farmer;
  if (world.plots.some((p) => p.stage === 3)) {
    startTask(world, 'harvest', rng, line);
  } else if (f.cigsToday < 3 && rng.chance(0.25)) {
    startTask(world, 'smoke', rng, line);
  } else {
    startTask(world, rng.pick(['water', 'water', 'water', 'water', 'feed', 'feed', 'check', 'check']), rng, line);
  }
}

// updateFarmer advances Gary by dt seconds.
export function updateFarmer(world, dt, rng, line) {
  const f = world.farmer;

  if (f.day !== world.day) {
    f.day = world.day;
    f.cigsToday = 0;
  }

  // Waking, or going to bed.
  if (f.state === 'sleep' && !isNight(world)) {
    f.hidden = false;
    f.state = 'idle';
    f.task = null;
    f.target = null;
    f.timer = 1;
    f.y = DOOR.y + 6;
    speak(world, line('morning'), 'thought');
  } else if (isNight(world) && f.task !== 'bed' && f.state !== 'sleep') {
    startTask(world, 'bed', rng, line);
  }

  // Walking.
  if (f.target) {
    const dx = f.target.x - f.x;
    const dy = f.target.y - f.y;
    const dist = Math.hypot(dx, dy);
    if (dx > 0) f.facing = 'right';
    else if (dx < 0) f.facing = 'left';
    f.frame = Math.floor(world.time * 6) % 2;
    f.state = f.task === 'smoke' ? 'sneak' : 'walk';
    const step = SPEED * dt;
    if (dist <= step) {
      f.x = f.target.x;
      f.y = f.target.y;
      arrive(world, rng, line);
    } else {
      f.x += (dx / dist) * step;
      f.y += (dy / dist) * step;
    }
    return;
  }

  switch (f.state) {
    case 'idle': {
      f.timer -= dt;
      if (f.task === 'check') {
        if (f.timer <= 0) finishTask(f, rng.range(0.5, 2));
      } else if (f.task === null && f.timer <= 0 && !isNight(world)) {
        chooseTask(world, rng, line);
      }
      break;
    }
    case 'work': {
      const total = WORK_SECONDS[f.task] || 0;
      const before = total - f.timer;
      f.timer -= dt;
      if (f.task === 'water' && Math.floor((total - f.timer) / 0.25) > Math.floor(before / 0.25)) {
        puff(world, 'drop', f.x + 3, f.y - 8, 0, 10, 0.8);
      }
      if (f.timer <= 0) completeWork(world, rng, line);
      break;
    }
    case 'sneak': {
      // Face left, then right, then think something guilty and walk to HIDEOUT (set by goTo).
      f.timer -= dt;
      if (f.timer <= 0) {
        if (f.facing === 'left') {
          f.facing = 'right';
          f.timer = FACE_SECONDS;
        } else {
          speak(world, line('sneak'), 'thought');
          goTo(f, HIDEOUT.x, HIDEOUT.y);
        }
      }
      break;
    }
    case 'smoke': {
      const before = SMOKE_SECONDS - f.timer;
      f.timer -= dt;
      const now = SMOKE_SECONDS - f.timer;
      if (f.timer > 0 && Math.floor(now / 0.4) > Math.floor(before / 0.4)) {
        puff(world, 'smoke', f.x - 5, f.y - 13, 3, -5, 1.5);
      }
      if (f.frame >= 0 && now >= f.frame) {
        speak(world, line('cough'));
        f.frame = -1;
      }
      if (f.timer <= 0) {
        f.hidden = false;
        puff(world, 'sparkle', f.x, f.y - 20, 0, -6);
        speak(world, line('excuse'));
        finishTask(f, 1);
      }
      break;
    }
    case 'sleep': {
      f.timer += dt;
      if (f.timer >= 2) {
        f.timer -= 2;
        puff(world, 'zzz', DOOR.x + 6, DOOR.y - 30);
      }
      break;
    }
    default:
      break;
  }
}

// spriteRows picks the rows for Gary's current pose.
function spriteRows(world) {
  const f = world.farmer;
  if (f.state === 'smoke') {
    const head = HEAD.map((row, i) => (i === 5 ? HEAD_SMOKE_MOUTH : row));
    return [...head, ...BODY_SMOKE, ...LEGS_STAND];
  }
  const moving = f.target !== null && (f.state === 'walk' || f.state === 'sneak');
  const legs = moving && f.frame === 1 ? LEGS_STEP : LEGS_STAND;
  return [...HEAD, ...BODY, ...legs];
}

// bob is 1 while Gary is working and the seconds into the work are in an odd quarter second.
function bob(f) {
  if (f.state !== 'work') return 0;
  const elapsed = Math.max(0, (WORK_SECONDS[f.task] || 0) - f.timer);
  return Math.floor(elapsed / 0.25) % 2 === 1 ? 1 : 0;
}

// farmerDrawables returns [] when hidden is true and state is 'sleep' (he is inside); otherwise one
// {y, draw(ctx)} that draws his sprite with its feet at (x, y).
export function farmerDrawables(world) {
  const f = world.farmer;
  if (f.hidden && f.state === 'sleep') return [];
  return [{
    y: f.y,
    draw(ctx) {
      drawSprite(ctx, spriteRows(world), f.x - 6, f.y - 16 + bob(f), f.facing === 'left');
    },
  }];
}
