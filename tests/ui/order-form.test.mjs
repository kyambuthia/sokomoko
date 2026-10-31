import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";

class Element {
  constructor() { this.attributes = new Map(); }
  setAttribute(key, value) { this.attributes.set(key, value); }
  append() {}
  remove() {}
}
globalThis.HTMLElement = Element;
const { UIOrderForm } = await import("../../internal/ui/static/scripts/ui/order.js");

function setup(t, scope) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const previousDocument = globalThis.document;
  const previousLocation = globalThis.location;
  globalThis.document = {
    querySelector: () => null,
    createElement: () => new Element(),
    createElementNS: () => new Element(),
    body: new Element(),
  };
  let reloads = 0;
  globalThis.location = { reload() { reloads++; } };
  t.after(() => { globalThis.document = previousDocument; globalThis.location = previousLocation; });
  const requests = [];
  t.mock.method(globalThis, "fetch", (url, options) => new Promise((resolve) => {
    requests.push({ url, options, resolve(status = 200) {
      resolve({ ok: status === 200, status, json: async () => status === 200 ? { data: { message: "Fulfillment updated" } } : { error: { message: "Rejected" } } });
    } });
  }));
  t.mock.method(globalThis, "FormData", function(form) {
    return form.elements.filter((control) => !control.disabled).map(({ name, value }) => [name, value]);
  });
  const form = new Element();
  form.elements = [
    { name: "partner_status", value: "dispatched", disabled: false },
    { name: "delivery_notice", value: "On the way", disabled: false },
    { name: "status", value: "shipped", disabled: false },
    { name: "unavailable", value: "ignored", disabled: true },
    { name: "save", value: "", disabled: false },
  ];
  form.querySelector = () => form.elements.at(-1);
  const widget = new UIOrderForm();
  widget.form = form;
  widget.getAttribute = (key) => key === "scope" ? scope : "7";
  // Submitted values must not be painted as if they were the saved server state.
  widget.closest = () => { throw new Error("Do not paint speculative badges"); };
  const event = { preventDefault() {} };
  return { widget, form, requests, event, reloads: () => reloads };
}

for (const scope of ["partner", "admin", "unknown"]) {
  test(`${scope} fulfillment save snapshots values once and reloads committed state`, async (t) => {
    const { widget, form, requests, event, reloads } = setup(t, scope);
    const saving = widget.submit(event);
    await widget.submit(event);
    await setImmediate();
    assert.equal(requests.length, 1);
    assert.equal(form.attributes.get("aria-busy"), "true");
    assert.equal(form.elements.every((control) => control.disabled), true);
    const expectedScope = scope === "admin" ? "admin" : "partner";
    assert.equal(requests[0].url, `/api/v1/${expectedScope}/orders/7`);
    const body = JSON.parse(requests[0].options.body);
    assert.equal(body.partner_status, "dispatched");
    assert.equal(body.delivery_notice, "On the way");
    assert.equal(body.status, scope === "admin" ? "shipped" : undefined);
    requests[0].resolve();
    await saving;
    assert.equal(reloads(), 1);
    assert.equal(widget.pending, true);
    await widget.submit(event);
    assert.equal(requests.length, 1);
  });
}

test("a rejected fulfillment save preserves values and restores original control states for retry", async (t) => {
  const { widget, form, requests, event, reloads } = setup(t, "partner");
  const disabled = form.elements.map((control) => control.disabled);
  const values = form.elements.map((control) => control.value);
  const saving = widget.submit(event);
  requests[0].resolve(409);
  await saving;
  assert.equal(reloads(), 0);
  assert.equal(widget.pending, false);
  assert.equal(form.attributes.get("aria-busy"), "false");
  assert.deepEqual(form.elements.map((control) => control.disabled), disabled);
  assert.deepEqual(form.elements.map((control) => control.value), values);
  form.elements[0].value = "packing";
  const retry = widget.submit(event);
  assert.equal(requests.length, 2);
  assert.equal(JSON.parse(requests[1].options.body).partner_status, "packing");
  requests[1].resolve();
  await retry;
  assert.equal(reloads(), 1);
});
