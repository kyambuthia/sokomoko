import { icon, toast } from "./core.js";

// <ui-countdown deadline="RFC3339" expired-message="…">fallback</ui-countdown>
// Counts down to a deadline, used for checkout stock holds.
export class UICountdown extends HTMLElement {
  connectedCallback() {
    this.deadline = Date.parse(this.getAttribute("deadline") || "");
    if (Number.isNaN(this.deadline)) return;
    this.tick();
    this.timer = setInterval(() => this.tick(), 1000);
  }

  disconnectedCallback() {
    clearInterval(this.timer);
  }

  tick() {
    const left = Math.max(0, Math.round((this.deadline - Date.now()) / 1000));
    const target = this.querySelector("[data-countdown]") || this;
    if (left === 0) {
      clearInterval(this.timer);
      target.textContent = this.getAttribute("expired-message") || "Expired";
      return;
    }
    const minutes = Math.floor(left / 60);
    const seconds = String(left % 60).padStart(2, "0");
    target.textContent = `Held for ${minutes}:${seconds}`;
  }
}

// <ui-copy text="…"><button class="btn btn-sm btn-secondary">Copy</button></ui-copy>
export class UICopy extends HTMLElement {
  connectedCallback() {
    this.querySelector("button")?.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(this.getAttribute("text") || "");
        toast("Copied", "success");
      } catch {
        toast("Copy failed. Select the text instead.", "error");
      }
    });
  }
}

// <ui-char-counter for="textarea-id"></ui-char-counter>
export class UICharCounter extends HTMLElement {
  connectedCallback() {
    const field = document.getElementById(this.getAttribute("for") || "");
    if (!field) return;
    this.className = "hint";
    this.setAttribute("aria-live", "polite");
    const update = () => {
      const max = Number(field.maxLength);
      this.textContent = max > 0 ? `${field.value.length}/${max}` : `${field.value.length}`;
    };
    field.addEventListener("input", update);
    update();
  }
}

// <ui-table-filter for="table-id" placeholder="Filter"></ui-table-filter>
// Filters table rows by every whitespace-separated term.
export class UITableFilter extends HTMLElement {
  connectedCallback() {
    const table = document.getElementById(this.getAttribute("for") || "");
    if (!table || this.input) return;
    this.rows = [...(table.tBodies[0]?.rows || [])];
    const wrap = document.createElement("label");
    wrap.className = "search";
    this.input = document.createElement("input");
    this.input.type = "search";
    this.input.className = "input";
    this.input.placeholder = this.getAttribute("placeholder") || "Filter";
    this.input.setAttribute("aria-label", this.input.placeholder);
    this.status = document.createElement("p");
    this.status.className = "hint";
    this.status.setAttribute("aria-live", "polite");
    wrap.append(icon("search"), this.input);
    this.replaceChildren(wrap, this.status);
    this.input.addEventListener("input", () => this.apply());
  }

  apply() {
    const terms = this.input.value.trim().toLowerCase().split(/\s+/).filter(Boolean);
    let shown = 0;
    for (const row of this.rows) {
      const match = terms.every((term) => row.textContent.toLowerCase().includes(term));
      row.hidden = !match;
      if (match) shown++;
    }
    this.status.textContent = terms.length ? `${shown} of ${this.rows.length}` : "";
  }
}

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
const units = [["year", 31536000], ["month", 2592000], ["week", 604800], ["day", 86400], ["hour", 3600], ["minute", 60]];

// <ui-relative-time datetime="RFC3339">2026-10-18 10:00</ui-relative-time>
// Replaces an absolute time with "3 hours ago", keeping the full time as a tooltip.
export class UIRelativeTime extends HTMLElement {
  connectedCallback() {
    const time = Date.parse(this.getAttribute("datetime") || "");
    if (Number.isNaN(time)) return;
    this.title = new Date(time).toLocaleString();
    const seconds = Math.round((time - Date.now()) / 1000);
    const [unit, size] = units.find(([, s]) => Math.abs(seconds) >= s) || ["second", 1];
    this.textContent = Math.abs(seconds) < 45 ? "just now" : relative.format(Math.round(seconds / size), unit);
  }
}

// <ui-category-nav current="slug"><nav class="chips">…</nav></ui-category-nav>
// Fills the chip row with every category after the server-rendered "All".
export class UICategoryNav extends HTMLElement {
  async connectedCallback() {
    const nav = this.querySelector(".chips");
    if (!nav || this.loaded) return;
    this.loaded = true;
    try {
      const response = await fetch("/api/categories", { headers: { Accept: "application/json" }, credentials: "same-origin" });
      const body = await response.json();
      const current = this.getAttribute("current") || "";
      for (const category of body.categories || []) {
        const chip = document.createElement("a");
        chip.className = "chip";
        chip.href = category.url;
        chip.append(icon(category.icon || "category"), document.createTextNode(category.name));
        if (category.slug === current) chip.setAttribute("aria-current", "page");
        nav.append(chip);
      }
    } catch {
      // The "All" chip remains.
    }
  }
}

