// Source is always passed as data, never as HTML or executable JavaScript.
window.mdflickSVG = async function(source, dark, id) {
  mermaid.initialize({
    startOnLoad: false, securityLevel: 'strict', suppressErrorRendering: true,
    theme: dark ? 'dark' : 'default', fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif',
    htmlLabels: false, flowchart: { htmlLabels: false },
    maxTextSize: 50000, maxEdges: 500,
    secure: ['secure', 'securityLevel', 'startOnLoad', 'maxTextSize', 'maxEdges', 'suppressErrorRendering', 'htmlLabels', 'flowchart']
  });
  const result = await mermaid.render(id, source);
  return result.svg;
};
window.mdflickPNG = async function(source, dark) {
  const svg = await window.mdflickSVG(source, dark, 'diagram');
  const holder = document.createElement('div');
  holder.innerHTML = svg;
  document.body.appendChild(holder);
  try {
    const node = holder.querySelector('svg');
    const box = node.viewBox.baseVal;
    const w = Math.ceil(box.width), h = Math.ceil(box.height);
    if (!(w > 0 && h > 0) || w > 8192 || h > 8192 || w * h > 16000000)
      throw new Error('图表尺寸过大');
    node.setAttribute('width', w); node.setAttribute('height', h);
    const image = new Image();
    image.src = 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(new XMLSerializer().serializeToString(node))));
    await image.decode();
    const scale = Math.min(2, 8192 / w, 8192 / h, Math.sqrt(16000000 / (w * h)));
    const canvas = document.createElement('canvas');
    canvas.width = Math.ceil(w * scale); canvas.height = Math.ceil(h * scale);
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = dark ? '#1f2020' : '#ffffff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
    return {png: canvas.toDataURL('image/png').split(',')[1], width: w, height: h};
  } finally { holder.remove(); }
};
// EvalContext cancels the native wait, not JavaScript execution. Keep a timed
// out PNG evaluation serialized with later ones until its DOM work settles.
const renderPNG = window.mdflickPNG;
let pngPending = Promise.resolve();
window.mdflickPNG = function(source, dark) {
  const result = pngPending.then(() => renderPNG(source, dark));
  pngPending = result.catch(() => {});
  return result;
};
// Quick Look renders visible diagrams first. Offscreen blocks retain source;
// theme changes invalidate them without executing Mermaid until they enter view.
window.mdflickReady = (function() {
  const dark = matchMedia('(prefers-color-scheme: dark)');
  const states = Array.from(document.querySelectorAll('pre.mermaid-source'), (block, i) => ({
    block, i, code: block.textContent, visible: false, rendered: undefined, failed: undefined
  }));
  let pending = Promise.resolve();
  let queued = false;
  function schedule() {
    if (queued) return pending;
    queued = true;
    pending = pending.then(async () => {
      queued = false;
      for (const state of states) {
        const theme = dark.matches;
        if (!state.visible || state.rendered === theme || state.failed === theme) continue;
        try {
          const svg = await window.mdflickSVG(state.code, theme, 'diagram' + state.i);
          // A theme switch while Mermaid runs supersedes this result.
          if (theme !== dark.matches) continue;
          state.block.innerHTML = svg;
          state.block.classList.add('mermaid-rendered');
          state.block.setAttribute('aria-label', 'Mermaid 图表');
          state.block.removeAttribute('title');
          state.rendered = theme;
          state.failed = undefined;
        } catch (_) {
          if (theme !== dark.matches) continue;
          state.block.textContent = state.code;
          state.block.classList.remove('mermaid-rendered');
          state.block.title = 'Mermaid 图表无法渲染，显示源码';
          state.failed = theme;
          state.rendered = undefined;
        }
      }
    });
    return pending;
  }
  for (const state of states) {
    const box = state.block.getBoundingClientRect();
    state.visible = box.top < innerHeight + 200 && box.bottom > -200;
  }
  if (states.length && typeof IntersectionObserver !== 'undefined') {
    const byBlock = new Map(states.map(state => [state.block, state]));
    const observer = new IntersectionObserver(entries => {
      for (const entry of entries) byBlock.get(entry.target).visible = entry.isIntersecting;
      schedule();
    }, { rootMargin: '200px' });
    for (const state of states) observer.observe(state.block);
  } else {
    // Compatibility fallback; current WKWebView supports IntersectionObserver.
    for (const state of states) state.visible = true;
  }
  dark.addEventListener('change', schedule);
  // Initial viewport readiness is deliberately independent of offscreen work.
  return schedule();
})();
