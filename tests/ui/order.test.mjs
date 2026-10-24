import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";

// Keep the real rendering and polling code; supply only the DOM operations it
// uses so asynchronous state transitions can be tested without a browser.
class Element {
  constructor() { this.attributes = new Map(); this.dataset = {}; this.children = []; }
  getAttribute(key) { return this.attributes.get(key) ?? null; }
  setAttribute(key, value) { this.attributes.set(key, String(value)); }
  append(child) { this.children.push(child); }
  querySelector() { return this.children[0] || null; }
  replaceChildren(...children) { this.children = children; }
  closest() { return null; }
}
globalThis.HTMLElement = Element;
const { UIOrderProgress } = await import("../../internal/ui/static/scripts/ui/order.js");

function setup(t, status = "processing") {
  t.mock.timers.enable({ apis: ["setInterval"] });
  const previousDocument = globalThis.document;
  const document = { hidden: false, createElement: () => new Element() };
  globalThis.document = document;
  const requests = [];
  t.mock.method(globalThis, "fetch", () => new Promise((resolve, reject) => {
    requests.push({
      reject,
      resolve(order) { resolve({ ok: true, json: async () => ({ data: order }) }); },
    });
  }));
  const progress = new UIOrderProgress();
  progress.setAttribute("order-id", "7");
  progress.setAttribute("status", status);
  progress.setAttribute("partner-status", "accepted");
  progress.setAttribute("delivery-status", "processing");
  progress.connectedCallback();
  t.after(() => { progress.disconnectedCallback(); globalThis.document = previousDocument; });
  return { progress, requests, document };
}

const shipped = { status: "shipped", partner_status: "dispatched", delivery_status: "shipped" };
const delivered = { status: "delivered", partner_status: "accepted", delivery_status: "processing" };

for (const status of ["delivered", "cancelled"]) {
  test(`${status} order is terminal even with stale fulfillment fields`, (t) => {
    const { progress, requests } = setup(t, status);
    assert.equal(progress.terminal(), true);
    assert.equal(progress.children[0].getAttribute("aria-label"), `Order ${status}`);
    t.mock.timers.tick(90000);
    assert.equal(requests.length, 0);
  });
}

test("slow order polls cannot overlap and polling stops on delivery", async (t) => {
  const { progress, requests } = setup(t);
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 1);
  t.mock.timers.tick(60000);
  assert.equal(requests.length, 1);
  requests[0].resolve(shipped);
  await setImmediate();
  assert.equal(progress.children[0].getAttribute("aria-label"), "Order shipped");
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 2);
  requests[1].resolve(delivered);
  await setImmediate();
  assert.equal(progress.children[0].getAttribute("aria-label"), "Order delivered");
  t.mock.timers.tick(90000);
  assert.equal(requests.length, 2);
});

test("disconnected order widgets ignore pending responses", async (t) => {
  const { progress, requests } = setup(t);
  t.mock.timers.tick(30000);
  progress.disconnectedCallback();
  requests[0].resolve(delivered);
  await setImmediate();
  assert.equal(progress.getAttribute("status"), "processing");
  assert.equal(progress.children[0].getAttribute("aria-label"), "Order accepted");
  t.mock.timers.tick(90000);
  assert.equal(requests.length, 1);
});

test("a response from a previous connection cannot unlock or overwrite a new poll", async (t) => {
  const { progress, requests } = setup(t);
  t.mock.timers.tick(30000);
  progress.disconnectedCallback();
  progress.connectedCallback();
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 2);
  requests[0].resolve(delivered);
  await setImmediate();
  assert.equal(progress.getAttribute("status"), "processing");
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 2);
  requests[1].resolve(shipped);
  await setImmediate();
  assert.equal(progress.getAttribute("status"), "shipped");
});

test("hidden pages skip polls and a failed poll allows the next interval", async (t) => {
  const { progress, requests, document } = setup(t);
  document.hidden = true;
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 0);
  document.hidden = false;
  t.mock.timers.tick(30000);
  requests[0].reject(new Error("Offline"));
  await setImmediate();
  assert.equal(progress.children[0].getAttribute("aria-label"), "Order accepted");
  t.mock.timers.tick(30000);
  assert.equal(requests.length, 2);
  requests[1].resolve(shipped);
  await setImmediate();
});
