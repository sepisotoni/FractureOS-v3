// Measures how far the web dashboard's JS face drawing is from the C renderer.
// usage: node preview-drift-js.js DEVICE_RGBA_PREFIX OUT_DIR   (see tests/preview_drift_js.sh)
const { JSDOM } = require("jsdom");
const { createCanvas } = require("@napi-rs/canvas");
const fs = require("fs"), path = require("path");
const html = fs.readFileSync(path.join(__dirname, "../../host/ui.html"), "utf8");
const [prefix, outDir] = process.argv.slice(2);
const FIXED = 1767268800000; // 2026-01-01 12:00:00 UTC, same instant as the C harness
const faces = ["aqua", "neon", "sunset"];
const real = {};
const dom = new JSDOM(html, {
  url: "http://127.0.0.1/#x", runScripts: "dangerously", pretendToBeVisual: true,
  beforeParse(w) {
    const D = w.Date;
    w.Date = class extends D { constructor(...a) { super(...(a.length ? a : [FIXED])); } static now() { return FIXED; } };
    w.HTMLCanvasElement.prototype.getContext = function () { if (!this._c) this._c = createCanvas(this.width || 320, this.height || 240); return this._c.getContext("2d"); };
    w.fetch = async () => ({ ok: true, status: 200, text: async () => "{}" });
    w.scrollTo = () => {};
  },
});
setTimeout(() => {
  const w = dom.window;
  const results = [];
  for (const face of faces) {
    const model = fs.readFileSync(`${prefix}-${face}.model`, "utf8");
    const get = k => (model.match(new RegExp("^" + k.replace(".", "\\.") + "=(.*)$", "m")) || [])[1];
    const theme = get("theme_name") || "midnight";
    const canvas = w.document.createElement("canvas"); canvas.width = 320; canvas.height = 240;
    w.eval(`renderHomePreviewCanvas(document.querySelector("canvas") && arguments[0], {})`.replace("arguments[0]", "window.__c"));
    w.__c = canvas;
    w.eval(`renderHomePreviewCanvas(window.__c, {face:${JSON.stringify(face)}, theme:${JSON.stringify(theme)}, profile:"minimal", offset:0, cpu:-1, ram:-1, weather_live:false, timer_seconds:0})`);
    const js = canvas._c.getContext("2d").getImageData(0, 0, 320, 240).data;
    const bgra = fs.readFileSync(`${prefix}-${face}.rgba`); // preview.c emits BGRA (Windows DIB order)
    const dev = Buffer.from(bgra);
    for (let i = 0; i < 320 * 240; i++) { dev[i*4] = bgra[i*4+2]; dev[i*4+2] = bgra[i*4]; }
    let diff = 0, big = 0;
    for (let i = 0; i < 320 * 240; i++) {
      const d = Math.max(Math.abs(js[i*4]-dev[i*4]), Math.abs(js[i*4+1]-dev[i*4+1]), Math.abs(js[i*4+2]-dev[i*4+2]));
      if (d > 0) diff++; if (d > 32) big++;
    }
    const sbs = createCanvas(650, 240), c = sbs.getContext("2d");
    const put = (px, x) => { const im = c.createImageData(320, 240); im.data.set(px); c.putImageData(im, x, 0); };
    c.fillStyle = "#444"; c.fillRect(0, 0, 650, 240);
    put(new Uint8ClampedArray(dev), 0); put(new Uint8ClampedArray(js), 330);
    fs.writeFileSync(path.join(outDir, `web-vs-device-${face}.png`), sbs.toBuffer("image/png"));
    results.push({ face, differing: diff, significant: big, pct: +(100 * diff / 76800).toFixed(2), pctSig: +(100 * big / 76800).toFixed(2) });
  }
  console.log(JSON.stringify(results, null, 1));
  process.exit(0);
}, 400);
