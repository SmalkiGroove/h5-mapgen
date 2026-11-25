// Map generator UI: paints a layout, previews it, writes the map. Go methods are bound by Wails as
// window.go.main.App.* (see app.go). Layout cells are stored row by row, row 0 at the bottom (map y up).

const app = () => window.go.main.App;
const $ = (id) => document.getElementById(id);
const GRIDS = [8, 12, 16, 20, 24, 32, 40];
const SIZE_NAMES = { 72: "Tiny", 96: "Small", 136: "Medium", 176: "Large", 216: "Extra large", 256: "Huge", 320: "Impossible" };
const PLAYER_COLORS = ["#d8433a", "#3b6fd8", "#3fa34a", "#d8c03a", "#e0802c", "#9a4ad8", "#35b5b0", "#e06fa8"];
const STORE_KEY = "h5mapgen.layout";
const ROAD_COLOR = [222, 206, 160];
const PATH_TILE = "Grass/Road", PATH_COLOR = [168, 128, 82]; // dirt paths from the buildings to the roads
const RAMP_COLOR = [238, 226, 190];

let setup = null;     // Setup from Go
let L = null;         // current layout
let mode = "biome";   // brush mode: biome | relief | start
let brush = { biome: "grass", relief: "hills", start: 0, object: "mine_gold" };
let brushSize = 1;
let undo = [], redo = [];
let view = null;      // MapView shown in the preview (generated preview or a game map)
let hover = null;     // hovered cell {x, y}
let autoLinks = [];   // roads of the last preview (pairs of player numbers)
let linkFrom = -1;    // Roads brush: index of the first town picked, -1 when none

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
    autoLinks = v.links || [];
    showView(v);
    if ((v.warnings || []).length) status(`\u26a0 ${v.warnings.join(" \u00b7 ")}`, "error");
    drawLayout();
    if (mode === "road") renderPalette();
  }).catch((e) => { if (token === previewToken) { $("busy").textContent = ""; fail(e); } });
}

function syncForm() {
  $("name").value = L.name;
  $("size").value = L.size;
  $("grid").value = L.grid;
  $("seed").value = L.seed;
  $("roads").value = roadMode();
  for (const f of AMOUNT_FIELDS) $(f).value = L[f] || "normal";
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

// ---------- objects ----------
// Objects of the brush, by group: catalog key, name, colour, mark
const OBJECT_GROUPS = [
  ["Mines", [
    ["mine_wood", "Sawmill", "#8b5a2b", "W"], ["mine_ore", "Ore pit", "#7d7d7d", "O"],
    ["mine_mercury", "Alchemist's lab", "#c04040", "M"], ["mine_crystal", "Crystal cavern", "#4fb3c8", "C"],
    ["mine_sulfur", "Sulfur dune", "#d0c030", "S"], ["mine_gem", "Gem pond", "#4caf50", "G"],
    ["mine_gold", "Gold mine", "#e8b020", "$"], ["mine_abandoned", "Abandoned mine", "#555", "?"]]],
  ["Stat givers", [["stat_learning_stone", "Learning stone (+1000 xp)", "#38c0b0", "X"], ["stat_mercenary_camp", "Mercenary camp (+1 attack)", "#38c0b0", "A"], ["stat_marletto_tower", "Marletto tower (+1 defence)", "#38c0b0", "D"], ["stat_star_axis", "Star axis (+1 spellpower)", "#38c0b0", "S"], ["stat_garden_of_revelation", "Garden of revelation (+1 knowledge)", "#38c0b0", "K"], ["stat_arena", "Arena (+2 attack or defence)", "#38c0b0", "2"], ["stat_school_of_magic", "School of magic (+2 spellpower or knowledge)", "#38c0b0", "2"], ["stat_tree_of_knowledge", "Tree of knowledge (a level)", "#38c0b0", "L"], ["stat_library", "Library of enlightenment (neutral areas)", "#38c0b0", "E"], ["stat_war_academy", "War academy (weekly +1)", "#38c0b0", "W"], ["stat_temple", "Temple (weekly experience)", "#38c0b0", "T"], ["stat_tomb_of_the_warrior", "Tomb of the warrior (+1 attack and defence)", "#38c0b0", "+"], ["stat_witch_hut", "Witch hut (weekly brew)", "#38c0b0", "H"]]],
  ["Buildings", [["building_windmill", "Windmill", "#4f8fe0", "W"], ["building_water_wheel", "Water wheel", "#4f8fe0", "$"], ["building_mystical_garden", "Mystical garden", "#4f8fe0", "G"], ["building_magic_well", "Magic well", "#4f8fe0", "M"], ["building_magic_spring", "Magic spring", "#4f8fe0", "M"], ["building_rally_flag", "Rally flag", "#4f8fe0", "F"], ["building_idol_of_fortune", "Idol of fortune", "#4f8fe0", "I"], ["building_fountain_of_fortune", "Fountain of fortune", "#4f8fe0", "L"], ["building_faerie_ring", "Faerie ring", "#4f8fe0", "L"], ["building_fountain_of_youth", "Fountain of youth", "#4f8fe0", "Y"], ["building_stables", "Stables", "#4f8fe0", "S"], ["building_redwood_observatory", "Redwood observatory", "#4f8fe0", "O"], ["building_shrine_1", "Shrine of magic, circle 1", "#4f8fe0", "1"], ["building_shrine_2", "Shrine of magic, circle 2", "#4f8fe0", "2"], ["building_shrine_3", "Shrine of magic, circle 3", "#4f8fe0", "3"], ["building_trading_post", "Trading post", "#4f8fe0", "$"], ["building_tavern", "Tavern", "#4f8fe0", "T"], ["building_black_market", "Black market", "#4f8fe0", "B"], ["building_sanctuary", "Sanctuary", "#4f8fe0", "+"]]],
  ["Dwellings", [["dwelling_1", "Dwelling, level 1 (player's faction)", "#f08a30", "1"], ["dwelling_2", "Dwelling, level 2 (player's faction)", "#f08a30", "2"], ["dwelling_3", "Dwelling, level 3 (player's faction)", "#f08a30", "3"], ["dwelling_military_post", "Military post (player's faction)", "#f08a30", "M"], ["dwelling_refugee_camp", "Refugee camp", "#f08a30", "R"], ["dwelling_elemental_conflux", "Elemental conflux", "#f08a30", "E"]]],
  ["Banks", [["bank_crypt", "Crypt", "#a0522d", "C"], ["bank_treant_thicket", "Treant thicket", "#a0522d", "T"], ["bank_gargoyle_stonevault", "Gargoyle stonevault", "#a0522d", "G"], ["bank_dwarven_treasury", "Dwarven treasury", "#a0522d", "D"], ["bank_naga_bank", "Naga bank", "#a0522d", "N"], ["bank_elemental_stockpile", "Elemental stockpile", "#a0522d", "E"], ["bank_cyclops_stockpile", "Cyclops stockpile", "#a0522d", "Y"], ["bank_blood_temple", "Blood temple", "#a0522d", "B"], ["bank_pyramid", "Pyramid (vault)", "#a0522d", "P"], ["bank_dragon_utopia", "Dragon utopia (vault)", "#a0522d", "U"]]],
  ["Treasures", [
    ["chest", "Chest", "#b8862f", "\u25a3"], ["campfire", "Campfire", "#e06a2c", "\u25b2"], ["res_gold", "Gold pile", "#e8b020", "$"],
    ["res_wood", "Wood", "#8b5a2b", "w"], ["res_ore", "Ore", "#7d7d7d", "o"], ["res_mercury", "Mercury", "#c04040", "m"],
    ["res_crystal", "Crystal", "#4fb3c8", "c"], ["res_sulfur", "Sulfur", "#d0c030", "s"], ["res_gem", "Gems", "#4caf50", "g"]]],
  ["Artifacts", [
    ["artifact_minor", "Minor artifact", "#8a6ad0", "a"], ["artifact_major", "Major artifact", "#7a4ae0", "A"],
    ["artifact_relic", "Relic", "#c040e0", "R"]]],
  ["Monsters", [1, 2, 3, 4, 5, 6, 7].map((n) => [`monster_${n}`, `Monster, level ${n}`, "#c03030", String(n)])],
];
const OBJECTS = Object.fromEntries(OBJECT_GROUPS.flatMap(([, list]) => list.map(([key, name, color, mark]) => [key, { name, color, mark }])));
const objectCategory = (key) => key.startsWith("mine_") ? "Mine" : key.startsWith("stat_") ? "Stat giver" : key.startsWith("building_") ? "Building" : key.startsWith("dwelling_") ? "Dwelling" : key.startsWith("bank_") ? "Bank"
  : key.startsWith("artifact_") ? "Artifact" : key.startsWith("monster_") ? "Monster" : "Treasure";
const CATEGORY_COLORS = { Mine: "#ffffff", "Stat giver": "#38c0b0", Building: "#4f8fe0", Dwelling: "#f08a30", Bank: "#a0522d", Treasure: "#ffd84a", Artifact: "#d070ff", Monster: "#ff3030" };
const BIG_CATEGORIES = ["Mine", "Stat giver", "Building", "Dwelling", "Bank"]; // drawn larger in the preview

// Index of the brush object under a pointer position (cell units), -1 when none
function objectAt(c) {
  const r = $("layout").getBoundingClientRect(), cellPx = r.width / L.grid;
  const reach = Math.max(7, Math.min(12, cellPx * 0.35)) / cellPx + 0.1;
  let best = -1, bestD = reach;
  (L.objects || []).forEach((o, i) => {
    const d = Math.hypot(o.x - c.fx, o.y - c.fy);
    if (d < bestD) { best = i; bestD = d; }
  });
  return best;
}

function drawObjectMark(ctx, x, y, key, r, hot) {
  const o = OBJECTS[key] || { color: "#888", mark: "?" };
  ctx.fillStyle = o.color; ctx.strokeStyle = hot ? "#fff" : "#000c"; ctx.lineWidth = Math.max(1.5, r / 5);
  ctx.beginPath(); ctx.rect(x - r, y - r, 2 * r, 2 * r); ctx.fill(); ctx.stroke();
  ctx.fillStyle = "#fff"; ctx.font = `700 ${Math.round(r * 1.3)}px Segoe UI, sans-serif`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText(o.mark, x, y + r * 0.05);
}

// ---------- drawing ----------
// Reliefs, lowest first: level 0 water, 1 ground, 2 and 3 one and two cliffs up
const RELIEFS = [
  { id: "water", name: "Water", color: "#2f64a8", mark: "≈", level: 0 },
  { id: "flat", name: "Flat", color: "#777", mark: "", level: 1 },
  { id: "hills", name: "Hills", color: "#777", mark: "⌒", level: 1 },
  { id: "plateau", name: "Plateau", color: "#8f8f8f", mark: "▲", level: 2 },
  { id: "high", name: "High plateau", color: "#a8a8a8", mark: "▲▲", level: 3 },
];
const reliefLevel = (id) => (RELIEFS.find((r) => r.id === id) || RELIEFS[1]).level;

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
      const lv = reliefLevel(L.relief[i]);
      if (lv === 0) { ctx.fillStyle = "rgba(38,88,160,0.85)"; ctx.fillRect(sx, sy, w, h); }
      if (lv >= 2) { ctx.fillStyle = `rgba(255,255,255,${lv === 2 ? 0.22 : 0.42})`; ctx.fillRect(sx, sy, w, h); }
      if (L.relief[i] === "hills") { ctx.fillStyle = hatch; ctx.fillRect(sx, sy, w, h); }
    }
  // cliffs and shores: the cell edges between two levels
  ctx.strokeStyle = "rgba(0,0,0,0.6)"; ctx.lineWidth = Math.max(1.5, px / 10); ctx.lineCap = "butt"; ctx.beginPath();
  for (let y = 0; y < g; y++)
    for (let x = 0; x < g; x++) {
      const lv = reliefLevel(L.relief[y * g + x]);
      if (x + 1 < g && reliefLevel(L.relief[y * g + x + 1]) !== lv) {
        const ex = Math.floor((x + 1) * px); ctx.moveTo(ex, Math.floor((g - 1 - y) * px)); ctx.lineTo(ex, Math.floor((g - y) * px));
      }
      if (y + 1 < g && reliefLevel(L.relief[(y + 1) * g + x]) !== lv) {
        const ey = Math.floor((g - 1 - y) * px); ctx.moveTo(Math.floor(x * px), ey); ctx.lineTo(Math.floor((x + 1) * px), ey);
      }
    }
  ctx.stroke();
  if (px >= 8) {
    ctx.strokeStyle = "rgba(0,0,0,0.18)"; ctx.lineWidth = 1; ctx.beginPath();
    for (let k = 1; k < g; k++) {
      const p = Math.floor(k * px) + 0.5;
      ctx.moveTo(p, 0); ctx.lineTo(p, side); ctx.moveTo(0, p); ctx.lineTo(side, p);
    }
    ctx.stroke();
  }
  if (hover && (mode === "biome" || mode === "relief")) {
    ctx.strokeStyle = "#fff"; ctx.lineWidth = Math.max(1.5, side / 400);
    for (const c of brushCells(hover)) ctx.strokeRect(c.x * px + 1, (g - 1 - c.y) * px + 1, px - 2, px - 2);
  }
  const r = Math.max(7, Math.min(16, px * 0.45));
  const at = (p) => [(L.starts[p].x / g) * side, (1 - L.starts[p].y / g) * side];
  ctx.lineCap = "round";
  for (const [a, b] of roadLinks()) {
    if (!L.starts[a - 1] || !L.starts[b - 1]) continue;
    ctx.setLineDash(roadMode() === "auto" ? [r * 0.6, r * 0.6] : []);
    for (const [w, color] of [[r * 0.55, "#0008"], [r * 0.3, "#decea0"]]) {
      ctx.lineWidth = w; ctx.strokeStyle = color;
      ctx.beginPath(); ctx.moveTo(...at(a - 1)); ctx.lineTo(...at(b - 1)); ctx.stroke();
    }
  }
  ctx.setLineDash([]);
  if (mode === "road" && linkFrom >= 0 && hover && L.starts[linkFrom]) {
    ctx.lineWidth = r * 0.25; ctx.strokeStyle = "#fff";
    ctx.beginPath(); ctx.moveTo(...at(linkFrom)); ctx.lineTo((hover.fx / g) * side, (1 - hover.fy / g) * side); ctx.stroke();
  }
  const hot = mode === "object" && hover ? objectAt(hover) : -1;
  (L.objects || []).forEach((o, i) => drawObjectMark(ctx, (o.x / g) * side, (1 - o.y / g) * side, o.kind, r * 0.6, i === hot));
  L.starts.forEach((s, p) => drawStart(ctx, (s.x / g) * side, (1 - s.y / g) * side, p, r,
    (mode === "start" && brush.start === p) || (mode === "road" && linkFrom === p)));
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
    if (b.road === name) return ROAD_COLOR;
    if (b.accents.includes(name)) return hex(b.color).map((v, k) => Math.max(0, Math.min(255, v + ((hash >> (k * 4)) % 31) - 15)));
  }
  if (name === PATH_TILE) return PATH_COLOR;
  if (/road/i.test(name)) return ROAD_COLOR;
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
  viewCache = { dominant: bytes(v.dominant), height: bytes(v.height), levels: bytes(v.levels), ramps: bytes(v.ramps), decor: bytes(v.decor), colors: v.layers.map(tileColor) };
  drawPreview();
  $("legend").innerHTML = "";
  v.layers.forEach((href, i) => {
    const li = document.createElement("li");
    li.innerHTML = `<span class="swatch" style="background:rgb(${viewCache.colors[i].join(",")})"></span><span class="name"></span>`;
    li.querySelector(".name").textContent = tileName(href);
    li.title = href;
    $("legend").append(li);
  });
  const { levels, ramps } = viewCache;
  for (const [name, color, present] of [
    ["Water", "rgb(46,96,168)", levels.some((l) => l === 0)],
    ["Cliff", "rgb(40,40,40)", levels.some((l) => l > 16)],
    ["Ramp", `rgb(${RAMP_COLOR.join(",")})`, ramps.some((r) => r)],
    ["Obstacle", "rgb(40,52,30)", viewCache.decor.some((d) => d === 1)],
    ...Object.entries(CATEGORY_COLORS).map(([cat, color]) => [cat, color, (v.objects || []).some((o) => objectCategory(o.key) === cat)]),
  ]) {
    if (!present) continue;
    const li = document.createElement("li");
    li.innerHTML = `<span class="swatch" style="background:${color}"></span><span class="name">${name}</span>`;
    $("legend").append(li);
  }
}

function drawPreview() {
  if (!view) return;
  const n = view.size + 1, { dominant, height, levels, ramps, decor, colors } = viewCache;
  const img = new ImageData(n, n);
  const clampAt = (a, x, y) => a[Math.max(0, Math.min(n - 1, y)) * n + Math.max(0, Math.min(n - 1, x))];
  const hAt = (x, y) => clampAt(height, x, y);
  const hasLevels = levels.length === n * n;
  for (let y = 0; y < n; y++)
    for (let x = 0; x < n; x++) {
      const i = y * n + x, o = ((n - 1 - y) * n + x) * 4;
      let c = colors[dominant[i]] || [0, 0, 0];
      // light from the top left
      let shade = 1 + ((hAt(x - 1, y) - hAt(x + 1, y)) + (hAt(x, y + 1) - hAt(x, y - 1))) * 0.1;
      if (hasLevels) {
        const lv = levels[i];
        if (lv === 0) { c = [46, 96, 168]; shade = 1; }
        else if (ramps[i]) c = c.map((v, k) => v * 0.4 + RAMP_COLOR[k] * 0.6);
        else if ([[1, 0], [-1, 0], [0, 1], [0, -1]].some(([dx, dy]) => { const o = clampAt(levels, x + dx, y + dy); return o > lv && o > 0 && !clampAt(ramps, x + dx, y + dy); }))
          shade *= 0.35; // foot of a cliff
        shade *= 0.85 + 0.05 * Math.min(4, lv / 16); // higher levels a bit lighter
      }
      if (decor.length === view.size * view.size && x < view.size && y < view.size) {
        const d = decor[y * view.size + x];
        if (d === 1) c = c.map((v, k) => v * 0.45 + [20, 30, 10][k] * 0.55);
        else if (d === 2) shade *= 0.9;
      }
      shade = Math.max(0.3, Math.min(1.35, shade));
      img.data[o] = c[0] * shade; img.data[o + 1] = c[1] * shade; img.data[o + 2] = c[2] * shade; img.data[o + 3] = 255;
    }
  const off = document.createElement("canvas");
  off.width = off.height = n;
  off.getContext("2d").putImageData(img, 0, 0);
  const canvas = $("preview"), ctx = canvas.getContext("2d"), side = fitCanvas(canvas);
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(off, 0, 0, side, side);
  const k = side / view.size, r = Math.max(6, Math.min(14, k * 4));
  for (const o of view.objects || []) {
    const cat = objectCategory(o.key), s = Math.max(2, k * (BIG_CATEGORIES.includes(cat) ? 1.6 : 0.8));
    const x = (o.x + 0.5) * k, y = (view.size - o.y - 0.5) * k;
    ctx.fillStyle = cat === "Mine" ? (OBJECTS[o.key] || {}).color || "#fff" : CATEGORY_COLORS[cat];
    ctx.strokeStyle = "#000"; ctx.lineWidth = Math.max(1, k / 4);
    ctx.beginPath();
    if (cat === "Monster") ctx.arc(x, y, s, 0, Math.PI * 2); else ctx.rect(x - s, y - s, 2 * s, 2 * s);
    ctx.fill(); ctx.stroke();
  }
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
  $("brush-size").style.display = mode === "biome" || mode === "relief" ? "" : "none";
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
    for (const r of RELIEFS) item(r.name, r.color, brush.relief === r.id, () => { brush.relief = r.id; renderPalette(); }, r.mark);
    hint("Plateaus end in cliffs and get ramps by themselves. A town stands on the level of its cell, never in water; water and cliffs can cut roads.");
  } else if (mode === "object") {
    hint("Click the layout to place an object, click an object to remove it. It moves to the nearest tile where it fits; mines, the stronger buildings, chests and artifacts get a guard. The content settings add more by themselves.");
    for (const [group, list] of OBJECT_GROUPS) {
      const h = document.createElement("li"); h.className = "hint group"; h.textContent = group; ul.append(h);
      for (const [key, name, color, mark] of list) item(name, color, brush.object === key, () => { brush.object = key; renderPalette(); }, mark);
    }
    if ((L.objects || []).length) item(`Remove the ${L.objects.length} placed objects`, "#555", false, () => {
      remember(); delete L.objects; renderPalette(); changed();
    }, "\u2715");
  } else if (mode === "road") {
    const auto = roadMode() === "auto";
    hint(auto ? "Automatic roads (dashed). Click a town, then another, to add or remove a road: the roads become chosen ones."
      : roadMode() === "none" ? "No roads. Click a town, then another, to add a road." : "Click a town, then another, to add or remove the road between them.");
    for (const [a, b] of roadLinks()) {
      const li = item(`Player ${a} \u2014 player ${b}`, PLAYER_COLORS[a - 1], false, () => {}, String(a));
      li.style.cursor = "default";
      if (!auto) {
        const del = document.createElement("button");
        del.textContent = "\u2715"; del.title = "Remove this road"; del.className = "icon"; del.style.marginLeft = "auto";
        del.onclick = (e) => { e.stopPropagation(); toggleLink(a, b); };
        li.append(del);
      }
    }
    if (!auto) item("Back to automatic roads", "#555", false, () => {
      remember(); L.roads = "auto"; delete L.links; delete L.noRoads; syncForm(); changed();
    }, "\u21ba");
  } else {
    hint("Pick a player, then click the layout to move its town.");
    L.starts.forEach((s, p) => {
      const li = item(`Player ${p + 1}`, PLAYER_COLORS[p], brush.start === p, () => { brush.start = p; renderPalette(); drawLayout(); }, String(p + 1));
      if (L.starts.length > 1) {
        const del = document.createElement("button");
        del.textContent = "✕"; del.title = "Remove this player"; del.className = "icon"; del.style.marginLeft = "auto";
        del.onclick = (e) => {
          e.stopPropagation(); remember();
          L.starts.splice(p, 1); dropPlayerLinks(p); brush.start = Math.min(brush.start, L.starts.length - 1);
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

// ---------- roads ----------
const roadMode = () => L.roads || (L.noRoads ? "none" : "auto");
// roads shown on the layout: the chosen ones, or those of the last preview in automatic mode
const roadLinks = () => roadMode() === "custom" ? (L.links || []) : roadMode() === "auto" ? autoLinks : [];
const samePair = (k, a, b) => (k[0] === a && k[1] === b) || (k[0] === b && k[1] === a);

// Adds or removes the road between players a and b (1-based); switches the layout to chosen roads
function toggleLink(a, b) {
  remember();
  if (roadMode() !== "custom") L.links = roadMode() === "auto" ? autoLinks.map((k) => [...k]) : [];
  L.roads = "custom"; delete L.noRoads;
  const i = L.links.findIndex((k) => samePair(k, a, b));
  if (i >= 0) L.links.splice(i, 1); else L.links.push([a, b]);
  syncForm(); changed();
}

// Index of the player start under a pointer position (cell units), -1 when none
function startAt(c) {
  const r = $("layout").getBoundingClientRect(), cellPx = r.width / L.grid;
  const reach = Math.max(10, Math.min(16, cellPx * 0.45)) / cellPx + 0.15;
  let best = -1, bestD = reach;
  L.starts.forEach((s, p) => {
    const d = Math.hypot(s.x - c.fx, s.y - c.fy);
    if (d < bestD) { best = p; bestD = d; }
  });
  return best;
}

// Removing player p (0-based) renumbers the chosen roads
function dropPlayerLinks(p) {
  if (!L.links) return;
  L.links = L.links.filter((k) => k[0] !== p + 1 && k[1] !== p + 1).map((k) => k.map((q) => (q > p + 1 ? q - 1 : q)));
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
  if (mode === "object") {
    const c = cellAt(e);
    if (!c.inside) return;
    remember();
    const i = objectAt(c);
    L.objects = L.objects || [];
    if (i >= 0) L.objects.splice(i, 1);
    else L.objects.push({ kind: brush.object, x: Math.round(c.fx * 100) / 100, y: Math.round(c.fy * 100) / 100 });
    if (!L.objects.length) delete L.objects;
    renderPalette(); changed();
    return;
  }
  if (mode === "road") {
    const p = startAt(cellAt(e));
    if (p < 0) linkFrom = -1;
    else if (linkFrom < 0 || linkFrom === p) linkFrom = linkFrom === p ? -1 : p;
    else { toggleLink(linkFrom + 1, p + 1); linkFrom = -1; }
    drawLayout();
    return;
  }
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

document.querySelectorAll("#modes button").forEach((b) => (b.onclick = () => { mode = b.dataset.mode; linkFrom = -1; renderPalette(); drawLayout(); }));
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && linkFrom >= 0) { linkFrom = -1; drawLayout(); } });
document.querySelectorAll("#sizes button").forEach((b) => (b.onclick = () => {
  brushSize = Number(b.dataset.size);
  document.querySelectorAll("#sizes button").forEach((o) => o.classList.toggle("on", o === b));
}));

// ---------- map settings ----------
$("name").oninput = () => { L.name = $("name").value; store(); };
$("name").onchange = () => { remember(); changed(); };
$("size").onchange = () => { remember(); L.size = Number($("size").value); syncForm(); changed(); };
$("seed").onchange = () => { remember(); L.seed = Math.max(0, Math.floor(Number($("seed").value) || 0)); syncForm(); changed(); };
$("roads").onchange = () => {
  remember();
  if ($("roads").value === "custom" && roadMode() !== "custom") L.links = roadLinks().map((k) => [...k]);
  L.roads = $("roads").value; delete L.noRoads;
  renderPalette(); changed();
};
const AMOUNT_FIELDS = ["mines", "treasures", "stats", "buildings", "dwellings", "banks", "guards", "obstacles"];
for (const f of AMOUNT_FIELDS) $(f).onchange = () => { remember(); L[f] = $(f).value; changed(); };
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
  if (L.objects) L.objects = L.objects.map((o) => ({ ...o, x: (o.x * g) / g0, y: (o.y * g) / g0 }));
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
window.addEventListener("resize", () => { if (L) { drawLayout(); drawPreview(); } });

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
