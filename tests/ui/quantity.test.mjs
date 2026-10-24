import assert from "node:assert/strict";
import test from "node:test";

globalThis.HTMLElement = class {};
const { UIQuantity } = await import("../../internal/ui/static/scripts/ui/quantity.js");

for (const disconnect of [false, true]) {
  test(`quantity autosubmit ${disconnect ? "is cancelled on disconnect" : "submits after debounce"}`, (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const quantity = new UIQuantity();
    let submits = 0;
    quantity.input = { value: "2", min: "0", max: "10", form: { requestSubmit() { submits++; } } };
    quantity.minus = {};
    quantity.plus = {};
    quantity.dispatchEvent = () => true;
    quantity.hasAttribute = () => true;
    quantity.changed();
    t.mock.timers.tick(499);
    assert.equal(submits, 0);
    if (disconnect) quantity.disconnectedCallback();
    t.mock.timers.tick(1);
    assert.equal(submits, disconnect ? 0 : 1);
  });
}
