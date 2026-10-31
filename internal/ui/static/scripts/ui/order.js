import { api, toast } from "./core.js";

const STEPS = ["Placed", "Accepted", "Shipped", "Delivered"];

function stepFor(status, partnerStatus, deliveryStatus) {
  if (status === "delivered") return 3;
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
    clearInterval(this.timer);
    this.generation = (this.generation || 0) + 1;
    this.connected = true;
    this.pending = false;
    this.render();
    if (!this.terminal()) {
      this.timer = setInterval(() => this.refresh(), 30000);
    }
  }

  disconnectedCallback() {
    clearInterval(this.timer);
    this.connected = false;
    this.generation = (this.generation || 0) + 1;
  }

  terminal() {
    return this.getAttribute("status") === "cancelled" || stepFor(this.getAttribute("status"), this.getAttribute("partner-status"), this.getAttribute("delivery-status")) === 3;
  }

  async refresh() {
    if (document.hidden || !this.connected || this.pending || this.terminal()) return;
    const generation = this.generation;
    this.pending = true;
    try {
      const order = await api(`/account/orders/${this.getAttribute("order-id")}`);
      if (!this.connected || generation !== this.generation) return;
      this.setAttribute("status", order.status);
      this.setAttribute("partner-status", order.partner_status);
      this.setAttribute("delivery-status", order.delivery_status);
      const notice = this.closest("[data-order]")?.querySelector("[data-order-notice]");
      if (notice && order.delivery_notice) notice.textContent = order.delivery_notice;
      this.render();
      if (this.terminal()) clearInterval(this.timer);
    } catch {
      // The server-rendered state stays visible.
    } finally {
      if (generation === this.generation) this.pending = false;
    }
  }

  render() {
    const cancelled = this.getAttribute("status") === "cancelled";
    const current = stepFor(this.getAttribute("status"), this.getAttribute("partner-status"), this.getAttribute("delivery-status"));
    const list = document.createElement("ol");
    list.className = "progress";
    if (cancelled) {
      list.dataset.cancelled = "";
      list.setAttribute("aria-hidden", "true");
    } else {
      list.setAttribute("aria-label", `Order ${STEPS[current].toLowerCase()}`);
    }
    STEPS.forEach((label, index) => {
      const item = document.createElement("li");
      item.textContent = label;
      if (!cancelled && index <= current) item.dataset.done = "";
      if (!cancelled && index === current) {
        item.dataset.current = "";
        item.setAttribute("aria-current", "step");
      }
      list.append(item);
    });
    if (cancelled) {
      const status = document.createElement("p");
      status.className = "small muted";
      status.setAttribute("role", "status");
      status.textContent = "Order cancelled";
      this.replaceChildren(list, status);
    } else {
      this.replaceChildren(list);
    }
  }
}

// <ui-order-form scope="admin|partner" order-id="12"><form>…</form></ui-order-form>
// Saves fulfillment changes through the API, then reloads server-rendered state
// so summaries, filters and available controls reflect the committed result.
// The plain form post still works without JavaScript.
export class UIOrderForm extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    this.form?.addEventListener("submit", (event) => this.submit(event));
  }

  async submit(event) {
    event.preventDefault();
    if (this.pending) return;
    const data = Object.fromEntries(new FormData(this.form));
    const scope = this.getAttribute("scope") === "admin" ? "admin" : "partner";
    const body = { partner_status: data.partner_status, delivery_status: data.delivery_status, delivery_notice: data.delivery_notice || "" };
    if (scope === "admin") body.status = data.status;
    const controls = [...this.form.elements].map((control) => ({ control, disabled: control.disabled }));
    this.pending = true;
    this.form.setAttribute("aria-busy", "true");
    controls.forEach(({ control }) => { control.disabled = true; });
    let saved = false;
    try {
      await api(`/${scope}/orders/${this.getAttribute("order-id")}`, { method: "PATCH", body });
      saved = true;
      location.reload();
    } catch (error) {
      toast(error.status === 409 ? "That status change is not allowed" : error.message, "error");
    } finally {
      if (!saved) {
        this.pending = false;
        this.form.setAttribute("aria-busy", "false");
        controls.forEach(({ control, disabled }) => { control.disabled = disabled; });
      }
    }
  }
}

// <ui-payment-status order-id="12">server-rendered notice</ui-payment-status>
// Polls the payment status while the customer approves on their phone, then
// reloads the page, which redirects on success or shows the failure.
export class UIPaymentStatus extends HTMLElement {
  connectedCallback() {
    clearInterval(this.timer);
    this.generation = (this.generation || 0) + 1;
    this.connected = true;
    this.pending = false;
    this.stopped = false;
    this.started = Date.now();
    this.timer = setInterval(() => this.poll(), 4000);
  }

  disconnectedCallback() {
    clearInterval(this.timer);
    this.connected = false;
    this.generation = (this.generation || 0) + 1;
  }

  stopWaiting() {
    clearInterval(this.timer);
    this.stopped = true;
    const text = this.querySelector("[data-status-text]");
    if (text) text.textContent = "Still waiting. If you approved the payment, refresh this page.";
  }

  async poll() {
    if (document.hidden || !this.connected || this.stopped) return;
    if (Date.now() - this.started >= 5 * 60 * 1000) {
      this.stopWaiting();
      return;
    }
    if (this.pending) return;
    const generation = this.generation;
    this.pending = true;
    try {
      const status = await api(`/account/orders/${this.getAttribute("order-id")}/payment`);
      if (!this.connected || this.stopped || generation !== this.generation) return;
      if (Date.now() - this.started >= 5 * 60 * 1000) {
        this.stopWaiting();
        return;
      }
      if (["paid", "failed", "cancelled", "offline"].includes(status?.status)) {
        clearInterval(this.timer);
        this.stopped = true;
        location.reload();
      }
    } catch {
      // Keep the server-rendered message and try again on the next tick.
    } finally {
      if (generation === this.generation) this.pending = false;
    }
  }
}
