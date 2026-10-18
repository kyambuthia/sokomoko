// Shared helpers for Sokomoko components: API calls, cart events, toasts,
// icons, and money formatting. Components import only what they use.

export const CART_CHANGED = "sokomoko:cart";

export function csrfToken() {
  return document.querySelector('meta[name="csrf-token"]')?.content || "";
}

// api calls /api/v1 with JSON and the session CSRF token. It resolves to the
// response's data field and throws an Error carrying status and code.
export async function api(path, { method = "GET", body } = {}) {
  const headers = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && method !== "HEAD") headers["X-CSRF-Token"] = csrfToken();
  const response = await fetch(`/api/v1${path}`, {
    method,
    headers,
    credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let payload = null;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }
  if (!response.ok) {
    const error = new Error(payload?.error?.message || `Request failed (${response.status})`);
    error.status = response.status;
    error.code = payload?.error?.code || "request_failed";
    throw error;
  }
  return payload?.data;
}

// announceCart tells cart-aware components (badge, lines) about a new cart.
export function announceCart(cart) {
  const count = (cart?.items || []).reduce((sum, item) => sum + Number(item.quantity || 0), 0);
  document.dispatchEvent(new CustomEvent(CART_CHANGED, { detail: { cart, count } }));
}

const moneyFormat = new Intl.NumberFormat("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// Prefixes match internal/money.Currencies so client and server agree.
const currencyPrefix = {
  USD: "$", KES: "KES ", NGN: "₦", GHS: "GH₵", ZAR: "R ", UGX: "UGX ", TZS: "TZS ", RWF: "RWF ", EUR: "€", GBP: "£",
};

export function money(amount) {
  const value = Number(amount);
  if (!Number.isFinite(value)) return "";
  const code = document.querySelector('meta[name="currency"]')?.content || "USD";
  const prefix = currencyPrefix[code] ?? `${code} `;
  return `${value < 0 ? "-" : ""}${prefix}${moneyFormat.format(Math.abs(value))}`;
}

export function icon(name) {
  const sprite = document.querySelector('meta[name="icon-sprite"]')?.content || "/static/icons/sprite.svg";
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("focusable", "false");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", `${sprite}#i-${name}`);
  svg.append(use);
  return svg;
}

let toastRegion;

// toast shows a short status message. Messages are announced politely and
// dismiss themselves; there is no colour, only an icon.
export function toast(message, kind = "info") {
  if (!toastRegion) {
    toastRegion = document.createElement("div");
    toastRegion.className = "toasts";
    toastRegion.setAttribute("role", "status");
    toastRegion.setAttribute("aria-live", "polite");
    document.body.append(toastRegion);
  }
  const item = document.createElement("div");
  item.className = "toast";
  const names = { success: "success", error: "alert", info: "info" };
  const text = document.createElement("span");
  text.textContent = message;
  item.append(icon(names[kind] || "info"), text);
  toastRegion.append(item);
  setTimeout(() => item.remove(), kind === "error" ? 6000 : 3500);
}

// loginRedirect sends the browser to sign in and come back here.
export function loginRedirect() {
  const next = `${location.pathname}${location.search}`;
  location.assign(`/login?next=${encodeURIComponent(next)}`);
}

// busy toggles a submit button's pending state.
export function busy(button, pending) {
  if (!button) return;
  button.disabled = pending;
  button.setAttribute("aria-busy", String(pending));
}
