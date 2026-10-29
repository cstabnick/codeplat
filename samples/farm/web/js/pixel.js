// Pixel-art drawing helpers for a 2D canvas: a palette, sprites written as strings, a 3x5
// pixel font, and speech bubbles. This module imports nothing and touches no globals on load.

// The PICO-8 palette, keyed by the character used in sprite strings.
export const PALETTE = {
  '0': '#000000', '1': '#1d2b53', '2': '#7e2553', '3': '#008751',
  '4': '#ab5236', '5': '#5f574f', '6': '#c2c3c7', '7': '#fff1e8',
  '8': '#ff004d', '9': '#ffa300', 'a': '#ffec27', 'b': '#00e436',
  'c': '#29adff', 'd': '#83769c', 'e': '#ff77a8', 'f': '#ffccaa',
};

const has = (obj, key) => Object.prototype.hasOwnProperty.call(obj, key);
const upper = (text) => String(text).toUpperCase();
const len = (s) => Array.from(s).length;

// A sprite is an array of equal-length strings; each character is a PALETTE key, or '.' for a
// transparent pixel.
export function spriteSize(rows) {
  return { w: rows.length ? rows[0].length : 0, h: rows.length };
}

// drawSprite draws rows with its top-left at (x, y), one canvas unit per character. Runs of one
// colour in a row are drawn as one fillRect. An unknown character is skipped like '.'.
export function drawSprite(ctx, rows, x, y, flip = false) {
  const x0 = Math.round(x);
  const y0 = Math.round(y);
  for (let r = 0; r < rows.length; r++) {
    const cells = Array.from(rows[r]);
    if (flip) cells.reverse();
    let c = 0;
    while (c < cells.length) {
      const ch = cells[c];
      if (!has(PALETTE, ch)) {
        c += 1;
        continue;
      }
      let end = c + 1;
      while (end < cells.length && cells[end] === ch) end += 1;
      ctx.fillStyle = PALETTE[ch];
      ctx.fillRect(x0 + c, y0 + r, end - c, 1);
      c = end;
    }
  }
}

// Each glyph is five rows of three characters, '#' for ink and '.' for none.
const glyph = (s) => s.split(' ');

// GLYPHS maps each supported character to 5 strings of 3 characters.
export const GLYPHS = {
  'A': glyph('.#. #.# ### #.# #.#'),
  'B': glyph('##. #.# ##. #.# ##.'),
  'C': glyph('.## #.. #.. #.. .##'),
  'D': glyph('##. #.# #.# #.# ##.'),
  'E': glyph('### #.. ##. #.. ###'),
  'F': glyph('### #.. ##. #.. #..'),
  'G': glyph('.## #.. #.# #.# .##'),
  'H': glyph('#.# #.# ### #.# #.#'),
  'I': glyph('### .#. .#. .#. ###'),
  'J': glyph('..# ..# ..# #.# .#.'),
  'K': glyph('#.# #.# ##. #.# #.#'),
  'L': glyph('#.. #.. #.. #.. ###'),
  'M': glyph('#.# ### ### #.# #.#'),
  'N': glyph('##. #.# #.# #.# #.#'),
  'O': glyph('.#. #.# #.# #.# .#.'),
  'P': glyph('##. #.# ##. #.. #..'),
  'Q': glyph('.#. #.# #.# ##. .##'),
  'R': glyph('##. #.# ##. #.# #.#'),
  'S': glyph('.## #.. .#. ..# ##.'),
  'T': glyph('### .#. .#. .#. .#.'),
  'U': glyph('#.# #.# #.# #.# ###'),
  'V': glyph('#.# #.# #.# #.# .#.'),
  'W': glyph('#.# #.# ### ### #.#'),
  'X': glyph('#.# #.# .#. #.# #.#'),
  'Y': glyph('#.# #.# .#. .#. .#.'),
  'Z': glyph('### ..# .#. #.. ###'),
  '0': glyph('### #.# #.# #.# ###'),
  '1': glyph('.#. ##. .#. .#. ###'),
  '2': glyph('##. ..# .#. #.. ###'),
  '3': glyph('##. ..# .#. ..# ##.'),
  '4': glyph('#.# #.# ### ..# ..#'),
  '5': glyph('### #.. ##. ..# ##.'),
  '6': glyph('.## #.. ### #.# ###'),
  '7': glyph('### ..# .#. .#. .#.'),
  '8': glyph('### #.# ### #.# ###'),
  '9': glyph('### #.# ### ..# ##.'),
  ' ': glyph('... ... ... ... ...'),
  '.': glyph('... ... ... ... .#.'),
  ',': glyph('... ... ... .#. #..'),
  '!': glyph('.#. .#. .#. ... .#.'),
  '?': glyph('##. ..# .#. ... .#.'),
  "'": glyph('.#. .#. ... ... ...'),
  '"': glyph('#.# #.# ... ... ...'),
  '-': glyph('... ... ### ... ...'),
  ':': glyph('... .#. ... .#. ...'),
  ';': glyph('... .#. ... .#. #..'),
  '*': glyph('... #.# .#. #.# ...'),
  '(': glyph('..# .#. .#. .#. ..#'),
  ')': glyph('#.. .#. .#. .#. #..'),
  '/': glyph('..# ..# .#. #.. #..'),
  '+': glyph('... .#. ### .#. ...'),
  '=': glyph('... ### ... ### ...'),
  '%': glyph('#.# ..# .#. #.. #.#'),
  '#': glyph('#.# ### #.# ### #.#'),
  '&': glyph('.#. #.# .#. #.# .##'),
  '~': glyph('... ... .## ##. ...'),
  '$': glyph('.## ##. .#. .## ##.'),
};

// drawText draws text with its top-left at (x, y). The text is upper-cased; each glyph is 3x5
// and advances 4 units; '\n' starts a new line 6 units lower; unknown characters are drawn as '?'.
export function drawText(ctx, text, x, y, color = '#fff1e8') {
  ctx.fillStyle = color;
  upper(text).split('\n').forEach((line, li) => {
    Array.from(line).forEach((ch, ci) => {
      const rows = has(GLYPHS, ch) ? GLYPHS[ch] : GLYPHS['?'];
      const gx = x + 4 * ci;
      const gy = y + 6 * li;
      rows.forEach((row, r) => {
        for (let c = 0; c < row.length; c++) {
          if (row[c] === '#') ctx.fillRect(gx + c, gy + r, 1, 1);
        }
      });
    });
  });
}

// textWidth is the width in units of the longest line: 4 * chars - 1, or 0 for empty text.
export function textWidth(text) {
  const longest = Math.max(0, ...upper(text).split('\n').map(len));
  return longest === 0 ? 0 : 4 * longest - 1;
}

// wrapText splits text into lines of at most maxChars characters, breaking at spaces; a word
// longer than maxChars is cut. A '\n' also starts a new line. Returns [] for ''.
export function wrapText(text, maxChars) {
  if (text === '') return [];
  const width = Math.max(1, Math.floor(maxChars));
  const out = [];
  for (const paragraph of String(text).split('\n')) {
    const words = paragraph.split(' ').filter((w) => w !== '');
    if (words.length === 0) {
      out.push('');
      continue;
    }
    let cur = '';
    for (const original of words) {
      let chars = Array.from(original);
      let word = original;
      if (chars.length > width) {
        if (cur !== '') {
          out.push(cur);
          cur = '';
        }
        while (chars.length > width) {
          out.push(chars.slice(0, width).join(''));
          chars = chars.slice(width);
        }
        word = chars.join('');
      }
      if (cur === '') cur = word;
      else if (len(cur) + 1 + len(word) <= width) cur += ` ${word}`;
      else {
        out.push(cur);
        cur = word;
      }
    }
    if (cur !== '') out.push(cur);
  }
  return out;
}

const BUBBLE_STYLE = {
  speech: { fill: '#fff1e8', border: '#000000', text: '#000000' },
  thought: { fill: '#fff1e8', border: '#83769c', text: '#000000' },
  whisper: { fill: '#c2c3c7', border: '#5f574f', text: '#1d2b53' },
};

// strokeBox draws a one-unit outline of a box; a dotted outline keeps every other unit.
function strokeBox(ctx, left, top, w, h, color, dotted) {
  const right = left + w - 1;
  const bottom = top + h - 1;
  ctx.fillStyle = color;
  for (let i = 0; i < w; i++) {
    if (!dotted || i % 2 === 0) {
      ctx.fillRect(left + i, top, 1, 1);
      ctx.fillRect(left + i, bottom, 1, 1);
    }
  }
  for (let j = 0; j < h; j++) {
    if (!dotted || j % 2 === 0) {
      ctx.fillRect(left, top + j, 1, 1);
      ctx.fillRect(right, top + j, 1, 1);
    }
  }
}

// drawBubble draws a comic bubble whose tail points at (x, y), with the box above it. The text
// is upper-cased and wrapped to 22 characters per line. The box is centred on x but kept on
// columns 1..318 and at row 1 or below.
export function drawBubble(ctx, x, y, text, kind = 'speech') {
  const cx = Math.round(x);
  const cy = Math.round(y);
  const style = has(BUBBLE_STYLE, kind) ? BUBBLE_STYLE[kind] : BUBBLE_STYLE.speech;
  const lines = wrapText(upper(text), 22);
  const joined = lines.join('\n');
  const w = textWidth(joined) + 6;
  const h = lines.length * 6 + 5;
  const left = Math.min(Math.max(1, Math.round(cx - w / 2)), 319 - w);
  // The box's bottom row is 3 rows above the tip, so the tail fills the three rows below it.
  const top = Math.max(1, cy - 2 - h);

  ctx.fillStyle = style.fill;
  ctx.fillRect(left, top, w, h);
  strokeBox(ctx, left, top, w, h, style.border, kind === 'whisper');

  // The tail and dots stay on columns 1..318 too, so a tip at the canvas edge does not leave it.
  const col = (c) => Math.min(318, Math.max(1, c));
  if (kind === 'thought') {
    ctx.fillStyle = style.border;
    ctx.fillRect(col(cx - 1), cy - 2, col(cx) - col(cx - 1) + 1, 2);
    ctx.fillRect(col(cx), cy, 1, 1);
  } else {
    for (let k = 0; k < 3; k++) {
      const hw = 2 - k;
      const ty = cy - 2 + k;
      if (hw === 0) {
        ctx.fillStyle = style.border;
        ctx.fillRect(col(cx), ty, 1, 1);
      } else {
        ctx.fillStyle = style.fill;
        ctx.fillRect(col(cx - hw), ty, col(cx + hw) - col(cx - hw) + 1, 1);
        ctx.fillStyle = style.border;
        ctx.fillRect(col(cx - hw), ty, 1, 1);
        ctx.fillRect(col(cx + hw), ty, 1, 1);
      }
    }
  }

  drawText(ctx, joined, left + 3, top + 3, style.text);
}
