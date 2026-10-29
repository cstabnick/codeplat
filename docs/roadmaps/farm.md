# Roadmap: farm sample (Gossip Acres)

## Goal

A sample project in `samples/farm/`: **Gossip Acres**, a farm simulation drawn in 8-bit pixel art on a canvas in the browser. It is meant to be shown off, so it has a day/night cycle and a cast of characters with running gags:

- **Farmer Gary** waters crops, harvests, feeds the chickens and checks on the cows. A few times a day he looks left, looks right, and sneaks behind the barn for a cigarette, coughs, then comes back "minty fresh". At night he goes to bed.
- **The cows** (Daisy, Bessie, Moolinda, Clarabelle) gossip in whispers whenever Gary can't see them. When he turns towards them, they snap to "innocent" mode ("MOO.", whistling).
- **The chickens** peck and occasionally panic. **Sir Oinksalot** the pig wallows in mud and has grand thoughts.
- **At night a UFO** may beam up a cow and put it back. The cow is never the same.
- A HUD shows the day, the clock, coins, and the important statistics (cigarettes today, gossip sessions, abductions), plus a ticker of events. Clicking an animal makes it deny everything.

All art is custom 8-bit sprites written as strings of PICO-8 palette characters, and all text is drawn with a built-in 3×5 pixel font, so nothing is fetched from the internet.

Shape of the code: its own Go module, `farm`, whose only Go is a static file server (`web/web.go` embeds `web/index.html` and `web/js/`, and `cmd/farm` serves it on `127.0.0.1:8088`). Everything else is plain browser ES modules in `web/js/` with no build step and no npm dependencies, tested with `node --test` against a recording fake canvas context. Tracker project: `FARM`.

The world is one plain object created by `state.js`. Each module owns some of its fields and exposes `update…(world, dt, rng, line)` and `draw…`/`…Drawables(world)` functions; `main.js` runs them in a fixed order. Drawables are `{y, draw(ctx)}` and are sorted by `y` so that sprites overlap correctly. Gag text comes in through a `line(kind)` function argument, so no module imports `gags.js` except `main.js`.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module, the world model, the seeded RNG, the test fake context and the pixel toolkit (palette, sprites, font, speech bubbles) exist and pass their tests. | 2, parallel |
| 2. Components | Sky, buildings, crops, farmer, animals, UFO, gag lines, HUD and the Go server each pass their own tests. | 9, parallel |
| 3. Wiring | `main.js` runs the game in the browser and a headless smoke test passes; README, ARCHITECTURE and the repository's `start` script cover the sample. | 2, parallel |

No issue has a dependency inside its epic.

Why three: every component imports `state.js`, `rng.js` and `pixel.js`, so those exist first (epic 1; `state.js` and `pixel.js` do not import each other, so they are two parallel issues). The components share no file and do not import one another: cross-module needs go through fields of the world object (fixed in 1.1) and through the `line(kind)` argument (epic 2). `main.js` imports all of them (epic 3).

---

## Epic 1: Foundation

Done when: `samples/farm` exists; `node --test test/state.test.js test/rng.test.js test/pixel.test.js` passes.

### 1.1 Create the farm module, the world model and the seeded RNG

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Create `samples/farm/` under the repository root and run every command from it. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`. Node 22 is installed.
- This sample is its own Go module plus plain browser ES modules. The root CLAUDE.md and ARCHITECTURE.md describe the tracker, not this sample; do not edit anything outside `samples/farm/`.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported, so `node --test` can import it. The code_map and symbol tools cover Go only; read JS files directly.
- Another worker is writing `samples/farm/web/js/pixel.js` and `samples/farm/test/pixel.test.js` at the same time. Do not create or edit those.

## Goal
The module skeleton, the world model every other module reads and writes (`web/js/state.js`), a seeded random number generator (`web/js/rng.js`), and a recording fake canvas context that the tests of every later module use (`test/fakectx.js`).

## Read first
- samples/chess/Makefile: the shape of a sample Makefile.

## Change
- samples/farm/go.mod: exactly two lines, `module farm` and `go 1.27.2`. No go.sum (there are no dependencies).
- samples/farm/package.json: `{"name": "farm", "private": true, "type": "module"}`
- samples/farm/Makefile: `.PHONY: test vet run smoke`; `test` runs `go test ./...` then `node --test test/*.test.js`; `vet` runs `go vet ./...`; `run` runs `go run ./cmd/farm`; `smoke` runs `node test/smoke.cjs`.
- samples/farm/web/index.html: a placeholder page, replaced in a later epic: `<!doctype html>`, `<meta charset="utf-8">`, `<title>Gossip Acres</title>`, and `<canvas id="farm" width="320" height="180"></canvas>`.
- samples/farm/web/js/rng.js and samples/farm/web/js/state.js: the Contract below, exactly.
- samples/farm/test/fakectx.js: the Contract below.
- samples/farm/test/rng.test.js, samples/farm/test/state.test.js (the testing worker writes these).

## Contract

rng.js:
```js
// makeRng returns a deterministic generator (mulberry32) seeded with the integer seed.
export function makeRng(seed) // -> { next, int, pick, chance, range }
//   next()        float in [0, 1)
//   int(n)        integer in [0, n)
//   range(a, b)   float in [a, b)
//   pick(array)   one element
//   chance(p)     true with probability p (p <= 0 is never, p >= 1 is always)
```
mulberry32, for reference:
```js
let a = seed >>> 0;
function next() {
  a = (a + 0x6D2B79F5) >>> 0;
  let t = a;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
```

state.js — constants (coordinates are logical canvas pixels; the canvas is 320×180; an entity's `x, y` is the point between its feet, so a sprite of size w×h is drawn with its top-left at `(round(x - w/2), round(y - h))`):
```js
export const WIDTH = 320;
export const HEIGHT = 180;
export const HORIZON = 64;        // sky above, ground below
export const DAY_LENGTH = 180;    // seconds of simulated time per day
export const HOUSE = { x: 8, y: 22, w: 60, h: 56 };
export const DOOR = { x: 38, y: 78 };                 // feet position at the front door
export const WINDOWS = [ { x: 16, y: 50, w: 8, h: 8 }, { x: 52, y: 50, w: 8, h: 8 } ];
export const FIELD = { x: 14, y: 96, cols: 6, rows: 3, tile: 16 };  // plot (col,row) top-left = (x + col*tile, y + row*tile)
export const COOP = { x: 120, y: 58, w: 32, h: 26 };
export const YARD = { x: 112, y: 90, w: 56, h: 40 };  // where chickens roam
export const BARN = { x: 230, y: 24, w: 72, h: 60 };
export const HIDEOUT = { x: 310, y: 80 };             // feet position behind the barn, where Gary smokes
export const PASTURE = { x: 176, y: 96, w: 138, h: 72 };  // fenced; cows stay inside
export const MUD = { x: 60, y: 150, w: 44, h: 16 };   // the pig's puddle
export const CROPS = {
  carrot:  { grow: 40, value: 3 },   // grow: seconds of watered time from planted to ripe
  corn:    { grow: 60, value: 5 },
  pumpkin: { grow: 90, value: 12 },
};
```

state.js — functions:
```js
// createWorld returns a new world. Same seed, same world.
export function createWorld(seed)

// advanceTime adds dt seconds: time += dt; day = 1 + floor(time / DAY_LENGTH);
// clock = (time % DAY_LENGTH) / DAY_LENGTH. Clock 0 is 06:00.
export function advanceTime(world, dt)

// hourOf returns the hour of day as a float in [0, 24): (6 + clock * 24) % 24.
export function hourOf(world)

// isNight is true when hourOf(world) >= 21 or < 6.
export function isNight(world)

// setHour moves time forward (never back) to the next moment whose hour is h, keeping the day if
// that moment is later today. Used by the "skip" button and by tests.
export function setHour(world, h)

// farmerSees reports whether Gary can see the point (x, y): false when farmer.hidden is true or
// farmer.state is 'smoke' or 'sleep'; otherwise true when the point is on the side he faces
// (facing 'right': x >= farmer.x - 8; facing 'left': x <= farmer.x + 8) and within 140 px.
export function farmerSees(world, x, y)

// say adds a bubble {x, y, text, kind, ttl}. (x, y) is where its tail points, normally just above a
// head. kind is 'speech', 'thought' or 'whisper'. It first removes any bubble whose x and y are both
// within 6 px of the new one, and afterwards drops the oldest bubbles so at most 8 remain.
export function say(world, x, y, text, kind = 'speech', ttl = 3)

// logEvent puts {day, hour: Math.floor(hourOf(world)), text} at the front of world.log and keeps
// at most 5 entries.
export function logEvent(world, text)

// puff adds an effect {kind, x, y, vx, vy, ttl, age: 0} and keeps at most 120 effects (drops oldest).
// kinds: 'smoke', 'zzz', 'note', 'coin', 'drop', 'feather', 'sparkle', 'heart', 'mud'.
export function puff(world, kind, x, y, vx = 0, vy = -6, ttl = 1.5)
```

The world returned by createWorld(seed):
```js
{
  seed,
  time: 0, day: 1, clock: 0, speed: 1, paused: false,
  coins: 0,
  stats: { cigarettes: 0, gossip: 0, harvests: 0, abductions: 0 },
  farmer: { x: 38, y: 84, facing: 'right', state: 'idle', task: null, target: null,
            timer: 2, frame: 0, hidden: false, cigsToday: 0, day: 1 },
  animals: [
    // every animal also has: facing, state, target: null, timer: 1, frame: 0,
    //                        partner: null, noticed: 0, lift: 0, abducted: false
    { id: 'cow-1', kind: 'cow', name: 'Daisy',      x: 200, y: 122, facing: 'right', state: 'graze' },
    { id: 'cow-2', kind: 'cow', name: 'Bessie',     x: 236, y: 144, facing: 'left',  state: 'graze' },
    { id: 'cow-3', kind: 'cow', name: 'Moolinda',   x: 280, y: 118, facing: 'left',  state: 'graze' },
    { id: 'cow-4', kind: 'cow', name: 'Clarabelle', x: 262, y: 160, facing: 'right', state: 'graze' },
    { id: 'chicken-1', kind: 'chicken', name: 'Henrietta',    x: 118, y: 102, facing: 'right', state: 'peck' },
    { id: 'chicken-2', kind: 'chicken', name: 'Cluck Norris', x: 130, y: 118, facing: 'left',  state: 'peck' },
    { id: 'chicken-3', kind: 'chicken', name: 'Eggatha',      x: 146, y: 106, facing: 'right', state: 'peck' },
    { id: 'chicken-4', kind: 'chicken', name: 'Nugget',       x: 156, y: 124, facing: 'left',  state: 'peck' },
    { id: 'chicken-5', kind: 'chicken', name: 'Pecky',        x: 124, y: 126, facing: 'right', state: 'peck' },
    { id: 'pig-1', kind: 'pig', name: 'Sir Oinksalot', x: 82, y: 160, facing: 'right', state: 'wallow' },
  ],
  // 18 plots, row-major (row 0 first). For plot (col, row):
  //   crop = ['carrot', 'corn', 'pumpkin'][col % 3]
  //   growth = ((col * 7 + row * 3) % 10) / 10
  //   stage = growth >= 1 ? 3 : Math.floor(growth * 3)
  //   water = 0.5
  plots: [ { col, row, crop, growth, stage, water }, ... ],
  bubbles: [],
  effects: [],
  log: [],
  ufo: { state: 'away', x: -30, y: 20, targetId: null, timer: 0, lastDay: 0 },
}
```

Field ownership, for the record (later modules rely on it): `time/day/clock` by advanceTime; `farmer`, `coins`, `stats.cigarettes`, `stats.harvests` and plot `crop`/`water` changes made by the farmer by farmer.js; `animals` (except while `abducted`) and `stats.gossip` by animals.js; plot `growth`/`stage`/`water` decay by crops.js; `ufo`, an animal's `lift`/`abducted`, `stats.abductions` by ufo.js; bubble and effect aging by hud.js.

test/fakectx.js:
```js
// fakeCtx returns a stand-in for CanvasRenderingContext2D. Every method call is recorded as
// {op: <method name>, args: [...]} in ctx.calls, and every property assignment as
// {op: 'set', name, value}; reading a property returns the last value assigned. Methods return
// undefined, except measureText (returns {width: 0}) and createLinearGradient /
// createRadialGradient (return {addColorStop() {}}). ctx.canvas is {width: 320, height: 180}.
// ctx.count(op) returns how many calls of that method were recorded.
export function fakeCtx()
```
Implement it with a Proxy over a plain object.

## Done when
- `node --test test/state.test.js test/rng.test.js` passes. The tests cover: createWorld is deterministic for a seed and has 4 cows, 5 chickens, 1 pig and 18 plots with the fields above; advanceTime/hourOf (clock 0 is hour 6, and after DAY_LENGTH*1.5 seconds day is 2 and hour is 18); isNight at hours 22, 3, 12; setHour(world, 21) from hour 10 leaves day unchanged and hourOf ≈ 21, and setHour(world, 6) from hour 22 advances the day; farmerSees for facing left/right, out of range, hidden and smoking; say replaces a nearby bubble and caps at 8; logEvent caps at 5 with the newest first; puff caps at 120; makeRng gives the same sequence for the same seed and int/pick/chance stay in range; fakeCtx records calls and sets and counts.
- `go vet ./...` exits 0 (there is no Go code yet; a "matched no packages" warning is fine).

## Out of scope
- web/js/pixel.js and test/pixel.test.js (another worker). Any Go code (a later issue).
~~~~

### 1.2 Add the pixel toolkit: palette, sprites, pixel font and speech bubbles

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Another worker is creating the rest of this directory (go.mod, package.json, Makefile, web/index.html, web/js/state.js, web/js/rng.js, test/fakectx.js) at the same time; do not create or edit those. If `samples/farm/` does not exist yet, create just the directories you need.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only.
- `pixel.js` must not import any other file of this project, so it does not depend on the other worker. Its test must not import test/fakectx.js for the same reason: write a small recording object inside the test file instead.
- Node 22 is installed. If `package.json` is not there yet when you run your test, Node still loads `.js` files as ES modules (you may see a warning about module type; that is fine).

## Goal
`web/js/pixel.js`: everything needed to draw 8-bit art on a 2D canvas context: a 16-colour palette, a sprite drawer for sprites written as strings, a 3×5 pixel font, text wrapping, and speech/thought/whisper bubbles. Every other drawing module uses it.

## Read first
- Nothing in the repository; this file stands alone.

## Change
- samples/farm/web/js/pixel.js: the Contract below.
- samples/farm/test/pixel.test.js (the testing worker writes it).

## Contract
```js
// The PICO-8 palette, keyed by the character used in sprite strings.
export const PALETTE = {
  '0': '#000000', '1': '#1d2b53', '2': '#7e2553', '3': '#008751',
  '4': '#ab5236', '5': '#5f574f', '6': '#c2c3c7', '7': '#fff1e8',
  '8': '#ff004d', '9': '#ffa300', 'a': '#ffec27', 'b': '#00e436',
  'c': '#29adff', 'd': '#83769c', 'e': '#ff77a8', 'f': '#ffccaa',
};

// A sprite is an array of equal-length strings; each character is a PALETTE key, or '.' for a
// transparent pixel. spriteSize returns {w: rows[0].length, h: rows.length}.
export function spriteSize(rows)

// drawSprite draws rows with its top-left at integer (x, y), one canvas unit per character,
// using ctx.fillStyle and ctx.fillRect. Horizontal runs of the same colour in a row are one
// fillRect. flip = true mirrors it left-to-right. x and y are rounded with Math.round.
// An unknown character is skipped like '.'.
export function drawSprite(ctx, rows, x, y, flip = false)

// GLYPHS maps each supported character to 5 strings of 3 characters, '#' for ink and '.' for none.
export const GLYPHS

// drawText draws text with its top-left at (x, y): the text is upper-cased; each glyph is 3×5 and
// advances 4 units; '\n' starts a new line 6 units lower at the original x; a character missing
// from GLYPHS is drawn as '?'. It uses ctx.fillStyle = color and fillRect.
export function drawText(ctx, text, x, y, color = '#fff1e8')

// textWidth is the width in units of the longest line: 4 * chars - 1, or 0 for empty text.
export function textWidth(text)

// wrapText splits text into lines of at most maxChars characters, breaking at spaces; a single
// word longer than maxChars is cut at maxChars. Returns an array of strings (never empty for
// non-empty text; [] for '').
export function wrapText(text, maxChars)

// drawBubble draws a comic bubble whose tail points at (x, y), with the box above it.
// Text is upper-cased and wrapped to 22 characters per line. The box is textWidth + 6 wide and
// lines*6 + 5 tall, centred on x but kept inside 1..318 horizontally and at least y = 1 at the top.
// kind 'speech': fill '#fff1e8', 1-unit border '#000000', text '#000000', a 3-unit tail.
// kind 'thought': same box with border '#83769c' and instead of a tail two small dots
//   (2x2 and 1x1) between the box and (x, y).
// kind 'whisper': fill '#c2c3c7', border '#5f574f' drawn dotted (every other unit), text '#1d2b53',
//   3-unit tail.
export function drawBubble(ctx, x, y, text, kind = 'speech')
```

GLYPHS, exactly (each glyph is five rows top to bottom):
```
A .#. #.# ### #.# #.#     B ##. #.# ##. #.# ##.     C .## #.. #.. #.. .##
D ##. #.# #.# #.# ##.     E ### #.. ##. #.. ###     F ### #.. ##. #.. #..
G .## #.. #.# #.# .##     H #.# #.# ### #.# #.#     I ### .#. .#. .#. ###
J ..# ..# ..# #.# .#.     K #.# #.# ##. #.# #.#     L #.. #.. #.. #.. ###
M #.# ### ### #.# #.#     N ##. #.# #.# #.# #.#     O .#. #.# #.# #.# .#.
P ##. #.# ##. #.. #..     Q .#. #.# #.# ##. .##     R ##. #.# ##. #.# #.#
S .## #.. .#. ..# ##.     T ### .#. .#. .#. .#.     U #.# #.# #.# #.# ###
V #.# #.# #.# #.# .#.     W #.# #.# ### ### #.#     X #.# #.# .#. #.# #.#
Y #.# #.# .#. .#. .#.     Z ### ..# .#. #.. ###
0 ### #.# #.# #.# ###     1 .#. ##. .#. .#. ###     2 ##. ..# .#. #.. ###
3 ##. ..# .#. ..# ##.     4 #.# #.# ### ..# ..#     5 ### #.. ##. ..# ##.
6 .## #.. ### #.# ###     7 ### ..# .#. .#. .#.     8 ### #.# ### #.# ###
9 ### #.# ### ..# ##.
space ... ... ... ... ...  . ... ... ... ... .#.   , ... ... ... .#. #..
! .#. .#. .#. ... .#.     ? ##. ..# .#. ... .#.     ' .#. .#. ... ... ...
" #.# #.# ... ... ...     - ... ... ### ... ...     : ... .#. ... .#. ...
; ... .#. ... .#. #..     * ... #.# .#. #.# ...     ( ..# .#. .#. .#. ..#
) #.. .#. .#. .#. #..     / ..# ..# .#. #.. #..     + ... .#. ### .#. ...
= ... ### ... ### ...     % #.# ..# .#. #.. #.#     # #.# ### #.# ### #.#
& .#. #.# .#. #.# .##     ~ ... ... .## ##. ...     $ .## ##. .#. .## ##.
```

## Done when
- `node --test test/pixel.test.js` passes. The tests use a small recording ctx defined in the test file and cover: spriteSize; drawSprite merges a run of one colour into one fillRect, skips '.', mirrors with flip, rounds x/y; every GLYPHS entry is 5 strings of 3 characters of '#' and '.'; GLYPHS covers A–Z, 0–9 and every punctuation character above; drawText upper-cases and advances 4 per character and draws unknown characters as '?'; textWidth for '', 'A', 'HELLO' and a two-line text; wrapText at spaces and cutting a long word; drawBubble keeps the box inside the canvas for x = 0 and x = 320 and y = 2, for all three kinds, without throwing.
- `node -e "import('./web/js/pixel.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `GLYPHS,PALETTE,drawBubble,drawSprite,drawText,spriteSize,textWidth,wrapText`.

## Out of scope
- Every other file in samples/farm (another worker owns them).
~~~~

---

## Epic 2: Components

Done when: each module below passes its own test file; `go test ./...` passes.

Shared by every JS issue of this epic (pasted into each): the Environment block, and the facts that `state.js`, `rng.js`, `pixel.js` and `test/fakectx.js` exist and are the only project modules a component may import.

### 2.1 Draw the sky: day/night colours, sun, moon, stars, clouds, hills and the night shade

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- The look is 8-bit: draw with fillRect at whole units on a 320×180 canvas; no anti-aliased curves or blur. Use `PALETTE` colours from pixel.js except where this issue gives other colours.

## Goal
`web/js/sky.js` draws everything behind the ground (sky, sun or moon, stars, clouds, distant hills) for any time of day, and the translucent night shade that darkens the whole scene after dark, with glowing farmhouse windows on top of it.

## Read first
- web/js/state.js: WIDTH, HEIGHT, HORIZON, WINDOWS, hourOf, and the world's `time` field.
- web/js/pixel.js: PALETTE, drawSprite.
- web/js/rng.js: makeRng.
- test/fakectx.js: fakeCtx and ctx.count.

## Change
- samples/farm/web/js/sky.js: the Contract below.
- samples/farm/test/sky.test.js (the testing worker writes it).

## Contract
```js
// skyColors returns the sky gradient for an hour in [0, 24) as {top, bottom}, each '#rrggbb'
// lower-case, interpolating each channel linearly (rounded) between these keyframes:
//   hour  0: top #0b0f2a  bottom #1d2b53
//   hour  5: top #1d2b53  bottom #7e2553
//   hour  7: top #29adff  bottom #ffccaa
//   hour 10: top #29adff  bottom #bdebff
//   hour 17: top #29adff  bottom #bdebff
//   hour 19: top #7e2553  bottom #ffa300
//   hour 21: top #1d2b53  bottom #7e2553
//   hour 24: top #0b0f2a  bottom #1d2b53
export function skyColors(hour)

// darkness returns how dark the scene is, 0 to 0.55: 0 from 07:00 to 19:00, 0.55 from 21:00 to
// 05:00, and linear between (rising 19→21, falling 05→07).
export function darkness(hour)

// drawSky fills y 0..HORIZON-1 and draws, in this order:
// 1. The gradient from skyColors(hourOf(world)) as 8 flat horizontal bands (stepped, 8-bit style).
// 2. Stars when darkness > 0.1: 40 positions fixed by makeRng(7) within y 2..HORIZON-20; each is a
//    1×1 '#fff1e8' pixel, and about a third of them blink off and on with world.time.
// 3. The sun from 06:00 to 20:00, an 8×8 pixel disc ('a' with a '9' rim) moving left to right on an
//    arc: x = WIDTH * (hour - 6) / 14, y = HORIZON - 6 - sin(PI * (hour - 6) / 14) * 48.
//    The moon from 20:00 to 06:00 the same way over its 10 hours: a 7×7 '#fff1e8' disc with two '6'
//    craters.
// 4. Four clouds, each built from 3–4 overlapping '#fff1e8' rectangles (with a '6' underside), that
//    drift right at 3–6 units per second of world.time and wrap around (x from -40 to WIDTH + 40).
//    At night draw them in '#5f574f'.
// 5. Two layers of rolling hills along the horizon, built from 1-unit-wide columns whose height
//    comes from sums of sines of x: the far layer '#008751' up to 14 units above HORIZON, the near
//    layer '#00e436' up to 7 units above HORIZON. Both end exactly at y = HORIZON.
export function drawSky(ctx, world)

// drawNightShade, when darkness(hour) > 0, fills the whole canvas with '#1d2b53' at
// globalAlpha = darkness, then restores globalAlpha to 1 and draws each of WINDOWS as a lit
// '#ffec27' rectangle with a 1-unit '#ffa300' glow ring around it, at globalAlpha
// min(1, darkness * 2). It must leave ctx.globalAlpha at 1. Between 18:00 and 22:00 it also draws
// 6 fireflies: 1×1 '#ffec27' pixels over the field and yard (x 20..170, y 90..140) that wander with
// world.time and blink.
export function drawNightShade(ctx, world)
```

## Done when
- `node --test test/sky.test.js` passes. The tests cover: skyColors at hours 0, 7, 12, 19 (exact keyframe values), and that 6 and 20 lie between neighbours; darkness at 3, 6, 12, 20, 22 (0.55, 0.275, 0, 0.275, 0.55); drawSky at hours 3, 6.5, 12 and 19.5 calls fillRect and never paints below HORIZON (check every fillRect's y + h <= HORIZON); drawNightShade at noon makes no fillRect and at 23:00 does, and ctx.globalAlpha is 1 afterwards.
- `node -e "import('./web/js/sky.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `darkness,drawNightShade,drawSky,skyColors`.

## Out of scope
- Everything else in samples/farm. The ground, buildings and trees belong to another worker.
~~~~

### 2.2 Draw the ground and the buildings: grass, paths, mud, house, barn, coop, fences, trees and the scarecrow

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- The look is 8-bit: draw with fillRect (and pixel.js drawSprite) at whole units on a 320×180 canvas; no anti-aliased curves or blur. Use only `PALETTE` colours. Cheesy and charming beats realistic: chunky outlines, bright colours, small details (a horseshoe over the barn door, a weathervane, a crooked mailbox).

## Goal
`web/js/buildings.js` draws the ground layer (everything flat that nothing stands behind), and returns the standing scenery as depth-sorted drawables so animals and the farmer can walk in front of and behind it.

## Read first
- web/js/state.js: HORIZON, WIDTH, HEIGHT, HOUSE, DOOR, WINDOWS, FIELD, COOP, YARD, BARN, HIDEOUT, PASTURE, MUD.
- web/js/pixel.js: PALETTE, drawSprite, drawText.
- test/fakectx.js: fakeCtx and ctx.count.

## Change
- samples/farm/web/js/buildings.js: the Contract below.
- samples/farm/test/buildings.test.js (the testing worker writes it).

## Contract
```js
// drawGround fills y HORIZON..HEIGHT with grass ('3' with a scatter of 'b' tufts at positions that
// depend only on x and y, never on Math.random, so it does not flicker), then draws:
// - a dirt path ('4' with '9' pebbles) 6 units wide from DOOR down to y 88, then along y 86..92
//   from x 30 to x 300;
// - the MUD puddle: a rounded '4' blob filling MUD with a darker '2' rim and two '5' bubbles;
// - the YARD as lighter ground ('f' at the edges blending to '9' dots) — chickens scratch here;
// - the back rail of the PASTURE fence: a '4' rail along the top edge (y = PASTURE.y) with posts
//   every 12 units. The field's soil is NOT drawn here (another module draws plots).
export function drawGround(ctx, world)

// sceneDrawables returns an array of {y, draw(ctx)} for the standing scenery. y is the scenery's
// base line (its bottom edge), used to sort it with characters. It returns, at least:
// - the house: HOUSE rectangle; a red '8' roof with '2' shading over the top 20 units (pointed or
//   sloped), cream '7' walls with '4' trim, a '4' door centred on DOOR.x (10 wide, 16 tall, bottom at
//   DOOR.y), the two WINDOWS ('c' panes with a '7' cross; do not light them, another module does),
//   and a chimney. y = HOUSE.y + HOUSE.h.
// - the barn: BARN rectangle; classic red '8' with '7' white X-braced doors in the middle of the
//   front, a '2' hayloft door with a '9' hay tuft, a gambrel roof in '2', and a weathervane.
//   y = BARN.y + BARN.h.
// - the coop: COOP rectangle; small '4' wooden hut with a '9' ramp and a tiny 'CLUCK HQ' sign in
//   drawText. y = COOP.y + COOP.h.
// - three trees at feet (96, 66), (172, 62) and (214, 70): a '4' trunk and a round '3'/'b' crown
//   about 20 wide; y = the feet y.
// - the scarecrow at feet (6, 140) next to the field: hat, '9' straw arms, and a '0' crow on its
//   arm; y = 140.
// - a mailbox at feet (74, 90) saying 'GARY'; y = 90.
// - the front rail of the PASTURE fence along its bottom edge plus its left and right sides
//   ('4'/'f' rails, posts every 12 units, a gate gap 14 wide on the left side between
//   y 126 and 140); y = PASTURE.y + PASTURE.h.
export function sceneDrawables(world)
```
Draw nothing outside the canvas. Pre-render nothing; drawing per frame is fine at this size.

## Done when
- `node --test test/buildings.test.js` passes. The tests cover: drawGround draws only at y >= HORIZON (check every fillRect) and draws the same calls twice for the same world (no randomness); sceneDrawables returns at least 9 items, each with a numeric y and a draw function; drawing every item into a fakeCtx does not throw and calls fillRect; the house item's y is HOUSE.y + HOUSE.h and the barn item's y is BARN.y + BARN.h.
- `node -e "import('./web/js/buildings.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `drawGround,sceneDrawables`.

## Out of scope
- Sky, hills and night lighting (another worker), crop plots (another worker), and everything else in samples/farm.
~~~~

### 2.3 Grow the crops and draw the field

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- The look is 8-bit: draw with fillRect and pixel.js drawSprite at whole units. Use only `PALETTE` colours.

## Goal
`web/js/crops.js` makes watered crops grow over time, draws the soil of the field, and returns each plant as a drawable. The farmer (another module) waters, harvests and replants by setting plot fields directly; this module only grows and draws.

## Read first
- web/js/state.js: FIELD, CROPS, createWorld (the `plots` array), puff.
- web/js/pixel.js: drawSprite, spriteSize.
- test/fakectx.js: fakeCtx and ctx.count.

## Change
- samples/farm/web/js/crops.js: the Contract below.
- samples/farm/test/crops.test.js (the testing worker writes it).

## Contract
A plot is `{col, row, crop, growth, stage, water}`: crop is 'carrot', 'corn', 'pumpkin' or null (empty); growth is 0..1; stage is 0 (seedling), 1, 2 (growing) or 3 (ripe); water is 0..1.
```js
// updateCrops, for every plot with a crop:
//   if water > 0: growth = min(1, growth + dt / CROPS[crop].grow)
//   water = max(0, water - dt / 60)
//   stage = growth >= 1 ? 3 : Math.floor(growth * 3)
// Plots with crop null are left alone. When a plot becomes ripe (stage changes to 3) it adds one
// 'sparkle' effect with puff(world, 'sparkle', x, y) at the plot's centre.
export function updateCrops(world, dt)

// drawField draws the soil of each plot as a 16×16 tile at (FIELD.x + col*16, FIELD.y + row*16):
// tilled '4' soil with darker '2' furrow lines, drawn darker ('2' base with '4' furrows) when
// water > 0.3 (wet), and a 1-unit '5' border around the whole field. Nothing else.
export function drawField(ctx, world)

// cropDrawables returns one {y, draw(ctx)} per plot with a crop; y = plot top + 14. Each draws the
// crop's sprite for its stage, centred in its tile with its bottom at y:
//   stage 0: a 3×3 'b' sprout (same for all crops)
//   carrot  1: small 'b' leaves; 2: bushy 'b'/'3' leaves; 3: leaves plus an '9' carrot top showing
//   corn    1: short '3' stalk; 2: tall '3' stalk with leaves; 3: tall stalk with two 'a' cobs
//   pumpkin 1: 'b' vine; 2: vine with a small '9' bud; 3: a fat 9×7 '9' pumpkin with '4' stem and
//           '2' ribs
// Stage-3 crops blink a 1×1 '#fff1e8' glint on and off every half second of world.time.
export function cropDrawables(world)
```

## Done when
- `node --test test/crops.test.js` passes. The tests cover: a watered carrot plot (growth 0, water 1) reaches stage 3 after 40 s of updates in 0.1 s steps and adds a 'sparkle' effect when it does; a plot with water 0 does not grow; water falls by about dt/60 and never below 0; an empty plot is unchanged; drawField and every cropDrawables item draw without throwing; cropDrawables returns one item per planted plot and none for crop null.
- `node -e "import('./web/js/crops.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `cropDrawables,drawField,updateCrops`.

## Out of scope
- Watering, harvesting and replanting (the farmer module does those), and everything else in samples/farm.
~~~~

### 2.4 Make Farmer Gary: chores, sneaky cigarette breaks, bedtime, and his sprite

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` and `../web/js/rng.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- Gag lines come in through the `line(kind)` argument: a function that returns a string. Never import gags.js. In tests pass `(kind) => 'LINE ' + kind`.

## Goal
`web/js/farmer.js`: Farmer Gary's behaviour and his sprite. By day he picks chores (water, harvest, feed the chickens, check the cows) and, up to three times a day, sneaks behind the barn for a cigarette: he looks left, looks right, thinks something guilty, walks to HIDEOUT, smokes (puffs of smoke, a cough), and returns "minty fresh". At night he walks home and sleeps. The cows (another module) gossip whenever `farmerSees` from state.js is false, so his facing direction, `hidden` and `state` are what make the gags work.

## Read first
- web/js/state.js: DOOR, FIELD, YARD, PASTURE, HIDEOUT, CROPS, the `farmer` and `plots` fields of createWorld, hourOf, isNight, say, logEvent, puff.
- web/js/pixel.js: drawSprite, spriteSize.
- web/js/rng.js: makeRng (next, int, pick, chance, range).
- test/fakectx.js: fakeCtx.

## Change
- samples/farm/web/js/farmer.js: the Contract below.
- samples/farm/test/farmer.test.js (the testing worker writes it).

## Contract
The farmer is `world.farmer = {x, y, facing, state, task, target, timer, frame, hidden, cigsToday, day}`.
state is one of 'idle', 'walk', 'work', 'sneak', 'smoke', 'sleep'. task is one of null, 'water',
'harvest', 'feed', 'check', 'smoke', 'bed'. target is null or {x, y}.

```js
export const SPEED = 28; // units per second when walking

// updateFarmer advances Gary by dt seconds.
// - If world.day !== farmer.day: set farmer.day = world.day and cigsToday = 0.
// - Bedtime: when isNight(world) and task is not 'bed' and state is not 'sleep', startTask('bed').
//   'bed' walks to DOOR; on arrival state = 'sleep', hidden = true, say(line('night')) just before
//   hiding. While asleep, every 2 s puff('zzz') at (DOOR.x + 6, DOOR.y - 30).
// - Waking: when state is 'sleep' and !isNight(world): hidden = false, state = 'idle', timer = 1,
//   y = DOOR.y + 6, say a thought line('morning').
// - Walking: while target is set, move towards it at SPEED * dt (never overshoot), set facing from
//   the horizontal direction ('right' if dx > 0, 'left' if dx < 0, unchanged if 0) and
//   frame = Math.floor(world.time * 6) % 2. On arrival clear target and run the task's next step.
// - When state is 'idle' and timer <= 0 (by day), choose the next task:
//     'harvest' if any plot has stage 3 (always first);
//     else 'smoke' with probability 0.25 if cigsToday < 3;
//     else one of 'water' (weight 4), 'feed' (2), 'check' (2), using rng.
// Steps of each task (timer counts down by dt; "work N s" means state 'work' for N seconds):
//   water:   walk to the plot with the lowest water (bottom of its tile: x = FIELD.x + col*16 + 8,
//            y = FIELD.y + row*16 + 16), work 1.5 s puffing 'drop' effects, then water = 1.
//   harvest: walk to a ripe plot, work 1.5 s, then coins += CROPS[crop].value, stats.harvests += 1,
//            puff 'coin', say line('harvest'), logEvent "GARY HARVESTED A <CROP>" (upper case),
//            and replant: crop = rng.pick(['carrot','corn','pumpkin']), growth 0, stage 0, water 0.
//   feed:    walk to (YARD.x + 28, YARD.y + 20), work 2 s, say line('farmer').
//   check:   walk to (PASTURE.x - 6, PASTURE.y + 38), then state 'idle' facing 'right' for 3 s
//            staring at the cows (this makes them act innocent).
//   smoke:   state 'sneak': face 'left' 0.7 s, face 'right' 0.7 s, then say(line('sneak'), 'thought')
//            and walk to HIDEOUT (state stays 'sneak' while walking). On arrival: state 'smoke',
//            hidden = true, facing 'left', cigsToday += 1, stats.cigarettes += 1,
//            logEvent "GARY IS 'CHECKING THE BARN' AGAIN", timer 8. While smoking puff 'smoke'
//            from his mouth (x - 5, y - 13) every 0.4 s drifting up and right, and once, at a random
//            moment, say line('cough'). When the timer ends: hidden = false, puff 'sparkle',
//            say line('excuse'), state 'idle', timer 1.
//   After every task: task = null, state = 'idle', timer = rng.range(0.5, 2).
// Bubbles from Gary use say(world, farmer.x, farmer.y - 20, text, kind).
export function updateFarmer(world, dt, rng, line)

// startTask starts a task right now, dropping the current one. name is one of 'water', 'harvest',
// 'feed', 'check', 'smoke', 'bed'. 'harvest' with no ripe plot does nothing.
export function startTask(world, name, rng, line)

// farmerDrawables returns [] when hidden is true and state is 'sleep' (he is inside); otherwise one
// {y: farmer.y, draw(ctx)} that draws his sprite with drawSprite, feet at (x, y), flipped when
// facing 'left'. While smoking he IS drawn (the viewer sees him behind the barn even though the
// cows cannot).
export function farmerDrawables(world)
```

Sprites, facing right, 12 wide and 16 tall (use exactly these):
```js
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
```
Frames: idle/work = HEAD + BODY + LEGS_STAND; walk/sneak while moving alternates LEGS_STAND and LEGS_STEP by `frame`; work bobs the whole sprite down 1 unit every other quarter second; smoke = HEAD with row 5 replaced by HEAD_SMOKE_MOUTH, + BODY_SMOKE + LEGS_STAND.

## Done when
- `node --test test/farmer.test.js` passes. The tests use createWorld(1), makeRng(1), a line stub, and setHour from state.js, and step in 0.1 s. They cover: at 23:00 Gary reaches the door within 30 s, then state 'sleep', hidden true, and farmerDrawables is []; at 06:30 the next day he is awake, not hidden; startTask 'smoke' at 10:00 leads within 30 s to state 'smoke' with hidden true, cigsToday 1, stats.cigarettes 1, at least one 'smoke' effect, and afterwards hidden false and state 'idle'; while state is 'smoke', farmerSees(world, 200, 120) from state.js is false; startTask 'harvest' on a plot set to stage 3 raises coins by that crop's value within 30 s and replants it at stage 0; startTask 'water' sets the driest plot's water to 1; cigsToday never exceeds 3 over a simulated day of 0.1 s steps from 06:00 to 21:00; facing becomes 'left' when walking left.
- `node -e "import('./web/js/farmer.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `SPEED,farmerDrawables,startTask,updateFarmer`.

## Out of scope
- The animals' reactions (another worker), crop growth (another worker), bubble and effect drawing (another worker), and everything else in samples/farm.
~~~~

### 2.5 Bring the animals to life: gossiping cows, panicking chickens and a philosophical pig

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` and `../web/js/rng.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- Gag lines come in through the `line(kind)` argument: a function that returns a string. Never import gags.js. In tests pass `(kind) => 'LINE ' + kind`.

## Goal
`web/js/animals.js`: the animals' behaviour and sprites. The core gag: cows gossip in whisper bubbles whenever Farmer Gary cannot see them (`farmerSees` in state.js is false), and the moment he can, they snap to acting innocent. They also comment when he sneaks off to smoke. Chickens peck and sometimes panic; the pig wallows and thinks big thoughts.

## Read first
- web/js/state.js: PASTURE, YARD, MUD, the `animals`, `farmer` and `stats` fields of createWorld, farmerSees, isNight, say, logEvent, puff.
- web/js/pixel.js: drawSprite, spriteSize.
- web/js/rng.js: makeRng (next, int, pick, chance, range).
- test/fakectx.js: fakeCtx.

## Change
- samples/farm/web/js/animals.js: the Contract below.
- samples/farm/test/animals.test.js (the testing worker writes it).

## Contract
An animal is `{id, kind, name, x, y, facing, state, target, timer, frame, partner, noticed, lift, abducted}`.
Cow states: 'graze', 'walk', 'gossip', 'innocent', 'abducted'. Chicken: 'peck', 'walk', 'panic', 'sleep'. Pig: 'wallow', 'walk', 'sleep'.

```js
// updateAnimals advances every animal whose abducted is false (an abducted animal belongs to
// another module; do not move it or change its state). Probabilities "per second" mean
// rng.chance(p * dt). Bubbles use say(world, a.x, a.y - <sprite height> - 1, text, kind).
//
// Cows stay inside PASTURE shrunk by 10 units on each side (clamp x and y every update).
// - graze: stand, frame toggles slowly (chewing); timer counts down; at 0 either pick a random
//   target in the pasture and 'walk' (speed 8), or keep grazing (timer = rng.range(2, 5)).
// - Start gossip: a grazing or walking cow that the farmer cannot see (!farmerSees(world, x, y)),
//   with another such cow, not gossiping, within 48 units: per second probability 0.35. Both get
//   state 'gossip', partner = the other's id, face each other, timer 7; the first one walks to
//   within 14 units of the second. stats.gossip += 1 and logEvent
//   "<NAME> AND <NAME> ARE GOSSIPING" (names upper case).
// - gossip: every 1.6 s the two take turns saying line('gossip') as a 'whisper' with ttl 2.2.
//   It ends when the timer runs out (both back to 'graze'), or the moment farmerSees either cow:
//   then both turn 'innocent' for 2.5 s, face away from the farmer, one says line('innocent') as
//   'speech' and the other puffs a 'note' (whistling).
// - innocent: stand still; afterwards 'graze'.
// - Smoke reaction: when world.farmer.state is 'smoke' and stats.cigarettes > cow.noticed, the
//   first cow to notice says line('smoke') as a 'whisper' and every cow sets noticed =
//   stats.cigarettes (so it happens once per cigarette).
// Chickens stay inside YARD.
// - peck: frame alternates head up/down every 0.3 s; occasionally walk (speed 12) to a nearby
//   point. At night (isNight) they 'sleep' (no movement; puff 'zzz' rarely).
// - Per second probability 0.01 a chicken panics: state 'panic' for 3 s running at speed 40 in
//   random zig-zags (new target every 0.3 s), puffing 'feather' effects, and says line('chicken').
//   A panicking chicken next to (within 16 units of) a calm one makes it panic too.
// Pig stays within MUD widened by 12 units on each side.
// - wallow: sits in the mud, puffs 'mud' now and then, per second probability 0.04 says
//   line('pig') as a 'thought' (ttl 4). Occasionally walks a few units. At night 'sleep'.
// frame for walking animals = Math.floor(world.time * 6) % 2. facing follows horizontal movement.
export function updateAnimals(world, dt, rng, line)

// animalDrawables returns one {y, draw(ctx)} per animal with y = a.y; the sprite is drawn with its
// feet at (a.x, a.y - a.lift), flipped when facing 'left'. A cow with lift > 0 also gets a small
// '5' shadow ellipse on the ground at (a.x, a.y). Sleeping animals are drawn with their eyes
// closed or lower (your choice) — keep it readable.
export function animalDrawables(world)

// animalAt returns the animal whose feet are nearest to (x, y) within 14 units, or null.
// Used when the player clicks the canvas.
export function animalAt(world, x, y)
```

Sprites, facing right (use exactly these; make the walk and peck frames by changing the leg or head rows):
```js
const COW = [
  '..............6...6.',
  '..............77777.',
  '.777007777700777077.',
  '.770007777000777777.',
  '0777007777700777eeee',
  '0777777700777777eee0',
  '.77777770077777.....',
  '.777777777777777....',
  '..7ee77777777.......',
  '..77.77....77.77....',
  '..77.77....77.77....',
  '..00.00....00.00....',
];
const COW_LEGS_STEP = ['..77..77..77..77....', '..77..77..77..77....', '..00..00..00..00....'];
const CHICKEN = [
  '.....8..',
  '....777.',
  '....7079',
  '7...777.',
  '7777777.',
  '.777777.',
  '..7777..',
  '...9.9..',
];
const PIG = [
  '.........ee...',
  '..eeeeeeeeee..',
  '2eeeeeeeeee0e.',
  'eeeeeeeeeeeeff',
  'eeeeeeeeeeee2f',
  '.eeeeeeeeeee..',
  '..ee..ee.ee...',
  '..22..22.22...',
];
```
The pig in the mud: draw its bottom two rows in '4' (muddy legs) and a couple of '4' splotches on its body.

## Done when
- `node --test test/animals.test.js` passes. The tests use createWorld(1), makeRng(1), a line stub, and step in 0.1 s. They cover: with the farmer hidden and two cows placed 20 units apart, within 20 s at least one cow is in 'gossip', stats.gossip >= 1, and a 'whisper' bubble exists; then with the farmer unhidden, state 'idle', at (170, 130) facing 'right', after one more update both gossiping cows are 'innocent'; with farmer.state 'smoke' and stats.cigarettes 1, exactly one 'smoke' line is said and every cow's noticed is 1; over 300 s every cow stays inside PASTURE, every chicken inside YARD; an animal with abducted true does not move or change state; animalAt finds a cow clicked 5 units from its feet and returns null 40 units from anything; every animalDrawables item draws into a fakeCtx without throwing.
- `node -e "import('./web/js/animals.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `animalAt,animalDrawables,updateAnimals`.

## Out of scope
- The farmer (another worker), the UFO (another worker, which sets lift and abducted), bubble drawing (another worker), and everything else in samples/farm.
~~~~

### 2.6 Add the UFO that borrows a cow at night

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` and `../web/js/rng.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- Gag lines come in through the `line(kind)` argument: a function that returns a string. Never import gags.js. In tests pass `(kind) => 'LINE ' + kind`.

## Goal
`web/js/ufo.js`: once in a while at night, a flying saucer drifts in, beams a cow up into the sky, holds it for a moment, puts it back, and leaves. The cow then says something haunted. The animals module skips any animal whose `abducted` is true and draws it raised by `lift`, so this module only sets those two fields on the cow.

## Read first
- web/js/state.js: WIDTH, PASTURE, the `ufo`, `animals` and `stats` fields of createWorld, isNight, say, logEvent, puff.
- web/js/pixel.js: drawSprite, spriteSize.
- web/js/rng.js: makeRng.
- test/fakectx.js: fakeCtx.

## Change
- samples/farm/web/js/ufo.js: the Contract below.
- samples/farm/test/ufo.test.js (the testing worker writes it).

## Contract
`world.ufo = {state, x, y, targetId, timer, lastDay}`; state is 'away', 'arrive', 'beam', 'hold', 'return', 'leave'. x, y is the saucer's centre.
```js
// updateUfo:
// - away: if isNight(world) and ufo.lastDay !== world.day, per second probability 0.03 call
//   summonUfo. (So at most one visit per night; lastDay is set when it arrives.)
// - arrive: fly from x = -20 at y = 18 towards (cow.x, cow.y - 70) at 60 units/s; on arrival
//   state 'beam', timer 3.
// - beam: cow.abducted = true, cow.state = 'abducted'; cow.lift rises linearly from 0 to 58 over
//   the 3 s; then state 'hold', timer 1.5 (lift stays 58; the cow is hidden inside the beam's top).
// - hold: then 'return', timer 3.
// - return: lift falls linearly to 0 over 3 s; then cow.abducted = false, cow.state = 'graze',
//   cow.timer = 2, say(world, cow.x, cow.y - 14, line('ufo'), 'speech', 4),
//   stats.abductions += 1, logEvent "<NAME> WAS ABDUCTED. SHE SEEMS FINE." (name upper case),
//   state 'leave'.
// - leave: fly up and right at 80 units/s; when x > WIDTH + 30, state 'away', targetId null.
// Also, each second while not 'away', puff a 'sparkle' under the saucer at random.
export function updateUfo(world, dt, rng, line)

// summonUfo starts a visit now (state 'arrive', x -20, y 18, lastDay = world.day) and picks a
// random cow that is not abducted as targetId. Returns false (and does nothing) if a visit is
// already under way.
export function summonUfo(world, rng)

// ufoDrawables returns [] when state is 'away'. Otherwise two items:
// - the saucer: y = 0 (it is always behind everything on the ground... it is in the sky, but draw
//   order puts it first), drawn with drawSprite centred on (x, y), its row of lights alternating
//   'a' and '8' every 0.25 s of world.time;
// - while 'beam', 'hold' or 'return': the beam, y = cow.y - 1 (just behind the cow): a trapezoid of
//   1-unit rows from the saucer's bottom (6 wide) down to the cow's feet (24 wide), filled
//   '#ffec27' at globalAlpha 0.35 with a brighter '#fff1e8' edge at 0.5; restore globalAlpha to 1.
export function ufoDrawables(world)
```
Saucer sprite (24×8, use exactly; `L` marks a light, replaced by 'a' or '8' when drawn):
```js
const SAUCER = [
  '.........cccccc.........',
  '........cc7ccccc........',
  '.......cccccccccc.......',
  '...666666666666666666...',
  '.6666666666666666666666.',
  '5L55L55L55L55L55L55L55L5',
  '.5555555555555555555555.',
  '....5555555555555555....',
];
```

## Done when
- `node --test test/ufo.test.js` passes. The tests use createWorld(1), makeRng(1), a line stub, and setHour from state.js, stepping in 0.1 s. They cover: from 07:00, 100 s of updates (all daytime) never leave 'away'; at 23:00, summonUfo then 25 s of updates passes through 'beam' (a cow has abducted true and lift > 0 at some point) and ends 'away' with that cow back at lift 0, abducted false, state 'graze', stats.abductions 1 and one 'speech' bubble said; summonUfo returns false during a visit; a second visit cannot start the same night by itself (lastDay); ufoDrawables is [] when away and draws without throwing during 'beam', leaving ctx.globalAlpha at 1.
- `node -e "import('./web/js/ufo.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `summonUfo,ufoDrawables,updateUfo`.

## Out of scope
- The animals' own movement (another worker), and everything else in samples/farm.
~~~~

### 2.7 Write the gag lines

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported.
- You may import only `./pixel.js` from the module (for GLYPHS in the test, `../web/js/pixel.js`) and `../web/js/rng.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".

## Goal
`web/js/gags.js`: every line of dialogue in the game, by kind, and a picker that never repeats the last line of a kind. This is the comedy; it is what people will quote. Keep it family-friendly, dry and silly, like a sitcom about farm animals who know too much. The farmer is Gary; his wife thinks he quit smoking. The cows are Daisy, Bessie, Moolinda and Clarabelle; the chickens Henrietta, Cluck Norris, Eggatha, Nugget and Pecky; the pig Sir Oinksalot.

## Read first
- web/js/pixel.js: GLYPHS (the only characters the pixel font can draw).
- web/js/rng.js: makeRng (pick, int).

## Change
- samples/farm/web/js/gags.js: the Contract below.
- samples/farm/test/gags.test.js (the testing worker writes it).

## Contract
```js
// LINES maps each kind to an array of at least 10 distinct strings. Every string is UPPER CASE,
// at most 44 characters, and uses only characters that are keys of GLYPHS in pixel.js
// (A–Z, 0–9, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ $).
export const LINES = {
  gossip: [...],    // cows whispering about Gary, the other animals, farm scandal
  innocent: [...],  // cows caught gossiping, pretending to be normal cows
  smoke: [...],     // cows noticing Gary sneak off to smoke
  sneak: [...],     // Gary's guilty thoughts on his way behind the barn
  cough: [...],     // Gary coughing mid-cigarette
  excuse: [...],    // Gary coming back from behind the barn
  farmer: [...],    // Gary feeding the chickens / talking to animals
  harvest: [...],   // Gary harvesting
  morning: [...],   // Gary waking up
  night: [...],     // Gary going to bed
  chicken: [...],   // a panicking chicken
  pig: [...],       // Sir Oinksalot's grand thoughts in the mud
  ufo: [...],       // a cow just returned by aliens
  click: [...],     // an animal the player clicked on, denying everything
};

// KINDS is Object.keys(LINES).
export const KINDS

// pickLine returns a random line of that kind using rng.pick or rng.int, never the same as the
// previous line returned for that kind (remember the last per kind in a Map inside the module).
// An unknown kind returns '...'.
export function pickLine(kind, rng)
```
Starting lines to keep, and to set the tone (write the rest in the same voice; aim for 12–16 per kind):
- gossip: "DID YOU SEE HIS HAT? HE THINKS IT'S A LOOK.", "HE TALKS TO THE CARROTS. EVERY. DAY.", "I HEARD THE TRACTOR IS SEEING SOMEONE.", "HIS WIFE THINKS HE QUIT. HE DID NOT QUIT.", "THE CHICKENS ARE UNIONIZING.", "PSST. THE SCARECROW BLINKED AT ME."
- innocent: "MOO.", "...MOO?", "~WHISTLES~", "JUST EATING GRASS. NORMAL COW THINGS.", "HI GARY! LOVE THE HAT!"
- smoke: "THERE HE GOES. 'CHECKING THE BARN.'", "I CAN SMELL THAT FROM HERE, GARY.", "SHE'S GOING TO FIND OUT, GARY."
- sneak: "COAST IS CLEAR...", "JUST GONNA... CHECK THE BARN.", "SHE'S AT BINGO. PERFECT."
- cough: "*COUGH*", "*HACK* ...WORTH IT.", "*WHEEZE*"
- excuse: "*MINTY FRESH*", "BARN'S FINE. CHECKED IT.", "WHAT SMOKE? THAT'S FOG."
- farmer: "WHO'S A GOOD CHICKEN?", "EAT UP, LADIES!"
- harvest: "CHA-CHING!", "PUMPKIN MONEY!"
- morning: "ANOTHER DAY IN PARADISE.", "COCK-A-DOODLE... WHERE'S MY COFFEE?"
- night: "NIGHT, GIRLS. NO FUNNY BUSINESS."
- chicken: "THE SKY IS FALLING!", "BAWK?! BAWK!!", "WHO MOVED MY WORM?"
- pig: "I AM A BEAUTIFUL MUD PRINCE.", "MUD: THE SPA OF KINGS."
- ufo: "I'VE SEEN THINGS.", "THEY ONLY PROBED THE GRASS. ONLY THE GRASS.", "WE DON'T TALK ABOUT TUESDAY."
- click: "I WASN'T SAYING ANYTHING!", "WHO TOLD YOU THAT?"

## Done when
- `node --test test/gags.test.js` passes. The tests cover: KINDS is exactly the 14 kinds above; every kind has at least 10 distinct lines; every line is upper case, at most 44 characters, and every character is a key of GLYPHS (import it from ../web/js/pixel.js); pickLine with makeRng(1) never returns the same line twice in a row over 200 calls per kind; pickLine('nope', rng) is '...'.
- `node -e "import('./web/js/gags.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `KINDS,LINES,pickLine`.

## Out of scope
- Everything else in samples/farm.
~~~~

### 2.8 Draw the HUD, the speech bubbles and the particle effects

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. Node 22 is installed.
- JavaScript rules: ES modules, no build step, no npm packages (never run `npm install`), no TypeScript. A module must not touch `window` or `document` when it is imported. The code_map and symbol tools cover Go only; read JS files directly.
- You may import only `./state.js`, `./rng.js` and `./pixel.js` from the module, and `./fakectx.js` from the test. Eight other workers are writing the other modules in `web/js/` and their tests in `test/` at the same time; touch only the files under "Change".
- The look is 8-bit: fillRect at whole units, the pixel font from pixel.js for all text, PALETTE colours.

## Goal
`web/js/hud.js`: ages and draws the bubbles and particle effects that every other module adds with `say` and `puff` (state.js), and draws the heads-up display: the title, day and clock, coins, the running statistics, and a ticker with the latest event.

## Read first
- web/js/state.js: WIDTH, HEIGHT, the `bubbles`, `effects`, `log`, `stats`, `coins`, `day`, `paused`, `speed` fields, hourOf, say, puff, logEvent.
- web/js/pixel.js: PALETTE, drawSprite, drawText, textWidth, drawBubble.
- test/fakectx.js: fakeCtx and ctx.count.

## Change
- samples/farm/web/js/hud.js: the Contract below.
- samples/farm/test/hud.test.js (the testing worker writes it).

## Contract
A bubble is `{x, y, text, kind, ttl}`; an effect is `{kind, x, y, vx, vy, ttl, age}`.
```js
// updateOverlay: every bubble's ttl -= dt and bubbles with ttl <= 0 are removed; every effect
// moves (x += vx*dt, y += vy*dt), age += dt, ttl -= dt, and is removed at ttl <= 0.
// 'smoke' effects also grow and drift: vx += 2*dt.
export function updateOverlay(world, dt)

// formatClock returns hourOf(world) as 'HH:MM' with minutes rounded down to a multiple of 5,
// e.g. '06:00', '13:25', '00:05'.
export function formatClock(world)

// drawEffects draws each effect by kind, fading out (globalAlpha = min(1, ttl / 0.5)) and
// restoring globalAlpha to 1 afterwards:
//   smoke: a '6' puff of 2×2 growing to 5×5 with age;   zzz: drawText 'Z' in '#fff1e8' (bigger
//   later: 'Z' then 'ZZ' with age);   note: a tiny 3×5 '#ffec27' music note sprite;
//   coin: a 3×3 '#ffec27' coin with a '#ffa300' edge, spinning (1 wide every other 0.1 s);
//   drop: a 1×2 '#29adff' drop;   feather: a 2×1 '#fff1e8' feather;
//   sparkle: a 3×3 '+' of '#fff1e8' that blinks;   heart: a 5×4 '#ff004d' heart;
//   mud: a 2×2 '#ab5236' blob.   An unknown kind is skipped.
export function drawEffects(ctx, world)

// drawBubbles draws each bubble with drawBubble(ctx, b.x, b.y, b.text, b.kind), oldest first.
export function drawBubbles(ctx, world)

// drawHud draws, on top of everything:
// - a top bar y 0..8, full width, '#000000' at globalAlpha 0.55: at x 2 'GOSSIP ACRES' in
//   '#ffec27'; then 'DAY <n>  <formatClock>' in '#fff1e8'; at the right edge '$<coins>' in
//   '#ffec27' with a small coin to its left.
// - a bottom bar y 171..179, same background: at x 2 the newest log entry's text (world.log[0])
//   in '#fff1e8', cut to fit; at the right edge 'CIGS <cigarettes> GOSSIP <gossip> UFO <abductions>'
//   in '#c2c3c7'.
// - when world.paused, 'PAUSED' centred in the middle of the canvas in '#ffec27' on a '#000000'
//   box; when world.speed > 1 and not paused, '>> <speed>X' under the top bar on the right.
// Restore globalAlpha to 1.
export function drawHud(ctx, world)
```

## Done when
- `node --test test/hud.test.js` passes. The tests use createWorld(1) and setHour from state.js. They cover: updateOverlay removes a bubble after its ttl and moves and removes effects; formatClock at 06:00, 13:27 (→ '13:25') and 00:07 (→ '00:05'); drawEffects for every kind in the list and an unknown kind does not throw and leaves globalAlpha 1; drawBubbles calls fillRect once there is a bubble; drawHud draws with an empty log, with a long log entry, paused and at speed 4, and leaves globalAlpha 1.
- `node -e "import('./web/js/hud.js').then(m => console.log(Object.keys(m).sort().join(',')))"` prints `drawBubbles,drawEffects,drawHud,formatClock,updateOverlay`.

## Out of scope
- Everything else in samples/farm.
~~~~

### 2.9 Serve the page from a small Go server

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- `samples/farm` is its own Go module (`module farm`, `go 1.27.2`, standard library only; do not add dependencies or run `go mod tidy`). The root CLAUDE.md and ARCHITECTURE.md describe another project; do not edit anything outside `samples/farm/`.
- Eight other workers are writing JavaScript under `web/js/` and `test/` at the same time. Do not touch those, `web/index.html`, the Makefile or package.json.

## Goal
`cmd/farm` serves the game: the static files under `web/` (index.html and js/) are embedded in the binary by package `farm/web`, so the binary runs from anywhere.

## Read first
- samples/chess/internal/web/web.go: how a sample embeds and serves files.
- samples/chess/cmd/chess/main.go: the shape of a sample's main, its graceful shutdown on SIGINT/SIGTERM.
- samples/farm/web/index.html: the page that is served (it already exists).

## Change
- samples/farm/web/web.go: package web, with `//go:embed index.html js` into `var files embed.FS` and the Contract below.
- samples/farm/cmd/farm/main.go: package main.
- samples/farm/web/web_test.go (the testing worker writes it).

## Contract
```go
// Package web serves the Gossip Acres page and its scripts.
package web

// Handler serves the embedded files: GET / returns index.html; GET /js/<name>.js returns that
// script with Content-Type "text/javascript; charset=utf-8"; anything else that does not exist is
// 404. Every response has "Cache-Control: no-cache".
func Handler() http.Handler
```
Use `http.FileServerFS(files)` wrapped to set the Cache-Control header; set the .js content type explicitly rather than relying on the system's MIME table.

cmd/farm/main.go: the listen address is the `-addr` flag; its default is the environment variable `FARM_ADDR` if set, else `127.0.0.1:8088`. It logs `farm: serving http://<addr>` and serves web.Handler() until SIGINT or SIGTERM, then shuts down with a 5 s timeout. Keep main thin: no other logic.

## Done when
- `gofmt -l .` prints nothing and `go vet ./...` exits 0.
- `go build -o /dev/null ./cmd/farm` succeeds.
- `go test ./web/` passes. The tests use httptest against Handler(): GET / is 200, text/html, and contains `<canvas id="farm"`; GET /js/state.js is 200 with Content-Type "text/javascript; charset=utf-8"; GET /js/nope.js is 404; every response has Cache-Control no-cache.

## Out of scope
- Everything under web/js/, web/index.html, test/, the Makefile and package.json.
~~~~

---

## Epic 3: Wiring

Done when: the game runs in a browser from `make run`; `make test` and `make smoke` pass; the docs and the repository's `start` script include the sample.

### 3.1 Run the game: main loop, controls and the page

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/farm/` and run every command from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`. Node 22 is installed, and Playwright with Chromium is installed globally: `require('playwright')` works from a .cjs file and `chromium.launch()` needs no options. Never run `npm install` or `playwright install`.
- JavaScript rules: ES modules, no build step, no npm packages, no TypeScript. The code_map and symbol tools cover Go only; read JS files directly.
- Another worker is writing README.md and ARCHITECTURE.md in samples/farm and editing the repository's root `start` script at the same time. Do not touch those, or any module in web/js other than main.js.

## Goal
The game runs in the browser: `web/js/main.js` creates the world and runs every module in a fixed-step loop, draws the frame in the right order, scales the 320×180 canvas up crisply, and handles the controls; `web/index.html` is the real page; `test/smoke.cjs` proves it runs in a real browser without errors.

## Read first
The modules main.js wires together, by their exports (read the top comment of each function you call):
- web/js/state.js: createWorld, advanceTime, setHour, isNight, say, WIDTH, HEIGHT.
- web/js/rng.js: makeRng.
- web/js/sky.js: drawSky, drawNightShade.
- web/js/buildings.js: drawGround, sceneDrawables.
- web/js/crops.js: updateCrops, drawField, cropDrawables.
- web/js/farmer.js: updateFarmer, farmerDrawables, startTask.
- web/js/animals.js: updateAnimals, animalDrawables, animalAt.
- web/js/ufo.js: updateUfo, summonUfo, ufoDrawables.
- web/js/gags.js: pickLine.
- web/js/hud.js: updateOverlay, drawEffects, drawBubbles, drawHud.

## Change
- samples/farm/web/js/main.js:
  - Read URL parameters: `seed` (integer; default `Date.now() % 100000`), `hour` (0–23; if given, `setHour(world, hour)` at start), `speed` (1, 2 or 4; default 1).
  - `const rng = makeRng(seed)`, `const world = createWorld(seed)`, `const line = (kind) => pickLine(kind, rng)`.
  - Fixed step of 1/30 s: each animation frame add `min(realDt, 0.1) * world.speed` to an accumulator (nothing when paused) and run steps while it holds at least 1/30. One step, in this order: `advanceTime(world, dt)`, `updateFarmer(world, dt, rng, line)`, `updateAnimals(world, dt, rng, line)`, `updateCrops(world, dt)`, `updateUfo(world, dt, rng, line)`, `updateOverlay(world, dt)`.
  - Draw once per animation frame, in this order: `drawSky`, `drawGround`, `drawField`; then the drawables from `sceneDrawables`, `cropDrawables`, `farmerDrawables`, `animalDrawables`, `ufoDrawables` concatenated, sorted by `y` ascending (stable), each `draw(ctx)`; then `drawEffects`, `drawNightShade`, `drawBubbles`, `drawHud`. Before drawing set `ctx.imageSmoothingEnabled = false`.
  - Scaling: the canvas element keeps width 320 and height 180; set its CSS width/height to 320×k and 180×k, where k is the largest integer that fits the available space (window width minus 32, window height minus 160), at least 1. Recompute on resize.
  - Mouse: on click, convert to canvas units (divide by k); if `animalAt` finds an animal, `say` above it `<NAME>: <line('click')>` (name upper case) as 'speech'; else if the click is within 10 units of the farmer's feet and he is not hidden, say a 'thought' `line('farmer')`.
  - Buttons (ids in index.html): `pause` toggles world.paused and its label (Pause / Play); `speed-1`, `speed-2`, `speed-4` set world.speed and mark the active one with class `on`; `skip` calls `setHour(world, isNight(world) ? 6 : 21)`.
  - Keys: Space = pause, 1/2/4 = speed, N = skip, U = summonUfo(world, rng) when isNight(world) (an easter egg), F = startTask(world, 'smoke', rng, line) (another easter egg: "make Gary take a smoke break").
  - Expose `window.farm = { world }` for debugging and for the smoke test.
- samples/farm/web/index.html: replace the placeholder. Keep `<canvas id="farm" width="320" height="180">`. A dark page (`#1d2b53` to `#0b0f2a` background), centred column: a big title 'GOSSIP ACRES' in a chunky monospace style with a '#ffec27' text-shadow, a subtitle "A farm where everyone is talking. Except when Gary's looking.", the canvas with `image-rendering: pixelated` and a 4-px '#000' border with a drop shadow, a row of chunky retro buttons (Pause, 1x, 2x, 4x, Skip to night/morning) with ids pause, speed-1, speed-2, speed-4, skip, and a small hint line: "Click an animal to hear what it's really thinking. Keys: Space, 1/2/4, N. Psst: try U after dark, or F." Load `<script type="module" src="js/main.js"></script>`. All CSS inline in a <style>; no external fonts or scripts. Responsive down to a 360-px-wide phone (buttons wrap).
- samples/farm/test/smoke.cjs (the testing worker writes it).

## Contract
The update and draw order above, and `window.farm.world` being the live world object.

## Done when
- `node --check web/js/main.js` succeeds and `node -e "import('./web/js/state.js')"` still succeeds.
- `make test` passes.
- `make smoke` passes: test/smoke.cjs starts `go run ./cmd/farm -addr 127.0.0.1:8098` (waits up to 60 s for it to answer), opens `http://127.0.0.1:8098/?seed=42&hour=10` in Chromium, waits 3 s, and checks: no page errors and no console errors; `window.farm.world.time` has advanced; the canvas is not one flat colour (sample pixels with getImageData). It repeats at `?seed=42&hour=23` and, there, presses U and checks `window.farm.world.ufo.state` is not 'away'. It kills the server at the end, also on failure, and leaves no files behind (when the environment variable `SMOKE_SHOTS` names a directory, it saves a screenshot of each page there).

## Out of scope
- Every module in web/js except main.js; README.md, ARCHITECTURE.md and the root `start` script (another worker).
~~~~

### 3.2 Document the farm sample and add it to the start script

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/farm/` for the docs; the `start` script is at the repository root. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Another worker is writing web/js/main.js, web/index.html and test/smoke.cjs at the same time. Do not touch those or any other code.

## Goal
Someone who opens `samples/farm` knows what Gossip Acres is, how to run it and how it is built, and the repository's `./start` script runs it with the other samples.

## Read first
- samples/chess/README.md and samples/chess/ARCHITECTURE.md: the house style of a sample's docs.
- start (repository root): the `services` array and the build loop (`for b in "llmrouter:llmrouter" ...`).
- samples/farm/Makefile, and the top comment of each exported function in samples/farm/web/js/*.js (state, rng, pixel, sky, buildings, crops, farmer, animals, ufo, gags, hud).

## Change
- samples/farm/README.md: what it is (a farm where the cows gossip whenever Farmer Gary isn't looking, Gary sneaks cigarettes behind the barn, a pig with delusions of grandeur, chickens on the edge, and a UFO at night); how to run (`make run`, then open http://127.0.0.1:8088; `FARM_ADDR` or `-addr` to change it; URL parameters `seed`, `hour`, `speed`); controls (click an animal; Pause, 1x/2x/4x, Skip; keys Space, 1/2/4, N, U after dark, F); `make test`, `make smoke`; and the file layout.
- samples/farm/ARCHITECTURE.md: the shape (a static Go server; browser ES modules with no build step; 320×180 logical canvas scaled by an integer), the world object and who owns which fields (copy the list from the top of web/js/state.js's createWorld and the ownership note below), the update order (advanceTime, updateFarmer, updateAnimals, updateCrops, updateUfo, updateOverlay at a fixed 1/30 s step) and the draw order (drawSky, drawGround, drawField, y-sorted drawables from scenery, crops, farmer, animals and UFO, drawEffects, drawNightShade, drawBubbles, drawHud), how gags reach modules (the `line(kind)` argument; only main.js imports gags.js), how sprites are written (strings of PICO-8 palette characters, see pixel.js), the 3×5 pixel font, and how to add a character (a module with update/drawables, its fields in createWorld, two lines in main.js). Ownership note: advanceTime owns time/day/clock; farmer.js owns farmer, coins, stats.cigarettes, stats.harvests, and plot crop/water when Gary works; animals.js owns animals except while abducted, and stats.gossip; crops.js owns plot growth/stage and water decay; ufo.js owns ufo, an animal's lift/abducted, stats.abductions; hud.js ages bubbles and effects.
- start: add `"farm|samples/farm|$bin/farm|8088|Gossip Acres farm sim"` to `services` before the runner entry, and `"farm:farm"` to the build loop's list.

## Done when
- `bash -n start` succeeds and `./start --list` shows a `farm` line with port 8088.
- Every command in README.md that does not start a long-running server runs as written from samples/farm (`make test`).
- Every file path in README.md and ARCHITECTURE.md exists.

## Out of scope
- All code in samples/farm, and every other file in the repository.
~~~~
