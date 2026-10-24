import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";

// Exercise request ordering independently of browser layout. The component's
// API helper and async control flow are real; only DOM presentation is stubbed.
globalThis.HTMLElement = class {};
const { UISearch } = await import("../../internal/ui/static/scripts/ui/search.js");

function setup(t) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const requests = [];
  t.mock.method(globalThis, "fetch", () => new Promise((resolve, reject) => {
    requests.push({
      resolve(items) { resolve({ ok: true, json: async () => ({ data: { items } }) }); },
      reject,
    });
  }));
  const search = new UISearch();
  const attributes = new Map();
  search.input = {
    value: "",
    setAttribute(key, value) { attributes.set(key, value); },
    removeAttribute(key) { attributes.delete(key); },
  };
  search.list = { hidden: true, querySelectorAll() { return []; } };
  const rendered = [];
  search.render = (items, query) => {
    rendered.push({ items, query });
    search.list.hidden = false;
    search.input.setAttribute("aria-expanded", "true");
  };
  const type = (value) => { search.input.value = value; search.schedule(); };
  const start = (value) => { type(value); t.mock.timers.tick(200); };
  return { search, attributes, requests, rendered, type, start };
}

test("clearing input prevents an in-flight response reopening suggestions", async (t) => {
  const { search, attributes, requests, rendered, type, start } = setup(t);
  start("headphones");
  type("");
  requests[0].resolve([{ slug: "old" }]);
  await setImmediate();
  assert.deepEqual(rendered, []);
  assert.equal(search.list.hidden, true);
  assert.equal(attributes.get("aria-expanded"), "false");
});

test("new input invalidates old results during debounce", async (t) => {
  const { requests, rendered, type, start } = setup(t);
  start("headphones");
  type("speaker");
  requests[0].resolve([{ slug: "old" }]);
  await setImmediate();
  assert.deepEqual(rendered, []);
  t.mock.timers.tick(200);
  requests[1].resolve([{ slug: "speaker" }]);
  await setImmediate();
  assert.deepEqual(rendered, [{ items: [{ slug: "speaker" }], query: "speaker" }]);
});

test("old failures cannot hide a newer successful result", async (t) => {
  const { search, requests, rendered, start } = setup(t);
  start("headphones");
  start("speaker");
  requests[1].resolve([{ slug: "speaker" }]);
  await setImmediate();
  requests[0].reject(new Error("old network failure"));
  await setImmediate();
  assert.equal(rendered.length, 1);
  assert.equal(search.list.hidden, false);
});

for (const dismiss of ["close", "Escape", "disconnect"]) {
  test(`${dismiss} prevents late responses and cancels pending debounce`, async (t) => {
    const { search, requests, rendered, start, type } = setup(t);
    const dismissSearch = () => {
      if (dismiss === "Escape") search.key({ key: "Escape" });
      else if (dismiss === "disconnect") search.disconnectedCallback();
      else search.close();
    };
    start("headphones");
    dismissSearch();
    requests[0].resolve([{ slug: "old" }]);
    await setImmediate();
    assert.deepEqual(rendered, []);
    assert.equal(search.list.hidden, true);
    type("speaker");
    dismissSearch();
    t.mock.timers.tick(200);
    assert.equal(requests.length, 1);
  });
}

test("an older success cannot replace the latest successful suggestions", async (t) => {
  const { requests, rendered, start } = setup(t);
  start("headphones");
  start("speaker");
  requests[1].resolve([{ slug: "speaker" }]);
  await setImmediate();
  requests[0].resolve([{ slug: "old" }]);
  await setImmediate();
  assert.deepEqual(rendered, [{ items: [{ slug: "speaker" }], query: "speaker" }]);
});

test("a current failure closes the dropdown and clears active option state", async (t) => {
  const { search, attributes, requests, start } = setup(t);
  start("headphones");
  search.list.hidden = false;
  attributes.set("aria-activedescendant", "old-option");
  requests[0].reject(new Error("current network failure"));
  await setImmediate();
  assert.equal(search.list.hidden, true);
  assert.equal(attributes.get("aria-expanded"), "false");
  assert.equal(attributes.has("aria-activedescendant"), false);
});
