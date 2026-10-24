import assert from "node:assert/strict";
import test from "node:test";

globalThis.HTMLElement = class {};
const { UIQuantity } = await import("../../internal/ui/static/scripts/ui/quantity.js");

for (const disconnect of [false, true]) {
  test(`quantity autosubmit ${disconnect ? "is cancelled on disconnect" : "submits after debounce"}`, (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const quantity = new UIQuantity();
    let submits = 0;
    quantity.input = { value: "2", min: "0", max: "10", checkValidity: () => true, form: { requestSubmit() { submits++; } } };
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


for (const [name, value, valid] of [
  ["blank", "", true],
  ["fractional", "1.5", false],
  ["above stock", "11", false],
  ["negative", "-1", false],
  ["explicit zero", "0", true],
]) {
  test(`quantity autosubmit handles ${name} without treating an unfinished edit as removal`, (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const quantity = new UIQuantity();
    let submits = 0;
    quantity.input = { value: "2", min: "0", max: "10", checkValidity: () => true, form: { requestSubmit() { submits++; } } };
    quantity.minus = {};
    quantity.plus = {};
    quantity.dispatchEvent = () => true;
    quantity.hasAttribute = () => true;
    quantity.changed();
    t.mock.timers.tick(250);
    quantity.input.value = value;
    quantity.input.checkValidity = () => valid;
    quantity.changed();
    t.mock.timers.tick(500);
    assert.equal(submits, valid && value !== "" ? 1 : 0);
  });
}

test("quantity constraints are rechecked when the debounce expires", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const quantity = new UIQuantity();
  let submits = 0;
  let valid = true;
  quantity.input = { value: "2", min: "0", max: "10", checkValidity: () => valid, form: { requestSubmit() { submits++; } } };
  quantity.minus = {};
  quantity.plus = {};
  quantity.dispatchEvent = () => true;
  quantity.hasAttribute = () => true;
  quantity.changed();
  valid = false;
  t.mock.timers.tick(500);
  assert.equal(submits, 0);
});
