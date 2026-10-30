# farm

Gossip Acres is a small farm scene in the browser. Farmer Gary tends the crops
and the cows, chickens and pig gossip whenever he isn't looking. Gary sneaks
cigarettes behind the barn, the chickens hang around the edge of the yard, and
after dark a UFO may come for a cow.

It is also an idle game. Cows give milk and hens lay eggs, which sell for coins
on their own. Cows and chickens have young while there is room, and in time
they die of old age. The pig, Sir Oinksalot, is an interdimensional prince
trapped in our world, and he runs the shop. Click him to buy:

| Item | What it does |
|------|--------------|
| Cow, Chicken | another adult; the price rises with the herd |
| UFO contract | aliens borrow one free adult cow for 45 s and pay $0.60 a second while they have her; cheap, one saucer per cow |
| Royal cigarettes | Gary smokes one, and productivity, the multiplier on milk and eggs, goes up 1.25x per pack |
| Bigger pasture, Bigger coop | room for 3 more cows (up to 24) or 4 more chickens (up to 28) |
| Fix the portal home | $25,000, the goal: the prince goes home, comes back for the mud, and productivity doubles |

The farm is saved in the browser's localStorage and pays for up to two hours of
milk and eggs while you are away. New farm starts over.

The scene is drawn on a 320 by 180 canvas with pixel-art sprites. The Go server
only serves the page and its scripts; the simulation runs in the browser.

## Run

    make run

Then open http://127.0.0.1:8088.

The server listens on `127.0.0.1:8088` by default. To change that, set
`FARM_ADDR` or pass `-addr`:

    FARM_ADDR=127.0.0.1:9000 make run
    go run ./cmd/farm -addr 127.0.0.1:9000

The page reads three URL parameters. Each is an integer and an invalid value is
ignored:

| Parameter | Meaning |
|-----------|---------|
| `seed` | the world's random seed; the same seed gives the same world (default: the time, modulo 100000). A seeded page is a sandbox: it is not saved and does not load the saved farm |
| `hour` | start at this hour of the day, 0 to 23 (default: 6, morning) |
| `speed` | start at this speed: 1, 2 or 4 (default: 1) |

For example, http://127.0.0.1:8088/?seed=42&hour=23&speed=2 starts a seeded
night at double speed.

## Controls

Click an animal to hear what it is really thinking, or click Gary to hear what
he is thinking. Click the pig to open his shop.

| Control | Keys | What it does |
|---------|------|--------------|
| Pause / Play | Space | stops and starts the world |
| 1x, 2x, 4x | 1, 2, 4 | sets the simulation speed |
| Skip to night/morning | N | jumps to 21:00, or to 06:00 when it is already night |
| Visit the pig | S | opens or closes the shop (Esc closes it) |
| New farm | (none) | deletes the save and starts again |
| (none) | U | after dark, starts a UFO visit to a random cow that is not already abducted |
| (none) | F | makes Gary smoke a cigarette behind the barn |

The clock runs a day in 180 seconds of simulated time, starting at 06:00.

## Test

    make test     # go test ./... and the node tests in test/*.test.js
    make vet      # go vet ./...
    make smoke    # test/smoke.cjs: runs the page in Chromium and checks it

`make test` needs only Go and Node. `make smoke` starts its own server on
`127.0.0.1:8098` and stops it when it finishes. It needs the `playwright`
package, which the repository does not install, and a Chromium that Playwright
can launch. Set `SMOKE_SHOTS` to a directory to save a screenshot of each
checked page there.

## Layout

    cmd/farm/main.go         the entrypoint: serves the page on FARM_ADDR or -addr
    web/web.go               the HTTP handler for index.html and js/*.js (embedded)
    web/index.html           the page: canvas, controls and styles
    web/js/main.js           the entry point: fixed-step loop, drawing order, controls
    web/js/state.js          the world model and its time and clock
    web/js/rng.js            seeded random numbers (mulberry32)
    web/js/pixel.js          palette, sprites, the 3x5 font and speech bubbles
    web/js/sky.js            sky, stars, sun and moon, night shade and fireflies
    web/js/buildings.js      the farmhouse, barn, coop, fences and ground
    web/js/crops.js          the field, crop growth and water
    web/js/farmer.js         Gary: his tasks, walking and smoking
    web/js/animals.js        cows, chickens and the pig: gossip, births and old age
    web/js/ufo.js            the UFO visit and abductions
    web/js/economy.js        milk, eggs, alien pay and the credit for time away
    web/js/shop.js           the pig's shop: what is for sale, prices and buying
    web/js/save.js           saving and loading the world in localStorage
    web/js/gags.js           the lines the characters say, by kind
    web/js/hud.js            the top and bottom bars, bubbles and effects
    test/*.test.js           node tests for the web modules
    test/fakectx.js          a fake 2D canvas context for those tests
    test/smoke.cjs           the browser smoke test
    web/web_test.go          Go tests for the handler
    Makefile                 test, vet, run and smoke
    ARCHITECTURE.md          how the code is organised and why

The project is listed in the repository's `./start` script on port 8088.
