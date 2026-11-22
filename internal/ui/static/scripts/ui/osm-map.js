const TILE_SIZE = 256;
const MIN_ZOOM = 3;
const MAX_ZOOM = 18;
const DEFAULT_LAT = -1.286389;
const DEFAULT_LNG = 36.817223;
const DEFAULT_ZOOM = 15;

export function clampLat(lat) {
  return Math.max(-85.05112878, Math.min(85.05112878, lat));
}

export function wrapLng(lng) {
  return ((((lng + 180) % 360) + 360) % 360) - 180;
}

export function latLngToWorld(lat, lng, zoom) {
  const scale = TILE_SIZE * Math.pow(2, zoom);
  const sinLat = Math.sin((clampLat(lat) * Math.PI) / 180);
  const x = ((lng + 180) / 360) * scale;
  const y = (0.5 - Math.log((1 + sinLat) / (1 - sinLat)) / (4 * Math.PI)) * scale;
  return { x, y };
}

export function worldToLatLng(x, y, zoom) {
  const scale = TILE_SIZE * Math.pow(2, zoom);
  const lng = wrapLng((x / scale) * 360 - 180);
  const n = Math.PI - (2 * Math.PI * y) / scale;
  const lat = clampLat((180 / Math.PI) * Math.atan(0.5 * (Math.exp(n) - Math.exp(-n))));
  return { lat, lng };
}

export function osmLink(lat, lng, zoom = 16) {
  const la = Number(lat).toFixed(6);
  const lo = Number(lng).toFixed(6);
  return `https://www.openstreetmap.org/?mlat=${la}&mlon=${lo}#map=${zoom}/${la}/${lo}`;
}

export function formatLocationPin(lat, lng, note = "") {
  const base = `Delivery location: ${osmLink(lat, lng, 16)}`;
  const cleanNote = String(note || "").trim();
  return cleanNote ? `${base}\nNote: ${cleanNote}` : base;
}

const PIN_RE = /https:\/\/www\.openstreetmap\.org\/\?mlat=(-?\d+(?:\.\d+)?)&mlon=(-?\d+(?:\.\d+)?)(?:#map=\d+\/[-\d.]+\/[-\d.]+)?/;

export function parseLocationPin(text) {
  if (!text || typeof text !== "string") return null;
  const match = text.match(PIN_RE);
  if (!match) return null;
  const lat = Number(match[1]);
  const lng = Number(match[2]);
  if (!Number.isFinite(lat) || !Number.isFinite(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180) {
    return null;
  }
  const lines = text.split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
  let note = "";
  for (const line of lines) {
    if (line.startsWith("Note:")) {
      note = line.slice(5).trim();
      break;
    }
  }
  return { lat, lng, url: osmLink(lat, lng, 16), note };
}

const BaseElement = globalThis.HTMLElement || class {};
export class UIOsmMap extends BaseElement {
  connectedCallback() {
    if (this._built) return;
    this._built = true;
    this._lat = Number(this.getAttribute("lat"));
    this._lng = Number(this.getAttribute("lng"));
    if (!Number.isFinite(this._lat)) this._lat = DEFAULT_LAT;
    if (!Number.isFinite(this._lng)) this._lng = DEFAULT_LNG;
    this._zoom = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, Number(this.getAttribute("zoom")) || DEFAULT_ZOOM));
    this._centerLat = this._lat;
    this._centerLng = this._lng;

    this.classList.add("osm-map");
    this.tiles = document.createElement("div");
    this.tiles.className = "osm-map__tiles";
    this.pin = document.createElement("div");
    this.pin.className = "osm-map__pin";
    this.pin.setAttribute("aria-hidden", "true");
    this.attribution = document.createElement("a");
    this.attribution.className = "osm-map__attr";
    this.attribution.href = "https://www.openstreetmap.org/copyright";
    this.attribution.target = "_blank";
    this.attribution.rel = "noopener noreferrer";
    this.attribution.textContent = "© OpenStreetMap";
    this.append(this.tiles, this.pin, this.attribution);

    if (!this.hasAttribute("readonly")) {
      this._initControls();
      this._initPointer();
    }
    this._ro = new ResizeObserver(() => this.render());
    this._ro.observe(this);
    this.render();
  }

  disconnectedCallback() {
    this._ro?.disconnect();
  }

  get lat() { return this._lat; }
  get lng() { return this._lng; }

  setPin(lat, lng, recenter = true) {
    if (!Number.isFinite(lat) || !Number.isFinite(lng)) return;
    this._lat = clampLat(lat);
    this._lng = wrapLng(lng);
    if (recenter) {
      this._centerLat = this._lat;
      this._centerLng = this._lng;
    }
    this.setAttribute("lat", this._lat.toFixed(6));
    this.setAttribute("lng", this._lng.toFixed(6));
    this.render();
    this.dispatchEvent(new CustomEvent("change", { detail: { lat: this._lat, lng: this._lng }, bubbles: true }));
  }

  refresh() {
    this.render();
  }

  _initControls() {
    const controls = document.createElement("div");
    controls.className = "osm-map__controls";
    const zoomIn = document.createElement("button");
    zoomIn.type = "button";
    zoomIn.className = "btn btn-secondary btn-icon btn-sm";
    zoomIn.textContent = "+";
    zoomIn.setAttribute("aria-label", "Zoom in");
    zoomIn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (this._zoom < MAX_ZOOM) { this._zoom++; this.render(); }
    });

    const zoomOut = document.createElement("button");
    zoomOut.type = "button";
    zoomOut.className = "btn btn-secondary btn-icon btn-sm";
    zoomOut.textContent = "−";
    zoomOut.setAttribute("aria-label", "Zoom out");
    zoomOut.addEventListener("click", (e) => {
      e.stopPropagation();
      if (this._zoom > MIN_ZOOM) { this._zoom--; this.render(); }
    });
    controls.append(zoomIn, zoomOut);

    if ("geolocation" in navigator) {
      const locate = document.createElement("button");
      locate.type = "button";
      locate.className = "btn btn-secondary btn-sm";
      locate.textContent = "My location";
      locate.addEventListener("click", (e) => {
        e.stopPropagation();
        locate.disabled = true;
        navigator.geolocation.getCurrentPosition(
          (pos) => {
            locate.disabled = false;
            this._zoom = 16;
            this.setPin(pos.coords.latitude, pos.coords.longitude, true);
          },
          () => { locate.disabled = false; },
          { enableHighAccuracy: true, timeout: 8000, maximumAge: 60000 },
        );
      });
      controls.append(locate);
    }
    this.append(controls);
  }

  _initPointer() {
    let dragging = false;
    let moved = false;
    let startX = 0;
    let startY = 0;
    let startWorld = null;

    this.addEventListener("pointerdown", (e) => {
      if (e.target.closest(".osm-map__controls, .osm-map__attr")) return;
      dragging = true;
      moved = false;
      startX = e.clientX;
      startY = e.clientY;
      startWorld = latLngToWorld(this._centerLat, this._centerLng, this._zoom);
      this.setPointerCapture?.(e.pointerId);
    });

    this.addEventListener("pointermove", (e) => {
      if (!dragging || !startWorld) return;
      const dx = e.clientX - startX;
      const dy = e.clientY - startY;
      if (Math.hypot(dx, dy) > 4) moved = true;
      if (!moved) return;
      const next = worldToLatLng(startWorld.x - dx, startWorld.y - dy, this._zoom);
      this._centerLat = next.lat;
      this._centerLng = next.lng;
      this.render();
    });

    const end = (e) => {
      if (!dragging) return;
      dragging = false;
      if (!moved) {
        const rect = this.getBoundingClientRect();
        const centerWorld = latLngToWorld(this._centerLat, this._centerLng, this._zoom);
        const px = e.clientX - rect.left - rect.width / 2;
        const py = e.clientY - rect.top - rect.height / 2;
        const picked = worldToLatLng(centerWorld.x + px, centerWorld.y + py, this._zoom);
        this.setPin(picked.lat, picked.lng, false);
      }
    };
    this.addEventListener("pointerup", end);
    this.addEventListener("pointercancel", () => { dragging = false; });
  }

  render() {
    const width = this.clientWidth || 320;
    const height = this.clientHeight || 200;
    const center = latLngToWorld(this._centerLat, this._centerLng, this._zoom);
    const topLeftX = center.x - width / 2;
    const topLeftY = center.y - height / 2;
    const maxTile = Math.pow(2, this._zoom);
    const minTX = Math.floor(topLeftX / TILE_SIZE);
    const maxTX = Math.floor((topLeftX + width) / TILE_SIZE);
    const minTY = Math.max(0, Math.floor(topLeftY / TILE_SIZE));
    const maxTY = Math.min(maxTile - 1, Math.floor((topLeftY + height) / TILE_SIZE));

    this.tiles.replaceChildren();
    for (let tx = minTX; tx <= maxTX; tx++) {
      const wrappedX = ((tx % maxTile) + maxTile) % maxTile;
      for (let ty = minTY; ty <= maxTY; ty++) {
        const img = document.createElement("img");
        img.alt = "";
        img.draggable = false;
        img.loading = "lazy";
        img.src = `https://tile.openstreetmap.org/${this._zoom}/${wrappedX}/${ty}.png`;
        img.style.left = `${Math.round(tx * TILE_SIZE - topLeftX)}px`;
        img.style.top = `${Math.round(ty * TILE_SIZE - topLeftY)}px`;
        this.tiles.append(img);
      }
    }

    const pinWorld = latLngToWorld(this._lat, this._lng, this._zoom);
    this.pin.style.left = `${Math.round(pinWorld.x - topLeftX)}px`;
    this.pin.style.top = `${Math.round(pinWorld.y - topLeftY)}px`;
  }
}
