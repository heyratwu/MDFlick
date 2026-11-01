const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { runInNewContext } = require('node:vm');
const script = readFileSync(__dirname + '/render.js', 'utf8');
const flush = () => new Promise(resolve => setImmediate(resolve));

function setup(render) {
  const calls = [];
  const blocks = [0, 3000].map((top, i) => ({
    textContent: 'source ' + i, innerHTML: '', title: '',
    getBoundingClientRect: () => ({ top, bottom: top + 100 }),
    classList: { add() {}, remove() {} }, setAttribute() {}, removeAttribute() {}
  }));
  let intersection, themeChange;
  const dark = { matches: false, addEventListener(_, fn) { themeChange = fn; } };
  const window = {};
  runInNewContext(script, {
    window, innerHeight: 600, matchMedia: () => dark,
    document: { querySelectorAll: () => blocks },
    IntersectionObserver: class { constructor(fn) { intersection = fn; } observe() {} },
    mermaid: { initialize() {}, async render(id, source) {
      calls.push({ id, source, dark: dark.matches });
      return { svg: await render(id, source) };
    } }
  });
  return { window, blocks, calls,
    visible(index, value) { intersection([{ target: blocks[index], isIntersecting: value }]); },
    theme(value) { dark.matches = value; themeChange(); }
  };
}

test('only viewport diagrams render; offscreen themes wait until visible', async () => {
  const s = setup(async id => '<svg>' + id + '</svg>');
  await s.window.mdflickReady;
  assert.equal(s.calls.length, 1);
  s.visible(0, false);
  s.theme(true);
  await flush();
  assert.equal(s.calls.length, 1);
  s.visible(1, true);
  await flush();
  assert.equal(s.calls.length, 2);
  assert.equal(s.calls[1].dark, true);
  s.visible(0, true);
  await flush();
  assert.equal(s.calls.length, 3);
  s.visible(0, true);
  await flush();
  assert.equal(s.calls.length, 3);
});

test('theme changes supersede in-flight results and stay serial', async () => {
  let release;
  let active = 0, peak = 0;
  const s = setup(async () => {
    peak = Math.max(peak, ++active);
    if (!release) await new Promise(resolve => { release = resolve; });
    active--;
    return '<svg>' + active + '</svg>';
  });
  await flush();
  s.theme(true); s.theme(false); s.theme(true);
  release();
  await s.window.mdflickReady;
  await flush();
  assert.equal(peak, 1);
  assert.equal(s.calls.length, 2);
  assert.equal(s.calls[1].dark, true);
});

test('a failed visible diagram keeps source without repeated retries', async () => {
  const s = setup(async () => { throw Error('syntax'); });
  await s.window.mdflickReady;
  assert.equal(s.blocks[0].textContent, 'source 0');
  assert.match(s.blocks[0].title, /无法渲染/);
  s.visible(0, true);
  await flush();
  assert.equal(s.calls.length, 1);
  s.theme(true);
  await flush();
  assert.equal(s.calls.length, 2);
});
