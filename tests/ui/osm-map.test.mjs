import assert from "node:assert/strict";
import test from "node:test";
import { formatLocationPin, parseLocationPin } from "../../internal/ui/static/scripts/ui/osm-map.js";

test("formatLocationPin and parseLocationPin round-trip coordinates and note", () => {
  const text = formatLocationPin(-1.286389, 36.817223, "Gate B, call on arrival");
  const parsed = parseLocationPin(text);
  assert.ok(parsed);
  assert.equal(parsed.lat, -1.286389);
  assert.equal(parsed.lng, 36.817223);
  assert.equal(parsed.note, "Gate B, call on arrival");
  assert.equal(parsed.url, "https://www.openstreetmap.org/?mlat=-1.286389&mlon=36.817223#map=16/-1.286389/36.817223");
});

test("parseLocationPin ignores regular chat text and out-of-range coordinates", () => {
  assert.equal(parseLocationPin("Hello, when will my order arrive?"), null);
  assert.equal(parseLocationPin("https://www.openstreetmap.org/?mlat=95.0&mlon=36.8"), null);
  assert.equal(parseLocationPin("https://www.openstreetmap.org/?mlat=-1.2&mlon=200.0"), null);
});
