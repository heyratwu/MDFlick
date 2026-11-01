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
// Quick Look keeps the original code if a diagram fails to render.
window.mdflickReady = (async function() {
  const blocks = Array.from(document.querySelectorAll('pre.mermaid-source'));
  const dark = matchMedia('(prefers-color-scheme: dark)');
  async function render() {
    for (let i = 0; i < blocks.length; i++) {
      const block = blocks[i];
      const code = block.dataset.source || block.textContent;
      block.dataset.source = code;
      try {
        const svg = await window.mdflickSVG(code, dark.matches, 'diagram' + i);
        block.innerHTML = svg;
        block.classList.add('mermaid-rendered');
        block.setAttribute('aria-label', 'Mermaid 图表');
      } catch (_) {
        block.textContent = code;
        block.classList.remove('mermaid-rendered');
        block.title = 'Mermaid 图表无法渲染，显示源码';
      }
    }
  }
  // Serialize theme changes with ongoing Mermaid work.
  let pending = render();
  dark.addEventListener('change', () => { pending = pending.then(render); });
  await pending;
})();
