// Map generator UI: paints a layout, previews it, writes the map. Go methods are bound by Wails as
// window.go.main.App.* (see app.go). Layout cells are stored row by row, row 0 at the bottom (map y up).

const app = () => window.go.main.App;
const $ = (id) => document.getElementById(id);
const GRIDS = [8, 12, 16, 20, 24, 32, 40];
const SIZE_NAMES = { 72: "Tiny", 96: "Small", 136: "Medium", 176: "Large", 216: "Extra large", 256: "Huge", 320: "Impossible" };
const PLAYER_COLORS = ["#d8433a", "#3b6fd8", "#3fa34a", "#d8c03a", "#e0802c", "#9a4ad8", "#35b5b0", "#e06fa8"];
const STORE_KEY = "h5mapgen.layout";

let setup = null;     // Setup from Go
let L = null;         // current layout
let mode = "biome";   // brush mode: biome | relief | start
let brush = { biome: "grass", relief: "hills", start: 0 };
let brushSize = 1;
let undo = [], redo = [];
let view = null;      // MapView shown in the preview (generated preview or a game map)
let hover = null;     // hovered cell {x, y}

// ---------- status ----------
function status(text, kind = "") {
  $("status").innerHTML = "";
  const span = document.createElement("span");
  span.className = kind;
  span.textContent = text;
  $("status").append(span);
}
const fail = (e) => status(String(e), "error");

// ---------- layout state ----------
const snapshot = () => JSON.stringify(L);
function remember() { undo.push(snapshot()); if (undo.length > 200) undo.shift(); redo = []; updateUndo(); }
function restore(json) { L = JSON.parse(json); syncForm(); changed(); }
function updateUndo() { $("undo").disabled = !undo.length; $("redo").disabled = !redo.length; }

function store() { try { localStorage.setItem(STORE_KEY, snapshot()); } catch (e) { /* storage off */ } }
function stored() { try { return JSON.parse(localStorage.getItem(STORE_KEY)); } catch (e) { return null; } }

let previewTimer = null, previewToken = 0;
function changed() {
  store();
  drawLayout();
  clearTimeout(previewTimer);
  previewTimer = setTimeout(refreshPreview, 200);
}

function refreshPreview() {
  const token = ++previewToken;
  $("busy").textContent = "— generating…";
  app().Preview(L).then((v) => {
    if (token !== previewToken) return;
    $("busy").textContent = "";
    $("preview-title").textContent = `Preview — ${L.size} × ${L.size}`;
    document.querySelectorAll("#maps li").forEach((li) => li.classList.remove("active"));
    showView(v);
  }).catch((e) => { if (token === previewToken) { $("busy").textContent = ""; fail(e); } });
}

function syncForm() {
  $("name").value = L.name;
  $("size").value = L.size;
  $("grid").value = L.grid;
  $("seed").value = L.seed;
  $("cell").textContent = `One cell = ${(L.size / L.grid).toFixed(1)} tiles`;
  renderPalette();
}

function setLayout(layout, keepHistory) {
  if (!keepHistory) { undo = []; redo = []; updateUndo(); }
  L = layout;
  syncForm();
  changed();
}

// ---------- geometry ----------
function fitCanvas(canvas) {
  const frame = canvas.parentElement.getBoundingClientRect();
  const side = Math.max(64, Math.floor(Math.min(frame.width, frame.height)));
  const dpr = window.devicePixelRatio || 1;
  canvas.style.width = canvas.style.height = `${side}px`;
  canvas.width = canvas.height = Math.round(side * dpr);
  return canvas.width;
}

// Cell under a pointer event, y up; also the position in cell units
function cellAt(e) {
  const r = $("layout").getBoundingClientRect();
  const fx = ((e.clientX - r.left) / r.width) * L.grid;
  const fy = (1 - (e.clientY - r.top) / r.height) * L.grid;
  return { x: Math.floor(fx), y: Math.floor(fy), fx, fy, inside: fx >= 0 && fy >= 0 && fx < L.grid && fy < L.grid };
}

function brushCells(c) {
  const lo = -Math.floor((brushSize - 1) / 2), cells = [];
  for (let dy = lo; dy < lo + brushSize; dy++)
    for (let dx = lo; dx < lo + brushSize; dx++) {
      const x = c.x + dx, y = c.y + dy;
      if (x >= 0 && y >= 0 && x < L.grid && y < L.grid) cells.push({ x, y });
    }
  return cells;
}

// ---------- drawing ----------
function hatchPattern(ctx, px) {
  const s = Math.max(6, Math.round(px / 2));
  const tile = document.createElement("canvas");
  tile.width = tile.height = s;
  const t = tile.getContext("2d");
  t.strokeStyle = "rgba(0,0,0,0.35)";
  t.lineWidth = Math.max(1, s / 6);
  t.beginPath(); t.arc(s / 2, s * 0.85, s * 0.35, Math.PI * 1.1, Math.PI * 1.9); t.stroke();
  return ctx.createPattern(tile, "repeat");
}

function drawStart(ctx, x, y, player, r, selected) {
  ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fillStyle = PLAYER_COLORS[player]; ctx.fill();
  ctx.lineWidth = Math.max(1.5, r / 5); ctx.strokeStyle = selected ? "#fff" : "#000a"; ctx.stroke();
  ctx.fillStyle = "#fff"; ctx.font = `700 ${Math.round(r * 1.1)}px Segoe UI, sans-serif`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText(String(player + 1), x, y + r * 0.05);
}

function drawLayout() {
  const canvas = $("layout"), ctx = canvas.getContext("2d");
  const side = fitCanvas(canvas), g = L.grid, px = side / g;
  const colors = Object.fromEntries(setup.biomes.map((b) => [b.id, b.color]));
  const hatch = hatchPattern(ctx, px);
  for (let y = 0; y < g; y++)
    for (let x = 0; x < g; x++) {
      const i = y * g + x, sx = Math.floor(x * px), sy = Math.floor((g - 1 - y) * px);
      const w = Math.floor((x + 1) * px) - sx, h = Math.floor((g - y) * px) - sy;
      ctx.fillStyle = colors[L.biomes[i]] || "#000";
      ctx.fillRect(sx, sy, w, h);
      if (L.relief[i] === "hills") { ctx.fillStyle = hatch; ctx.fillRect(sx, sy, w, h); }
    }
  if (px >= 8) {
    ctx.strokeStyle = "rgba(0,0,0,0.18)"; ctx.lineWidth = 1; ctx.beginPath();
    for (let k = 1; k < g; k++) {
      const p = Math.floor(k * px) + 0.5;
      ctx.moveTo(p, 0); ctx.lineTo(p, side); ctx.moveTo(0, p); ctx.lineTo(side, p);
    }
    ctx.stroke();
  }
  if (hover && mode !== "start") {
    ctx.strokeStyle = "#fff"; ctx.lineWidth = Math.max(1.5, side / 400);
    for (const c of brushCells(hover)) ctx.strokeRect(c.x * px + 1, (g - 1 - c.y) * px + 1, px - 2, px - 2);
  }
  const r = Math.max(7, Math.min(16, px * 0.45));
  L.starts.forEach((s, p) => drawStart(ctx, (s.x / g) * side, (1 - s.y / g) * side, p, r, mode === "start" && brush.start === p));
}

function tileName(href) {
  const m = href.match(/AdvMapTile\)\/(.+?)\.xdb/i);
  return m ? m[1] : href;
}

// Texture colours: the biome colour for its tiles (accents a bit off), else a colour from the name
function tileColor(href) {
  const name = tileName(href);
  let hash = 0;
  for (const c of name) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
  const hex = (h) => [1, 3, 5].map((k) => parseInt(h.slice(k, k + 2), 16));
  for (const b of setup.biomes) {
    if (b.base === name) return hex(b.color);
    if (b.road === name) return [150, 140, 125];
    if (b.accents.includes(name)) return hex(b.color).map((v, k) => Math.max(0, Math.min(255, v + ((hash >> (k * 4)) % 31) - 15)));
  }
  if (/road/i.test(name)) return [150, 140, 125];
  const families = { grass: "#5c8f3e", dirt: "#8a6a45", sand: "#d6be78", snow: "#e4ebf0", lava: "#6e3a2c", subterrain: "#5f5068", orc_terrain: "#7f9a4a", water: "#2f5f9a" };
  const fam = families[name.split("/")[0].toLowerCase()];
  if (fam) return hex(fam).map((v) => Math.max(0, Math.min(255, v + (hash % 41) - 20)));
  const h = (hash % 360) / 360, s = 0.45, l = 0.5;
  const f = (n) => { const k = (n + h * 12) % 12; return Math.round(255 * (l - s * Math.min(l, 1 - l) * Math.max(-1, Math.min(k - 3, 9 - k, 1)))); };
  return [f(0), f(8), f(4)];
}

const bytes = (b64) => Uint8Array.from(atob(b64 || ""), (c) => c.charCodeAt(0));

let viewCache = null; // decoded current view
function showView(v) {
  view = v;
  viewCache = { dominant: bytes(v.dominant), height: bytes(v.height), colors: v.layers.map(tileColor) };
  drawPreview();
  $("legend").innerHTML = "";
  v.layers.forEach((href, i) => {
    const li = document.createElement("li");
    li.innerHTML = `<span class="swatch" style="background:rgb(${viewCache.colors[i].join(",")})"></span><span class="name"></span>`;
    li.querySelector(".name").textContent = tileName(href);
    li.title = href;
    $("legend").append(li);
  });
}

function drawPreview() {
  if (!view) return;
  const n = view.size + 1, { dominant, height, colors } = viewCache;
  const img = new ImageData(n, n);
  const hAt = (x, y) => height[Math.max(0, Math.min(n - 1, y)) * n + Math.max(0, Math.min(n - 1, x))];
  for (let y = 0; y < n; y++)
    for (let x = 0; x < n; x++) {
      const i = y * n + x, o = ((n - 1 - y) * n + x) * 4;
      const c = colors[dominant[i]] || [0, 0, 0];
      // light from the top left; lowered ground (height < 1) darker
      let shade = 1 + ((hAt(x - 1, y) - hAt(x + 1, y)) + (hAt(x, y + 1) - hAt(x, y - 1))) * 0.05;
      if (height[i] < 50) shade *= 0.55;
      shade = Math.max(0.45, Math.min(1.35, shade));
      img.data[o] = c[0] * shade; img.data[o + 1] = c[1] * shade; img.data[o + 2] = c[2] * shade; img.data[o + 3] = 255;
    }
  const off = document.createElement("canvas");
  off.width = off.height = n;
  off.getContext("2d").putImageData(img, 0, 0);
  const canvas = $("preview"), ctx = canvas.getContext("2d"), side = fitCanvas(canvas);
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(off, 0, 0, side, side);
  const k = side / view.size, r = Math.max(6, Math.min(14, k * 4));
  for (const t of view.towns || []) {
    const x = (t.x + 0.5) * k, y = (view.size - t.y - 0.5) * k;
    // entrance tile: (1, -5) turned counter-clockwise t.turns times
    let ex = 1, ey = -5;
    for (let q = 0; q < t.turns; q++) [ex, ey] = [-ey, ex];
    ctx.strokeStyle = "#fff"; ctx.lineWidth = Math.max(2, k / 2);
    ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + ex * k, y - ey * k); ctx.stroke();
    drawStart(ctx, x, y, t.player - 1, r, false);
  }
}

// ---------- palette ----------
function renderPalette() {
  const ul = $("palette");
  ul.innerHTML = "";
  document.querySelectorAll("#modes button").forEach((b) => b.classList.toggle("on", b.dataset.mode === mode));
  $("brush-size").style.display = mode === "start" ? "none" : "";
  const item = (label, color, on, onclick, text = "") => {
    const li = document.createElement("li");
    li.innerHTML = `<span class="swatch" style="background:${color}">${text}</span><span></span>`;
    li.lastChild.textContent = label;
    li.classList.toggle("on", on);
    li.onclick = onclick;
    ul.append(li);
    return li;
  };
  const hint = (text) => { const li = document.createElement("li"); li.className = "hint"; li.textContent = text; ul.append(li); };
  if (mode === "biome") {
    for (const b of setup.biomes) item(b.name, b.color, brush.biome === b.id, () => { brush.biome = b.id; renderPalette(); });
  } else if (mode === "relief") {
    item("Flat", "#777", brush.relief === "flat", () => { brush.relief = "flat"; renderPalette(); });
    item("Hills", "#777", brush.relief === "hills", () => { brush.relief = "hills"; renderPalette(); }, "⌒");
    hint("Water and cliffs come later.");
  } else {
    hint("Pick a player, then click the layout to move its town.");
    L.starts.forEach((s, p) => {
      const li = item(`Player ${p + 1}`, PLAYER_COLORS[p], brush.start === p, () => { brush.start = p; renderPalette(); drawLayout(); }, String(p + 1));
      if (L.starts.length > 1) {
        const del = document.createElement("button");
        del.textContent = "✕"; del.title = "Remove this player"; del.className = "icon"; del.style.marginLeft = "auto";
        del.onclick = (e) => {
          e.stopPropagation(); remember();
          L.starts.splice(p, 1); brush.start = Math.min(brush.start, L.starts.length - 1);
          renderPalette(); changed();
        };
        li.append(del);
      }
    });
    if (L.starts.length < setup.maxPlayers) {
      item("Add a player", "#555", false, () => {
        remember();
        L.starts.push({ x: L.grid / 2, y: L.grid / 2 });
        brush.start = L.starts.length - 1;
        renderPalette(); changed();
      }, "+");
    }
  }
}

// ---------- painting ----------
let painting = false, lastCell = null;
// Paints every cell on the line from the previous pointer position, so fast strokes have no gaps
function strokeTo(c) {
  if (!lastCell || mode === "start") { paintAt(c); lastCell = c; return; }
  const steps = Math.max(Math.abs(c.x - lastCell.x), Math.abs(c.y - lastCell.y), 1);
  for (let k = 1; k <= steps; k++) {
    const x = Math.round(lastCell.x + ((c.x - lastCell.x) * k) / steps), y = Math.round(lastCell.y + ((c.y - lastCell.y) * k) / steps);
    paintAt({ ...c, x, y, inside: x >= 0 && y >= 0 && x < L.grid && y < L.grid });
  }
  lastCell = c;
}
function paintAt(c) {
  if (!c.inside) return;
  if (mode === "start") {
    const s = L.starts[brush.start];
    if (s) { s.x = Math.floor(c.fx) + 0.5; s.y = Math.floor(c.fy) + 0.5; }
  } else {
    const field = mode === "biome" ? L.biomes : L.relief;
    for (const cell of brushCells(c)) field[cell.y * L.grid + cell.x] = brush[mode];
  }
  drawLayout();
}

$("layout").addEventListener("pointerdown", (e) => {
  if (e.button !== 0) return;
  remember();
  painting = true;
  $("layout").setPointerCapture(e.pointerId);
  lastCell = null;
  strokeTo(cellAt(e));
});
$("layout").addEventListener("pointermove", (e) => {
  const c = cellAt(e);
  hover = c.inside ? c : null;
  if (painting) strokeTo(c); else drawLayout();
  if (c.inside) {
    const i = c.y * L.grid + c.x, b = setup.biomes.find((b) => b.id === L.biomes[i]);
    status(`Cell ${c.x}, ${c.y} — tiles ${Math.floor(c.x * L.size / L.grid)}…${Math.floor((c.x + 1) * L.size / L.grid) - 1}, ${Math.floor(c.y * L.size / L.grid)}…${Math.floor((c.y + 1) * L.size / L.grid) - 1} — ${b ? b.name : L.biomes[i]}, ${L.relief[i]}`);
  }
});
const endPaint = () => {
  if (!painting) return;
  painting = false;
  lastCell = null;
  if (undo.length && undo[undo.length - 1] === snapshot()) { undo.pop(); updateUndo(); } // nothing changed
  changed();
};
$("layout").addEventListener("pointerup", endPaint);
$("layout").addEventListener("pointercancel", endPaint);
$("layout").addEventListener("pointerleave", () => { hover = null; if (!painting) drawLayout(); });

document.querySelectorAll("#modes button").forEach((b) => (b.onclick = () => { mode = b.dataset.mode; renderPalette(); drawLayout(); }));
document.querySelectorAll("#sizes button").forEach((b) => (b.onclick = () => {
  brushSize = Number(b.dataset.size);
  document.querySelectorAll("#sizes button").forEach((o) => o.classList.toggle("on", o === b));
}));

// ---------- map settings ----------
$("name").oninput = () => { L.name = $("name").value; store(); };
$("name").onchange = () => { remember(); changed(); };
$("size").onchange = () => { remember(); L.size = Number($("size").value); syncForm(); changed(); };
$("seed").onchange = () => { remember(); L.seed = Math.max(0, Math.floor(Number($("seed").value) || 0)); syncForm(); changed(); };
$("dice").onclick = () => { remember(); L.seed = Math.floor(Math.random() * 1e6); syncForm(); changed(); };
$("grid").onchange = () => {
  remember();
  const g0 = L.grid, g = Number($("grid").value), biomes = [], relief = [];
  for (let y = 0; y < g; y++)
    for (let x = 0; x < g; x++) {
      const i = Math.floor((y * g0) / g) * g0 + Math.floor((x * g0) / g);
      biomes.push(L.biomes[i]); relief.push(L.relief[i]);
    }
  L.starts = L.starts.map((s) => ({ x: (s.x * g) / g0, y: (s.y * g) / g0 }));
  Object.assign(L, { grid: g, biomes, relief });
  syncForm(); changed();
};

$("new").onclick = () => app().NewLayout(L.size, L.grid).then((l) => { remember(); setLayout(l, true); status("New layout"); }).catch(fail);
$("open").onclick = () => app().OpenLayout().then((l) => { if (l) { setLayout(l); status(`Opened ${l.name}`); } }).catch(fail);
$("save").onclick = () => app().SaveLayout(L).then((p) => p && status(`Saved ${p}`, "ok")).catch(fail);
$("generate").onclick = () => {
  $("generate").disabled = true;
  status("Generating…");
  app().GenerateMap(L).then((path) => { status(`Wrote ${path}`, "ok"); loadMaps(); })
    .catch(fail).finally(() => ($("generate").disabled = false));
};
$("undo").onclick = () => { if (undo.length) { redo.push(snapshot()); restore(undo.pop()); updateUndo(); } };
$("redo").onclick = () => { if (redo.length) { undo.push(snapshot()); restore(redo.pop()); updateUndo(); } };
document.addEventListener("keydown", (e) => {
  if (e.target.matches("input, select") || !e.ctrlKey) return;
  if (e.key === "z") { e.preventDefault(); $("undo").click(); }
  if (e.key === "y") { e.preventDefault(); $("redo").click(); }
});
window.addEventListener("resize", () => { drawLayout(); drawPreview(); });

// ---------- game maps ----------
function loadMaps() {
  return app().Setup().then((s) => {
    setup = s;
    $("game").textContent = s.folder ? `Game: ${s.folder}` : "Game folder not found";
    $("maps").innerHTML = "";
    for (const path of s.maps) {
      const li = document.createElement("li");
      li.textContent = path.split(/[\\/]/).pop();
      li.title = `${path}\nClick to view its terrain`;
      li.onclick = () => app().LoadMap(path).then((v) => {
        previewToken++; // drop a pending preview
        $("preview-title").textContent = `${li.textContent} — ${v.size} × ${v.size}`;
        document.querySelectorAll("#maps li").forEach((o) => o.classList.toggle("active", o === li));
        showView(v);
        status(`${path} — ${v.folder} (any layout change shows the preview again)`);
      }).catch(fail);
      $("maps").append(li);
    }
  });
}

// ---------- start ----------
loadMaps().then(() => {
  for (const s of setup.sizes) $("size").add(new Option(`${s} — ${SIZE_NAMES[s] || ""}`, s));
  for (const g of GRIDS) $("grid").add(new Option(`${g} × ${g}`, g));
  brush.biome = setup.biomes[0].id;
  const saved = stored();
  if (saved && saved.biomes && saved.biomes.length === saved.grid * saved.grid) {
    setLayout(saved);
    status("Restored the last layout");
  } else {
    app().NewLayout(136, 16).then((l) => setLayout(l)).catch(fail);
  }
  updateUndo();
}).catch(fail);
