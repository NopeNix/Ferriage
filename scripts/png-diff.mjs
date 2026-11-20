// Pixel-diff two Chrome screenshots (RGBA8 PNG). Exit 0 when the
// differing-pixel ratio is below the threshold (arg 3, percent).
// Minimal decoder: IHDR/IDAT only, no deps (node:zlib only).
import { readFileSync } from "node:fs";
import { inflateSync } from "node:zlib";

function decode(path) {
  const buf = readFileSync(path);
  let pos = 8;
  const idat = [];
  let width = 0, height = 0, bitDepth = 0, colorType = 0;
  while (pos < buf.length) {
    const len = buf.readUInt32BE(pos);
    const type = buf.toString("ascii", pos + 4, pos + 8);
    const data = buf.subarray(pos + 8, pos + 8 + len);
    if (type === "IHDR") {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8];
      colorType = data[9];
    }
    if (type === "IDAT") idat.push(data);
    pos += 12 + len;
  }
  // Chrome screenshots are RGB (colorType 2) or RGBA (6), 8-bit.
  const bpp = colorType === 6 ? 4 : colorType === 2 ? 3 : 0;
  if (bitDepth !== 8 || bpp === 0) {
    console.error(`unsupported png: bitDepth=${bitDepth} colorType=${colorType}`);
    process.exit(2);
  }
  const raw = inflateSync(Buffer.concat(idat));
  const rowBytes = width * bpp;
  const stride = rowBytes + 1;
  const out = Buffer.alloc(width * height * 4);
  let prev = Buffer.alloc(rowBytes);
  for (let y = 0; y < height; y++) {
    const f = raw[y * stride];
    const cur = Buffer.from(raw.subarray(y * stride + 1, (y + 1) * stride));
    for (let i = 0; i < cur.length; i++) {
      const a = i >= bpp ? cur[i - bpp] : 0;
      const b = prev[i];
      const c = i >= bpp ? prev[i - bpp] : 0;
      if (f === 1) cur[i] = (cur[i] + a) & 255;
      else if (f === 2) cur[i] = (cur[i] + b) & 255;
      else if (f === 3) cur[i] = (cur[i] + ((a + b) >> 1)) & 255;
      else if (f === 4) {
        const p = a + b - c;
        const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c);
        cur[i] = (cur[i] + (pa <= pb && pa <= pc ? a : pb <= pc ? b : c)) & 255;
      }
    }
    for (let x = 0; x < width; x++) {
      const src = x * bpp, dst = (y * width + x) * 4;
      out[dst] = cur[src];
      out[dst + 1] = cur[src + 1];
      out[dst + 2] = cur[src + 2];
      out[dst + 3] = bpp === 4 ? cur[src + 3] : 255;
    }
    prev = cur;
  }
  return { width, height, px: out };
}

const a = decode(process.argv[2]);
const b = decode(process.argv[3]);
const threshold = Number(process.argv[4] ?? 2);
if (a.width !== b.width || a.height !== b.height) {
  console.log(`size mismatch ${a.width}x${a.height} vs ${b.width}x${b.height}`);
  process.exit(1);
}
let diff = 0;
for (let i = 0; i < a.px.length; i += 4) {
  if (
    Math.abs(a.px[i] - b.px[i]) > 24 ||
    Math.abs(a.px[i + 1] - b.px[i + 1]) > 24 ||
    Math.abs(a.px[i + 2] - b.px[i + 2]) > 24
  ) diff++;
}
const pct = (diff / (a.width * a.height)) * 100;
console.log(`${pct.toFixed(2)}% pixels differ (threshold ${threshold}%)`);
process.exit(pct < threshold ? 0 : 1);
