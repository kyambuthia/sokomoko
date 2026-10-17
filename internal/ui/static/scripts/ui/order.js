import { api, toast } from "./core.js";

const STEPS = ["Placed", "Accepted", "Shipped", "Delivered"];

function stepFor(partnerStatus, deliveryStatus) {
  if (deliveryStatus === "delivered" || partnerStatus === "completed") return 3;
  if (deliveryStatus === "shipped" || partnerStatus === "dispatched") return 2;
  if (partnerStatus === "accepted" || partnerStatus === "packing" || deliveryStatus === "processing") return 1;
  return 0;
}

// <ui-order-progress order-id status partner-status delivery-status>
// Shows a four-step progress bar and, for open orders, refreshes it from the
// account API every 30 seconds while the page is visible.
export class UIOrderProgress extends HTMLElement {
  connectedCallback() {
    this.render();
    if (!this.terminal()) {
      this.timer = setInterval(() => this.refresh(), 30000);
    }
  }

  disconnectedCallback() {
    clearInterval(this.timer);
  }

  terminal() {
    return this.getAttribute("status") === "cancelled" || stepFor(this.getAttribute("partner-status"), this.getAttribute("delivery-status")) === 3;
  }

  async refresh() {
    if (document.hidden) return;
    try {
      const order = await api(`/account/orders/${this.getAttribute("order-id")}`);
      this.setAttribute("status", order.status);
      this.setAttribute("partner-status", order.partner_status);
      this.setAttribute("delivery-status", order.delivery_status);
      const notice = this.closest("[data-order]")?.querySelector("[data-order-notice]");
      if (notice && order.delivery_notice) notice.textContent = order.delivery_notice;
      this.render();
      if (this.terminal()) clearInterval(this.timer);
    } catch {
      // The server-rendered state stays visible.
    }
  }

  render() {
    const cancelled = this.getAttribute("status") === "cancelled";
    const current = stepFor(this.getAttribute("partner-status"), this.getAttribute("delivery-status"));
    const list = document.createElement("ol");
    list.className = "progress";
    if (cancelled) list.dataset.cancelled = "";
    list.setAttribute("aria-label", cancelled ? "Order cancelled" : `Order ${STEPS[current].toLowerCase()}`);
    STEPS.forEach((label, index) => {
      const item = document.createElement("li");
      item.textContent = label;
      if (!cancelled && index <= current) item.dataset.done = "";
      if (!cancelled && index === current) item.dataset.current = "";
      list.append(item);
    });
    if (cancelled) list.querySelector("li").textContent = "Cancelled";
    this.replaceChildren(list);
  }
}

// <ui-order-form scope="admin|partner" order-id="12"><form>…</form></ui-order-form>
// Saves fulfillment changes through the API without a page reload. The plain
// form post still works without JavaScript.
export class UIOrderForm extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    this.form?.addEventListener("submit", (event) => this.submit(event));
  }

  async submit(event) {
    event.preventDefault();
    const data = Object.fromEntries(new FormData(this.form));
    const scope = this.getAttribute("scope") === "admin" ? "admin" : "partner";
    const body = { partner_status: data.partner_status, delivery_status: data.delivery_status, delivery_notice: data.delivery_notice || "" };
    if (scope === "admin") body.status = data.status;
    const button = this.form.querySelector('[type="submit"]');
    if (button) button.disabled = true;
    try {
      await api(`/${scope}/orders/${this.getAttribute("order-id")}`, { method: "PATCH", body });
      toast(`Order #${this.getAttribute("order-id")} updated`, "success");
      this.closest("[data-order]")?.querySelectorAll("[data-badge]").forEach((badge) => {
        const value = body[badge.dataset.badge];
        if (value) badge.textContent = value;
      });
    } catch (error) {
      toast(error.status === 409 ? "That status change is not allowed" : error.message, "error");
    } finally {
      if (button) button.disabled = false;
    }
  }
}
