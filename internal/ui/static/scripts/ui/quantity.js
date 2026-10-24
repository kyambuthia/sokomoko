import { icon } from "./core.js";

// <ui-quantity><input type="number" name="quantity" min="1" max="9" value="1"></ui-quantity>
// Wraps a number input in a minus/plus stepper. The input still submits with
// its form. With the autosubmit attribute, changes submit the form after a
// short pause.
export class UIQuantity extends HTMLElement {
  connectedCallback() {
    this.input = this.querySelector("input");
    if (!this.input || this.minus) return;
    const wrap = document.createElement("span");
    wrap.className = this.classList.contains("small") ? "stepper stepper-sm" : "stepper";
    this.minus = this.button("minus", "Decrease quantity", -1);
    this.plus = this.button("plus", "Increase quantity", 1);
    this.input.replaceWith(wrap);
    wrap.append(this.minus, this.input, this.plus);
    this.input.addEventListener("input", () => this.changed());
    this.sync();
  }

  disconnectedCallback() {
    clearTimeout(this.timer);
  }

  button(name, label, step) {
    const button = document.createElement("button");
    button.type = "button";
    button.setAttribute("aria-label", label);
    button.append(icon(name));
    button.addEventListener("click", () => this.step(step));
    return button;
  }

  bounds() {
    const min = Number(this.input.min || 0);
    const max = this.input.max === "" ? Infinity : Number(this.input.max);
    return { min, max };
  }

  step(delta) {
    const { min, max } = this.bounds();
    const value = Math.min(max, Math.max(min, (Number(this.input.value) || 0) + delta));
    this.input.value = String(value);
    this.changed();
  }

  changed() {
    this.sync();
    this.dispatchEvent(new CustomEvent("quantity-change", { bubbles: true, detail: { value: Number(this.input.value) } }));
    if (this.hasAttribute("autosubmit")) {
      clearTimeout(this.timer);
      // Clearing a quantity is an intermediate edit, not a request to remove
      // the item. Let native constraints reject invalid values before submit.
      if (this.input.value === "" || !this.input.checkValidity()) return;
      this.timer = setTimeout(() => {
        if (this.input.value !== "" && this.input.checkValidity()) this.input.form?.requestSubmit();
      }, 500);
    }
  }

  sync() {
    const { min, max } = this.bounds();
    const value = Number(this.input.value) || 0;
    this.minus.disabled = value <= min;
    this.plus.disabled = value >= max;
  }
}
