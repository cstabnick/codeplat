# Roadmap: the farm empire (Gossip Acres at scale)

## Goal

Gossip Acres grows from a farm of a few animals into an idle empire of millions, in `samples/farm/`:

- Herds are counts, not individuals, so they reach millions; the scene draws a few animals to stand for each herd.
- Animals have genders. Young are born female or male at random; the market sells females and males (cows and bulls, hens and cocks), with buy 1, buy 10 and buy MAX.
- Housing physically levels up per species: cows go from a pasture through stables, barns, a dairy ranch, the Cow Hotel and Moo York Tower to the Burj Moolifa; chickens from a coop to the Burj Al Hen; sheep and goats have their own ladders.
- The farm expands outward: buying land adds parcels to the right (the Sheep Meadow, Goat Hill) with new animals, and the camera pans across it.
- The carrot field is a clicker: click to pluck, with gardening gloves, a golden hoe, fertilizer and the combine harvester as upgrades.
- UFO leases scale: any number of cows can go to the aliens for a flat rate, with a few saucers on show.

Tracker project: `FARM`. The browser modules are plain ES modules in `samples/farm/web/js/`, tested with `node --test`.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | `species.js` and the new world fields and helpers in `state.js` exist; the field is all carrots. (Done directly, as FARM-16.) | 1 |
| 2. Components | The herd model, housing drawing, carrot clicker, alien leases, sheep and goats, new lines, save v2, HUD and buildings each pass their own tests. | 9, parallel |
| 3. Switch-over | The visible animals follow the herds, the economy pays from counts, the shop sells the new catalogue, and `main.js` runs it all with panning and clicks. | 4, parallel |
| 4. Cleanup and docs | `ufo.js` and the old per-animal fields are gone; README and ARCHITECTURE describe the game. | 2, parallel |

No issue has a dependency inside its epic.

Why this cut: every component reads the world fields and the species table, so those come first and are fixed in each issue's text. The epic 2 modules are new files (or single files with their own tests) that import only `state.js`, `species.js` and `pixel.js`, so they can be written at the same time while the old game keeps running. Epic 3 changes the four files that import them (`animals.js`, `economy.js`, `shop.js`, `main.js`), which import one another only through epic 2 modules. Epic 4 removes what nothing imports any more.

---

## Epic 2: Components

### Add the herd model: births, growing up, genders, deaths and buying, at any scale

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/herds.js` advances every herd as counts, so a farm can hold millions of animals, and sells animals, housing tiers and land. Nothing calls it yet; a later issue wires it into the game.

## Read first
- `web/js/species.js`: the species table described above.
- `web/js/state.js`: `createWorld`, `herdTotal`, `housingCapacity`, `hasKind`, `logEvent`.

## Change
- Create `web/js/herds.js`, importing only `./state.js` and `./species.js`.
- Create `test/herds.test.js`.

## Contract
```js
export function updateHerds(world, dt, rng)
export function room(world, kind)                // Math.max(0, housingCapacity(world, kind) - herdTotal(world, kind))
export function unitPrice(kind, sex)             // SPECIES[kind].price[sex]; sex is 'female' or 'male'
export function maxBuy(world, kind, sex)         // 0 when !hasKind(world, kind); else Math.min(Math.floor(world.coins / unitPrice(kind, sex)), room(world, kind))
export function buyAnimals(world, kind, sex, n)  // n is a positive integer or 'max'; returns the number bought
export function nextTier(world, kind)            // SPECIES[kind].tiers[herd.tier + 1], or null at the top
export function upgradeHousing(world, kind)      // returns true when bought
export function nextLand(world)                  // LAND[world.land], or null when all land is owned
export function buyLand(world)                   // returns true when bought
```
- `buyAnimals`: count = Math.min(n === 'max' ? Infinity : n, maxBuy(world, kind, sex)). When count > 0: `world.coins -= count * unitPrice`; `herd.females += count` (sex 'female') or `herd.males += count` ('male'); push `{kind, type: 'buy', sex, count}` onto `world.herdEvents`. Return count.
- `upgradeHousing`: false unless hasKind, nextTier exists and `world.coins >= tier.cost`. Then coins -= cost, `herd.tier += 1`, `logEvent(world, `THE ${SPECIES[kind].female.toUpperCase()}S MOVED INTO THE ${tier.name.toUpperCase()}`)`, return true.
- `buyLand`: false unless nextLand exists and is affordable. Then coins -= cost, `world.land += 1`, `logEvent(world, `BOUGHT ${land.name.toUpperCase()}`)`, return true.
- `updateHerds(world, dt, rng)`, for every kind in KINDS with hasKind true, using that species `s` and herd `h`:
  1. Births: `breeders = Math.min(h.females, h.males * s.matesPerMale)`; `h.acc.birth += breeders * s.birth * dt`; `n = Math.floor(h.acc.birth)`; `h.acc.birth -= n`; `n = Math.min(n, room(world, kind))`. If n > 0: `h.young += n`, `world.stats.births += n`, event `{kind, type: 'birth', count: n}`. Births with no room are lost, not saved up.
  2. Growing up: if `h.young === 0`, set `h.acc.grow = 0`; otherwise `h.acc.grow += h.young * dt / s.grow`, `n = Math.min(Math.floor(h.acc.grow), h.young)`, `h.acc.grow -= n`. If n > 0: `f = split(n, 0.5, rng)`; `h.young -= n`; `h.females += f`; `h.males += n - f`; event `{kind, type: 'grow', count: n}`.
  3. Deaths: `adults = h.females + h.males`; `h.acc.death += adults * dt / s.life`; `n = Math.min(Math.floor(h.acc.death), adults)`; `h.acc.death -= Math.floor(h.acc.death)`. If n > 0: `fd = split(n, h.females / adults, rng)`, clamped to `[Math.max(0, n - h.males), Math.min(n, h.females)]`; `h.females -= fd`; `h.males -= n - fd`; `world.stats.deaths += n`; event `{kind, type: 'death', count: n}`.
- `split(n, p, rng)` (not exported) is a random binomial count: for n <= 32, the number of `rng.chance(p)` successes in n tries; above that, `Math.round(n * p + z * Math.sqrt(n * p * (1 - p)))` clamped to [0, n], where z is a standard normal from two `rng.next()` calls (Box-Muller; use `1 - rng.next()` inside the log so it is never log(0)). This is how genders spawn at random without looping over millions of animals.
- After pushing events, keep at most the last 50 entries of `world.herdEvents` (drop from the front).
- Nothing in herds.js loops once per animal: updateHerds must cost the same for 10 animals and for 10 million.

## Done when
- `node --test test/herds.test.js` passes, with tests for:
  - a fresh world's cows (3 + 1 bull) have young within 300 s of 0.1 s steps (use `makeRng(1)`), and the young grow up into both females and males over time;
  - a herd with no males has no births; one with no room (set `tier` 0 and fill it to capacity 10) has none either;
  - with `h.young = 1e6`, `h.tier = 6` (cow) and one 1 s update, more than 0 grow up and the female share of those is between 0.45 and 0.55;
  - deaths: with `h.females = 1e6`, `h.males = 1e6`, `tier = 6` and 10 s, about `2e6 * 10 / 1080` die (within 5%), and `stats.deaths` counts them;
  - `maxBuy` and `buyAnimals(world, 'cow', 'male', 'max')` buy `min(floor(coins / 60), room)` bulls and spend exactly that many prices; `buyAnimals` on sheep with `world.land = 1` buys 0;
  - `upgradeHousing` refuses when poor, then moves up a tier and raises `housingCapacity`; refuses at the top tier;
  - `buyLand` raises `world.land` from 1 to 2 for 3000 coins and refuses when all land is owned;
  - `herdEvents` never holds more than 50 entries;
  - 10,000 updates of a herd of 2e6 cows finish in under 1 s.

## Out of scope
Every other file. Wiring updateHerds into `main.js`, and the scene's visible animals, are later issues.
~~~~

### Draw the housing tiers, from cow stables to the Burj Moolifa

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/housing.js` draws each species' housing tier as a building at its site, so that housing physically levels up: from a shed, through barns, hotels and towers, to the Burj Moolifa (a needle skyscraper with cow spots) and the Burj Al Hen (a sail-shaped hotel with a rooster's comb). Nothing calls it yet; a later issue adds its drawables to the scene.

## Read first
- `web/js/species.js`: each tier's `look`.
- `web/js/pixel.js`: `PALETTE` (PICO-8 colours keyed '0'..'9', 'a'..'f'), `drawSprite`, `drawText`.
- `web/js/buildings.js`: `drawBarn` and the `rect` helper, for the drawing style (flat palette colours, one fillRect per run, no anti-aliasing).

## Change
- Create `web/js/housing.js`, importing only `./state.js`, `./species.js` and `./pixel.js`.
- Create `test/housing.test.js`.

## Contract
```js
export function drawBuilding(ctx, look, x, baseY, time)   // draws one building; left edge x, bottom row baseY - 1
export function housingDrawables(world)                    // one {y, draw(ctx)} per owned kind whose tier look.shape is not 'none'
```
- `housingDrawables`: for each kind in KINDS with `hasKind(world, kind)`: `look = SPECIES[kind].tiers[world.herds[kind].tier].look`; skip shape 'none'; item `{y: SITES[kind].y, draw: (ctx) => drawBuilding(ctx, look, SITES[kind].x, SITES[kind].y, world.time)}`.
- `drawBuilding` paints only inside the box x..x+look.w-1, baseY-look.h..baseY-1, and never above row 0, using `ctx.fillStyle = PALETTE[key]` and integer `fillRect` calls. Shapes:
  - 'shed': a body in `wall` for the lower two thirds and a sloped roof in `roof` above (each roof row 1 unit narrower on the right), a door in '0' (8 wide, 12 tall, centred at the bottom) and one '1' window.
  - 'barn': a body in `wall` with 1-unit '7' trim on both sides, a gambrel roof in `roof` over the top third (narrowing 2 units per row near its top), and a '7' double door with an X brace at the bottom centre.
  - 'block' (hotels and factories): a body in `wall`, a 3-row `roof` stripe on top, a grid of 2x2 '1' windows every 4 units (lit 'a' instead when `Math.floor(time) % 7 === col % 7` for a little life), and a 6x8 '0' entrance at the bottom centre.
  - 'tower': a base block the full width for the lower 60% with '1' windows, a middle block half as wide for the next 30%, and a spire of width 2 in `roof` for the rest.
  - 'burj' (the Burj Moolifa): a stepped needle of 5 segments of equal height whose widths are w, 0.8w, 0.6w, 0.4w and 0.2w (rounded, centred), then a 2-wide spire to the top. The body is `wall` with '0' cow spots: blotches of 3x2 units placed by a deterministic hash of their row and column (no Math.random), and a pink 'e' light at the very top shown while `Math.floor(time * 2) % 2 === 0`.
  - 'sail' (the Burj Al Hen): a vertical 2-wide mast in '6' on the right edge, and to its left a sail in `wall` whose width at row r from the bottom is `Math.round(look.w * Math.sin(Math.PI * (r + 1) / (look.h + 1)) * 0.9)` with '6' rib lines every 6 rows, and a 3-bump rooster comb in `roof` on top of the mast.
  - 'mountain': a triangle in `wall` (width at the bottom look.w, a point at the top), the top 20% in `roof` like snow, and a '0' cave mouth (a half disc of radius look.w / 8) at the bottom centre.
  - 'none': draws nothing.
- No globals, no Math.random, and the same picture for the same arguments.

## Done when
- `node --test test/housing.test.js` passes, with tests for:
  - for every tier of every species (loop over SPECIES), `drawBuilding` with `fakeCtx()` at x 100, baseY 90 draws at least one fillRect (except 'none', which draws none), and every fillRect lies inside the box above and at y >= 0;
  - the Burj Moolifa (`SPECIES.cow.tiers.at(-1).look`) paints some '0' (#000000) and some '7' (#fff1e8) pixels, and its topmost pixel is at y >= 0 when baseY is 84;
  - `housingDrawables(createWorld(1))` is empty (both tier 0 looks are 'none'); with `world.herds.cow.tier = 2` it has one item with y 84; with `world.land = 2` (sheep tier 0 is a shed) it includes an item with y 92;
  - drawing the same building twice records identical calls.

## Out of scope
Every other file. `buildings.js` hiding the old barn and coop, and adding these drawables to the scene, are other issues.
~~~~

### Make the carrot field a clicker: plucking, click power and the combine harvester

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
The farm's carrots become a clicker game: clicking the field plucks a carrot for coins, upgrades raise what a click earns and how fast carrots grow, and combine harvesters pluck on their own. `web/js/carrots.js` holds the rules and draws the combine; `web/js/crops.js` grows faster with fertilizer. Nothing calls carrots.js yet; a later issue wires clicks to `pluck`.

## Read first
- `web/js/crops.js`: `updateCrops` (growth and stage), `cropDrawables`, `TILE`.
- `web/js/state.js`: `FIELD`, `CROPS`, `CARROT_VALUE`, `RIPE_BONUS`, `productivity`, `puff`, `money`.
- `test/crops.test.js`: the existing tests, which must keep passing.

## Change
- Create `web/js/carrots.js`, importing only `./state.js` and `./pixel.js`.
- `web/js/crops.js`: in `updateCrops`, a watered plot grows by `dt * 1.5 ** (world.carrots?.fertilizer ?? 0) / CROPS[plot.crop].grow` instead of `dt / CROPS[plot.crop].grow`. Change nothing else there.
- Create `test/carrots.test.js`; add one test to `test/crops.test.js` for the fertilizer speed-up.

## Contract
```js
export const CARROT_UPGRADES = [
  { id: 'gloves', name: 'Gardening gloves', blurb: 'Each pair adds 1 carrot to every click.', base: 15, growth: 1.5, max: Infinity },
  { id: 'hoe', name: 'Golden hoe', blurb: 'Doubles what every click plucks.', base: 250, growth: 3, max: 30 },
  { id: 'fertilizer', name: 'Fertilizer', blurb: 'Carrots grow 1.5x faster, so more of them are ripe.', base: 60, growth: 2, max: 8 },
  { id: 'combines', name: 'Combine harvester', blurb: 'Plucks 2 carrots a second on its own, times click power and productivity.', base: 400, growth: 1.35, max: Infinity },
];
export function clickPower(world)            // (1 + world.carrots.gloves) * 2 ** world.carrots.hoe
export function combineRate(world)           // coins per second from combines: world.carrots.combines * 2 * CARROT_VALUE * clickPower(world) * productivity(world)
export function upgradeCost(world, id)       // Math.round(base * growth ** world.carrots[id])
export function carrotUpgrades(world)        // [{id, name, blurb, level, cost, ok, reason}], one per CARROT_UPGRADES entry, in that order
export function buyCarrotUpgrade(world, id, n = 1)  // n is a positive integer or 'max'; returns the number bought
export function plotAt(x, y)                 // index into world.plots of the plot whose tile contains world point (x, y), or -1
export function pluck(world, index)          // returns the coins earned
export function updateCarrots(world, dt)
export function carrotDrawables(world)       // [] with no combines; else one {y, draw(ctx)} for the combine
```
- `carrotUpgrades`: `level = world.carrots[id]`; `reason` is 'MAXED' when level >= max, else 'NOT ENOUGH COINS' when coins < cost, else ''; `ok = reason === ''`.
- `buyCarrotUpgrade`: repeat up to n times (at most 1000 for 'max') while that upgrade is ok: coins -= cost, level += 1. Return the count.
- `plotAt`: plot (col, row) covers x in [FIELD.x + col * 16, +16) and y in [FIELD.y + row * 16, +16); plots are row-major, index = row * FIELD.cols + col.
- `pluck(world, index)`: when index is -1, use the plot with the largest growth. earned = `CARROT_VALUE * clickPower(world) * (plot.stage === 3 ? RIPE_BONUS : 1)`. Then reset the plot: growth 0, stage 0, `water = Math.max(plot.water, 0.5)`; `world.coins += earned`; `world.stats.plucks += 1`; `puff(world, 'carrot', tileX + 8, tileY + 4, 0, -14, 0.8)`. Return earned.
- `updateCarrots(world, dt)`: `c.acc += c.combines * 2 * dt`; n = Math.floor(c.acc); c.acc -= n; when n > 0: `world.coins += n * CARROT_VALUE * clickPower(world) * productivity(world)`, and reset the one plot with the largest growth (growth 0, stage 0) with one 'carrot' puff, no matter how large n is. Move `c.combineX` by `30 * dt * c.dir` (treat a missing `c.dir` as 1; this module owns `world.carrots`, so it may add `dir`) and bounce between FIELD.x and FIELD.x + FIELD.cols * 16 - 16.
- `carrotDrawables`: a combine sprite about 16x10 (green '3' body, yellow 'a' cab window, dark '5' wheels, a '9' reel at the front) drawn with drawSprite at (combineX, FIELD.y + FIELD.rows * 16 - 10), flipped when dir is -1, with y = FIELD.y + FIELD.rows * 16 + 1; when combines > 1, `drawText(ctx, `X${money(combines)}`, combineX, FIELD.y + FIELD.rows * 16 - 16, '#ffec27')`.
- carrots.js owns `world.carrots`, `stats.plucks`, a plucked plot's growth, stage and water, and the coins from plucking and combines.

## Done when
- `node --test test/carrots.test.js test/crops.test.js` passes, with carrots tests for:
  - `clickPower` is 1 on a fresh world, 3 with 2 gloves, 12 with 2 gloves and 2 hoes;
  - `pluck` on a ripe plot earns `RIPE_BONUS * clickPower`, on an unripe one `clickPower`, resets the plot to stage 0, counts `stats.plucks` and adds a 'carrot' effect; `pluck(world, -1)` picks the plot with the largest growth;
  - `plotAt` finds plot 0 at (FIELD.x + 1, FIELD.y + 1), plot 7 at (FIELD.x + 16 + 1, FIELD.y + 16 + 1), and -1 outside the field;
  - `buyCarrotUpgrade(world, 'gloves', 'max')` with 100 coins buys while affordable, each costing `Math.round(15 * 1.5 ** level)`, and leaves the remainder; 'hoe' stops at 30 ('MAXED');
  - with 3 combines, 10 s of `updateCarrots` in 0.1 s steps earns about `combineRate * 10` (within one pluck's worth) and the combine stays within the field;
  - `carrotDrawables` is empty with no combines and draws without throwing with some.

## Out of scope
Every other file, including `farmer.js` and `main.js`. The shop and the click handler are later issues.
~~~~

### Lease cows to the aliens by the thousand, with a few saucers on show

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/aliens.js` lets the farm lease any number of cows to UFOs: each leased cow is gone for 45 s and pays a flat 0.6 coins a second. At most six saucers hover over the pasture to show it. It also keeps the nightly gag where one saucer lifts a cow and puts it back. It replaces `web/js/ufo.js` later; nothing calls it yet.

## Read first
- `web/js/ufo.js`: the saucer sprite, speeds, beam drawing and the arrive/beam/hold/return/leave state machine for lifting one visible cow (`abducted`, `lift`); copy what you need.
- `test/ufo.test.js`: how those are tested.
- `web/js/state.js`: `PASTURE`, `isNight`, `say`, `logEvent`, `puff`, `money`, `WIDTH`.

## Change
- Create `web/js/aliens.js`, importing only `./state.js` and `./pixel.js`.
- Create `test/aliens.test.js`.

## Contract
```js
export const LEASE = { stay: 45, rate: 0.6, price: 5 };
export const MAX_SAUCERS = 6;
export function leasable(world)              // world.herds.cow.females
export function maxLease(world)              // Math.min(leasable(world), Math.floor(world.coins / LEASE.price))
export function leaseCows(world, n)          // n is a positive integer or 'max'; returns the number leased
export function alienRate(world)             // world.herds.cow.leased * LEASE.rate, coins per second; productivity does not apply
export function summonVisit(world, rng)      // starts the night visit now; returns false if one is under way or no visible cow is free
export function updateAliens(world, dt, rng, line)
export function alienDrawables(world)
```
- `leaseCows`: count = Math.min(n === 'max' ? Infinity : n, maxLease(world)). When count > 0: coins -= count * LEASE.price; `herds.cow.females -= count`; `herds.cow.leased += count`; push `{count, timer: LEASE.stay}` onto `world.aliens.batches`; `logEvent(world, `${money(count)} COWS WENT ON AN ALIEN INTERNSHIP`)`. Return count.
- `updateAliens`:
  1. Pay: `world.coins += alienRate(world) * dt`.
  2. Batches: each timer -= dt; a batch at <= 0 is removed, its count goes back (`leased -= count`, `females += count`), `world.stats.abductions += count`, `logEvent(world, `${money(count)} COWS ARE BACK FROM SPACE`)`, and `say(..., line('ufo'), 'speech', 4)` above one visible cow (an entry of `world.animals` with kind 'cow' and not abducted), if there is one.
  3. Show saucers: `world.aliens.saucers` holds `{slot, x, y, state}` with state 'arrive', 'hover' or 'leave'. Wanted = Math.min(MAX_SAUCERS, world.aliens.batches.length). Add saucers for missing slots 0..wanted-1, arriving from x -20, y 18; slot i hovers at x = PASTURE.x + 16 + i * 20, y = 28 + 2 * Math.sin(world.time * 2 + i). Saucers whose slot >= wanted leave to the right at 80 units/s and are removed past landWidth(world) + 30. Speeds as in ufo.js.
  4. The night visit: if `isNight(world)`, `world.aliens.lastDay !== world.day`, no visit is under way, and `rng.chance(0.03 * dt)`, call summonVisit. The visit is `world.aliens.visit` (null when none; this module owns it): `{state, x, y, targetId, timer}`. It runs the ufo.js state machine on a visible cow: it sets the cow's `abducted` and `lift`, holds for 1.5 s, puts the cow back (abducted false, lift 0, state 'graze', timer 2), says `line('ufo')`, adds 1 to `stats.abductions`, logs `${cow.name.toUpperCase()} WAS ABDUCTED. SHE SEEMS FINE.`, then leaves and sets visit to null. summonVisit sets `world.aliens.lastDay = world.day`. If its cow disappears, the visit ends.
- `alienDrawables`: for each saucer and the visit, `{y: Infinity, draw}` for the saucer; each hovering saucer also gets a beam `{y: PASTURE.y + PASTURE.h - 1, draw}` from the saucer down to the pasture's middle (y PASTURE.y + 30) at alpha 0.35, like ufo.js; the visit gets its beam to its cow in beam/hold/return, as in ufo.js. globalAlpha is back to 1 afterwards.
- aliens.js owns `world.aliens`, `herds.cow.leased`, the cows it moves between `herds.cow.females` and `leased`, the alien pay, `stats.abductions`, and a visible cow's `abducted` and `lift` during a visit.

## Done when
- `node --test test/aliens.test.js` passes, with tests for:
  - `leaseCows(world, 'max')` on a fresh world (3 cows, 10 coins) leases 2, spends 10, and moves 2 from females to leased; leasing with no cows or no coins leases 0;
  - `alienRate` is 0.6 per leased cow and does not change with `world.upgrades.cigs`; 10 s of updates with 1000 leased cows (set them up directly) pay about 6000 coins;
  - a batch comes back after 45 s: females restored, leased 0, `stats.abductions` counts the cows, and a log entry says they are back;
  - with 10 batches at most 6 saucers exist, and they leave and are removed once the batches end;
  - the night visit lifts a visible cow and puts it back, as the old ufo test did, and does not start by day;
  - `alienDrawables` draws without throwing and leaves globalAlpha at 1.

## Out of scope
Every other file. Do not edit or delete `ufo.js` or `test/ufo.test.js`; a later issue removes them.
~~~~

### Bring sheep and goats onto the new land, with rams and billy goats

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/flock.js` draws a few sheep in the Sheep Meadow and goats on Goat Hill to stand for their herds, males with horns and young ones small, and lets them graze and wander. Nothing calls it yet; a later issue adds it to the game loop.

## Read first
- `web/js/animals.js`: the COW, CHICKEN and PIG sprites, `stepToward`, `randomNear` and the graze/walk loop in `updateCow`, for the style to copy.
- `web/js/pixel.js`: `drawSprite(ctx, rows, x, y, flip)`, `spriteSize`, `PALETTE`.
- `web/js/state.js`: `MEADOW`, `HILL`, `herdTotal`, `hasKind`.

## Change
- Create `web/js/flock.js`, importing only `./state.js` and `./pixel.js`.
- Create `test/flock.test.js`.

## Contract
```js
export const FLOCK_MAX = 10;                 // most entries shown per kind
export function updateFlock(world, dt, rng)
export function flockDrawables(world)         // one {y, draw(ctx)} per entry of world.flock
```
- An entry of `world.flock`: `{kind, sex, young, x, y, facing, state, target, timer, frame}`, where kind is 'sheep' or 'goat', sex 'female' or 'male', young a boolean, state 'graze' or 'walk'.
- `updateFlock`, for kind 'sheep' (area MEADOW) and 'goat' (area HILL):
  - When `hasKind` is false, remove that kind's entries. Otherwise target = Math.min(FLOCK_MAX, herdTotal(world, kind)); split it into youngN = Math.round(target * h.young / total), maleN = Math.round((target - youngN) * h.males / Math.max(1, h.females + h.males)), and the rest female adults. Make the entries match these three counts: remove surplus entries of a group (the last ones), and add missing ones at a random point in the area (`rng.range`), facing left or right at random, state 'graze', timer `rng.range(2, 5)`.
  - Each entry: in 'graze', timer -= dt and at 0 pick a random target in its area and 'walk'; in 'walk', move toward the target at 6 units/s (sheep) or 10 (goats), frame = Math.floor(world.time * 6) % 2, face the direction of movement, and at the target go back to 'graze'. Keep every entry inside its area.
- Sprites, as arrays of PALETTE strings with '.' for transparent, facing right: SHEEP (about 14x10: a fluffy '7' body with '6' shading, '0' face and legs), RAM (SHEEP plus curled '9' horns), LAMB (about 9x7), GOAT (about 13x11: a '6' body, a '5' beard, short '5' horns), BILLY (GOAT with long '5' horns and a longer beard), KID (about 8x7). A walking adult changes its leg row on frame 1. The sprite's feet are at (x, y): draw at `x - floor(w / 2)`, `y - h`, flipped when facing is 'left'.
- flock.js owns `world.flock`.

## Done when
- `node --test test/flock.test.js` passes, with tests for:
  - a fresh world (land 1) has no flock after an update;
  - with `world.land = 3`, `herds.sheep = {females: 20, males: 5, young: 5, ...}` and goats likewise, one update makes 10 sheep and 10 goats with group counts as specified, and lowering the herd removes entries;
  - over 300 s of 0.1 s updates every entry stays inside its area;
  - every drawable draws at least one fillRect without throwing, for each of the six sprites.

## Out of scope
Every other file. The cows, chickens and pig are `animals.js`; do not touch them.
~~~~

### Write the lines for bulls, cocks, sheep, goats, carrots, housing and land

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/gags.js` gets the lines the new characters and events need, so later issues can call `line(kind)` for them.

## Read first
- `web/js/gags.js`: `LINES`, the rules in its top comment, and the existing voice (gossiping cows, Farmer Gary who sneaks cigarettes, Sir Oinksalot the pig who is an interdimensional prince trapped in our world and runs the farm's shop).
- `test/gags.test.js`: `WANT_KINDS` and the count assertion (17 kinds now).

## Change
- `web/js/gags.js`: add seven kinds after `death`, each with 12 to 14 distinct lines:
  - `bull`: a bull, proud and dim, who thinks he runs the herd ('I AM THE BULL. THE FENCE FEARS ME.');
  - `rooster`: the cock of the yard, loud and vain, crowing at the wrong times;
  - `sheep`: sheep, anxious followers who all agree with each other;
  - `goat`: goats, chaotic, eat everything, climb things;
  - `pluck`: what is said when a carrot is plucked ('POP!', 'CARROT!', 'ANOTHER ONE FOR THE PILE.');
  - `tier`: the prince announcing a housing upgrade with real-estate grandeur, mentioning things like the cow hotel or the Burj Moolifa ('BEHOLD: THE BURJ MOOLIFA. NO, IT HAS NO ELEVATOR.');
  - `land`: the prince selling a new parcel of land ('MORE LAND! I FOLDED IT IN FROM NEXT DOOR.').
- `test/gags.test.js`: add the seven kinds to `WANT_KINDS` and change the count and the test name from 17 to 24.

## Contract
Every line upper case, at most 44 characters (spaces and punctuation count), distinct within its kind, and only characters that are keys of `GLYPHS` in `web/js/pixel.js`. Count characters carefully.

## Done when
- `node --test test/gags.test.js` passes.

## Out of scope
Every other file.
~~~~

### Save version 2 of the world, with herds, carrots, aliens and land

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/save.js` saves and loads the new world fields, so a farm of millions survives a reload. Saves from version 1 are dropped and the farm starts fresh.

## Read first
- `web/js/save.js`: `SAVE_KEY`, `SAVE_VERSION`, `saveWorld`, `loadWorld`, `clearSave`, and how loadWorld merges nested objects over a fresh `createWorld`.
- `test/save.test.js`: the existing tests.

## Change
- `web/js/save.js`: `SAVE_VERSION = 2`. In `loadWorld`, after the existing merge, also merge one level deeper so a save missing a key gets the default: `carrots`, `aliens` and `camera` like `stats`; `herds` per kind as `{...fresh.herds[k], ...saved.herds?.[k], acc: {...fresh.herds[k].acc, ...saved.herds?.[k]?.acc}}` for every kind in `fresh.herds`; and set `herdEvents` to `[]`. Arrays (`flock`, `animals`, `aliens.batches`, `aliens.saucers`) come from the save as they are.
- `test/save.test.js`: update for version 2 and add the new cases.

## Contract
The exports and their signatures do not change. A version 1 save (`{version: 1, ...}`) makes `loadWorld` return null.

## Done when
- `node --test test/save.test.js` passes, with tests for:
  - a round trip of a world with `herds.cow.females = 2e6`, `herds.cow.tier = 6`, `carrots.combines = 12`, `land = 3` and one alien batch keeps all of them;
  - a save whose `herds.goat` is missing, or whose `herds.cow.acc` lacks `product`, loads with the defaults;
  - `herdEvents` is `[]` after loading even if the save had some;
  - a version 1 save returns null.

## Out of scope
Every other file.
~~~~

### Show big herds, the carrot effect and the pan arrows in the HUD

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/hud.js` shows herd sizes into the millions, draws the new 'carrot' effect, draws speech bubbles in the right place when the camera has panned, and shows arrows when there is more land to either side.

## Read first
- `web/js/hud.js`: `drawHud`, `drawBubbles`, `drawEffects`, `KNOWN_EFFECTS`, `SPRITES`.
- `test/hud.test.js`: the `fills` and `effectRects` helpers and the drawHud tests.
- `web/js/state.js`: `herdTotal`, `hasKind`, `landWidth`, `money`, `productivity`.

## Change
- `web/js/hud.js`:
  - Bottom bar statistics: for each kind in KINDS (import from `./species.js`) with `hasKind`, `${LABEL} ${money(herdTotal(world, kind))}` where LABEL is COWS, HENS, SHEEP, GOATS, joined by one space, then ` X${productivity(world).toFixed(2)}`. Still right-aligned at RIGHT, and the log entry still clipped to end before statsX - 4. For a fresh world that is 'COWS 4 HENS 5 X1.00'.
  - New effect 'carrot' in KNOWN_EFFECTS and drawOne: sprite `['b.b', '.b.', '999', '99.', '.9.']`.
  - `export function drawBubbles(ctx, world, offsetX = 0)`: draws each bubble at `(b.x - offsetX, b.y)`, and skips a bubble whose `b.x - offsetX` is below -40 or above WIDTH + 40.
  - Pan arrows in drawHud: when `world.camera.x > 0`, a '<' in GOLD on a black 7x9 box at x 1, y 85; when `world.camera.x < landWidth(world) - WIDTH`, a '>' likewise at x 312, y 85.
- `test/hud.test.js`: update the stats text in the long-log test and add the new cases.

## Contract
`drawHud(ctx, world, rate = 0)` keeps its signature. `drawBubbles(ctx, world, offsetX = 0)` with offset 0 draws exactly as before.

## Done when
- `node --test test/hud.test.js` passes, with tests for:
  - the stats text for a fresh world, and for `world.land = 3` with `herds.sheep.females = 1.5e6` it contains 'SHEEP 1.5M';
  - the 'carrot' effect draws rects;
  - drawBubbles with offset 320 draws a bubble at world x 400 near screen x 80, and draws nothing for one at world x 40;
  - no arrows on a fresh world; with `world.land = 2` and camera.x 0 only the '>' box; with camera.x 320 only the '<' box.

## Out of scope
Every other file.
~~~~

### Widen the ground for new land, hide the barn and coop when housing moves up, and put up a FOR SALE sign

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`web/js/buildings.js` draws the ground across all the land the farm owns (the camera pans across it later), fences the Sheep Meadow and Goat Hill, leaves room for the housing buildings at the barn and the coop once a herd's housing has moved up a tier, and shows a FOR SALE sign for the next parcel.

## Read first
- `web/js/buildings.js`: `rect` (it clips to the 320x180 canvas), `drawGround`, `sceneDrawables`, `drawBarn`, `drawCoop`, `drawPastureFront`, `drawPastureSides`.
- `test/buildings.test.js`: the existing tests and what they assert about clipping.
- `web/js/species.js`: `LAND`; `web/js/state.js`: `landWidth`, `MEADOW`, `HILL`, `money`.

## Change
- `web/js/buildings.js`:
  - `rect` clips only to y in [0, HEIGHT) and x >= 0, not to x < WIDTH, because the scene is drawn translated by the camera and the land is wider than the canvas. Keep the existing tests' guarantees for the first parcel.
  - `drawGround(ctx, world)` paints the grass, tufts and the rest from x 0 to `landWidth(world)`, not to WIDTH. With `world.land >= 2`, add a dirt path along y 92..95 from x 320 to landWidth, and with `world.land >= 3` a gentle '3'/'b' mound across HILL. A missing world argument behaves as land 1.
  - `sceneDrawables(world)`: leave out the barn when `world.herds.cow.tier > 0` and the coop when `world.herds.chicken.tier > 0` (housing.js draws there instead). With land >= 2, add the MEADOW fence (same style as the pasture fence: a front rail drawable at its bottom edge and side posts); with land >= 3, the HILL fence likewise.
  - When `LAND[world.land]` exists, add a FOR SALE sign drawable at x `landWidth(world) - 30`, base y 128: a '4' post 2 wide and 12 tall, a '7' board with a '0' outline, and on it `FOR SALE` and `$${money(cost)}` in '0' with `drawText`.
- `test/buildings.test.js`: add the new cases.

## Contract
`drawGround(ctx, world)` and `sceneDrawables(world)` keep their names and return types.

## Done when
- `node --test test/buildings.test.js` passes, with tests for:
  - a fresh world draws as before (the existing tests);
  - with `world.land = 3`, drawGround paints at x >= 900;
  - `sceneDrawables` has the barn on a fresh world and not with `herds.cow.tier = 1` (compare item counts or the drawn colours), and the same for the coop;
  - the FOR SALE sign is in the scene for land 1 and 2 and not for land 3.

## Out of scope
Every other file. The housing buildings themselves are `housing.js`.
~~~~

---

## Epic 3: Switch-over

### Make the scene's cows and chickens stand for their herds, with bulls and cocks

~~~~text
Epic: 3 Switch-over
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

- New modules from the previous epic, already in the code:
  - `web/js/herds.js`: `updateHerds(world, dt, rng)`; `room(world, kind)`; `unitPrice(kind, sex)`; `maxBuy(world, kind, sex)`; `buyAnimals(world, kind, sex, n)` (n an integer or 'max', returns the count, spends the coins); `nextTier(world, kind)` (tier object or null); `upgradeHousing(world, kind)` (bool); `nextLand(world)` (LAND entry or null); `buyLand(world)` (bool). It pushes `{kind, type: 'birth'|'grow'|'death'|'buy', count, sex?}` onto `world.herdEvents` (at most 50 kept).
  - `web/js/aliens.js`: `LEASE = {stay: 45, rate: 0.6, price: 5}`; `leasable(world)`; `maxLease(world)`; `leaseCows(world, n)` (returns the count, spends the coins); `alienRate(world)` (coins/s from leased cows); `summonVisit(world, rng)`; `updateAliens(world, dt, rng, line)` (pays the alien rate itself); `alienDrawables(world)`.
  - `web/js/carrots.js`: `CARROT_UPGRADES`; `clickPower(world)`; `combineRate(world)` (coins/s); `upgradeCost(world, id)`; `carrotUpgrades(world)` (`[{id, name, blurb, level, cost, ok, reason}]`); `buyCarrotUpgrade(world, id, n)` (returns the count); `plotAt(x, y)` (plot index or -1); `pluck(world, index)` (returns coins earned, pays them); `updateCarrots(world, dt)` (pays the combines itself); `carrotDrawables(world)`.
  - `web/js/housing.js`: `housingDrawables(world)`. `web/js/flock.js`: `FLOCK_MAX`, `updateFlock(world, dt, rng)`, `flockDrawables(world)`.
  - `web/js/hud.js`: `drawHud(ctx, world, rate)`, `drawBubbles(ctx, world, offsetX)`, `drawEffects`, `updateOverlay`. `web/js/buildings.js`: `drawGround(ctx, world)` (paints the whole owned land), `sceneDrawables(world)`. `web/js/save.js`: version 2, same exports.
  - `web/js/gags.js` kinds include 'shop', 'birth', 'death', 'bull', 'rooster', 'sheep', 'goat', 'pluck', 'tier', 'land', 'ufo', 'pig', 'click'.

## Goal
The cows and chickens drawn in the scene become a sample of their herds: up to 12 cows and 14 chickens, with bulls, cocks and young ones in the same proportions as the herd, appearing and disappearing as the herd changes. Births and deaths in the herd show on them as hearts, ghosts and last words. The individual aging in `animals.js` goes, because `herds.js` now does births and deaths for the whole herd.

## Read first
- `web/js/animals.js`: all of it: sprites, `addAnimal`, `pickName`, `age`, `breed`, `updateAnimals`, `spriteRows`, `drawAnimal`.
- `test/animals.test.js`: the existing tests; the life-cycle tests (aging, `breed`, `addAnimal` age) test code this issue removes.

## Change
- `web/js/animals.js`:
  - Remove `LIFE`, `BREED`, `age`, `breed`, and the age/life/breed fields from new animals; stop importing `isAdult`, `herdSize`, `capacity` and `ADULT_AGE`. An animal's sprite now comes from two fields: `sex` ('female' or 'male'; treat missing as 'female') and `young` (boolean; treat missing as false).
  - Add sprites BULL (the COW sprite recoloured: body '4' instead of '7', no pink 'e' udder pixels, two '7' horns on the head row) and COCK (the CHICKEN sprite with a two-row '8' comb, and tail feathers in 'c' and '9' on the left). A young one draws as CALF or CHICK whatever its sex.
  - `export const VISIBLE = { cow: 12, chicken: 14 };`
  - `addAnimal(world, kind, rng, opts = {})` takes `opts.sex` and `opts.young` (defaults 'female' and false) as well as `opts.x` and `opts.y`, and sets those fields.
  - `updateAnimals(world, dt, rng, line)`, before the existing per-animal update:
    1. Events: read `world.herdEvents`, then set it to `[]`. For a 'birth' of cow or chicken: a 'heart' puff over one visible female of that kind and, at most once every 6 s of world time across all events (keep the time in `world.lifeQuietUntil`, which this module owns), a `line('birth')` bubble. For a 'death': at most once every 6 s likewise, pick a visible adult of that kind that is not abducted, give it a 'ghost' puff and a `line('death')` bubble, and log `${NAME} WENT TO THE BIG PASTURE IN THE SKY` (COOP for chickens), adding ` AND ${money(count - 1)} MORE` when count > 1. For a 'buy': a 'portal' puff over a visible animal of that kind.
    2. Reconcile, for kind cow and chicken: total = herdTotal(world, kind) - world.herds[kind].leased; target = Math.min(VISIBLE[kind], total); youngN = total ? Math.round(target * h.young / total) : 0; maleN = Math.round((target - youngN) * h.males / Math.max(1, h.females + h.males)); femaleN = target - youngN - maleN. Never remove an animal whose `abducted` is true, and leave the pig alone. When young are too many and adults too few, first turn young ones into adults in place (`young = false`, sex as needed). Then remove surplus entries of a group (prefer ones not gossiping) and add missing ones with `addAnimal`.
  - Bulls behave as cows and cocks as chickens. A cock that is awake at 06:00-06:30 says `line('rooster')` with chance 0.2 per second. A gossiping bull sometimes (chance 0.3 when a gossip starts) says `line('bull')` instead of the first whisper.
- `test/animals.test.js`: remove the tests for the removed life-cycle code and add the new cases.

## Contract
`updateAnimals(world, dt, rng, line)`, `animalDrawables(world)`, `animalAt(world, x, y)` and `addAnimal(world, kind, rng, opts)` keep their names. The pig, gossip, panics, sleep and the UFO's `abducted`/`lift` handling are unchanged.

## Done when
- `node --test test/animals.test.js` passes, with tests for:
  - a fresh world (3 cows + 1 bull, 4 hens + 1 cock) after one update shows 4 cows of which 1 male, and 5 chickens of which 1 male;
  - with `herds.cow = {females: 2e6, males: 1e5, young: 3e5, ...}` and tier 6, one update shows exactly 12 cows, about 1 young and about 1 male (match the formula), and no update ever shows more than 12 cows or 14 chickens;
  - lowering the herd removes visible animals but never an abducted one;
  - a 'death' event gives a ghost, a death bubble and the log line, and a second death within 6 s gives no second bubble;
  - a 'birth' event gives a heart;
  - BULL, COCK, CALF and CHICK drawables draw without throwing;
  - the existing gossip, smoke, bounds, abduction and click tests still pass.

## Out of scope
Every other file. `herds.js` owns the counts; this issue only reads them and the events.
~~~~

### Pay for milk, eggs, wool and cheese from the herd counts

~~~~text
Epic: 3 Switch-over
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

- New modules from the previous epic, already in the code:
  - `web/js/herds.js`: `updateHerds(world, dt, rng)`; `room(world, kind)`; `unitPrice(kind, sex)`; `maxBuy(world, kind, sex)`; `buyAnimals(world, kind, sex, n)` (n an integer or 'max', returns the count, spends the coins); `nextTier(world, kind)` (tier object or null); `upgradeHousing(world, kind)` (bool); `nextLand(world)` (LAND entry or null); `buyLand(world)` (bool). It pushes `{kind, type: 'birth'|'grow'|'death'|'buy', count, sex?}` onto `world.herdEvents` (at most 50 kept).
  - `web/js/aliens.js`: `LEASE = {stay: 45, rate: 0.6, price: 5}`; `leasable(world)`; `maxLease(world)`; `leaseCows(world, n)` (returns the count, spends the coins); `alienRate(world)` (coins/s from leased cows); `summonVisit(world, rng)`; `updateAliens(world, dt, rng, line)` (pays the alien rate itself); `alienDrawables(world)`.
  - `web/js/carrots.js`: `CARROT_UPGRADES`; `clickPower(world)`; `combineRate(world)` (coins/s); `upgradeCost(world, id)`; `carrotUpgrades(world)` (`[{id, name, blurb, level, cost, ok, reason}]`); `buyCarrotUpgrade(world, id, n)` (returns the count); `plotAt(x, y)` (plot index or -1); `pluck(world, index)` (returns coins earned, pays them); `updateCarrots(world, dt)` (pays the combines itself); `carrotDrawables(world)`.
  - `web/js/housing.js`: `housingDrawables(world)`. `web/js/flock.js`: `FLOCK_MAX`, `updateFlock(world, dt, rng)`, `flockDrawables(world)`.
  - `web/js/hud.js`: `drawHud(ctx, world, rate)`, `drawBubbles(ctx, world, offsetX)`, `drawEffects`, `updateOverlay`. `web/js/buildings.js`: `drawGround(ctx, world)` (paints the whole owned land), `sceneDrawables(world)`. `web/js/save.js`: version 2, same exports.
  - `web/js/gags.js` kinds include 'shop', 'birth', 'death', 'bull', 'rooster', 'sheep', 'goat', 'pluck', 'tier', 'land', 'ufo', 'pig', 'click'.

## Goal
`web/js/economy.js` earns from the herds as counts, so income scales to millions of animals, and its rate includes the combines and the aliens so the HUD shows the whole income.

## Read first
- `web/js/economy.js`: all of it.
- `test/economy.test.js`: the existing tests.

## Change
- `web/js/economy.js` (import `./state.js`, `./species.js`, `./carrots.js` and `./aliens.js`; nothing else):
  - `incomeRate(world)` = the sum over kinds with `hasKind` of `herds[kind].females * value / seconds * productivity(world)`, plus `combineRate(world)`, plus `alienRate(world)`.
  - `updateEconomy(world, dt)`: for each owned kind, `acc.product += females * dt / seconds`; n = Math.floor(acc.product); acc.product -= n; when n > 0: `world.coins += n * value * productivity(world)`; `world.stats[product] = (world.stats[product] ?? 0) + n` ('milk', 'eggs', 'wool', 'cheese'); and one puff for that kind in this update, no matter how large n is, over a random visible animal of that kind (`world.animals` for cow and chicken, `world.flock` for sheep and goat, female and not young if there is one): 'milk' for cow at y - 14, 'egg' for chicken at y - 9, 'coin' for sheep and goat at y - 12. It no longer pays the aliens (aliens.js does) and no longer reads `world.economy` or `world.ufos`.
  - `creditAway(world, seconds)`: `(sum of the herd product rates, productivity included, + combineRate(world)) * Math.max(0, seconds)` added to coins and returned. Aliens do not pay while away.
  - Remove `MILK` and `EGG`; per-species values come from SPECIES.
- `test/economy.test.js`: rewrite for the new rules.

## Contract
```js
export function incomeRate(world)
export function updateEconomy(world, dt)
export function creditAway(world, seconds)
```

## Done when
- `node --test test/economy.test.js` passes, with tests for:
  - a fresh world's incomeRate is 3 * 3 / 10 + 4 * 1 / 12 (3 cows and 4 hens; bulls and cocks make nothing);
  - with `herds.cow.females = 2e6` and tier 6, 10 s of 0.1 s updates earn about 2e6 * 0.3 * 10 coins (within 0.1%) and add only a few effects (no more than 100);
  - sheep earn nothing until `world.land >= 2`, then `females * 12 / 20` a second;
  - incomeRate includes combineRate and alienRate (set `carrots.combines` and `herds.cow.leased`);
  - creditAway(world, 100) pays the herds and combines but not the aliens.

## Out of scope
Every other file.
~~~~

### Restock the pig's shop: animals by the thousand, housing, land, field upgrades and leases

~~~~text
Epic: 3 Switch-over
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

- New modules from the previous epic, already in the code:
  - `web/js/herds.js`: `updateHerds(world, dt, rng)`; `room(world, kind)`; `unitPrice(kind, sex)`; `maxBuy(world, kind, sex)`; `buyAnimals(world, kind, sex, n)` (n an integer or 'max', returns the count, spends the coins); `nextTier(world, kind)` (tier object or null); `upgradeHousing(world, kind)` (bool); `nextLand(world)` (LAND entry or null); `buyLand(world)` (bool). It pushes `{kind, type: 'birth'|'grow'|'death'|'buy', count, sex?}` onto `world.herdEvents` (at most 50 kept).
  - `web/js/aliens.js`: `LEASE = {stay: 45, rate: 0.6, price: 5}`; `leasable(world)`; `maxLease(world)`; `leaseCows(world, n)` (returns the count, spends the coins); `alienRate(world)` (coins/s from leased cows); `summonVisit(world, rng)`; `updateAliens(world, dt, rng, line)` (pays the alien rate itself); `alienDrawables(world)`.
  - `web/js/carrots.js`: `CARROT_UPGRADES`; `clickPower(world)`; `combineRate(world)` (coins/s); `upgradeCost(world, id)`; `carrotUpgrades(world)` (`[{id, name, blurb, level, cost, ok, reason}]`); `buyCarrotUpgrade(world, id, n)` (returns the count); `plotAt(x, y)` (plot index or -1); `pluck(world, index)` (returns coins earned, pays them); `updateCarrots(world, dt)` (pays the combines itself); `carrotDrawables(world)`.
  - `web/js/housing.js`: `housingDrawables(world)`. `web/js/flock.js`: `FLOCK_MAX`, `updateFlock(world, dt, rng)`, `flockDrawables(world)`.
  - `web/js/hud.js`: `drawHud(ctx, world, rate)`, `drawBubbles(ctx, world, offsetX)`, `drawEffects`, `updateOverlay`. `web/js/buildings.js`: `drawGround(ctx, world)` (paints the whole owned land), `sceneDrawables(world)`. `web/js/save.js`: version 2, same exports.
  - `web/js/gags.js` kinds include 'shop', 'birth', 'death', 'bull', 'rooster', 'sheep', 'goat', 'pluck', 'tier', 'land', 'ufo', 'pig', 'click'.

## Goal
`web/js/shop.js` sells everything the bigger farm needs, in sections, with buy 1, buy 10 and buy MAX for animals, combines and leases: the market (cows and bulls, hens and cocks, and sheep and goats once their land is owned), housing tiers, land, the carrot field's upgrades, Gary's cigarettes, alien leases and the portal home.

## Read first
- `web/js/shop.js`: all of it (offers, buy, price, the prince's lines, PORTAL, how cigs send Gary to smoke).
- `test/shop.test.js`: the existing tests.
- `web/js/species.js`: `SPECIES`, `KINDS`, `LAND`.

## Change
- `web/js/shop.js` (import `./state.js`, `./species.js`, `./herds.js`, `./aliens.js`, `./carrots.js`, `./farmer.js`; not animals.js, ufo.js or economy.js):
  - `export const PORTAL = 1e9;`
  - Items, in this order and with these ids:
    - Market, for each kind in KINDS with `hasKind`: `${kind}:female` and `${kind}:male`. name: Cow, Bull, Hen, Cock, Ewe, Ram, Nanny goat, Billy goat. owned: `${money(h.females)} COWS` style (FEMALE/MALE display name upper-cased plus S, e.g. '4 HENS', '1 COCKS' is fine). cost: unitPrice. bulk: true; maxCount: maxBuy. reason: 'NO ROOM: UPGRADE HOUSING' when room is 0, else 'NOT ENOUGH COINS'. Blurbs: say the female gives its product and has young with a male; the male makes nothing but young; genders of the young are random.
    - Housing, for each owned kind: `house:${kind}`, name `nextTier.name`, owned `NOW: ${current tier name upper-cased} (${money(capacity)})`, cost nextTier.cost; reason 'TOP TIER' at the top. Blurb: room for `money(nextTier.capacity)`.
    - Land: `land`, name nextLand.name, owned `${world.land}/${LAND.length} PARCELS`, cost nextLand.cost, reason 'ALL LAND OWNED' when null. Blurb: says which animals live there (sheep, goats).
    - Field: `carrot:${id}` for each entry of `carrotUpgrades(world)`, owned `LEVEL ${level}`, bulk true only for 'gloves' and 'combines' (maxCount: how many 'max' would buy, computed by simulating the doubling costs without spending, at most 1000).
    - Gary: `cigs`, as now (`world.upgrades.cigs`, cost 20 * 2 ** cigs, x1.25 productivity, sends Gary to smoke by day).
    - Aliens: `lease`, name 'UFO lease', owned `${money(herds.cow.leased)} COWS IN SPACE`, cost LEASE.price, bulk true, maxCount maxLease; reason 'NO FREE COWS' when leasable is 0.
    - Portal: `portal`, cost PORTAL, as now (doubles productivity once).
  - `export function offers(world)` returns `[{section, id, name, blurb, owned, cost, ok, reason, bulk, maxCount}]`; section is 'MARKET', 'HOUSING', 'LAND', 'FIELD', 'GARY', 'ALIENS' or 'PORTAL'. ok is reason === ''. For bulk items ok means at least one can be bought.
  - `export function buy(world, id, n, rng, line)` returns `{ok, text, count}`. n is 1, 10 or 'max' for bulk items and ignored otherwise. It delivers through buyAnimals, upgradeHousing, buyLand, buyCarrotUpgrade, leaseCows (these spend the coins themselves) or, for cigs and the portal, spends here as now. ok is count > 0. text: the reason when nothing was bought; otherwise `line('tier')` for housing, `line('land')` for land, `line('shop')` otherwise, and the existing portal line. On success the prince says the text in a bubble, as now.
- `test/shop.test.js`: rewrite for the new catalogue.

## Contract
```js
export const PORTAL = 1e9;
export function offers(world)
export function buy(world, id, n, rng, line)
```

## Done when
- `node --test test/shop.test.js` passes, with tests for:
  - a fresh world's offers have the MARKET ids `cow:female`, `cow:male`, `chicken:female`, `chicken:male` and no sheep; after buying land, `sheep:female` appears;
  - `buy(world, 'cow:female', 'max', ...)` with 1e6 coins buys `min(floor(1e6 / 30), room)`, and with a full pasture returns ok false with 'NO ROOM: UPGRADE HOUSING';
  - `buy(world, 'house:cow', 1, ...)` moves to Cow Stables for 400 coins; at the top tier the offer says 'TOP TIER';
  - `buy(world, 'carrot:combines', 10, ...)` buys up to 10 and the count matches the coins spent;
  - `buy(world, 'lease', 'max', ...)` leases min(cows, floor(coins / 5));
  - cigs and the portal behave as before (productivity 1.25, then x2; portal once).

## Out of scope
Every other file. `main.js` renders these offers; do not edit it.
~~~~

### Wire the bigger farm into the page: camera panning, carrot clicks, the new modules and the shop with buy 1/10/MAX

~~~~text
Epic: 3 Switch-over
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

- New modules from the previous epic, already in the code:
  - `web/js/herds.js`: `updateHerds(world, dt, rng)`; `room(world, kind)`; `unitPrice(kind, sex)`; `maxBuy(world, kind, sex)`; `buyAnimals(world, kind, sex, n)` (n an integer or 'max', returns the count, spends the coins); `nextTier(world, kind)` (tier object or null); `upgradeHousing(world, kind)` (bool); `nextLand(world)` (LAND entry or null); `buyLand(world)` (bool). It pushes `{kind, type: 'birth'|'grow'|'death'|'buy', count, sex?}` onto `world.herdEvents` (at most 50 kept).
  - `web/js/aliens.js`: `LEASE = {stay: 45, rate: 0.6, price: 5}`; `leasable(world)`; `maxLease(world)`; `leaseCows(world, n)` (returns the count, spends the coins); `alienRate(world)` (coins/s from leased cows); `summonVisit(world, rng)`; `updateAliens(world, dt, rng, line)` (pays the alien rate itself); `alienDrawables(world)`.
  - `web/js/carrots.js`: `CARROT_UPGRADES`; `clickPower(world)`; `combineRate(world)` (coins/s); `upgradeCost(world, id)`; `carrotUpgrades(world)` (`[{id, name, blurb, level, cost, ok, reason}]`); `buyCarrotUpgrade(world, id, n)` (returns the count); `plotAt(x, y)` (plot index or -1); `pluck(world, index)` (returns coins earned, pays them); `updateCarrots(world, dt)` (pays the combines itself); `carrotDrawables(world)`.
  - `web/js/housing.js`: `housingDrawables(world)`. `web/js/flock.js`: `FLOCK_MAX`, `updateFlock(world, dt, rng)`, `flockDrawables(world)`.
  - `web/js/hud.js`: `drawHud(ctx, world, rate)`, `drawBubbles(ctx, world, offsetX)`, `drawEffects`, `updateOverlay`. `web/js/buildings.js`: `drawGround(ctx, world)` (paints the whole owned land), `sceneDrawables(world)`. `web/js/save.js`: version 2, same exports.
  - `web/js/gags.js` kinds include 'shop', 'birth', 'death', 'bull', 'rooster', 'sheep', 'goat', 'pluck', 'tier', 'land', 'ufo', 'pig', 'click'.

## Goal
`web/js/main.js` and `web/index.html` run the new modules, let the player pan across the land they own, pluck carrots by clicking the field, and buy in bulk in the pig's shop.

## Read first
- `web/js/main.js`: all of it (step, draw, fit, the click handler, the shop panel's buildShop/refreshShop, save/load, keys).
- `web/index.html`: the controls, the shop panel and its CSS.
- `test/smoke.cjs`: the browser checks.

## Change
- `web/js/main.js`:
  - Imports: add `updateHerds` (herds.js); `updateAliens`, `alienDrawables`, `summonVisit` (aliens.js); `updateCarrots`, `carrotDrawables`, `pluck`, `plotAt` (carrots.js); `housingDrawables` (housing.js); `updateFlock`, `flockDrawables` (flock.js); `landWidth` (state.js). Remove the ufo.js import and every use of it.
  - `step(dt)` order: advanceTime, updateFarmer, updateHerds, updateAnimals, updateFlock, updateCrops, updateCarrots, updateAliens, updateEconomy, updateOverlay.
  - Camera: `camX = Math.round(Math.min(Math.max(world.camera.x, 0), landWidth(world) - WIDTH))`, also written back to `world.camera.x`. `draw()`: drawSky (screen space); then `ctx.save(); ctx.translate(-camX, 0);` drawGround, drawField, the sorted drawables (sceneDrawables, housingDrawables, cropDrawables, carrotDrawables, farmerDrawables, animalDrawables, flockDrawables, alienDrawables), drawEffects, `ctx.restore();` then drawNightShade, `drawBubbles(ctx, world, camX)`, `drawHud(ctx, world, incomeRate(world))`.
  - Panning: ArrowLeft/A and ArrowRight/D held down move the camera 160 units per second of real time (track held keys; apply in `frame`); dragging on the canvas with the mouse button down pans by the dragged distance / scale; a press that moves less than 4 px is a click, not a drag. Two buttons in the controls, `#pan-left` ('< Pan') and `#pan-right` ('Pan >'), move the camera by 160.
  - Clicks: convert to world coordinates by adding camX to x. If `plotAt(x, y)` is a plot, `pluck(world, index)` and, with chance 0.15, `say` a `line('pluck')` thought over Gary. Otherwise the pig opens the shop and other animals talk, as now. The U key calls `summonVisit` at night.
  - Shop panel: group rows under an `<h3>` per section in the order offers gives. A bulk row has three buttons: `BUY 1 $${money(cost)}`, `10 $${money(cost * 10)}`, `MAX (${money(maxCount)})`, calling `buy(world, id, 1 | 10 | 'max', rng, line)`. Other rows have one `BUY $${money(cost)}` button. Disable a button when it cannot buy (bulk 10 needs maxCount >= 10). Rebuild the rows when the list of offer ids changes (new land adds rows); otherwise update text and disabled state in place, as refreshShop does now.
- `web/index.html`: add the two pan buttons to `.controls`; CSS for `.shop h3` (gold, 13px, margin 10px 0 4px) and for a row's group of buttons (`.item-buy` stays a column on narrow screens; three buttons side by side in a flex row with gap 4px). Update the hint text: drag or use the arrow keys to pan, click the field to pluck carrots.
- `test/smoke.cjs`: replace the `world.ufos` check with `world.aliens.visit !== null` after pressing U at night; the chicken buy now clicks the first button of the `chicken:female` row (give each row `data-id` set to the offer id so the test can select `li[data-id="chicken:female"] button`); add a check that clicking the canvas at the field (canvas point (30, 110) times the scale, relative to the canvas) raises `world.stats.plucks`.

## Contract
Offers are `[{section, id, name, blurb, owned, cost, ok, reason, bulk, maxCount}]` from `offers(world)` in shop.js, and `buy(world, id, n, rng, line)` returns `{ok, text, count}`.

## Done when
- `node --check web/js/main.js` passes.
- `cd samples/farm && go test ./...` passes (it serves the files).
- Serve the page with `go run ./cmd/farm -addr 127.0.0.1:8097` in the background and confirm with `curl -s http://127.0.0.1:8097/js/main.js | head -5` that it is served, then stop the server. (`make smoke` needs Playwright, which may be missing; if `node -e "require('playwright')"` fails, say so in your comment instead of running it.)

## Out of scope
Every other file under `web/js/`.
~~~~

---

## Epic 4: Cleanup and docs

### Remove the old UFO module and the per-animal economy fields

~~~~text
Epic: 4 Cleanup and docs
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
The code the bigger farm replaced is gone: `web/js/ufo.js` and its test, and the state fields and helpers that only it and the old individual life cycle used.

## Read first
- `web/js/state.js`: `ADULT_AGE`, `HERD`, `isAdult`, `herdSize`, `capacity`, and in `createWorld` the fields `upgrades.pasture`, `upgrades.coop`, `economy`, `ufos`, `ufoLastDay`, and the `life` table that sets `age`, `life` and `breed` on the first animals.
- Find every remaining use of each with `bin/codeplat-nav refs -dir samples/farm NAME` (or grep under `samples/farm/web` and `samples/farm/test`).

## Change
- Delete `web/js/ufo.js` and `test/ufo.test.js`.
- `web/js/state.js`: remove the names listed above that nothing outside `state.js` and `test/state.test.js` uses any more. If something else still uses one, leave it and say so in your comment.
- `test/state.test.js`: remove the tests of the removed names and fix the createWorld expectations.
- `web/js/save.js` needs no change; check that it does not mention the removed fields.

## Contract
No export that another module still imports is removed.

## Done when
- `cd samples/farm && make test` passes (all other work in the goal is done by now, so the whole suite must pass).
- `grep -rn "ufo.js\|herdSize\|isAdult\|ADULT_AGE" samples/farm/web samples/farm/test` prints nothing, except a name you had to keep and explained in your comment.

## Out of scope
README.md and ARCHITECTURE.md are the docs issue.
~~~~

### Document the bigger farm: herds, housing, land, carrots, leases and the shop

~~~~text
Epic: 4 Cleanup and docs
Depends on: none

## Environment
- Work in `samples/farm/`; every path below is relative to it. Run node commands from `samples/farm/`. The tracker CLI is `bin/codeplat` from the repository root.
- Plain browser ES modules in `web/js/`, no build step, no npm packages. Node runs only the tests: `node --test test/NAME.test.js`, using `node:test` and `node:assert/strict`.
- `ARCHITECTURE.md` (in `samples/farm/`) is binding: each world field has one writing module; modules touch no window or document on load; no module but `main.js` imports `gags.js`, so gag text comes in through a `line(kind)` argument and tests pass a stub such as `const line = (kind) => 'LINE ' + kind;`; drawables are `{y, draw(ctx)}` where y is the base line used to sort them.
- Other workers are editing other files under `web/js/` and `test/` at the same time. Change only the files listed under Change. Run only the commands under Done when, never `make test`, because another worker's half-done file may fail it. Do not commit.
- Style: a comment at the top of each file saying what it does, a short comment above every function, 2-space indent, single quotes, semicolons. Text drawn on the canvas must be upper case and use only the 3x5 font's characters: letters, digits, space and . , ! ? ' " - : ; * ( ) / + = % # & ~ > < $.

## World facts (already in the code)
- `web/js/species.js` exports `SPECIES`, `KINDS` (`['cow', 'chicken', 'sheep', 'goat']`), `LAND` (`[{name, cost}]`: Gossip Acres 0, Sheep Meadow 3000, Goat Hill 50000) and `PARCEL_W` (320). A species has `female`, `male`, `young` (display names: cow 'cow' 'bull' 'calf'; chicken 'hen' 'cock' 'chick'; sheep 'ewe' 'ram' 'lamb'; goat 'nanny' 'billy' 'kid'), `product` ('milk', 'eggs', 'wool', 'cheese'), `value` and `seconds` (each adult female makes one product worth `value` coins every `seconds`), `birth` (young per breeding female per second), `matesPerMale`, `grow` (seconds for a young one to grow up), `life` (mean adult lifetime in seconds), `price: {female, male}`, `parcel` (cow and chicken 0, sheep 1, goat 2), and `tiers: [{name, capacity, cost, look: {shape, w, h, wall, roof}}]`. Tier 0 is free. The cow tiers end with 'Burj Moolifa' (capacity 2.5e6) and the chicken tiers with 'Burj Al Hen' (4e6). `look.shape` is one of 'none', 'shed', 'barn', 'block', 'tower', 'burj', 'sail', 'mountain'; `wall` and `roof` are PALETTE keys from `web/js/pixel.js`.
- `createWorld(seed)` in `web/js/state.js` gives: `world.herds[kind] = {females, males, young, leased, tier, acc: {birth, grow, death, product}}` for every kind (cow 3 females and 1 male, chicken 4 and 1, sheep and goat 0, all tiers 0); `world.herdEvents = []`; `world.land = 1` (parcels owned, from the left); `world.camera = {x: 0}`; `world.carrots = {gloves: 0, hoe: 0, fertilizer: 0, combines: 0, acc: 0, combineX: FIELD.x}`; `world.aliens = {batches: [], saucers: [], lastDay: 0}`; `world.flock = []`; `world.coins` (a float, starts 10); `world.stats.plucks` (0); `world.upgrades.cigs` and `world.upgrades.portal`; `world.animals` (the cows, chickens and pig drawn in the scene); `world.plots` (18 carrot plots, each `{col, row, crop: 'carrot', growth, stage, water}`).
- `state.js` helpers: `herdTotal(world, kind)` (females + males + young + leased), `housingCapacity(world, kind)`, `hasKind(world, kind)` (its parcel is owned), `landWidth(world)` (land * 320), `productivity(world)`, `money(n)` (formats '12', '12.3K', '4.5M', '6.7B', '8.9T', '1.2QA'), `say(world, x, y, text, kind, ttl)`, `logEvent(world, text)`, `puff(world, kind, x, y, vx, vy, ttl)`, `isNight(world)`. Constants: `MEADOW = {x: 336, y: 100, w: 200, h: 64}` (sheep), `HILL = {x: 656, y: 100, w: 200, h: 64}` (goats), `SITES = {cow: {x: 230, y: 84}, chicken: {x: 120, y: 84}, sheep: {x: 552, y: 92}, goat: {x: 872, y: 92}}` (x is a building's left edge, y the ground line under it), `FIELD = {x: 14, y: 96, cols: 6, rows: 3, tile: 16}`, `CARROT_VALUE = 1`, `RIPE_BONUS = 3`, `PASTURE`, `YARD`, `BARN`, `COOP`, `WIDTH` (320), `HEIGHT` (180).
- `test/fakectx.js` exports `fakeCtx()`, which records canvas calls in `ctx.calls` and counts them with `ctx.count(op)`. `web/js/rng.js` exports `makeRng(seed)` with `next()`, `int(n)`, `pick(array)`, `chance(p)` and `range(a, b)`.

## Goal
`samples/farm/README.md` and `samples/farm/ARCHITECTURE.md` describe the game as it now is.

## Read first
- `samples/farm/README.md` and `samples/farm/ARCHITECTURE.md`: the current text (it describes the earlier version: individual animals with ages, a pasture and coop that expand, one saucer per cow, a $25,000 portal).
- `web/js/species.js`, `web/js/herds.js`, `web/js/aliens.js`, `web/js/carrots.js`, `web/js/shop.js`, `web/js/main.js`: the top comments and exported names, for the facts.

## Change
- `README.md`: the game section: herds as counts that reach millions; genders at random for the young, bulls and cocks bought at the market; the housing ladder per species, with the tier names from species.js (a table per species: tier, capacity, cost); land parcels and their animals; the carrot clicker and its upgrades (gloves, golden hoe, fertilizer, combine harvester); UFO leases; buy 1/10/MAX; the $1B portal. Controls: panning (drag, arrow keys or A/D, the pan buttons), clicking the field. The layout list: add species.js, herds.js, housing.js, carrots.js, aliens.js, flock.js; remove ufo.js.
- `ARCHITECTURE.md`: the world fields (herds, herdEvents, land, camera, carrots, aliens, flock; remove ufos, economy, upgrades.pasture/coop), field ownership per module (herds.js: herds counts, tiers, land and the coins spent on them; aliens.js: aliens, herds.cow.leased and the cows it moves, alien pay; carrots.js: carrots, stats.plucks, plucked plots, carrot coins; flock.js: flock; animals.js: animals, lifeQuietUntil, and it clears herdEvents; economy.js: product coins and stats), the update order from main.js's step, the draw order with the camera translate, and why herds are counts with a visible sample.

## Contract
None: documentation only.

## Done when
- Every file path named in the two documents exists: check each with `ls`.
- Every update and draw function named in ARCHITECTURE.md's order sections appears in `web/js/main.js`.

## Out of scope
Every file other than the two documents.
~~~~
