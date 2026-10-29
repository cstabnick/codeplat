import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeRng } from '../web/js/rng.js';

test('makeRng gives the mulberry32 sequence for a seed', () => {
  const rng = makeRng(1);
  const got = [rng.next(), rng.next(), rng.next()];
  const want = [0.6270739405881613, 0.002735721180215478, 0.5274470399599522];
  for (let i = 0; i < want.length; i++) {
    assert.ok(Math.abs(got[i] - want[i]) < 1e-12, `draw ${i}: ${got[i]} != ${want[i]}`);
  }
});

test('makeRng gives the same sequence for the same seed and a different one for another seed', () => {
  const a = makeRng(99);
  const b = makeRng(99);
  const c = makeRng(100);
  const seqA = Array.from({ length: 20 }, () => a.next());
  const seqB = Array.from({ length: 20 }, () => b.next());
  const seqC = Array.from({ length: 20 }, () => c.next());
  assert.deepEqual(seqA, seqB);
  assert.notDeepEqual(seqA, seqC);
});

test('next stays in [0, 1)', () => {
  const rng = makeRng(5);
  for (let i = 0; i < 10000; i++) {
    const v = rng.next();
    assert.ok(v >= 0 && v < 1, `out of range: ${v}`);
  }
});

test('int(n) is an integer in [0, n) and covers the range', () => {
  const rng = makeRng(7);
  const seen = new Set();
  for (let i = 0; i < 2000; i++) {
    const v = rng.int(7);
    assert.ok(Number.isInteger(v), `not an integer: ${v}`);
    assert.ok(v >= 0 && v < 7, `out of range: ${v}`);
    seen.add(v);
  }
  assert.equal(seen.size, 7);
});

test('range(a, b) is a float in [a, b)', () => {
  const rng = makeRng(11);
  for (let i = 0; i < 5000; i++) {
    const v = rng.range(-3, 4);
    assert.ok(v >= -3 && v < 4, `out of range: ${v}`);
  }
});

test('pick returns an element of the array', () => {
  const rng = makeRng(3);
  const items = ['a', 'b', 'c', 'd'];
  const seen = new Set();
  for (let i = 0; i < 500; i++) {
    const v = rng.pick(items);
    assert.ok(items.includes(v), `not from the array: ${v}`);
    seen.add(v);
  }
  assert.equal(seen.size, items.length);
});

test('chance(0) is never, chance(1) is always, and chance(p) follows p', () => {
  const rng = makeRng(42);
  for (let i = 0; i < 1000; i++) {
    assert.equal(rng.chance(0), false);
    assert.equal(rng.chance(-0.5), false);
    assert.equal(rng.chance(1), true);
    assert.equal(rng.chance(2), true);
  }
  let hits = 0;
  const n = 20000;
  for (let i = 0; i < n; i++) if (rng.chance(0.3)) hits++;
  const frac = hits / n;
  assert.ok(frac > 0.27 && frac < 0.33, `chance(0.3) hit rate ${frac}`);
});
