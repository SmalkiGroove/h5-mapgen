// UI of the map generator. Go methods are bound by Wails as window.go.main.App.*

const app = () => window.go.main.App;
const $ = (id) => document.getElementById(id);
let current = null; // MapView of the open map

// Short name of a tile href: "/MapObjects/_(AdvMapTile)/Snow/Snow_(2).xdb#..." -> "Snow/Snow_(2)"
function tileName(href) {
  const m = href.match(/AdvMapTile\)\/(.+?)\.xdb/i);
  return m ? m[1] : href;
}

// Base colours by terrain family (first folder of the tile path); others get a colour from their name
const FAMILIES = {
  grass: [86, 140, 62], dirt: [138, 106, 69], snow: [226, 234, 240], sand: [214, 190, 120],
  lava: [150, 48, 30], subterrain: [96, 80, 96], taiga: [70, 110, 90], swamp: [80, 96, 60],
  water: [40, 80, 140], conquest: [120, 120, 110],
};
function tileColor(href) {
  const name = tileName(href);
  let hash = 0;
  for (const c of name) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
  const base = /road/i.test(name) ? [150, 140, 125] : FAMILIES[name.split("/")[0].toLowerCase()];
  if (base) {
    const jitter = (hash % 41) - 20; // variants of one family stay close
    return base.map((v) => Math.max(0, Math.min(255, v + jitter)));
  }
  const h = (hash % 360) / 360, s = 0.45, l = 0.5; // HSL -> RGB
  const f = (n) => {
    const k = (n + h * 12) % 12;
    return Math.round(255 * (l - s * Math.min(l, 1 - l) * Math.max(-1, Math.min(k - 3, 9 - k, 1))));
  };
  return [f(0), f(8), f(4)];
}

const bytes = (b64) => Uint8Array.from(atob(b64 || ""), (c) => c.charCodeAt(0));

function draw(view) {
  const n = view.size + 1;
  const dominant = bytes(view.dominant), height = bytes(view.height);
  const colors = view.layers.map(tileColor);
  const canvas = $("view");
  canvas.width = n;
  canvas.height = n;
  const ctx = canvas.getContext("2d");
  const img = ctx.createImageData(n, n);
  for (let y = 0; y < n; y++) {
    for (let x = 0; x < n; x++) {
      const i = y * n + x;
      const o = ((n - 1 - y) * n + x) * 4; // row 0 is the bottom of the map
      const c = colors[dominant[i]] || [0, 0, 0];
      const shade = 0.55 + 0.45 * Math.min(1, height[i] / 100); // 2.0 (ground) -> full brightness
      img.data[o] = c[0] * shade;
      img.data[o + 1] = c[1] * shade;
      img.data[o + 2] = c[2] * shade;
      img.data[o + 3] = 255;
    }
  }
  ctx.putImageData(img, 0, 0);
  // Scale up to the stage, keeping whole pixels
  const stage = $("stage").getBoundingClientRect();
  const scale = Math.max(1, Math.floor(Math.min(stage.width - 24, stage.height - 24) / n));
  canvas.style.width = canvas.style.height = `${n * scale}px`;

  $("legend").innerHTML = "";
  view.layers.forEach((href, i) => {
    const li = document.createElement("li");
    li.innerHTML = `<span class="swatch" style="background:rgb(${colors[i].join(",")})"></span><span class="name"></span>`;
    li.querySelector(".name").textContent = tileName(href);
    li.title = href;
    $("legend").append(li);
  });
}

function show(view) {
  if (!view) return;
  current = view;
  $("title").textContent = `${view.path} — ${view.size} × ${view.size} — ${view.folder}`;
  $("status").textContent = "";
  document.querySelectorAll("#maps li").forEach((li) => li.classList.toggle("active", li.dataset.path === view.path));
  draw(view);
}

function fail(error) {
  $("status").innerHTML = `<span id="error"></span>`;
  $("error").textContent = String(error);
}

$("open").onclick = () => app().OpenMap().then(show).catch(fail);

$("view").onmousemove = (e) => {
  if (!current) return;
  const r = e.target.getBoundingClientRect();
  const n = current.size + 1;
  const x = Math.floor(((e.clientX - r.left) / r.width) * n);
  const y = n - 1 - Math.floor(((e.clientY - r.top) / r.height) * n);
  if (x < 0 || y < 0 || x >= n || y >= n) return;
  const layer = bytes(current.dominant)[y * n + x];
  const name = layer < current.layers.length ? tileName(current.layers[layer]) : "none";
  $("status").textContent = `x ${x}, y ${y} — ${name}`;
};
window.onresize = () => current && draw(current);

app().Game().then((info) => {
  $("game").textContent = info.folder ? `Game: ${info.folder}` : "Game folder not found";
  for (const path of info.maps) {
    const li = document.createElement("li");
    li.textContent = path.split(/[\\/]/).pop();
    li.title = path;
    li.dataset.path = path;
    li.onclick = () => app().LoadMap(path).then(show).catch(fail);
    $("maps").append(li);
  }
}).catch(fail);
