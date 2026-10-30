// Smoke test: runs the page in a real Chromium and checks that it starts, animates and responds.
// Starts the server itself, and always stops it. When SMOKE_SHOTS names a directory, a screenshot
// of each page is saved there; otherwise nothing is written.

const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const { chromium } = require('playwright');

const ROOT = path.resolve(__dirname, '..');
const ADDR = '127.0.0.1:8098';
const BASE = `http://${ADDR}`;
const SERVER_WAIT_MS = 60_000;
const SETTLE_MS = 3_000;

function fail(message) {
  throw new Error(message);
}

function startServer() {
  const child = spawn('go', ['run', './cmd/farm', '-addr', ADDR], {
    cwd: ROOT,
    detached: true, // own process group, so the go tool and the built server both get stopped
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let output = '';
  child.stdout.on('data', (d) => { output += d; });
  child.stderr.on('data', (d) => { output += d; });
  child.exited = false;
  child.once('exit', () => { child.exited = true; });
  child.output = () => output;
  return child;
}

async function stopServer(child) {
  if (!child || child.exited) return;
  const exited = new Promise((resolve) => child.once('exit', resolve));
  try {
    process.kill(-child.pid, 'SIGTERM');
  } catch (_) {
    return; // already gone
  }
  await Promise.race([exited, new Promise((r) => setTimeout(r, 5_000))]);
}

async function waitForServer(child) {
  const deadline = Date.now() + SERVER_WAIT_MS;
  while (Date.now() < deadline) {
    if (child.exited) fail(`server exited early:\n${child.output()}`);
    try {
      const res = await fetch(`${BASE}/`);
      if (res.ok) return;
    } catch (_) {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  fail(`server did not answer within ${SERVER_WAIT_MS / 1000} s:\n${child.output()}`);
}

// visit opens the page at the given query, waits, and returns what the checks need.
async function visit(browser, query, shotName) {
  const context = await browser.newContext({ viewport: { width: 800, height: 600 } });
  const page = await context.newPage();
  const problems = [];
  page.on('pageerror', (err) => problems.push(`page error: ${err.message}`));
  page.on('console', (msg) => {
    if (msg.type() === 'error') problems.push(`console error: ${msg.text()}`);
  });

  try {
    await page.goto(`${BASE}/?${query}`, { waitUntil: 'load' });
    await page.waitForFunction(() => window.farm && window.farm.world, null, { timeout: 10_000 });
    const t0 = await page.evaluate(() => window.farm.world.time);
    await page.waitForTimeout(SETTLE_MS);
    const t1 = await page.evaluate(() => window.farm.world.time);

    const colours = await page.evaluate(() => {
      const canvas = document.getElementById('farm');
      const data = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data;
      const seen = new Set();
      for (let i = 0; i < data.length; i += 4 * 7) {
        seen.add((data[i] << 16) | (data[i + 1] << 8) | data[i + 2]);
      }
      return seen.size;
    });

    if (process.env.SMOKE_SHOTS) {
      fs.mkdirSync(process.env.SMOKE_SHOTS, { recursive: true });
      await page.screenshot({ path: path.join(process.env.SMOKE_SHOTS, `${shotName}.png`) });
    }

    return { page, t0, t1, colours, problems };
  } catch (err) {
    await context.close();
    throw err;
  }
}

async function main() {
  const server = startServer();
  let browser;
  try {
    await waitForServer(server);
    browser = await chromium.launch();

    // Daytime page: time advances, the canvas is drawn, no errors.
    const day = await visit(browser, 'seed=42&hour=10', 'seed42-hour10');
    if (!(day.t1 > day.t0)) fail(`world.time did not advance: ${day.t0} -> ${day.t1}`);
    if (day.colours < 2) fail(`canvas is one flat colour (${day.colours} sampled colours)`);
    if (day.problems.length) fail(`day page reported:\n  ${day.problems.join('\n  ')}`);

    // The pig's shop: S opens it, and buying a chicken spends coins and adds one.
    await day.page.keyboard.press('s');
    if (await day.page.isHidden('#shop')) fail('pressing S did not open the shop');
    const before = await day.page.evaluate(() => ({
      coins: window.farm.world.coins,
      hens: window.farm.world.animals.filter((a) => a.kind === 'chicken').length,
    }));
    await day.page.click('#shop-items li:nth-child(2) button');
    const after = await day.page.evaluate(() => ({
      coins: window.farm.world.coins,
      hens: window.farm.world.animals.filter((a) => a.kind === 'chicken').length,
    }));
    if (!(after.hens === before.hens + 1 && after.coins < before.coins)) {
      fail(`buying a chicken: ${JSON.stringify(before)} -> ${JSON.stringify(after)}`);
    }
    if (day.problems.length) fail(`shop reported:\n  ${day.problems.join('\n  ')}`);
    await day.page.context().close();

    // Night page: the U easter egg starts a UFO visit.
    const night = await visit(browser, 'seed=42&hour=23', 'seed42-hour23');
    if (!(night.t1 > night.t0)) fail(`night: world.time did not advance: ${night.t0} -> ${night.t1}`);
    if (night.colours < 2) fail(`night: canvas is one flat colour (${night.colours} sampled colours)`);
    await night.page.keyboard.press('u');
    const saucers = await night.page.evaluate(() => window.farm.world.ufos.length);
    if (saucers === 0) fail('night: pressing U did not start a UFO visit');
    if (night.problems.length) fail(`night page reported:\n  ${night.problems.join('\n  ')}`);
    await night.page.context().close();

    console.log('smoke: ok');
  } finally {
    if (browser) await browser.close();
    await stopServer(server);
  }
}

main().catch((err) => {
  console.error(`smoke: FAILED\n${err.message}`);
  process.exitCode = 1;
});
