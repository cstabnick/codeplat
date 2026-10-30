// The ground layer and the standing scenery of the farm: grass, paths, the mud puddle, the yard,
// the house, barn, coop, trees, scarecrow, mailbox and the pasture fence. drawGround paints the
// flat parts that nothing stands behind; sceneDrawables returns the rest as depth-sorted drawables
// so that animals and the farmer can pass in front of and behind them. Nothing here touches
// window or document on load, and nothing uses Math.random, so every frame draws the same picture.

import { WIDTH, HEIGHT, HORIZON, HOUSE, DOOR, WINDOWS, COOP, YARD, BARN, PASTURE, MUD, MEADOW, HILL, landWidth, money } from './state.js';
import { LAND, PARCEL_W } from './species.js';
import { PALETTE, drawSprite, drawText } from './pixel.js';

// rect fills a box of one palette colour. It is clipped to y in 0..HEIGHT and to x >= 0, but not
// to the first parcel's width, because the land is wider than the canvas and the camera pans
// across it. An empty box draws nothing.
function rect(ctx, key, x, y, w, h) {
  const x0 = Math.max(0, x);
  const y0 = Math.max(0, y);
  const x1 = x + w;
  const y1 = Math.min(HEIGHT, y + h);
  if (x1 <= x0 || y1 <= y0) return;
  ctx.fillStyle = PALETTE[key];
  ctx.fillRect(x0, y0, x1 - x0, y1 - y0);
}

// speck is a deterministic value in 0..n-1 for a cell. It stands in for randomness, so the
// scatter of tufts and pebbles is the same on every frame.
function speck(x, y, n) {
  let h = Math.imul(x, 374761393) ^ Math.imul(y, 668265263);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) % n;
}

// drawMud paints the puddle as a rounded '4' blob with a darker '2' rim and two '5' bubbles.
function drawMud(ctx) {
  const h = MUD.h;
  const inset = (r) => (r === 0 || r === h - 1 ? 4 : r === 1 || r === h - 2 ? 2 : 0);
  for (let r = 0; r < h; r++) {
    const i = inset(r);
    rect(ctx, '2', MUD.x + i, MUD.y + r, MUD.w - 2 * i, 1);
    if (r > 0 && r < h - 1) {
      rect(ctx, '4', MUD.x + i + 1, MUD.y + r, MUD.w - 2 * i - 2, 1);
    }
  }
  rect(ctx, '5', MUD.x + 12, MUD.y + 6, 2, 2);
  rect(ctx, '5', MUD.x + 30, MUD.y + 9, 3, 3);
  rect(ctx, '5', MUD.x + 22, MUD.y + 3, 1, 1);
}

// drawYard paints the chicken yard as lighter ground: 'f' at the edges, with '9' dots that get
// denser towards the middle.
function drawYard(ctx) {
  rect(ctx, 'f', YARD.x, YARD.y, YARD.w, YARD.h);
  for (let y = YARD.y + 3; y < YARD.y + YARD.h - 3; y++) {
    for (let x = YARD.x + 3; x < YARD.x + YARD.w - 3; x++) {
      const edge = Math.min(x - YARD.x, YARD.x + YARD.w - 1 - x, y - YARD.y, YARD.y + YARD.h - 1 - y);
      if (speck(x, y, edge < 8 ? 9 : 4) === 0) rect(ctx, '9', x, y, 1, 1);
    }
  }
}

// drawMound paints a grass mound across the hill as a dome of '3' rows, 12 rows high, centred on
// x 756 (HILL.x + 100) and rising from the hill's top at HILL.y - 12. The dome is the same colour
// as the grass around it, so a 'b' highlight on its upper left shoulder (rows 8 and 9, inside the
// dome's outline) makes the shape show.
function drawMound(ctx) {
  const cx = HILL.x + 100;
  for (let k = 0; k <= 11; k++) {
    const half = Math.round(Math.sqrt(144 - (12 - k) ** 2));
    rect(ctx, '3', cx - half, HILL.y - 12 + k, 2 * half + 1, 1);
  }
  rect(ctx, 'b', cx - 9, HILL.y - 12 + 8, 6, 2);
}

// drawGround fills y HORIZON..HEIGHT with grass ('3' with 'b' tufts) across all the land the farm
// owns, then the dirt paths, the mud puddle, the yard and the back rail of the pasture fence.
// Every rectangle it draws starts at or below HORIZON. A missing world is land 1.
export function drawGround(ctx, world) {
  const land = world ? world.land : 1;
  const width = land * PARCEL_W;
  rect(ctx, '3', 0, HORIZON, width, HEIGHT - HORIZON);
  for (let y = HORIZON; y < HEIGHT - 1; y++) {
    for (let x = 0; x < width; x++) {
      if (speck(x, y, 60) === 0) rect(ctx, 'b', x, y, 1, 2);
    }
  }

  // Land 2 and up have a dirt path running right from the first parcel's edge, and land 3 has a
  // grassy mound across the hill.
  if (land >= 2) rect(ctx, '4', PARCEL_W, 92, width - PARCEL_W, 4);
  if (land >= 3) drawMound(ctx);

  // The path from the front door down to y 88, then the road along y 86..92 from x 30 to 300.
  const doorPathX = DOOR.x - 3;
  rect(ctx, '4', doorPathX, DOOR.y, 6, 11);
  rect(ctx, '4', 30, 86, 271, 7);
  for (let y = 86; y <= 92; y++) {
    for (let x = 30; x <= 300; x++) {
      if (speck(x, y, 29) === 0) rect(ctx, '9', x, y, 1, 1);
    }
  }
  for (let y = DOOR.y; y <= 88; y++) {
    for (let x = doorPathX; x < doorPathX + 6; x++) {
      if (speck(x, y, 29) === 0) rect(ctx, '9', x, y, 1, 1);
    }
  }

  drawMud(ctx);
  drawYard(ctx);

  // The back rail of the pasture fence, with a post every 12 units. The field's soil is drawn
  // by the plots module, not here.
  rect(ctx, '4', PASTURE.x, PASTURE.y, PASTURE.w, 1);
  for (let x = PASTURE.x; x < PASTURE.x + PASTURE.w; x += 12) {
    rect(ctx, '4', x, PASTURE.y - 4, 1, 4);
  }
}

// drawHouse draws the house. Its base line is HOUSE.y + HOUSE.h.
function drawHouse(ctx) {
  const { x, y, w, h } = HOUSE;
  const roofRows = 20;

  rect(ctx, '5', x + w - 14, y - 8, 6, 14);

  // A pitched roof: each row is narrower than the one below, and its right third is shaded '2'.
  for (let r = 0; r < roofRows; r++) {
    const inset = Math.round((roofRows - 1 - r) * 0.45);
    const span = w - 2 * inset;
    const shade = Math.round(span / 3);
    rect(ctx, '8', x + inset, y + r, span, 1);
    rect(ctx, '2', x + w - inset - shade, y + r, shade, 1);
  }

  const wallY = y + roofRows;
  const wallH = h - roofRows;
  rect(ctx, '7', x, wallY, w, wallH);
  rect(ctx, '4', x, wallY, w, 1);
  rect(ctx, '4', x, y + h - 1, w, 1);
  rect(ctx, '4', x, wallY, 2, wallH);
  rect(ctx, '4', x + w - 2, wallY, 2, wallH);

  for (const win of WINDOWS) {
    rect(ctx, '4', win.x - 1, win.y - 1, win.w + 2, win.h + 2);
    rect(ctx, 'c', win.x, win.y, win.w, win.h);
    rect(ctx, '7', win.x + Math.floor(win.w / 2), win.y, 1, win.h);
    rect(ctx, '7', win.x, win.y + Math.floor(win.h / 2), win.w, 1);
  }

  // The door is 10 wide and 16 tall, centred on DOOR.x, with its bottom row just above DOOR.y.
  rect(ctx, '4', DOOR.x - 5, DOOR.y - 16, 10, 16);
  rect(ctx, '9', DOOR.x + 2, DOOR.y - 8, 1, 1);
}

// drawBarn draws the barn: a red body with white corner trim, a gambrel roof in '2', white
// X-braced doors in the middle of the front, a hayloft door with a hay tuft, and a weathervane.
function drawBarn(ctx) {
  const { x, y, w, h } = BARN;
  const cx = x + w / 2;
  const gambrel = [14, 13, 12, 11, 11, 10, 8, 6, 4, 3, 2, 1, 0, 0, 0, 0];
  const roofRows = gambrel.length;

  // The weathervane sits on the ridge: a pole, a gold ball and an arrow.
  rect(ctx, '5', cx, y - 10, 1, 10);
  rect(ctx, 'a', cx, y - 12, 1, 2);
  rect(ctx, '9', cx - 6, y - 9, 13, 1);
  rect(ctx, 'a', cx + 6, y - 10, 2, 3);

  for (let r = 0; r < roofRows; r++) {
    const i = gambrel[r];
    rect(ctx, '2', x + i, y + r, w - 2 * i, 1);
  }

  const bodyY = y + roofRows;
  rect(ctx, '8', x, bodyY, w, h - roofRows);
  rect(ctx, '7', x, bodyY, 1, h - roofRows);
  rect(ctx, '7', x + w - 1, bodyY, 1, h - roofRows);

  // The double doors: white, with an X brace drawn from corner to corner.
  const doorW = 20;
  const doorH = 28;
  const doorX = Math.round(cx) - doorW / 2;
  const doorY = y + h - doorH;
  rect(ctx, '7', doorX, doorY, doorW, doorH);
  for (let r = 0; r < doorH; r++) {
    const col = Math.round((r * (doorW - 1)) / (doorH - 1));
    rect(ctx, '5', doorX + col, doorY + r, 1, 1);
    rect(ctx, '5', doorX + (doorW - 1 - col), doorY + r, 1, 1);
  }

  // The hayloft door sits above the big doors, with a '9' hay tuft poking out of it.
  rect(ctx, '2', Math.round(cx) - 4, y + 20, 8, 7);
  drawSprite(ctx, ['.9.9.9.', '9999999'], Math.round(cx) - 3, y + 18);
}

// drawCoop draws the chicken coop: a '4' wooden hut under a '5' roof, a black hatch with a '9'
// ramp, and a tiny 'CLUCK HQ' sign.
function drawCoop(ctx) {
  const { x, y, w, h } = COOP;

  for (let r = 0; r < 8; r++) {
    const inset = Math.max(0, 7 - r);
    rect(ctx, '5', x + inset, y + r, w - 2 * inset, 1);
  }

  rect(ctx, '4', x, y + 8, w, h - 8);
  rect(ctx, '0', x + 12, y + 16, 8, 10);
  rect(ctx, '9', x + 10, y + h, 14, 2);
  drawText(ctx, 'CLUCK HQ', x, y + 9, PALETTE['7']);
}

// drawTree draws a tree at its feet: a '4' trunk and a round '3' crown about 20 units wide with
// 'b' highlights on its upper left.
function drawTree(ctx, fx, fy) {
  rect(ctx, '4', fx - 1, fy - 6, 3, 6);
  const r = 10;
  const cy = fy - 16;
  for (let dy = -r; dy <= r; dy++) {
    const half = Math.floor(Math.sqrt(r * r - dy * dy));
    rect(ctx, '3', fx - half, cy + dy, 2 * half + 1, 1);
  }
  rect(ctx, 'b', fx - 6, cy - 6, 2, 2);
  rect(ctx, 'b', fx - 2, cy - 8, 2, 1);
  rect(ctx, 'b', fx - 7, cy - 2, 1, 2);
}

// drawScarecrow draws the scarecrow at its feet (6, 140): a '4' post, a hat, a '9' straw arm
// bar and a '0' crow perched on one arm.
function drawScarecrow(ctx) {
  const fx = 6;
  const fy = 140;
  rect(ctx, '4', fx - 1, fy - 30, 2, 30);
  rect(ctx, '9', fx - 5, fy - 20, 11, 2);
  rect(ctx, '2', fx - 2, fy - 34, 5, 4);
  rect(ctx, '2', fx - 4, fy - 30, 9, 1);
  rect(ctx, 'f', fx - 2, fy - 29, 5, 5);
  rect(ctx, '0', fx - 1, fy - 27, 1, 1);
  rect(ctx, '0', fx + 2, fy - 27, 1, 1);
  drawSprite(ctx, ['00.', '000'], fx + 4, fy - 23);
}

// drawMailbox draws the crooked mailbox at its feet (74, 90) with 'GARY' on its side and a red
// flag on a pole.
function drawMailbox(ctx) {
  const fx = 74;
  const fy = 90;
  rect(ctx, '4', fx - 1, fy - 9, 2, 9);
  rect(ctx, '8', fx - 9, fy - 17, 18, 8);
  drawText(ctx, 'GARY', fx - 7, fy - 15, PALETTE['7']);
  rect(ctx, '5', fx + 9, fy - 18, 1, 9);
  rect(ctx, '8', fx + 10, fy - 18, 2, 3);
}

// drawFenceFront draws the front rail along the bottom edge of a fenced area, with posts.
function drawFenceFront(ctx, area) {
  const bottom = area.y + area.h - 1;
  rect(ctx, '4', area.x, bottom, area.w, 1);
  rect(ctx, 'f', area.x, bottom - 3, area.w, 1);
  for (let x = area.x; x < area.x + area.w; x += 12) {
    rect(ctx, '4', x, bottom - 7, 1, 8);
  }
}

// drawFenceSides draws the left and right sides of a fenced area. A gate, if given as {top,
// bottom}, is a gap in the left side between those y values; null means no gap.
function drawFenceSides(ctx, area, gate) {
  const top = area.y;
  const bottom = area.y + area.h - 1;
  // With no gate the gap is empty: it starts and ends below the bottom of the fence.
  const gateTop = gate ? gate.top : bottom + 1;
  const gateBottom = gate ? gate.bottom : bottom + 1;
  const left = area.x;
  const right = area.x + area.w - 1;

  rect(ctx, '4', left, top, 1, gateTop - top);
  rect(ctx, '4', left, gateBottom, 1, bottom - gateBottom + 1);
  rect(ctx, 'f', left + 1, top, 1, gateTop - top);
  rect(ctx, 'f', left + 1, gateBottom, 1, bottom - gateBottom + 1);
  for (let y = top; y <= bottom; y += 12) {
    if (y >= gateTop && y < gateBottom) continue;
    rect(ctx, '4', left - 1, y, 3, 4);
  }

  rect(ctx, '4', right, top, 1, bottom - top + 1);
  rect(ctx, 'f', right - 1, top, 1, bottom - top + 1);
  for (let y = top; y <= bottom; y += 12) {
    rect(ctx, '4', right - 1, y, 3, 4);
  }
}

// PASTURE_GATE is the gap in the pasture's left side, between y 126 and 140.
const PASTURE_GATE = { top: 126, bottom: 140 };

// drawFenceFor returns the drawables of a fence around a paddock: its front rail and its sides.
// Each has the base line of the paddock's bottom edge.
function drawFenceFor(area, gate) {
  const base = area.y + area.h;
  return [
    { y: base, draw: (ctx) => drawFenceFront(ctx, area) },
    { y: base, draw: (ctx) => drawFenceSides(ctx, area, gate) },
  ];
}

// drawSign draws the FOR SALE sign for the next parcel, at the right edge of the land the farm
// owns: a post, a '0' outline, a '7' board, and the price in '0' under the words.
function drawSign(ctx, world) {
  const x0 = landWidth(world) - 40;
  rect(ctx, '4', x0 + 16, 116, 2, 12);
  rect(ctx, '0', x0 - 1, 103, 38, 18);
  rect(ctx, '7', x0, 104, 36, 16);
  drawText(ctx, 'FOR SALE', x0 + 2, 107, PALETTE['0']);
  drawText(ctx, `$${money(LAND[world.land].cost)}`, x0 + 2, 113, PALETTE['0']);
}

// sceneDrawables returns the standing scenery as {y, draw(ctx)}, where y is the base line (the
// bottom edge) used to sort it with characters. Items that draw later with a greater y appear
// in front of the ones that come before them. The barn and the coop are left out once their
// herd's housing has moved up a tier; the housing drawables take their place. Each fence is
// drawn once the farm owns its parcel, and the FOR SALE sign while there is a parcel to buy.
export function sceneDrawables(world) {
  const trees = [
    { x: 96, y: 66 },
    { x: 172, y: 62 },
    { x: 214, y: 70 },
  ].map((t) => ({ y: t.y, draw: (ctx) => drawTree(ctx, t.x, t.y) }));

  const items = [{ y: HOUSE.y + HOUSE.h, draw: drawHouse }];
  if (world.herds.cow.tier === 0) items.push({ y: BARN.y + BARN.h, draw: drawBarn });
  if (world.herds.chicken.tier === 0) items.push({ y: COOP.y + COOP.h, draw: drawCoop });
  items.push(
    ...trees,
    { y: 140, draw: drawScarecrow },
    { y: 90, draw: drawMailbox },
    ...drawFenceFor(PASTURE, PASTURE_GATE),
  );
  if (world.land >= 2) items.push(...drawFenceFor(MEADOW, null));
  if (world.land >= 3) items.push(...drawFenceFor(HILL, null));
  if (LAND[world.land]) items.push({ y: 128, draw: (ctx) => drawSign(ctx, world) });
  return items;
}
