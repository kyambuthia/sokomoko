import { api, announceCart, busy, CART_CHANGED, loginRedirect, money, toast } from "./core.js";

// Serialize this page's cart reads and writes, and publish each snapshot before
// starting the next request. A rejected request must not poison later actions.
let cartQueue = Promise.resolve();
export function requestCart(path, options) {
  const request = cartQueue.then(async () => {
    const cart = await api(path, options);
    announceCart(cart);
    return cart;
  });
  cartQueue = request.catch(() => {});
  return request;
}

// <ui-cart-badge signed-in><a class="has-count">…<span class="count" hidden></span></a></ui-cart-badge>
// Keeps the cart count current as items are added or removed.
export class UICartBadge extends HTMLElement {
  connectedCallback() {
    this.count = this.querySelector(".count");
    this.onChange = (event) => this.render(event.detail.count);
    document.addEventListener(CART_CHANGED, this.onChange);
    if (this.hasAttribute("signed-in") && this.count) {
      requestCart("/cart").catch(() => {});
    }
  }

  disconnectedCallback() {
    document.removeEventListener(CART_CHANGED, this.onChange);
  }

  render(count) {
    if (!this.count) return;
    this.count.hidden = !count;
    this.count.textContent = count > 99 ? "99+" : String(count);
    this.querySelector("a")?.setAttribute("aria-label", `Cart, ${count} item${count === 1 ? "" : "s"}`);
  }
}

// <ui-add-to-cart><form action="/cart/add">…</form></ui-add-to-cart>
// Adds without leaving the page. Falls back to the normal form post when the
// visitor is signed out or the API fails unexpectedly.
export class UIAddToCart extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    this.form?.addEventListener("submit", (event) => this.submit(event));
  }

  async submit(event) {
    event.preventDefault();
    const data = new FormData(this.form);
    const button = this.form.querySelector('[type="submit"]');
    busy(button, true);
    try {
      await requestCart("/cart/items", {
        method: "POST",
        body: { product_id: Number(data.get("product_id")), quantity: Number(data.get("quantity") || 1) },
      });
      toast("Added to cart", "success");
    } catch (error) {
      if (error.status === 401) {
        loginRedirect();
        return;
      }
      toast(error.message, "error");
    } finally {
      busy(button, false);
    }
  }
}

// <ui-cart-line product-id="7">…</ui-cart-line>
// Applies quantity changes and removals in place, updating the line total,
// the subtotal ([data-cart-subtotal]), and the badge.
export class UICartLine extends HTMLElement {
  connectedCallback() {
    this.productID = this.getAttribute("product-id");
    this.querySelectorAll("form").forEach((form) => {
      form.addEventListener("submit", (event) => this.submit(event, form));
    });
  }

  async submit(event, form) {
    event.preventDefault();
    if (this.removing) return;
    const removing = form.action.endsWith("/cart/remove");
    const quantity = Number(new FormData(form).get("quantity"));
    // A pending quantity autosubmit must not recreate a line after removal.
    const isRemoval = removing || quantity <= 0;
    if (isRemoval) this.removing = true;
    try {
      const cart = isRemoval
        ? await requestCart(`/cart/items/${this.productID}`, { method: "DELETE" })
        : await requestCart(`/cart/items/${this.productID}`, { method: "PATCH", body: { quantity } });
      const item = (cart.items || []).find((line) => String(line.product_id) === this.productID);
      if (!item) {
        this.remove();
      } else {
        const total = this.querySelector("[data-line-total]");
        if (total) total.textContent = money(item.line_total);
      }
      document.querySelectorAll("[data-cart-subtotal]").forEach((el) => {
        el.textContent = money(cart.subtotal);
      });
      if (!(cart.items || []).length) location.reload();
    } catch (error) {
      if (error.status === 401) {
        loginRedirect();
        return;
      }
      toast(error.message, "error");
    } finally {
      if (isRemoval) this.removing = false;
    }
  }
}
