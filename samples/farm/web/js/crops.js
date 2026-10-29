// Crops: watered plots grow over time, the field's soil is drawn, and each planted plot is
// returned as a drawable. The farmer module waters, harvests and replants by setting plot fields
// directly; this module only grows and draws. Imports no globals and touches no window or document
// on load.

import { FIELD, CROPS, puff } from './state.js';
import { drawSprite } from './pixel.js';

const TILE = 16;

// Soil colours are PALETTE keys '4' (tilled soil), '2' (furrow / wet soil) and '5' (border).
const SOIL = '#ab5236';
const WET = '#7e2553';
const BORDER = '#5f574f';
const GLINT = '#fff1e8';

// updateCrops advances every planted plot by dt seconds. A watered plot grows; every plot loses water.
// A plot that becomes ripe in this step gets one sparkle effect at its centre.
export function updateCrops(world, dt) {
  for (const plot of world.plots) {
    if (plot.crop === null) continue;
    const before = plot.stage;
    if (plot.water > 0) {
      plot.growth = Math.min(1, plot.growth + dt / CROPS[plot.crop].grow);
    }
    plot.water = Math.max(0, plot.water - dt / 60);
    plot.stage = plot.growth >= 1 ? 3 : Math.floor(plot.growth * 3);
    if (before !== 3 && plot.stage === 3) {
      const x = FIELD.x + plot.col * TILE + 8;
      const y = FIELD.y + plot.row * TILE + 8;
      puff(world, 'sparkle', x, y);
    }
  }
}

// drawField draws the soil tile of each plot and the '5' outline just outside the grid.
export function drawField(ctx, world) {
  for (const plot of world.plots) {
    const tx = FIELD.x + plot.col * TILE;
    const ty = FIELD.y + plot.row * TILE;
    const wet = plot.water > 0.3;
    ctx.fillStyle = wet ? WET : SOIL;
    ctx.fillRect(tx, ty, TILE, TILE);
    ctx.fillStyle = wet ? SOIL : WET;
    for (const furrow of [3, 7, 11]) {
      ctx.fillRect(tx, ty + furrow, TILE, 1);
    }
  }

  // One-unit outline around the grid, including the corners.
  const left = FIELD.x - 1;
  const top = FIELD.y - 1;
  const width = FIELD.cols * TILE + 2;
  const height = FIELD.rows * TILE + 2;
  ctx.fillStyle = BORDER;
  ctx.fillRect(left, top, width, 1);
  ctx.fillRect(left, top + height - 1, width, 1);
  ctx.fillRect(left, top, 1, height);
  ctx.fillRect(left + width - 1, top, 1, height);
}

// Sprites are arrays of PALETTE keys, '.' for transparent. Each is drawn with its last row at the
// drawable's y and horizontally centred in the plot's tile.
const SPROUT = ['b.b', 'bbb', '.b.'];

const SPRITES = {
  carrot: {
    1: ['b.b.b', '.bbb.', '..b..'],
    2: ['b.bbb.b', 'bbbbbbb', '.3bbb3.', '..bbb..'],
    3: ['.b.b.b.', 'bbbbbbb', '.3b3b3.', '...9...', '..999..'],
  },
  corn: {
    1: ['.3.', '.3.', '33.', '.3.'],
    2: ['..3..', '..3..', '3.3.3', '33.33', '..3..', '..3..', '..3..', '..3..'],
    3: ['..3..', '..3..', '3.3.3', '33.33', '.a3a.', '.a3a.', '..3..', '..3..'],
  },
  pumpkin: {
    1: ['bb..bb', '.bbbbb', 'b....b'],
    2: ['bb..bb', '.bbbbb', '..9b..', 'bb..bb'],
    3: [
      '....4....',
      '..99999..',
      '.9929929.',
      '.9929929.',
      '.9929929.',
      '..99999..',
      '...999...',
    ],
  },
};

function spriteFor(crop, stage) {
  if (stage === 0) return SPROUT;
  return SPRITES[crop][stage];
}

// cropDrawables returns one {y, draw(ctx)} per planted plot. y is the plot's top plus 14.
// Stage-3 crops also draw a blinking glint at the tile's top-right pixel: shown while
// Math.floor(world.time / 0.5) is even.
export function cropDrawables(world) {
  const out = [];
  for (const plot of world.plots) {
    if (plot.crop === null) continue;
    const rows = spriteFor(plot.crop, plot.stage);
    const w = rows[0].length;
    const h = rows.length;
    const tileX = FIELD.x + plot.col * TILE;
    const tileY = FIELD.y + plot.row * TILE;
    const y = tileY + 14;
    const x0 = tileX + Math.floor((TILE - w) / 2);
    const y0 = y - h + 1;
    const ripe = plot.stage === 3;
    out.push({
      y,
      draw(ctx) {
        drawSprite(ctx, rows, x0, y0);
        if (ripe && Math.floor(world.time / 0.5) % 2 === 0) {
          ctx.fillStyle = GLINT;
          ctx.fillRect(tileX + 13, tileY + 2, 1, 1);
        }
      },
    });
  }
  return out;
}
