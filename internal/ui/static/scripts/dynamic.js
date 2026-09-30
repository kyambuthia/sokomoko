(function () {
  function api() {
    return window.SokomokoAPI;
  }

  function showError(error) {
    if (window.UIToast) {
      window.UIToast.danger(error?.message || "Something went wrong. Please try again.");
    }
  }

  function setSubmitting(form, submitting) {
    form.querySelectorAll('button[type="submit"], input[type="submit"]').forEach((button) => {
      if (submitting) {
        button.dataset.originalLabel = button.tagName === "INPUT" ? button.value : button.textContent;
        if (button.tagName === "INPUT") button.value = "Saving...";
        else button.textContent = "Saving...";
        button.disabled = true;
      } else {
        const original = button.dataset.originalLabel;
        if (original) {
          if (button.tagName === "INPUT") button.value = original;
          else button.textContent = original;
        }
        button.disabled = false;
      }
    });
  }

  function updateCartSummary(cart) {
    document.querySelectorAll("[data-cart-subtotal]").forEach((element) => {
      element.textContent = `$${Number(cart.subtotal || 0).toFixed(2)}`;
    });
    document.querySelectorAll("[data-cart-count]").forEach((element) => {
      const count = (cart.items || []).reduce((total, item) => total + Number(item.quantity || 0), 0);
      element.textContent = count ? `Cart (${count})` : "Cart";
    });
  }

  function updateCartLine(productID, item) {
    const row = document.querySelector(`[data-cart-item="${CSS.escape(String(productID))}"]`);
    if (!row) return;
    if (!item) {
      row.remove();
      return;
    }
    row.querySelector("[data-cart-line-total]")?.replaceChildren(`$${Number(item.line_total || 0).toFixed(2)}`);
    const input = row.querySelector('input[name="quantity"]');
    if (input) input.value = item.quantity;
  }

  async function refreshCart() {
    const cart = await api().request("/cart");
    updateCartSummary(cart);
    return cart;
  }

  function initCartForms() {
    document.querySelectorAll('form[action="/cart/add"]').forEach((form) => {
      form.addEventListener("submit", async (event) => {
        if (!api() || !form.checkValidity()) return;
        event.preventDefault();
        setSubmitting(form, true);
        try {
          const data = new FormData(form);
          const cart = await api().request("/cart/items", {
            method: "POST",
            body: {
              product_id: Number(data.get("product_id")),
              quantity: Number(data.get("quantity") || 1),
            },
          });
          updateCartSummary(cart);
          window.UIToast?.success("Item added to cart.");
        } catch (error) {
          if (error.status === 401) {
            form.submit();
            return;
          }
          showError(error);
        } finally {
          setSubmitting(form, false);
        }
      });
    });

    document.querySelectorAll('form[action="/cart/update"], form[action="/cart/remove"]').forEach((form) => {
      form.addEventListener("submit", async (event) => {
        if (!api() || !form.checkValidity()) return;
        event.preventDefault();
        setSubmitting(form, true);
        const data = new FormData(form);
        const productID = Number(data.get("product_id"));
        try {
          const isRemove = form.action.endsWith("/cart/remove");
          const cart = await api().request(`/cart/items/${productID}`, {
            method: isRemove ? "DELETE" : "PATCH",
            body: isRemove ? undefined : { quantity: Number(data.get("quantity")) },
          });
          updateCartLine(productID, cart.items.find((item) => Number(item.product_id) === productID));
          updateCartSummary(cart);
          if (!cart.items.length) window.location.reload();
          else window.UIToast?.success(isRemove ? "Item removed." : "Cart updated.");
        } catch (error) {
          if (error.status === 401) {
            form.submit();
            return;
          }
          showError(error);
        } finally {
          setSubmitting(form, false);
        }
      });
    });
  }

  function initCheckoutForm() {
    const form = document.querySelector("[data-api-checkout]");
    if (!form || !api()) return;
    form.addEventListener("submit", async (event) => {
      if (!form.checkValidity()) return;
      event.preventDefault();
      setSubmitting(form, true);
      const data = new FormData(form);
      const token = form.dataset.checkoutToken || data.get("idempotency_key");
      try {
        await api().request(`/checkout/${encodeURIComponent(token)}/place-order`, {
          method: "POST",
          headers: { "Idempotency-Key": token },
          body: {
            delivery_address: data.get("delivery_address"),
            payment_method: data.get("payment_method"),
          },
        });
        window.location.assign("/account?message=Order+placed");
      } catch (error) {
        showError(error);
        setSubmitting(form, false);
      }
    });
  }

  async function refreshAccountOrders() {
    const container = document.querySelector("[data-account-orders]");
    if (!container || !api()) return;
    try {
      const data = await api().request("/account/orders");
      (data.items || []).forEach((order) => {
        const row = container.querySelector(`[data-order-id="${CSS.escape(String(order.id))}"]`);
        if (!row) return;
        row.querySelector("[data-order-status]")?.replaceChildren(`${order.status} (${order.partner_status})`);
        row.querySelector("[data-delivery-status]")?.replaceChildren(order.delivery_status);
        row.querySelector("[data-delivery-notice]")?.replaceChildren(order.delivery_notice || "No updates yet");
      });
    } catch (_) {
      // The server-rendered account page remains authoritative if refresh fails.
    }
  }

  function updateMetrics(selector, values) {
    Object.entries(values || {}).forEach(([key, value]) => {
      document.querySelector(`[${selector}="${CSS.escape(key)}"]`)?.setAttribute("value", String(value ?? 0));
    });
  }

  async function refreshAdminWorkspace() {
    if (!document.querySelector("[data-admin-workspace]") || !api()) return;
    try {
      const metrics = await api().request("/admin/metrics");
      updateMetrics("data-admin-metric", metrics);
      const summary = document.querySelector("[data-admin-users]");
      if (summary) {
        summary.innerHTML = `<strong>Users:</strong> ${Number(metrics.user_count || 0)} customers, ${Number(metrics.staff_count || 0)} staff, ${Number(metrics.admin_count || 0)} admins`;
      }
    } catch (_) {
      // The server-rendered workspace remains authoritative if refresh fails.
    }
  }

  async function refreshPartnerDashboard() {
    if (!document.querySelector("[data-partner-dashboard]") || !api()) return;
    try {
      const dashboard = await api().request("/partner/dashboard");
      const summary = dashboard.order_summary || {};
      updateMetrics("data-partner-metric", {
        product_count: dashboard.product_count,
        new_orders: summary.new_count,
        in_progress_orders: summary.in_progress_count,
        dispatched_orders: summary.dispatched_count,
      });
    } catch (_) {
      // The server-rendered workspace remains authoritative if refresh fails.
    }
  }

  function initWorkspaceOrderForms() {
    document.querySelectorAll("form[data-api-admin-order], form[data-api-partner-order]").forEach((form) => {
      form.addEventListener("submit", async (event) => {
        if (!api() || !form.checkValidity()) return;
        event.preventDefault();
        setSubmitting(form, true);
        const data = new FormData(form);
        const orderID = data.get("order_id");
        const isAdmin = form.hasAttribute("data-api-admin-order");
        const body = isAdmin
          ? {
              status: data.get("status"),
              partner_status: data.get("partner_status"),
              delivery_status: data.get("delivery_status"),
              delivery_notice: data.get("delivery_notice"),
            }
          : {
              partner_status: data.get("partner_status"),
              delivery_status: data.get("delivery_status"),
              delivery_notice: data.get("delivery_notice"),
            };
        try {
          await api().request(`${isAdmin ? "/admin/orders" : "/partner/orders"}/${encodeURIComponent(orderID)}`, {
            method: "PATCH",
            body,
          });
          window.UIToast?.success("Order update saved.");
          const card = form.closest(".order-card");
          const status = [body.status, body.partner_status, body.delivery_status].filter(Boolean).join(" / ");
          card?.querySelector(".order-card__details p:nth-child(2) span")?.replaceChildren(status);
        } catch (error) {
          if (error.status === 401) {
            form.submit();
            return;
          }
          showError(error);
        } finally {
          setSubmitting(form, false);
        }
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initCartForms();
    initCheckoutForm();
    initWorkspaceOrderForms();
    refreshAccountOrders();
    refreshAdminWorkspace();
    refreshPartnerDashboard();
    if (document.querySelector("[data-admin-workspace], [data-partner-dashboard]")) {
      window.setInterval(() => {
        refreshAdminWorkspace();
        refreshPartnerDashboard();
      }, 30000);
    }
  });
})();
