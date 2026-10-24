import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";

globalThis.HTMLElement = class {};
const { requestCart, UICartBadge, UICartLine } = await import("../../internal/ui/static/scripts/ui/cart.js");

function setup(t) {
  const requests = [];
  const events = [];
  const document = new EventTarget();
  document.querySelector = () => null;
  document.addEventListener("sokomoko:cart", (event) => events.push(event.detail));
  const previousDocument = globalThis.document;
  globalThis.document = document;
  t.after(() => { globalThis.document = previousDocument; });
  t.mock.method(globalThis, "fetch", (url, options) => new Promise((resolve, reject) => {
    requests.push({
      url, options, reject,
      resolve(cart) { resolve({ ok: true, json: async () => ({ data: cart }) }); },
      fail(status) { resolve({ ok: false, status, json: async () => ({ error: { message: "Cart unavailable" } }) }); },
    });
  }));
  return { requests, events };
}

const cart = (quantity) => ({ items: [{ product_id: 7, quantity }], subtotal: quantity * 10 });

test("cart reads, quantity changes and removal execute and announce in order", async (t) => {
  const { requests, events } = setup(t);
  const read = requestCart("/cart");
  const update = requestCart("/cart/items/7", { method: "PATCH", body: { quantity: 3 } });
  const remove = requestCart("/cart/items/7", { method: "DELETE" });
  await setImmediate();
  assert.equal(requests.length, 1);
  assert.equal(requests[0].options.method, "GET");
  requests[0].resolve(cart(1));
  assert.deepEqual(await read, cart(1));
  await setImmediate();
  assert.equal(requests.length, 2);
  assert.equal(requests[1].url, "/api/v1/cart/items/7");
  assert.equal(requests[1].options.method, "PATCH");
  assert.deepEqual(JSON.parse(requests[1].options.body), { quantity: 3 });
  assert.deepEqual(events.map((event) => event.count), [1]);
  requests[1].resolve(cart(3));
  await update;
  await setImmediate();
  assert.equal(requests.length, 3);
  assert.equal(requests[2].options.method, "DELETE");
  requests[2].resolve({ items: [], subtotal: 0 });
  await remove;
  assert.deepEqual(events.map((event) => event.count), [1, 3, 0]);
});

for (const failure of ["network", "HTTP"]) {
  test(`${failure} failure does not block a queued cart action or announce a failed snapshot`, async (t) => {
    const { requests, events } = setup(t);
    const failed = requestCart("/cart/items/7", { method: "PATCH", body: { quantity: 2 } });
    const rejection = assert.rejects(failed);
    const next = requestCart("/cart/items/7", { method: "DELETE" });
    await setImmediate();
    assert.equal(requests.length, 1);
    if (failure === "network") requests[0].reject(new Error("Connection failed"));
    else requests[0].fail(409);
    await rejection;
    await setImmediate();
    assert.equal(requests.length, 2);
    assert.deepEqual(events, []);
    requests[1].resolve({ items: [], subtotal: 0 });
    await next;
    assert.deepEqual(events.map((event) => event.count), [0]);
  });
}

test("badge initialization shares the request order with a subsequent mutation", async (t) => {
  const { requests } = setup(t);
  const badge = new UICartBadge();
  const count = { hidden: true, textContent: "" };
  const labels = new Map();
  badge.querySelector = (selector) => selector === ".count" ? count : { setAttribute: (key, value) => labels.set(key, value) };
  badge.hasAttribute = () => true;
  badge.connectedCallback();
  const update = requestCart("/cart/items/7", { method: "PATCH", body: { quantity: 3 } });
  await setImmediate();
  assert.equal(requests.length, 1);
  requests[0].resolve(cart(1));
  await setImmediate();
  assert.equal(count.textContent, "1");
  assert.equal(requests.length, 2);
  requests[1].resolve(cart(3));
  await update;
  assert.equal(count.textContent, "3");
  assert.equal(count.hidden, false);
  assert.equal(labels.get("aria-label"), "Cart, 3 items");
  badge.disconnectedCallback();
});


test("a pending removal ignores later quantity autosubmits", async (t) => {
  const { requests } = setup(t);
  globalThis.document.querySelectorAll = () => [];
  t.mock.method(globalThis, "FormData", function(form) {
    return { get() { return form.quantity; } };
  });
  const line = new UICartLine();
  line.productID = "7";
  let removed = 0;
  line.remove = () => { removed++; };
  const event = { preventDefault() {} };
  const removing = line.submit(event, { action: "https://demo.invalid/cart/remove", quantity: null });
  await line.submit(event, { action: "https://demo.invalid/cart/update", quantity: "3" });
  await setImmediate();
  assert.equal(requests.length, 1);
  assert.equal(requests[0].options.method, "DELETE");
  requests[0].resolve({ items: [{ product_id: 8, quantity: 1 }], subtotal: 10 });
  await removing;
  await setImmediate();
  assert.equal(requests.length, 1);
  assert.equal(removed, 1);
  assert.equal(line.removing, false);
});


test("finishing an older update does not unlock a queued removal", async (t) => {
  const { requests } = setup(t);
  globalThis.document.querySelectorAll = () => [];
  t.mock.method(globalThis, "FormData", function(form) {
    return { get() { return form.quantity; } };
  });
  const line = new UICartLine();
  line.productID = "7";
  line.querySelector = () => null;
  line.remove = () => {};
  const event = { preventDefault() {} };
  const updating = line.submit(event, { action: "https://demo.invalid/cart/update", quantity: "2" });
  const removing = line.submit(event, { action: "https://demo.invalid/cart/remove", quantity: null });
  await setImmediate();
  requests[0].resolve(cart(2));
  await updating;
  await setImmediate();
  assert.equal(requests.length, 2);
  assert.equal(line.removing, true);
  await line.submit(event, { action: "https://demo.invalid/cart/update", quantity: "3" });
  requests[1].resolve({ items: [{ product_id: 8, quantity: 1 }], subtotal: 10 });
  await removing;
  await setImmediate();
  assert.equal(requests.length, 2);
  assert.equal(line.removing, false);
});
