import { icon } from "./core.js";

// <ui-theme-toggle><button class="btn btn-icon">…</button></ui-theme-toggle>
// Switches between light and dark and remembers the choice.
export class UIThemeToggle extends HTMLElement {
  connectedCallback() {
    this.button = this.querySelector("button");
    if (!this.button) return;
    this.button.addEventListener("click", () => this.toggle());
    this.render();
  }

  current() {
    const explicit = document.documentElement.dataset.theme;
    if (explicit) return explicit;
    return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  toggle() {
    const next = this.current() === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem("sokomoko-theme", next);
    } catch {
      // The choice still applies to this page.
    }
    this.render();
  }

  render() {
    const dark = this.current() === "dark";
    this.button.replaceChildren(icon(dark ? "sun" : "moon"));
    this.button.setAttribute("aria-label", dark ? "Use light appearance" : "Use dark appearance");
  }
}
