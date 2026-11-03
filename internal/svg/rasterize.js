// Untrusted source is used only as an SVG image, never inserted into the page.
// Browser SVG image mode disables scripts, navigation and external resources,
// while retaining CSS, foreignObject and embedded data images.
window.mdflickRasterizeSVG = async function(base64) {
  const image = new Image();
  image.src = 'data:image/svg+xml;base64,' + base64;
  await image.decode();
  const width = image.naturalWidth, height = image.naturalHeight;
  if (!(width > 0 && height > 0) || width > 8192 || height > 8192 || width * height > 16000000)
    throw new Error('SVG 图片尺寸过大');
  const scale = Math.min(2, 8192 / width, 8192 / height, Math.sqrt(16000000 / (width * height)));
  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.floor(width * scale));
  canvas.height = Math.max(1, Math.floor(height * scale));
  canvas.getContext('2d').drawImage(image, 0, 0, canvas.width, canvas.height);
  return {png: canvas.toDataURL('image/png').split(',')[1], width, height};
};
