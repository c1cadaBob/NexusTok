// 由主站静态 Logo 生成 Electron 托盘图标。
// 运行方式：node create-tray-icon.js

const fs = require('fs');
const path = require('path');
const { createCanvas } = require('canvas');
const { loadImage } = require('canvas');

const logoPath = path.resolve(__dirname, '../web/default/public/logo.png');

function drawResizedLogo(image, size) {
  const canvas = createCanvas(size, size);
  const ctx = canvas.getContext('2d');

  ctx.clearRect(0, 0, size, size);
  ctx.drawImage(image, 0, 0, size, size);

  return canvas.toBuffer('image/png');
}

async function createTrayIcon() {
  // Electron 托盘图标与应用主 Logo 共用同一源文件，避免桌面端继续保留旧占位图。
  const logo = await loadImage(logoPath);

  fs.writeFileSync('tray-iconTemplate.png', drawResizedLogo(logo, 22));
  fs.writeFileSync('tray-iconTemplate@2x.png', drawResizedLogo(logo, 44));
  fs.writeFileSync('tray-icon-windows.png', drawResizedLogo(logo, 32));
  console.log('Tray icon created successfully!');
}

createTrayIcon().catch((err) => {
  console.error('Failed to create tray icon:', err);
  process.exit(1);
});
