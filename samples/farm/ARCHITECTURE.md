# farm architecture

These rules are binding for every change under `samples/farm/`.

In the sections below, a bare module name such as `state.js` or `main.js` is in `web/js/`, and `index.html` is in `web/`.

## Shape

- A static Go server (`cmd/farm`, `web`) serves `index.html` and the scripts in `web/js`. It has no state and no API.
- The page is browser ES modules with no build step and no npm packages. The page runs in the browser; Node runs only the tests.
- The canvas is a 320 by 180 logical grid. It is scaled by a whole number, so each unit is an exact block of pixels. `fit` in `main.js` picks the scale for the window.
- Sprites are arrays of strings of PICO-8 palette characters, with `.` for transparent. `drawSprite` in `pixel.js` draws them, and `flip` mirrors them.
- Text uses a 3×5 pixel font in `pixel.js`: `#` is ink, each glyph advances 4 units, and `drawText` draws it.

## The world

`createWorld(seed)` in `web/js/state.js` returns the one object that holds the simulation. It is plain data; the modules read and write it directly. Its fields:

- `seed`, `time`, `day`, `clock`, `speed`, `paused`
- `coins` (a float; the HUD rounds it down with `money`)
- `stats`: `cigarettes`, `gossip`, `harvests`, `abductions`, `milk`, `eggs`, `births`, `deaths`
- `upgrades`: `cigs`, `pasture`, `coop`, `portal`, the counts bought from the pig's shop
- `economy`: `milk`, `eggs`, `alien`, fractional amounts not yet paid out
- `farmer`: Gary's position, state, task, timer and animation frame
- `animals`: the cows, chickens and pig. A cow or chicken also has `age`, `life` (the age at which it dies) and `breed` (seconds to its next young); `nextAnimal` numbers new ones
- `plots`: the field's plots, each with `col`, `row`, `crop`, `growth`, `stage` and `water`
- `bubbles`, `effects`, `log`
- `ufos`: the saucers in flight, each `{state, x, y, targetId, timer, stay, rate}`; `rate` is 0 for the free night visit. `ufoLastDay` is the day of the last free visit

`state.js` also holds the rules several modules read: `isAdult`, `herdSize`, `capacity` (from `HERD` and the upgrades), `productivity` (`CIG_BOOST` per pack, `PORTAL_BOOST` once the portal is fixed) and `money`.

Ownership: each field has one writer. Other modules may read it.

- `advanceTime` (`state.js`) owns `time`, `day` and `clock`. `setHour`, which skips to an hour, also writes them.
- `farmer.js` owns `farmer`, the harvest's `coins`, `stats.cigarettes`, `stats.harvests`, the plot's `water` when Gary waters, and the plot's `crop`, `water`, `growth` and `stage` when he harvests and replants.
- `crops.js` owns the plot's `growth` and `stage` otherwise, and the water decay of every plot.
- `animals.js` owns `animals` and `nextAnimal`, except an animal's `lift` and `abducted` while the UFO has it, and `stats.gossip`, `stats.births` and `stats.deaths`. Other modules add an animal only through its `addAnimal`.
- `ufo.js` owns `ufos`, `ufoLastDay`, an animal's `lift` and `abducted`, and `stats.abductions`. Other modules start a saucer only through `summonUfo` or `hireUfo`.
- `economy.js` owns `economy`, `stats.milk`, `stats.eggs` and the passive `coins`: milk, eggs, alien pay, and the credit for time away.
- `shop.js` owns `upgrades` and the coins spent in the shop. It delivers through `addAnimal`, `hireUfo` and `startTask`.
- `hud.js` ages `bubbles` and `effects`.

`say` and `logEvent` in `state.js` are the only ways to add to `bubbles` and `log`; `puff` is the only way to add to `effects`.

## Update order

`main.js` calls `step(dt)` once per fixed step of 1/30 s, while the page is not paused. The order is fixed because later modules read what earlier ones wrote:

1. `advanceTime`
2. `updateFarmer`
3. `updateAnimals`
4. `updateCrops`
5. `updateUfo`
6. `updateEconomy`
7. `updateOverlay`

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

## The game

The farm is an idle game. Adult cows give milk and adult hens lay eggs (`MILK`, `EGG` in `economy.js`), sold at once for coins times `productivity`. Cows and chickens are born small, grow up at `ADULT_AGE`, have young while the herd is under `capacity` and another adult of their kind is around, and die when `age` reaches `life`. An abducted animal does not age. The pig is a prince from another dimension and never ages; clicking him opens his shop, and `offers` and `buy` in `shop.js` are its rules, so `main.js` only renders them. A hired saucer holds one free adult cow for `CONTRACT.stay` seconds, and `economy.js` pays its `rate` per second while it is in `hold`, untouched by `productivity`.

## Saving

Without a `seed` URL parameter, `main.js` saves the world to localStorage every 5 seconds and on `pagehide`, and loads it on the next visit through `save.js`. The world is plain data, so the save is `JSON.stringify(world)`; `loadWorld` merges it over a fresh `createWorld` so a save from before a new field still loads. On load, `creditAway` pays the milk and eggs for the time away, up to two hours. A seeded page is a sandbox and never loads or saves.

## Gags

`gags.js` holds the lines, by kind. Only `main.js` imports it. `main.js` passes `line(kind)` to the modules that need a line: `updateFarmer`, `updateAnimals` and `updateUfo` take it as their last argument, and the click handler uses it too. A module never imports `gags.js` itself, so the modules stay testable with a stub line function.

## Adding a character

1. Write a module with an `update` function that takes the world and advances the character, and a `drawables` function that returns `{y, draw(ctx)}` items.
2. Add the character's fields to `createWorld` in `state.js`.
3. In `main.js`, call the update in `step` and add the drawables to the sorted list in `draw`.

## Testing

Each module has a node test in `test/`. `test/fakectx.js` records canvas calls, so drawing is tested without a browser. `test/smoke.cjs` runs the real page in Chromium. The Go tests cover the handler in `web/web_test.go`.
