// Sokomoko component library entry point. Loaded as a module from the base
// layout; every component enhances server-rendered HTML that already works
// without JavaScript. See docs/design.md for the catalogue.
import { UIAddToCart, UICartBadge, UICartLine } from "./ui/cart.js";
import { UIConfirm } from "./ui/confirm.js";
import { UIOrderForm, UIOrderProgress } from "./ui/order.js";
import { UIPassword } from "./ui/password.js";
import { UIQuantity } from "./ui/quantity.js";
import { UISearch } from "./ui/search.js";
import { UIThemeToggle } from "./ui/theme-toggle.js";
import { UICategoryNav, UICharCounter, UICopy, UICountdown, UIRelativeTime, UITableFilter } from "./ui/widgets.js";

const components = {
  "ui-add-to-cart": UIAddToCart,
  "ui-cart-badge": UICartBadge,
  "ui-cart-line": UICartLine,
  "ui-category-nav": UICategoryNav,
  "ui-char-counter": UICharCounter,
  "ui-confirm": UIConfirm,
  "ui-copy": UICopy,
  "ui-countdown": UICountdown,
  "ui-order-form": UIOrderForm,
  "ui-order-progress": UIOrderProgress,
  "ui-password": UIPassword,
  "ui-quantity": UIQuantity,
  "ui-relative-time": UIRelativeTime,
  "ui-search": UISearch,
  "ui-table-filter": UITableFilter,
  "ui-theme-toggle": UIThemeToggle,
};

for (const [name, component] of Object.entries(components)) {
  if (!customElements.get(name)) customElements.define(name, component);
}

// Forms marked data-submit-once ignore repeat submissions, so a double tap
// cannot place an order twice.
document.addEventListener("submit", (event) => {
  const form = event.target;
  if (!(form instanceof HTMLFormElement) || !form.hasAttribute("data-submit-once") || event.defaultPrevented) return;
  if (form.dataset.submitted) {
    event.preventDefault();
    return;
  }
  form.dataset.submitted = "true";
  form.querySelectorAll('[type="submit"]').forEach((button) => button.setAttribute("aria-busy", "true"));
});
