import { api, money } from "./core.js";

let nextID = 0;

// <ui-search><form action="/search"><input name="q" class="input"></form></ui-search>
// Suggests matching products while typing (combobox pattern). Enter without a
// highlighted suggestion submits the normal search form.
export class UISearch extends HTMLElement {
  connectedCallback() {
    this.input = this.querySelector('input[name="q"]');
    if (!this.input || this.list) return;
    const id = `search-list-${nextID++}`;
    this.list = document.createElement("div");
    this.list.className = "search-results";
    this.list.id = id;
    this.list.setAttribute("role", "listbox");
    this.list.hidden = true;
    this.input.closest(".search")?.append(this.list);
    this.input.setAttribute("role", "combobox");
    this.input.setAttribute("aria-autocomplete", "list");
    this.input.setAttribute("aria-controls", id);
    this.input.setAttribute("aria-expanded", "false");
    this.input.setAttribute("autocomplete", "off");
    this.input.addEventListener("input", () => this.schedule());
    this.input.addEventListener("keydown", (event) => this.key(event));
    this.input.addEventListener("blur", () => setTimeout(() => this.close(), 150));
  }

  schedule() {
    clearTimeout(this.timer);
    const query = this.input.value.trim();
    if (query.length < 2) {
      this.close();
      return;
    }
    this.timer = setTimeout(() => this.fetch(query), 200);
  }

  async fetch(query) {
    this.seq = (this.seq || 0) + 1;
    const seq = this.seq;
    try {
      const data = await api(`/catalog/search?q=${encodeURIComponent(query)}&limit=6`);
      if (seq !== this.seq) return;
      this.render(data?.items || [], query);
    } catch {
      this.close();
    }
  }

  render(items, query) {
    this.active = -1;
    this.list.replaceChildren();
    for (const item of items.slice(0, 6)) {
      const link = document.createElement("a");
      link.href = `/products/${encodeURIComponent(item.slug)}`;
      link.setAttribute("role", "option");
      link.id = `${this.list.id}-${this.list.childElementCount}`;
      const name = document.createElement("span");
      name.className = "truncate";
      name.textContent = item.name;
      const price = document.createElement("span");
      price.className = "muted";
      price.textContent = money(item.price);
      link.append(name, price);
      this.list.append(link);
    }
    const all = document.createElement("a");
    all.href = `/search?q=${encodeURIComponent(query)}`;
    all.setAttribute("role", "option");
    all.id = `${this.list.id}-all`;
    all.textContent = items.length ? `All results for “${query}”` : `No quick matches. Search “${query}”`;
    this.list.append(all);
    this.list.hidden = false;
    this.input.setAttribute("aria-expanded", "true");
  }

  key(event) {
    const options = [...this.list.querySelectorAll('[role="option"]')];
    if (this.list.hidden || !options.length) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const delta = event.key === "ArrowDown" ? 1 : -1;
      this.active = (this.active + delta + options.length) % options.length;
      options.forEach((option, index) => option.setAttribute("aria-selected", String(index === this.active)));
      this.input.setAttribute("aria-activedescendant", options[this.active].id);
    } else if (event.key === "Enter" && this.active >= 0) {
      event.preventDefault();
      location.assign(options[this.active].href);
    } else if (event.key === "Escape") {
      this.close();
    }
  }

  close() {
    if (!this.list) return;
    this.list.hidden = true;
    this.input.setAttribute("aria-expanded", "false");
    this.input.removeAttribute("aria-activedescendant");
  }
}
