import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";

globalThis.HTMLElement = class {};
const { UIPaymentStatus } = await import("../../internal/ui/static/scripts/ui/order.js");

function setup(t) {
  t.mock.timers.enable({ apis: ["setInterval", "Date"], now: 0 });
  const previousDocument = globalThis.document;
  const previousLocation = globalThis.location;
  const document = { hidden: false };
  globalThis.document = document;
  let reloads = 0;
  globalThis.location = { reload() { reloads++; } };
  const requests = [];
  t.mock.method(globalThis, "fetch", () => new Promise((resolve, reject) => {
    requests.push({ reject, resolve(status) { resolve({ ok: true, json: async () => ({ data: status }) }); } });
  }));
  const text = { textContent: "Waiting for payment" };
  const payment = new UIPaymentStatus();
  payment.getAttribute = () => "7";
  payment.querySelector = () => text;
  payment.connectedCallback();
  t.after(() => {
    payment.disconnectedCallback();
    globalThis.document = previousDocument;
    globalThis.location = previousLocation;
  });
  return { payment, requests, text, document, reloads: () => reloads };
}

for (const status of ["paid", "failed", "cancelled", "offline"]) {
  test(`payment ${status} reloads once and stops polling`, async (t) => {
    const { requests, reloads } = setup(t);
    t.mock.timers.tick(4000);
    requests[0].resolve({ status });
    await setImmediate();
    assert.equal(reloads(), 1);
    t.mock.timers.tick(12000);
    assert.equal(requests.length, 1);
    assert.equal(reloads(), 1);
  });
}

test("slow payment polls cannot overlap and unknown responses do not reload", async (t) => {
  const { requests, reloads } = setup(t);
  t.mock.timers.tick(4000);
  t.mock.timers.tick(12000);
  assert.equal(requests.length, 1);
  requests[0].resolve({ status: "unexpected" });
  await setImmediate();
  assert.equal(reloads(), 0);
  t.mock.timers.tick(4000);
  requests[1].resolve(null);
  await setImmediate();
  assert.equal(reloads(), 0);
  t.mock.timers.tick(4000);
  requests[2].resolve({ status: "pending" });
  await setImmediate();
  assert.equal(reloads(), 0);
});

test("responses from a previous connection cannot reload or unlock a current poll", async (t) => {
  const { payment, requests, reloads } = setup(t);
  t.mock.timers.tick(4000);
  payment.disconnectedCallback();
  payment.connectedCallback();
  t.mock.timers.tick(4000);
  requests[0].resolve({ status: "paid" });
  await setImmediate();
  assert.equal(reloads(), 0);
  t.mock.timers.tick(4000);
  assert.equal(requests.length, 2);
  requests[1].resolve({ status: "pending" });
  await setImmediate();
  payment.disconnectedCallback();
  t.mock.timers.tick(8000);
  assert.equal(requests.length, 2);
});

test("payment polling expires even while a request is pending and ignores its late response", async (t) => {
  const { requests, reloads, text } = setup(t);
  t.mock.timers.tick(4000);
  t.mock.timers.tick(296000);
  assert.equal(requests.length, 1);
  assert.match(text.textContent, /Still waiting/);
  requests[0].resolve({ status: "paid" });
  await setImmediate();
  assert.equal(reloads(), 0);
  t.mock.timers.tick(12000);
  assert.equal(requests.length, 1);
});

test("hidden pages skip payment polls and failures allow a retry", async (t) => {
  const { document, requests, reloads } = setup(t);
  document.hidden = true;
  t.mock.timers.tick(4000);
  assert.equal(requests.length, 0);
  document.hidden = false;
  t.mock.timers.tick(4000);
  requests[0].reject(new Error("Offline"));
  await setImmediate();
  t.mock.timers.tick(4000);
  assert.equal(requests.length, 2);
  requests[1].resolve({ status: "paid" });
  await setImmediate();
  assert.equal(reloads(), 1);
});


test("late payment responses honor the deadline even when timer callbacks were delayed", async (t) => {
  const { requests, reloads, text } = setup(t);
  t.mock.timers.tick(4000);
  t.mock.timers.setTime(300000);
  requests[0].resolve({ status: "paid" });
  await setImmediate();
  assert.equal(reloads(), 0);
  assert.match(text.textContent, /Still waiting/);
});
