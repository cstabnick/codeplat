// makeRng returns a deterministic generator (mulberry32) seeded with the integer seed.
// Same seed, same sequence.
export function makeRng(seed) {
  let a = seed >>> 0;

  function next() {
    a = (a + 0x6D2B79F5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }

  // int(n): integer in [0, n)
  function int(n) {
    return Math.floor(next() * n);
  }

  // range(a, b): float in [a, b)
  function range(a, b) {
    return a + next() * (b - a);
  }

  // pick(array): one element
  function pick(array) {
    return array[int(array.length)];
  }

  // chance(p): true with probability p (p <= 0 is never, p >= 1 is always)
  function chance(p) {
    if (p <= 0) return false;
    if (p >= 1) return true;
    return next() < p;
  }

  return { next, int, pick, chance, range };
}
