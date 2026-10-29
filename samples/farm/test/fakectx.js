// fakeCtx returns a stand-in for CanvasRenderingContext2D. Every method call is recorded as
// {op: <method name>, args: [...]} in ctx.calls, and every property assignment as
// {op: 'set', name, value}; reading a property returns the last value assigned. Methods return
// undefined, except measureText (returns {width: 0}) and createLinearGradient /
// createRadialGradient (return {addColorStop() {}}). ctx.canvas is {width: 320, height: 180}.
// ctx.count(op) returns how many calls of that method were recorded.
export function fakeCtx() {
  const calls = [];
  const values = new Map();

  const count = (op) => calls.filter((c) => c.op === op).length;

  const results = {
    measureText: () => ({ width: 0 }),
    createLinearGradient: () => ({ addColorStop() {} }),
    createRadialGradient: () => ({ addColorStop() {} }),
  };

  const handler = {
    get(_, name) {
      if (typeof name === 'symbol') return undefined;
      if (name === 'calls') return calls;
      if (name === 'count') return count;
      if (name === 'canvas') return { width: 320, height: 180 };
      if (values.has(name)) return values.get(name);
      const record = (...args) => {
        calls.push({ op: name, args });
        return Object.hasOwn(results, name) ? results[name](...args) : undefined;
      };
      return record;
    },
    set(_, name, value) {
      values.set(name, value);
      calls.push({ op: 'set', name, value });
      return true;
    },
  };

  return new Proxy({}, handler);
}
