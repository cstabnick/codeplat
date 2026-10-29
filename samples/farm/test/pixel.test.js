// Tests for web/js/pixel.js. The recording context below is defined here on purpose: this file
// does not import test/fakectx.js, so it does not depend on the other worker's files.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  PALETTE,
  spriteSize,
  drawSprite,
  GLYPHS,
  drawText,
  textWidth,
  wrapText,
  drawBubble,
} from '../web/js/pixel.js';

// recorder returns a ctx with only fillStyle and fillRect. Each fillRect call is stored as
// {x, y, w, h, fill} where fill is the fillStyle in effect at that call.
function recorder() {
  const rects = [];
  const ctx = {
    fillStyle: '#000000',
    fillRect(x, y, w, h) {
      rects.push({ x, y, w, h, fill: ctx.fillStyle });
    },
  };
  return { ctx, rects };
}

// pixelsOf expands rects into a set of 'x,y,fill' strings, one per unit pixel, so two drawings
// can be compared regardless of how they were split into fillRect calls.
function pixelsOf(rects) {
  const out = new Set();
  for (const r of rects) {
    for (let i = 0; i < r.w; i++) {
      for (let j = 0; j < r.h; j++) out.add(`${r.x + i},${r.y + j},${r.fill}`);
    }
  }
  return out;
}

// --- spriteSize -----------------------------------------------------------------------------

test('spriteSize returns the width of the first row and the row count', () => {
  assert.deepEqual(spriteSize(['abc', 'def']), { w: 3, h: 2 });
  assert.deepEqual(spriteSize(['a']), { w: 1, h: 1 });
});

test('spriteSize of no rows is zero by zero', () => {
  assert.deepEqual(spriteSize([]), { w: 0, h: 0 });
});

// --- drawSprite -----------------------------------------------------------------------------

test('drawSprite merges a run of one colour in a row into one fillRect', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['aaaa'], 0, 0);
  assert.deepEqual(rects, [{ x: 0, y: 0, w: 4, h: 1, fill: PALETTE.a }]);
});

test('drawSprite draws each colour run at its column and row', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['aa.a', '.bb.'], 0, 0);
  assert.deepEqual(rects, [
    { x: 0, y: 0, w: 2, h: 1, fill: PALETTE.a },
    { x: 3, y: 0, w: 1, h: 1, fill: PALETTE.a },
    { x: 1, y: 1, w: 2, h: 1, fill: PALETTE.b },
  ]);
});

test('drawSprite skips transparent pixels and splits runs on them', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['a.a'], 0, 0);
  assert.deepEqual(rects, [
    { x: 0, y: 0, w: 1, h: 1, fill: PALETTE.a },
    { x: 2, y: 0, w: 1, h: 1, fill: PALETTE.a },
  ]);
});

test('drawSprite skips an unknown character like a transparent pixel', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['aXa'], 0, 0);
  assert.deepEqual(rects, [
    { x: 0, y: 0, w: 1, h: 1, fill: PALETTE.a },
    { x: 2, y: 0, w: 1, h: 1, fill: PALETTE.a },
  ]);
});

test('drawSprite with flip mirrors the sprite left to right', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['ab.'], 0, 0, true);
  assert.deepEqual(rects, [
    { x: 1, y: 0, w: 1, h: 1, fill: PALETTE.b },
    { x: 2, y: 0, w: 1, h: 1, fill: PALETTE.a },
  ]);
});

test('drawSprite without flip leaves the sprite as written', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['ab.'], 0, 0, false);
  assert.deepEqual(rects, [
    { x: 0, y: 0, w: 1, h: 1, fill: PALETTE.a },
    { x: 1, y: 0, w: 1, h: 1, fill: PALETTE.b },
  ]);
});

test('drawSprite offsets by its top-left corner', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['9'], 10, 20);
  assert.deepEqual(rects, [{ x: 10, y: 20, w: 1, h: 1, fill: PALETTE['9'] }]);
});

test('drawSprite rounds x and y with Math.round', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['0'], 2.4, 5.5);
  assert.deepEqual(rects, [{ x: 2, y: 6, w: 1, h: 1, fill: PALETTE['0'] }]);
});

test('drawSprite rounds x up when it is at or above .5', () => {
  const { ctx, rects } = recorder();
  drawSprite(ctx, ['0'], 2.6, 3.4);
  assert.deepEqual(rects, [{ x: 3, y: 3, w: 1, h: 1, fill: PALETTE['0'] }]);
});

// --- GLYPHS ---------------------------------------------------------------------------------

const REQUIRED_GLYPHS = [
  ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
  ...'0123456789',
  ...' .,!?\'"-:;*()/+=%#&~$',
];

test('every GLYPHS entry is 5 strings of 3 characters, each "#" or "."', () => {
  for (const [ch, rows] of Object.entries(GLYPHS)) {
    assert.equal(rows.length, 5, `glyph ${JSON.stringify(ch)} has ${rows.length} rows`);
    for (const row of rows) {
      assert.match(row, /^[#.]{3}$/, `glyph ${JSON.stringify(ch)} row ${JSON.stringify(row)}`);
    }
  }
});

test('GLYPHS covers A-Z, 0-9 and every punctuation character in the contract', () => {
  for (const ch of REQUIRED_GLYPHS) {
    assert.ok(Object.hasOwn(GLYPHS, ch), `missing glyph ${JSON.stringify(ch)}`);
  }
});

test('GLYPHS holds the exact rows the contract gives for a sample of glyphs', () => {
  assert.deepEqual(GLYPHS.A, ['.#.', '#.#', '###', '#.#', '#.#']);
  assert.deepEqual(GLYPHS['0'], ['###', '#.#', '#.#', '#.#', '###']);
  assert.deepEqual(GLYPHS[' '], ['...', '...', '...', '...', '...']);
  assert.deepEqual(GLYPHS['$'], ['.##', '##.', '.#.', '.##', '##.']);
});

// --- drawText -------------------------------------------------------------------------------

test('drawText upper-cases its text', () => {
  const lower = recorder();
  const upper = recorder();
  drawText(lower.ctx, 'hello', 3, 4);
  drawText(upper.ctx, 'HELLO', 3, 4);
  assert.deepEqual(pixelsOf(lower.rects), pixelsOf(upper.rects));
  assert.ok(lower.rects.length > 0);
});

test('drawText advances 4 units per character', () => {
  const both = recorder();
  const a = recorder();
  const b = recorder();
  drawText(both.ctx, 'AB', 0, 0);
  drawText(a.ctx, 'A', 0, 0);
  drawText(b.ctx, 'B', 4, 0);
  const expected = new Set([...pixelsOf(a.rects), ...pixelsOf(b.rects)]);
  assert.deepEqual(pixelsOf(both.rects), expected);
});

test('drawText draws each ink pixel of a glyph at its offset', () => {
  const { ctx, rects } = recorder();
  drawText(ctx, 'I', 10, 20, '#ff004d');
  // I is '###', '.#.', '.#.', '.#.', '###': the middle column is ink in rows 1-3.
  const pixels = pixelsOf(rects);
  assert.equal(pixels.size, 3 + 3 + 3);
  for (let r = 0; r < 5; r++) {
    assert.ok(pixels.has(`${11},${20 + r},#ff004d`), `middle column row ${r}`);
  }
  assert.ok(pixels.has(`${10},${20},#ff004d`));
  assert.ok(pixels.has(`${12},${24},#ff004d`));
});

test('drawText uses the given colour, defaulting to #fff1e8', () => {
  const given = recorder();
  drawText(given.ctx, 'A', 0, 0, '#ff004d');
  assert.ok(given.rects.every((r) => r.fill === '#ff004d'));

  const dflt = recorder();
  drawText(dflt.ctx, 'A', 0, 0);
  assert.ok(dflt.rects.every((r) => r.fill === '#fff1e8'));
});

test('drawText draws a character missing from GLYPHS as "?"', () => {
  const missing = recorder();
  const question = recorder();
  drawText(missing.ctx, '@', 0, 0);
  drawText(question.ctx, '?', 0, 0);
  assert.ok(missing.rects.length > 0);
  assert.deepEqual(pixelsOf(missing.rects), pixelsOf(question.rects));
});

test('drawText starts a new line 6 units lower at the original x on newline', () => {
  const two = recorder();
  const a = recorder();
  const b = recorder();
  drawText(two.ctx, 'A\nB', 5, 7);
  drawText(a.ctx, 'A', 5, 7);
  drawText(b.ctx, 'B', 5, 13);
  assert.deepEqual(pixelsOf(two.rects), new Set([...pixelsOf(a.rects), ...pixelsOf(b.rects)]));
});

// --- textWidth ------------------------------------------------------------------------------

test('textWidth of empty text is 0', () => {
  assert.equal(textWidth(''), 0);
});

test('textWidth of one character is 3', () => {
  assert.equal(textWidth('A'), 3);
});

test('textWidth of HELLO is 19', () => {
  assert.equal(textWidth('HELLO'), 19);
});

test('textWidth of two lines is the width of the longest line', () => {
  assert.equal(textWidth('AB\nHELLO'), 19);
  assert.equal(textWidth('AB\nC'), 7);
});

// --- wrapText -------------------------------------------------------------------------------

test('wrapText of empty text is an empty array', () => {
  assert.deepEqual(wrapText('', 10), []);
});

test('wrapText breaks at spaces to at most maxChars per line', () => {
  assert.deepEqual(wrapText('the quick brown fox', 10), ['the quick', 'brown fox']);
});

test('wrapText keeps a line that fits exactly', () => {
  assert.deepEqual(wrapText('ab cd', 5), ['ab cd']);
});

test('wrapText cuts a single word longer than maxChars at maxChars', () => {
  assert.deepEqual(wrapText('abcdefghij', 4), ['abcd', 'efgh', 'ij']);
});

test('wrapText never returns a line longer than maxChars', () => {
  const lines = wrapText('a bb ccc dddd eeeee ffffff ggggggg hhhhhhhh', 6);
  assert.ok(lines.length > 0);
  for (const line of lines) assert.ok(line.length <= 6, `line too long: ${line}`);
});

// --- drawBubble -----------------------------------------------------------------------------

const KINDS = ['speech', 'thought', 'whisper'];

// boxOf returns the first fillRect of a bubble, which is its box.
function boxOf(rects) {
  return rects[0];
}

test('drawBubble keeps the box inside the canvas at the left edge, the right edge and the top', () => {
  for (const kind of KINDS) {
    for (const x of [0, 320]) {
      const { ctx, rects } = recorder();
      drawBubble(ctx, x, 2, 'HELLO THERE FRIEND', kind);
      const box = boxOf(rects);
      const where = `kind=${kind} x=${x}`;
      assert.ok(box.x >= 1, `box left ${box.x} (${where})`);
      assert.ok(box.x + box.w - 1 <= 318, `box right ${box.x + box.w - 1} (${where})`);
      assert.ok(box.y >= 1, `box top ${box.y} (${where})`);
      for (const r of rects) {
        assert.ok(r.x >= 0 && r.x + r.w - 1 <= 319, `rect ${JSON.stringify(r)} off canvas (${where})`);
        assert.ok(r.y >= 0 && r.y + r.h - 1 <= 179, `rect ${JSON.stringify(r)} off canvas (${where})`);
      }
    }
  }
});

test('drawBubble does not throw for any kind, including an unknown one', () => {
  for (const kind of [...KINDS, 'shout']) {
    const { ctx } = recorder();
    assert.doesNotThrow(() => drawBubble(ctx, 160, 100, 'Hi there', kind));
  }
});

test('drawBubble box is textWidth + 6 wide and lines*6 + 5 tall, centred on x', () => {
  const { ctx, rects } = recorder();
  drawBubble(ctx, 160, 100, 'HI');
  const box = boxOf(rects);
  assert.equal(box.w, textWidth('HI') + 6);
  assert.equal(box.h, 1 * 6 + 5);
  assert.ok(Math.abs(box.x + box.w / 2 - 160) <= 1, `box centre ${box.x + box.w / 2}`);
  assert.equal(box.y, 100 - 3 - box.h + 1);
});

test('drawBubble wraps text to 22 characters per line', () => {
  const { ctx, rects } = recorder();
  // 30 letters: wraps onto two lines, so the box is 2*6+5 tall.
  drawBubble(ctx, 160, 100, 'A'.repeat(30));
  const box = boxOf(rects);
  assert.equal(box.h, 2 * 6 + 5);
});

test('speech bubble has a white fill, black border and a three-unit black tail to (x, y)', () => {
  const { ctx, rects } = recorder();
  drawBubble(ctx, 160, 100, 'HI', 'speech');
  const box = boxOf(rects);
  assert.equal(box.fill, '#fff1e8');
  const tip = rects.find((r) => r.x === 160 && r.y === 100 && r.w === 1 && r.h === 1);
  assert.ok(tip, 'tail tip at (x, y)');
  assert.equal(tip.fill, '#000000');
  // The box bottom is 3 rows above y.
  assert.equal(box.y + box.h - 1, 100 - 3);
});

test('thought bubble has a purple border and two dots (2x2 and 1x1) instead of a tail', () => {
  const { ctx, rects } = recorder();
  drawBubble(ctx, 160, 100, 'HI', 'thought');
  const box = boxOf(rects);
  assert.equal(box.fill, '#fff1e8');
  const big = rects.find((r) => r.w === 2 && r.h === 2);
  const small = rects.find((r) => r.w === 1 && r.h === 1 && r.x === 160 && r.y === 100);
  assert.ok(big, 'a 2x2 dot');
  assert.equal(big.fill, '#83769c');
  assert.ok(small, 'a 1x1 dot at (x, y)');
  assert.equal(small.fill, '#83769c');
  // Dots lie between the box and (x, y).
  assert.ok(big.y > box.y + box.h - 1);
  assert.ok(big.y + big.h - 1 < 100);
});

test('whisper bubble has a grey fill, a dotted border and a three-unit tail', () => {
  const { ctx, rects } = recorder();
  drawBubble(ctx, 160, 100, 'HI', 'whisper');
  const box = boxOf(rects);
  assert.equal(box.fill, '#c2c3c7');
  const tip = rects.find((r) => r.x === 160 && r.y === 100 && r.w === 1 && r.h === 1);
  assert.ok(tip, 'tail tip at (x, y)');
  assert.equal(tip.fill, '#5f574f');
  // Dotted: the top border keeps every other unit, so the unit after the left corner is empty.
  const topBorder = rects.filter((r) => r.y === box.y && r.h === 1 && r.w === 1 && r.fill === '#5f574f');
  const xs = topBorder.map((r) => r.x);
  assert.ok(xs.includes(box.x), 'top-left corner is drawn');
  assert.ok(!xs.includes(box.x + 1), 'the unit after the corner is skipped');
});
