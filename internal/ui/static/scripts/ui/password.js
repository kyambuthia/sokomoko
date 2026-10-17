import { icon } from "./core.js";

// <ui-password><input type="password" …></ui-password>
// Adds a show/hide button inside the field.
export class UIPassword extends HTMLElement {
  connectedCallback() {
    const input = this.querySelector("input");
    if (!input || this.toggle) return;
    this.style.position = "relative";
    this.style.display = "block";
    input.style.paddingRight = "52px";
    this.toggle = document.createElement("button");
    this.toggle.type = "button";
    this.toggle.className = "btn btn-icon btn-sm";
    this.toggle.style.cssText = "position:absolute;right:6px;top:50%;transform:translateY(-50%)";
    const render = () => {
      const hidden = input.type === "password";
      this.toggle.replaceChildren(icon(hidden ? "eye" : "eye-off"));
      this.toggle.setAttribute("aria-label", hidden ? "Show password" : "Hide password");
    };
    this.toggle.addEventListener("click", () => {
      input.type = input.type === "password" ? "text" : "password";
      render();
      input.focus();
    });
    render();
    this.append(this.toggle);
  }
}
