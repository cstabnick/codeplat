import { test } from 'node:test';
import assert from 'node:assert/strict';
import { KINDS, LINES, pickLine } from '../web/js/gags.js';
import { GLYPHS } from '../web/js/pixel.js';
import { makeRng } from '../web/js/rng.js';

const WANT_KINDS = [
  'gossip', 'innocent', 'smoke', 'sneak', 'cough', 'excuse', 'farmer',
  'harvest', 'morning', 'night', 'chicken', 'pig', 'ufo', 'click',
];

test('KINDS is exactly the 14 kinds in the contract', () => {
  assert.equal(KINDS.length, 14);
  assert.deepEqual([...KINDS].sort(), [...WANT_KINDS].sort());
});

test('LINES has an entry for each kind and no others', () => {
  assert.deepEqual(Object.keys(LINES).sort(), [...WANT_KINDS].sort());
});

test('every kind has at least 10 distinct lines', () => {
  for (const kind of KINDS) {
    const lines = LINES[kind];
    assert.ok(Array.isArray(lines), `${kind} is not an array`);
    assert.ok(new Set(lines).size >= 10, `${kind} has ${new Set(lines).size} distinct lines`);
    assert.equal(new Set(lines).size, lines.length, `${kind} has duplicate lines`);
  }
});

test('every line is upper case, at most 44 characters, and uses only GLYPHS keys', () => {
  for (const kind of KINDS) {
    for (const line of LINES[kind]) {
      assert.equal(typeof line, 'string', `${kind}: not a string`);
      assert.equal(line, line.toUpperCase(), `${kind}: not upper case: ${line}`);
      const chars = Array.from(line);
      assert.ok(chars.length <= 44, `${kind}: ${chars.length} characters: ${line}`);
      for (const ch of chars) {
        assert.ok(
          Object.prototype.hasOwnProperty.call(GLYPHS, ch),
          `${kind}: character ${JSON.stringify(ch)} is not in GLYPHS: ${line}`,
        );
      }
    }
  }
});

test('pickLine never returns the same line twice in a row over 200 calls per kind', () => {
  for (const kind of KINDS) {
    const rng = makeRng(1);
    let previous = null;
    for (let i = 0; i < 200; i++) {
      const line = pickLine(kind, rng);
      assert.ok(LINES[kind].includes(line), `${kind}: ${line} is not one of its lines`);
      assert.notEqual(line, previous, `${kind}: repeated ${line} on call ${i}`);
      previous = line;
    }
  }
});

test("pickLine('nope', rng) is '...'", () => {
  assert.equal(pickLine('nope', makeRng(1)), '...');
});
