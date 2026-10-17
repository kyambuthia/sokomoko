// Sokomoko dynamic web components.
//
// Every component here enhances markup that the server already rendered: forms
// keep working with JavaScript disabled, and the components only add in-place
// updates. All components use the light DOM so the site stylesheet applies.

const CART_CHANGED = "sokomoko:cart-changed";

const currencyFormatter = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });

function formatMoney(amount) {
  const value = Number.parseFloat(amount);
  return Number.isFinite(value) ? currencyFormatter.format(value) : "";
}

function toast(message, variant = "info") {
  if (window.UIToast && typeof window.UIToast.show === "function") {
    window.UIToast.show(message, variant);
  }
}

// postForm submits a form with fetch and asks for JSON. It resolves to
// { response, body } and never throws for HTTP errors.
async function postForm(form, extra = {}) {
  const data = new FormData(form);
  for (const [key, value] of Object.entries(extra)) {
    data.set(key, value);
  }
  const csrf = data.get("csrf_token");
  const headers = { Accept: "application/json" };
  if (csrf) {
    headers["X-CSRF-Token"] = String(csrf);
  }
  const response = await fetch(form.action, {
    method: "POST",
    body: new URLSearchParams(data),
    credentials: "same-origin",
    headers,
  });
  let body = null;
  try {
    body = await response.json();
  } catch {
    body = null;
  }
  return { response, body };
}

function redirectToLogin() {
  const next = `${window.location.pathname}${window.location.search}`;
  window.location.assign(`/login?next=${encodeURIComponent(next)}`);
}

function announceCart(body) {
  if (!body) return;
  window.dispatchEvent(new CustomEvent(CART_CHANGED, { detail: body }));
}

// ---------------------------------------------------------------------------
// <ui-cart-count endpoint="/api/cart"> - live cart badge in the header.
// ---------------------------------------------------------------------------
class UICartCount extends HTMLElement {
  connectedCallback() {
    this.classList.add("cart-count");
    this.setAttribute("aria-live", "polite");
    this.onCartChanged = (event) => this.render(event.detail.itemCount);
    window.addEventListener(CART_CHANGED, this.onCartChanged);
    this.refresh();
  }

  disconnectedCallback() {
    window.removeEventListener(CART_CHANGED, this.onCartChanged);
  }

  async refresh() {
    const endpoint = this.getAttribute("endpoint") || "/api/cart";
    try {
      const response = await fetch(endpoint, { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!response.ok) {
        this.render(0);
        return;
      }
      const body = await response.json();
      this.render(body.itemCount);
    } catch {
      this.render(0);
    }
  }

  render(count) {
    const value = Number(count) || 0;
    this.hidden = value <= 0;
    this.textContent = value > 99 ? "99+" : String(value);
    const label = this.getAttribute("label") || "items in cart";
    this.setAttribute("aria-label", `${value} ${label}`);
  }
}

// ---------------------------------------------------------------------------
// <ui-add-to-cart> wrapping a /cart/add form - adds without a page reload.
// ---------------------------------------------------------------------------
class UIAddToCart extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    if (!this.form) return;
    this.onSubmit = (event) => this.submit(event);
    this.form.addEventListener("submit", this.onSubmit);
  }

  disconnectedCallback() {
    this.form?.removeEventListener("submit", this.onSubmit);
  }

  async submit(event) {
    event.preventDefault();
    const button = this.form.querySelector("button[type=submit]");
    if (button?.disabled) return;
    const label = button?.textContent;
    if (button) {
      button.disabled = true;
      button.textContent = "Adding…";
    }
    try {
      const { response, body } = await postForm(this.form);
      if (response.status === 401) {
        redirectToLogin();
        return;
      }
      if (response.ok && body?.ok) {
        announceCart(body);
        const name = this.getAttribute("product-name") || "Item";
        toast(`${name} added to your cart.`, "success");
      } else {
        toast(body?.error || "Could not add this item.", "danger");
      }
    } catch {
      // Network failure: fall back to a normal form submission.
      this.form.removeEventListener("submit", this.onSubmit);
      this.form.submit();
      return;
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = label;
      }
    }
  }
}

// ---------------------------------------------------------------------------
// <ui-cart-line product-id> wrapping a /cart/update form - quantity changes
// apply automatically and update line totals and the subtotal in place.
// ---------------------------------------------------------------------------
class UICartLine extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    this.input = this.form?.querySelector("input[name=quantity]");
    if (!this.form || !this.input) return;

    const submitButton = this.form.querySelector("[data-cart-line-submit]");
    if (submitButton) submitButton.classList.add("visually-hidden");

    this.lastValue = this.input.value;
    this.timer = null;
    this.input.addEventListener("input", () => {
      window.clearTimeout(this.timer);
      this.timer = window.setTimeout(() => this.update(), 450);
    });
    this.form.addEventListener("submit", (event) => {
      event.preventDefault();
      window.clearTimeout(this.timer);
      this.update();
    });
  }

  async update() {
    const value = this.input.value.trim();
    if (value === "" || value === this.lastValue) return;
    this.setAttribute("aria-busy", "true");
    try {
      const { response, body } = await postForm(this.form);
      if (response.status === 401) {
        redirectToLogin();
        return;
      }
      if (!response.ok || !body?.ok) {
        toast(body?.error || "Could not update quantity.", "danger");
        this.input.value = this.lastValue;
        return;
      }
      this.lastValue = value;
      this.applyCart(body);
      announceCart(body);
    } catch {
      toast("Network error. Your cart was not updated.", "danger");
      this.input.value = this.lastValue;
    } finally {
      this.removeAttribute("aria-busy");
    }
  }

  applyCart(body) {
    const productId = Number(this.getAttribute("product-id"));
    const line = body.lines.find((l) => l.productId === productId);
    const row = this.closest("[data-cart-line]");
    if (!line) {
      row?.remove();
      if (body.itemCount === 0) window.location.reload();
    } else {
      const total = document.querySelector(`[data-line-total="${productId}"]`);
      if (total) total.textContent = formatMoney(line.lineTotal);
    }
    const subtotal = document.querySelector("[data-cart-subtotal]");
    if (subtotal) subtotal.textContent = formatMoney(body.subtotal);
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
// <ui-relative-time datetime="RFC3339">fallback</ui-relative-time>
// ---------------------------------------------------------------------------
const relativeFormatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
const absoluteFormatter = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

class UIRelativeTime extends HTMLElement {
  connectedCallback() {
    this.time = Date.parse(this.getAttribute("datetime") || "");
    if (Number.isNaN(this.time)) return;
    this.title = absoluteFormatter.format(this.time);
    this.render();
    this.interval = window.setInterval(() => this.render(), 60000);
  }

  disconnectedCallback() {
    window.clearInterval(this.interval);
  }

  render() {
    const seconds = Math.round((this.time - Date.now()) / 1000);
    const units = [
      ["year", 31536000],
      ["month", 2592000],
      ["week", 604800],
      ["day", 86400],
      ["hour", 3600],
      ["minute", 60],
    ];
    for (const [unit, size] of units) {
      if (Math.abs(seconds) >= size) {
        this.textContent = relativeFormatter.format(Math.round(seconds / size), unit);
        return;
      }
    }
    this.textContent = "just now";
  }
}

// ---------------------------------------------------------------------------
// <ui-order-status order-id status partner-status delivery-status endpoint>
// - fulfillment progress tracker that polls for updates until the order is
// delivered or cancelled.
// ---------------------------------------------------------------------------
const ORDER_STEPS = [
  { key: "placed", label: "Placed" },
  { key: "accepted", label: "Accepted" },
  { key: "packing", label: "Packing" },
  { key: "dispatched", label: "Dispatched" },
  { key: "delivered", label: "Delivered" },
];

function orderStepIndex(partnerStatus, deliveryStatus) {
  if (deliveryStatus === "delivered" || partnerStatus === "completed") return 4;
  switch (partnerStatus) {
    case "dispatched":
      return 3;
    case "packing":
      return 2;
    case "accepted":
      return 1;
    default:
      return 0;
  }
}

class UIOrderStatus extends HTMLElement {
  static POLL_MS = 30000;

  connectedCallback() {
    this.classList.add("order-status");
    this.render({
      status: this.getAttribute("status"),
      partnerStatus: this.getAttribute("partner-status"),
      deliveryStatus: this.getAttribute("delivery-status"),
    });
    if (!this.isTerminal()) {
      this.onVisibility = () => {
        if (document.visibilityState === "visible") this.poll();
      };
      document.addEventListener("visibilitychange", this.onVisibility);
      this.interval = window.setInterval(() => this.poll(), UIOrderStatus.POLL_MS);
    }
  }

  disconnectedCallback() {
    window.clearInterval(this.interval);
    if (this.onVisibility) document.removeEventListener("visibilitychange", this.onVisibility);
  }

  isTerminal() {
    const status = this.getAttribute("status");
    return status === "delivered" || status === "cancelled";
  }

  async poll() {
    if (document.visibilityState !== "visible") return;
    const endpoint = this.getAttribute("endpoint");
    if (!endpoint) return;
    try {
      const response = await fetch(endpoint, { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!response.ok) return;
      const body = await response.json();
      const changed =
        body.status !== this.getAttribute("status") ||
        body.partnerStatus !== this.getAttribute("partner-status") ||
        body.deliveryStatus !== this.getAttribute("delivery-status");
      if (!changed) return;

      this.setAttribute("status", body.status);
      this.setAttribute("partner-status", body.partnerStatus);
      this.setAttribute("delivery-status", body.deliveryStatus);
      this.render(body);
      const notice = this.closest(".order-card")?.querySelector("[data-order-notice]");
      if (notice && body.deliveryNotice) notice.textContent = body.deliveryNotice;
      toast(`Order #${body.id} is now ${body.status}.`, "info");
      if (this.isTerminal()) window.clearInterval(this.interval);
    } catch {
      // Ignore transient polling failures; the next poll retries.
    }
  }

  render({ status, partnerStatus, deliveryStatus }) {
    const list = document.createElement("ol");
    list.className = "order-status__steps";
    if (status === "cancelled") {
      const item = document.createElement("li");
      item.className = "order-status__step order-status__step--cancelled";
      item.textContent = "Cancelled";
      list.append(item);
    } else {
      const current = orderStepIndex(partnerStatus, deliveryStatus);
      ORDER_STEPS.forEach((step, index) => {
        const item = document.createElement("li");
        item.className = "order-status__step";
        if (index < current) item.classList.add("order-status__step--done");
        if (index === current) {
          item.classList.add("order-status__step--current");
          item.setAttribute("aria-current", "step");
        }
        item.textContent = step.label;
        list.append(item);
      });
    }
    list.setAttribute("aria-label", `Order status: ${status}`);
    this.replaceChildren(list);
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
  "ui-cart-count": UICartCount,
  "ui-add-to-cart": UIAddToCart,
  "ui-cart-line": UICartLine,
  "ui-confirm": UIConfirm,
  "ui-countdown": UICountdown,
  "ui-relative-time": UIRelativeTime,
  "ui-order-status": UIOrderStatus,
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

window.SokomokoComponents = { fetchCategories, formatMoney, CART_CHANGED };

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
