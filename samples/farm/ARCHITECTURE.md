# farm architecture

These rules are binding for every change under `samples/farm/`.

## Shape

- A static Go server (`cmd/farm`, `web`) serves `index.html` and the scripts in `web/js`. It has no state and no API.
- The page is browser ES modules with no build step and no npm packages. The page runs in the browser; Node runs only the tests.
- The canvas is a 320 by 180 logical grid. It is scaled by a whole number, so each unit is an exact block of pixels. `fit` in `main.js` picks the scale for the window.
- Sprites are arrays of strings of PICO-8 palette characters, with `.` for transparent. `drawSprite` in `pixel.js` draws them, and `flip` mirrors them.
- Text uses a 3×5 pixel font in `pixel.js`: `#` is ink, each glyph advances 4 units, and `drawText` draws it.

## The world

`createWorld(seed)` in `web/js/state.js` returns the one object that holds the simulation. It is plain data; the modules read and write it directly. Its fields:

- `seed`, `time`, `day`, `clock`, `speed`, `paused`
- `coins`
- `stats`: `cigarettes`, `gossip`, `harvests`, `abductions`
- `farmer`: Gary's position, state, task, timer and animation frame
- `animals`: the cows, chickens and pig
- `plots`: the field's plots, each with `col`, `row`, `crop`, `growth`, `stage` and `water`
- `bubbles`, `effects`, `log`
- `ufo`: `state`, `x`, `y`, `targetId`, `timer` and `lastDay`

Ownership: each field has one writer. Other modules may read it.

- `advanceTime` (`state.js`) owns `time`, `day` and `clock`. `setHour`, which skips to an hour, also writes them.
- `farmer.js` owns `farmer`, `coins`, `stats.cigarettes`, `stats.harvests`, the plot's `water` when Gary waters, and the plot's `crop`, `water`, `growth` and `stage` when he harvests and replants.
- `crops.js` owns the plot's `growth` and `stage` otherwise, and the water decay of every plot.
- `animals.js` owns `animals`, except an animal's `lift` and `abducted` while the UFO has it, and `stats.gossip`.
- `ufo.js` owns `ufo`, an animal's `lift` and `abducted`, and `stats.abductions`.
- `hud.js` ages `bubbles` and `effects`.

`say` and `logEvent` in `state.js` are the only ways to add to `bubbles` and `log`; `puff` is the only way to add to `effects`.

## Update order

`main.js` calls `step(dt)` once per fixed step of 1/30 s, while the page is not paused. The order is fixed because later modules read what earlier ones wrote:

1. `advanceTime`
2. `updateFarmer`
3. `updateAnimals`
4. `updateCrops`
5. `updateUfo`
6. `updateOverlay`

`frame` runs once per animation frame and turns the elapsed real time, times the speed, into whole steps.

## Draw order

`main.js` calls `draw()` once per frame, in this order:

1. `drawSky`
2. `drawGround`
3. `drawField`
4. the sorted drawables: `sceneDrawables`, `cropDrawables`, `farmerDrawables`, `animalDrawables`, `ufoDrawables`. Each returns `{y, draw(ctx)}`, where `y` is the base line, and `main.js` sorts them so that a lower sprite draws in front.
5. `drawEffects`
6. `drawNightShade`
7. `drawBubbles`
8. `drawHud`

## Gags

`gags.js` holds the lines, by kind. Only `main.js` imports it. `main.js` passes `line(kind)` to the modules that need a line: `updateFarmer`, `updateAnimals` and `updateUfo` take it as their last argument, and the click handler uses it too. A module never imports `gags.js` itself, so the modules stay testable with a stub line function.

## Adding a character

1. Write a module with an `update` function that takes the world and advances the character, and a `drawables` function that returns `{y, draw(ctx)}` items.
2. Add the character's fields to `createWorld` in `state.js`.
3. In `main.js`, call the update in `step` and add the drawables to the sorted list in `draw`.

## Testing

Each module has a node test in `test/`. `test/fakectx.js` records canvas calls, so drawing is tested without a browser. `test/smoke.cjs` runs the real page in Chromium. The Go tests cover the handler in `web/web_test.go`.
