// Sokomoko dynamic web components.
//
// Every component here enhances markup that the server already rendered: forms
// keep working with JavaScript disabled, and the components only add in-place
// updates. All components use the light DOM so the site stylesheet applies.

function toast(message, variant = "info") {
  if (window.UIToast && typeof window.UIToast.show === "function") {
    window.UIToast.show(message, variant);
  }
}

// ---------------------------------------------------------------------------
// <ui-confirm message> wrapping a form - asks before destructive submissions.
// ---------------------------------------------------------------------------
class UIConfirm extends HTMLElement {
  connectedCallback() {
    this.querySelector("form")?.addEventListener("submit", (event) => {
      const message = this.getAttribute("message") || "Are you sure?";
      if (!window.confirm(message)) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    });
  }
}

// ---------------------------------------------------------------------------
// <ui-countdown deadline="RFC3339" expired-message> - live countdown, used for
// checkout stock holds.
// ---------------------------------------------------------------------------
class UICountdown extends HTMLElement {
  connectedCallback() {
    this.deadline = Date.parse(this.getAttribute("deadline") || "");
    if (Number.isNaN(this.deadline)) return;
    this.classList.add("countdown");
    this.setAttribute("role", "timer");
    this.output = document.createElement("p");
    this.output.className = "countdown__text";
    this.replaceChildren(this.output);
    this.tick();
    this.interval = window.setInterval(() => this.tick(), 1000);
  }

  disconnectedCallback() {
    window.clearInterval(this.interval);
  }

  tick() {
    const remaining = Math.max(0, this.deadline - Date.now());
    if (remaining === 0) {
      window.clearInterval(this.interval);
      this.classList.add("countdown--expired");
      this.output.textContent = this.getAttribute("expired-message") || "Time is up.";
      this.dispatchEvent(new CustomEvent("countdown:expired", { bubbles: true }));
      return;
    }
    const minutes = Math.floor(remaining / 60000);
    const seconds = Math.floor((remaining % 60000) / 1000);
    const clock = `${minutes}:${String(seconds).padStart(2, "0")}`;
    this.classList.toggle("countdown--urgent", remaining < 120000);
    this.output.textContent = `Items reserved for ${clock}`;
  }
}

// ---------------------------------------------------------------------------
// <ui-table-filter for="table-id" placeholder> - client-side row filter for
// admin and partner tables.
// ---------------------------------------------------------------------------
class UITableFilter extends HTMLElement {
  static nextID = 0;

  connectedCallback() {
    const table = document.getElementById(this.getAttribute("for") || "");
    if (!table || this.input) return;
    this.rows = Array.from(table.tBodies[0]?.rows || []);
    const id = `table-filter-${UITableFilter.nextID++}`;

    const label = document.createElement("label");
    label.className = "visually-hidden";
    label.htmlFor = id;
    label.textContent = this.getAttribute("placeholder") || "Filter rows";

    this.input = document.createElement("input");
    this.input.type = "search";
    this.input.id = id;
    this.input.className = "form__input table-filter__input";
    this.input.placeholder = this.getAttribute("placeholder") || "Filter";

    this.status = document.createElement("p");
    this.status.className = "table-filter__status form__hint";
    this.status.setAttribute("aria-live", "polite");

    this.classList.add("table-filter");
    this.replaceChildren(label, this.input, this.status);
    this.input.addEventListener("input", () => this.apply());
  }

  apply() {
    const terms = this.input.value.trim().toLowerCase().split(/\s+/).filter(Boolean);
    let visible = 0;
    for (const row of this.rows) {
      const text = row.textContent.toLowerCase();
      const match = terms.every((term) => text.includes(term));
      row.hidden = !match;
      if (match) visible++;
    }
    this.status.textContent = terms.length ? `${visible} of ${this.rows.length} rows shown` : "";
  }
}

// ---------------------------------------------------------------------------
// <ui-copy text label> - copies text to the clipboard.
// ---------------------------------------------------------------------------
class UICopy extends HTMLElement {
  connectedCallback() {
    if (!navigator.clipboard || this.button) return;
    this.button = document.createElement("button");
    this.button.type = "button";
    this.button.className = "btn btn--secondary btn--small";
    this.button.textContent = "Copy";
    this.button.setAttribute("aria-label", this.getAttribute("label") || "Copy to clipboard");
    this.button.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(this.getAttribute("text") || "");
        this.button.textContent = "Copied";
        window.setTimeout(() => (this.button.textContent = "Copy"), 2000);
      } catch {
        toast("Copy failed. Select the text and copy it manually.", "warning");
      }
    });
    this.replaceChildren(this.button);
  }
}

// ---------------------------------------------------------------------------
// <ui-char-counter for="textarea-id"> - remaining character count.
// ---------------------------------------------------------------------------
class UICharCounter extends HTMLElement {
  connectedCallback() {
    this.field = document.getElementById(this.getAttribute("for") || "");
    const max = Number(this.field?.getAttribute("maxlength"));
    if (!this.field || !max) return;
    this.max = max;
    this.classList.add("char-counter", "form__hint");
    this.setAttribute("aria-live", "polite");
    this.field.addEventListener("input", () => this.render());
    this.render();
  }

  render() {
    const remaining = this.max - this.field.value.length;
    this.textContent = `${remaining} characters left`;
    this.classList.toggle("char-counter--low", remaining < this.max * 0.1);
  }
}

// ---------------------------------------------------------------------------
// <ui-category-nav endpoint> - replaces the static department links with the
// live category list.
// ---------------------------------------------------------------------------
async function fetchCategories(endpoint) {
  if (!fetchCategories.cache) {
    fetchCategories.cache = fetch(endpoint, { headers: { Accept: "application/json" } })
      .then((response) => (response.ok ? response.json() : { categories: [] }))
      .then((body) => body.categories || [])
      .catch(() => []);
  }
  return fetchCategories.cache;
}

class UICategoryNav extends HTMLElement {
  async connectedCallback() {
    if (!this.getAttribute("role")) this.setAttribute("role", "navigation");
    const categories = await fetchCategories(this.getAttribute("endpoint") || "/api/categories");
    if (categories.length === 0) return;

    const params = new URLSearchParams(window.location.search);
    const active = params.get("category");
    const links = [{ name: "All", url: "/", slug: "" }, ...categories].map((category) => {
      const link = document.createElement("a");
      link.href = category.url;
      link.textContent = category.name;
      link.className = "storefront-header__category-link";
      const isActive = category.slug ? category.slug === active : !active && window.location.pathname === "/";
      if (isActive) {
        link.classList.add("storefront-header__category-link--active");
        link.setAttribute("aria-current", "page");
      }
      return link;
    });
    this.replaceChildren(...links);
  }
}

const components = {
  "ui-confirm": UIConfirm,
  "ui-countdown": UICountdown,
  "ui-table-filter": UITableFilter,
  "ui-copy": UICopy,
  "ui-char-counter": UICharCounter,
  "ui-category-nav": UICategoryNav,
};

for (const [name, component] of Object.entries(components)) {
  if (!customElements.get(name)) {
    customElements.define(name, component);
  }
}

window.SokomokoComponents = { fetchCategories };

// Disable submit buttons on forms marked data-submit-once after the first
// submission so a double click cannot post twice.
document.addEventListener("submit", (event) => {
  const form = event.target;
  if (!(form instanceof HTMLFormElement) || !form.hasAttribute("data-submit-once")) return;
  if (event.defaultPrevented) return;
  if (form.dataset.submitted === "true") {
    event.preventDefault();
    return;
  }
  form.dataset.submitted = "true";
  form.querySelectorAll("button[type=submit]").forEach((button) => {
    button.setAttribute("aria-disabled", "true");
  });
});
