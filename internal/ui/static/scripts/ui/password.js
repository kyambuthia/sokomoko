import { icon } from "./core.js";

// <ui-password><input type="password" …></ui-password>
// Adds a show/hide button inside the field.
export class UIPassword extends HTMLElement {
  connectedCallback() {
    const input = this.querySelector("input");
    if (!input || this.toggle) return;
    this.classList.add("password");
    this.toggle = document.createElement("button");
    this.toggle.type = "button";
    this.toggle.className = "btn btn-icon btn-sm password__toggle";
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
