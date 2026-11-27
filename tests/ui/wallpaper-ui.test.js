// Drives host/ui.html in jsdom with a stubbed hub API.
// Run: cd tests/ui && npm i jsdom && node wallpaper-ui.test.js
const { JSDOM } = require("jsdom");
const fs = require("fs"), path = require("path");
const html = fs.readFileSync(path.join(__dirname, "../../host/ui.html"), "utf8");
const TOKEN = "a".repeat(64);
let failures = 0;
const check = (ok, name) => { console.log((ok ? "PASS " : "FAIL ") + name); if (!ok) failures++; };
const sleep = ms => new Promise(r => setTimeout(r, ms));

async function boot(url, state) {
  const calls = [];
  const dom = new JSDOM(html, {
    url, runScripts: "dangerously", pretendToBeVisual: true,
    beforeParse(w) {
      const ctx = new Proxy({}, { get: (t, k) => (k in t ? t[k] : () => ({ width: 0 })), set: (t, k, v) => { t[k] = v; return true; } });
      w.HTMLCanvasElement.prototype.getContext = () => ctx;
      w.HTMLCanvasElement.prototype.toDataURL = () => "data:image/png;base64,AAAA";
      w.scrollTo = () => {};
      w.fetch = async (p, init = {}) => {
        const body = init.body ? JSON.parse(init.body) : null;
        calls.push({ path: p, body, session: init.headers && init.headers["X-Fracture-Session"] });
        let out = "{}";
        if (p === "/wallpaper") {
          if (body && body.dry_run) out = JSON.stringify({ dry_run: true, active_preview: "data:image/png;base64,AAAA", config: body });
          else if (body && body.operation === "restore") { state.can_restore = false; out = JSON.stringify({ device_applied: true }); }
          else if (body && body.mode) { state.config = { ...body, applied: "k" }; state.can_restore = true; out = JSON.stringify({ device_applied: true, device_connected: true }); }
          else if (body && body.operation === "sync") out = JSON.stringify({ pushed: false, device_connected: true });
          else out = JSON.stringify(state);
        }
        return { ok: true, status: 200, text: async () => out };
      };
    },
  });
  await sleep(500);
  return { dom, calls, w: dom.window, d: dom.window.document };
}
const baseState = () => ({ config: { mode: "color", color: "000000", applied: "k" }, history: [], presets: [{ name: "p1.jpg", preview: "data:image/png;base64,AAAA" }, { name: "p2.jpg", preview: "data:image/png;base64,AAAA" }], pc_available: false, active_preview: "", can_restore: true, device_connected: true });
const visible = (d, sel) => { const e = d.querySelector(sel); return !!e && !e.hidden && e.closest("[hidden]") === null && e.closest(".view").classList.contains("active"); };

(async () => {
  // 1. structure
  let t = await boot("http://127.0.0.1/#" + TOKEN, baseState());
  check(!t.d.querySelector('[data-page="wallpaper"]'), "no top-level Wallpaper nav button");
  check(!!t.d.querySelector('.view[data-view="appearance"] [data-section-panel="wallpaper"] #cardWallpaper'), "wallpaper card lives inside Appearance");
  check(!t.d.querySelector('.view[data-view="wallpaper"]'), "old standalone wallpaper view removed");
  check(t.calls.some(c => c.session === TOKEN), "launch hash is used as the session token");
  t.w.close();

  // 2. deep links
  for (const [url, label] of [
    ["http://127.0.0.1/?page=appearance/wallpaper#" + TOKEN, "launch ?page=appearance/wallpaper#token"],
    ["http://127.0.0.1/?page=wallpaper#" + TOKEN, "legacy launch ?page=wallpaper#token"],
  ]) {
    t = await boot(url, baseState());
    check(visible(t.d, "#cardWallpaper"), label + " opens the wallpaper section");
    check(t.calls.some(c => c.session === TOKEN), label + " keeps the session token");
    check(t.w.location.hash === "#appearance/wallpaper", label + " leaves a clean in-page route hash");
    t.w.close();
  }
  t = await boot("http://127.0.0.1/?page=appearance#" + TOKEN, baseState());
  check(!visible(t.d, "#cardWallpaper") && visible(t.d, "#face"), "?page=appearance opens Clock & faces");
  t.d.querySelector('.subtabs [data-section="wallpaper"]').click();
  check(visible(t.d, "#cardWallpaper") && t.w.location.hash === "#appearance/wallpaper", "Wallpaper tab click shows it and updates hash");
  t.w.location.hash = "#appearance/faces"; await sleep(100);
  check(visible(t.d, "#face"), "hashchange to #appearance/faces switches back");
  t.d.querySelector("[data-section-link]").click();
  check(visible(t.d, "#cardWallpaper"), "findable: link from Clock & faces reaches Wallpaper");
  t.w.close();

  // 3. staging is preview-only; apply is explicit
  const state = baseState();
  t = await boot("http://127.0.0.1/?page=appearance/wallpaper#" + TOKEN, state);
  const $ = id => t.d.getElementById(id);
  check($("applyWallpaper").disabled, "Apply disabled before any change");
  check($("restoreWallpaper").disabled === false, "Restore enabled when a previous selection exists");
  const before = t.calls.length;
  $("wallpaperMode").value = "preset"; $("wallpaperMode").dispatchEvent(new t.w.Event("change"));
  t.d.querySelector("#wallpaperPresetsGrid button").click();
  await sleep(700);
  const staged = t.calls.slice(before).filter(c => c.path === "/wallpaper" && c.body);
  check(staged.length > 0 && staged.every(c => c.body.dry_run === true), "staging sends only dry_run requests (" + staged.length + ")");
  check(staged.some(c => c.body.mode === "preset" && c.body.preset === "p1.jpg"), "dry run carries the chosen preset");
  check($("applyWallpaper").disabled === false && $("discardWallpaper").disabled === false, "Apply/Discard enabled after staging");
  check($("restoreWallpaper").disabled, "Restore disabled while changes are pending");
  check(/Preview only/.test($("wallpaperStatus").textContent), "status says preview only");
  $("applyWallpaper").click(); await sleep(400);
  const real = t.calls.filter(c => c.path === "/wallpaper" && c.body && c.body.mode && !c.body.dry_run);
  check(real.length === 1 && real[0].body.preset === "p1.jpg", "Apply sends exactly one real request");
  check($("applyWallpaper").disabled && /On your TomTom/.test($("wallpaperStatus").textContent), "after apply: clean state, status says on TomTom");
  $("restoreWallpaper").click(); await sleep(300);
  check(t.calls.some(c => c.body && c.body.operation === "restore"), "Restore sends operation=restore");
  t.w.close();

  // 4. discard
  t = await boot("http://127.0.0.1/?page=appearance/wallpaper#" + TOKEN, baseState());
  t.d.getElementById("wallpaperMode").value = "preset"; t.d.getElementById("wallpaperMode").dispatchEvent(new t.w.Event("change"));
  await sleep(500);
  const n = t.calls.filter(c => c.body && c.body.mode && !c.body.dry_run).length;
  t.d.getElementById("discardWallpaper").click(); await sleep(300);
  check(t.calls.filter(c => c.body && c.body.mode && !c.body.dry_run).length === n && t.d.getElementById("wallpaperMode").value === "color", "Discard reverts to saved selection without applying");
  t.w.close();

  console.log(failures ? failures + " FAILED" : "ALL PASSED");
  process.exit(failures ? 1 : 0);
})();
