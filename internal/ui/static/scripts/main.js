class UIAlert extends HTMLElement {
  static get observedAttributes() {
    return ["variant", "title"];
  }

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
  }

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback() {
    this.render();
  }

  render() {
    const variant = (this.getAttribute("variant") || "info").toLowerCase();
    const title = this.getAttribute("title") || "";

    const palettes = {
      info: { border: "#b7cada", bg: "#eef5fa", text: "#32516a" },
      success: { border: "#9ebda8", bg: "#edf7f0", text: "#24563a" },
      warning: { border: "#d3be8d", bg: "#faf4e6", text: "#6d571b" },
      danger: { border: "#d7aaaa", bg: "#fbefef", text: "#7b2929" },
    };

    const palette = palettes[variant] || palettes.info;
    const role = variant === "danger" ? "alert" : "status";
    const ariaLive = variant === "danger" ? "assertive" : "polite";

    this.root.innerHTML = `
      <style>
        :host {
          display: block;
          margin: 0;
        }

        .alert {
          border: 1px solid var(--alert-border, ${palette.border});
          background: var(--alert-bg, ${palette.bg});
          color: var(--alert-text, ${palette.text});
          border-radius: var(--radius-md, 0.625rem);
          padding: var(--space-3, 0.75rem) var(--space-4, 1rem);
          line-height: 1.45;
          font: inherit;
        }

        .title {
          display: block;
          margin: 0 0 var(--space-1, 0.25rem);
          font-weight: var(--font-weight-bold, 700);
          color: inherit;
        }

        ::slotted(p) {
          margin: 0;
        }
      </style>
      <section class="alert" role="${role}" aria-live="${ariaLive}">
        ${title ? `<span class="title">${title}</span>` : ""}
        <slot></slot>
      </section>
    `;
  }
}

class UIMetricCard extends HTMLElement {
  static get observedAttributes() {
    return ["label", "value", "note"];
  }

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
  }

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback() {
    this.render();
  }

  render() {
    const label = this.getAttribute("label") || "Metric";
    const value = this.getAttribute("value") || "0";
    const note = this.getAttribute("note") || "";

    this.root.innerHTML = `
      <style>
        :host {
          display: block;
        }

        .metric {
          border: var(--border-thin, 1px) solid var(--color-border-strong, #beb7a8);
          background: var(--color-bg-elevated, #fff);
          border-radius: var(--radius-md, 0.625rem);
          padding: var(--space-4, 1rem);
        }

        .label {
          margin: 0 0 var(--space-2, 0.5rem);
          font-size: var(--font-size-sm, 0.875rem);
          letter-spacing: var(--letter-spacing-wide, 0.04em);
          text-transform: uppercase;
          color: var(--color-fg-secondary, #666258);
          font-weight: var(--font-weight-semibold, 600);
        }

        .value {
          margin: 0;
          font-size: var(--font-size-2xl, 1.625rem);
          line-height: var(--line-height-tight, 1.2);
          color: var(--color-fg-primary, #1f1e1a);
          font-weight: var(--font-weight-bold, 700);
          word-break: break-word;
        }

        .note {
          margin: var(--space-2, 0.5rem) 0 0;
          color: var(--color-fg-secondary, #666258);
          font-size: var(--font-size-sm, 0.875rem);
        }
      </style>
      <article class="metric">
        <h3 class="label">${label}</h3>
        <p class="value">${value}</p>
        ${note ? `<p class="note">${note}</p>` : ""}
      </article>
    `;
  }
}

class UISearchForm extends HTMLElement {
  static nextID = 0;

  connectedCallback() {
    if (this.dataset.rendered === "true") {
      return;
    }
    this.dataset.rendered = "true";

    const action = this.getAttribute("action") || "/search";
    const queryValue = this.getAttribute("value") || "";
    const label = this.getAttribute("label") || "Search";
    const placeholder = this.getAttribute("placeholder") || "Search products...";
    const hint = this.getAttribute("hint") || "";
    const submitLabel = this.getAttribute("submit-label") || "Search";
    const showClear = this.getAttribute("show-clear") === "true";
    const clearURL = this.getAttribute("clear-url") || action;
    const variant = (this.getAttribute("variant") || "default").toLowerCase();
    const hideLabel = this.getAttribute("hide-label") === "true";
    const inputID = `ui-search-input-${UISearchForm.nextID++}`;

    this.classList.add("search-form", "search-form-component", `search-form--${variant}`);
    this.innerHTML = `
      <form action="${escapeHTML(action)}" method="GET" class="search-form__form search-form__form--${escapeHTML(variant)}">
        <div class="search-form__field">
          <label class="form__label search-form__label${hideLabel ? " search-form__label--hidden" : ""}" for="${inputID}">${escapeHTML(label)}</label>
          <div class="search-form__input-wrap">
            <svg class="search-form__icon" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <circle cx="6.5" cy="6.5" r="5" stroke="currentColor" stroke-width="1.5"/>
              <path d="M10 10L14 14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>
            <input
              type="search"
              id="${inputID}"
              name="q"
              placeholder="${escapeHTML(placeholder)}"
              value="${escapeHTML(queryValue)}"
              class="form__input search-form__input"
              required
              minlength="2"
            />
          </div>
          ${hint ? `<p class="form__hint search-form__hint">${escapeHTML(hint)}</p>` : ""}
        </div>
        <div class="search-form__actions">
          <button type="submit" class="btn btn--primary search-form__submit search-form__submit--${escapeHTML(variant)}">${escapeHTML(submitLabel)}</button>
          ${showClear ? `<a href="${escapeHTML(clearURL)}" class="btn btn--secondary search-form__clear">Clear</a>` : ""}
        </div>
      </form>
    `;
  }
}

class UISwitch extends HTMLElement {
  static get observedAttributes() {
    return ["checked", "disabled", "size"];
  }

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
  }

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback() {
    this.render();
  }

  render() {
    const checked = this.hasAttribute("checked");
    const disabled = this.hasAttribute("disabled");
    const size = this.getAttribute("size") || "default";
    const inputID = `ui-switch-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const labelText = this.getAttribute("label") || "";

    this.root.innerHTML = `
      <style>
        :host {
          display: inline-flex;
        }

        .switch {
          display: inline-flex;
          align-items: center;
          gap: var(--space-3, 0.75rem);
          cursor: pointer;
        }

        .switch--disabled {
          cursor: not-allowed;
          opacity: 0.6;
        }

        .switch__control {
          position: relative;
          inline-size: var(--switch-width, 2.75rem);
          block-size: var(--switch-height, 1.5rem);
          border: var(--border-thin, 1px) solid var(--color-border-strong, #bcb3a2);
          border-radius: var(--radius-pill, 999px);
          background: var(--color-bg-surface-muted, #f7f3eb);
          transition:
            background var(--transition-base, 220ms) var(--ease-standard, ease-in-out),
            border-color var(--transition-base, 220ms) var(--ease-standard, ease-in-out);
        }

        .switch--sm .switch__control {
          --switch-width: 2.25rem;
          --switch-height: 1.25rem;
        }

        .switch--lg .switch__control {
          --switch-width: 3.25rem;
          --switch-height: 1.75rem;
        }

        .switch__control::after {
          content: "";
          position: absolute;
          inset-block-start: 50%;
          inset-inline-start: 0.1875rem;
          inline-size: calc(var(--switch-height, 1.5rem) - 0.5rem);
          block-size: calc(var(--switch-height, 1.5rem) - 0.5rem);
          border-radius: 50%;
          background: var(--color-bg-elevated, #ffffff);
          transform: translateY(-50%);
          transition: transform var(--transition-base, 220ms) var(--ease-emphasized, cubic-bezier(0.2, 0, 0, 1));
        }

        .switch--sm .switch__control::after {
          inline-size: calc(var(--switch-height, 1.25rem) - 0.375rem);
          block-size: calc(var(--switch-height, 1.25rem) - 0.375rem);
        }

        .switch--lg .switch__control::after {
          inline-size: calc(var(--switch-height, 1.75rem) - 0.625rem);
          block-size: calc(var(--switch-height, 1.75rem) - 0.625rem);
        }

        .switch__input {
          position: absolute;
          width: 1px;
          height: 1px;
          padding: 0;
          margin: -1px;
          overflow: hidden;
          clip: rect(0, 0, 0, 0);
          white-space: nowrap;
          border: 0;
        }

        .switch__input:checked + .switch__control {
          background: var(--color-accent-soft, #dbe5ee);
          border-color: var(--color-accent, #2f3e4f);
        }

        .switch__input:checked + .switch__control::after {
          transform: translate(calc(var(--switch-width, 2.75rem) - var(--switch-height, 1.5rem)), -50%);
        }

        .switch--sm .switch__input:checked + .switch__control::after {
          transform: translate(calc(var(--switch-width, 2.25rem) - var(--switch-height, 1.25rem)), -50%);
        }

        .switch--lg .switch__input:checked + .switch__control::after {
          transform: translate(calc(var(--switch-width, 3.25rem) - var(--switch-height, 1.75rem)), -50%);
        }

        .switch__input:focus-visible + .switch__control {
          outline: 2px solid var(--color-focus-ring, #d9e5ef);
          outline-offset: 2px;
        }

        .switch__label {
          color: var(--color-fg-primary, #1f1e1a);
          font-size: var(--font-size-base, 0.96875rem);
        }
      </style>
      <label class="switch ${disabled ? "switch--disabled" : ""} ${size !== "default" ? `switch--${size}` : ""}">
        <input
          type="checkbox"
          class="switch__input"
          id="${inputID}"
          ${checked ? "checked" : ""}
          ${disabled ? "disabled" : ""}
        />
        <span class="switch__control" aria-hidden="true"></span>
        ${labelText ? `<span class="switch__label">${escapeHTML(labelText)}</span>` : ""}
      </label>
    `;
  }
}

// ---------------------------------------------------------------------------
// UIToast — shadow DOM toast notification
// ---------------------------------------------------------------------------

class UIToast extends HTMLElement {
  static get observedAttributes() {
    return ["variant", "message", "duration"];
  }

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
  }

  connectedCallback() {
    this.render();
    const duration = parseInt(this.getAttribute("duration") || "4000", 10);
    if (duration > 0) {
      setTimeout(() => this.dismiss(), duration);
    }
  }

  attributeChangedCallback() {
    this.render();
  }

  dismiss() {
    this.classList.add("is-dismissing");
    setTimeout(() => this.remove(), 300);
  }

  render() {
    const variant = (this.getAttribute("variant") || "info").toLowerCase();
    const message = this.getAttribute("message") || "";

    const palettes = {
      info: {
        border: "var(--color-info-border, #b7cada)",
        bg: "var(--color-info-soft, #eef5fa)",
        text: "var(--color-info, #32516a)",
      },
      success: {
        border: "var(--color-success-border, #9ebda8)",
        bg: "var(--color-success-soft, #edf7f0)",
        text: "var(--color-success, #24563a)",
      },
      warning: {
        border: "var(--color-warning-border, #d3be8d)",
        bg: "var(--color-warning-soft, #faf4e6)",
        text: "var(--color-warning, #6d571b)",
      },
      danger: {
        border: "var(--color-danger-border, #d7aaaa)",
        bg: "var(--color-danger-soft, #fbefef)",
        text: "var(--color-danger, #7b2929)",
      },
    };

    const palette = palettes[variant] || palettes.info;
    const ariaLive = variant === "danger" ? "assertive" : "polite";
    const role = variant === "danger" ? "alert" : "status";

    this.root.innerHTML = `
      <style>
        :host {
          display: block;
          animation: toast-in var(--transition-base, 220ms) var(--ease-emphasized, cubic-bezier(0.2, 0, 0, 1));
        }

        :host(.is-dismissing) {
          animation: toast-out var(--transition-slow, 300ms) var(--ease-exit, ease-in) forwards;
        }

        @keyframes toast-in {
          from { opacity: 0; transform: translateX(1rem); }
          to   { opacity: 1; transform: translateX(0); }
        }

        @keyframes toast-out {
          from { opacity: 1; transform: translateX(0); }
          to   { opacity: 0; transform: translateX(1rem); }
        }

        .toast {
          display: flex;
          align-items: flex-start;
          gap: var(--space-3, 0.75rem);
          padding: var(--space-3, 0.75rem) var(--space-4, 1rem);
          border: 1px solid ${palette.border};
          border-radius: var(--radius-md, 0.625rem);
          background: ${palette.bg};
          color: ${palette.text};
          box-shadow: 0 2px 8px var(--shadow-color, rgb(31 29 25 / 0.1));
          font: inherit;
          width: 100%;
        }

        .toast__message {
          flex: 1;
          font-size: var(--font-size-sm, 0.875rem);
          line-height: var(--line-height-base, 1.55);
        }

        .toast__close {
          flex-shrink: 0;
          appearance: none;
          background: none;
          border: none;
          padding: 0;
          margin: 0;
          cursor: pointer;
          color: inherit;
          opacity: 0.65;
          font-size: 0.9rem;
          line-height: 1;
          font-family: inherit;
        }

        .toast__close:hover {
          opacity: 1;
        }

        .toast__close:focus-visible {
          outline: 2px solid currentColor;
          outline-offset: 2px;
          border-radius: 2px;
        }
      </style>
      <div class="toast" role="${role}" aria-live="${ariaLive}">
        <span class="toast__message">${escapeHTML(message)}</span>
        <button class="toast__close" aria-label="Dismiss notification">✕</button>
      </div>
    `;

    this.root.querySelector(".toast__close")?.addEventListener("click", () => this.dismiss());
  }
}

// Global toast API: UIToast.show / .success / .danger / .warning / .info
const UIToastManager = {
  _container: null,

  _getContainer() {
    if (!this._container || !document.body.contains(this._container)) {
      this._container = document.createElement("div");
      this._container.className = "ui-toast-container";
      this._container.setAttribute("aria-label", "Notifications");
      document.body.appendChild(this._container);
    }
    return this._container;
  },

  show(message, variant = "info", duration = 4000) {
    const toast = document.createElement("ui-toast");
    toast.setAttribute("message", message);
    toast.setAttribute("variant", variant);
    toast.setAttribute("duration", String(duration));
    this._getContainer().appendChild(toast);
    return toast;
  },

  success(message, duration) { return this.show(message, "success", duration); },
  danger(message, duration) { return this.show(message, "danger", duration); },
  warning(message, duration) { return this.show(message, "warning", duration); },
  info(message, duration) { return this.show(message, "info", duration); },
};

window.UIToast = UIToastManager;

// ---------------------------------------------------------------------------
// UIDialog — shadow DOM modal dialog with backdrop
// ---------------------------------------------------------------------------

class UIDialog extends HTMLElement {
  static get observedAttributes() {
    return ["open", "title"];
  }

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
    this._handleKeydown = this._handleKeydown.bind(this);
  }

  connectedCallback() {
    this.render();
  }

  attributeChangedCallback(name) {
    this.render();
    if (name === "open") {
      if (this.hasAttribute("open")) {
        document.addEventListener("keydown", this._handleKeydown);
        document.body.style.overflow = "hidden";
        requestAnimationFrame(() => {
          this.root.querySelector(".dialog")?.focus();
        });
      } else {
        document.removeEventListener("keydown", this._handleKeydown);
        document.body.style.removeProperty("overflow");
      }
    }
  }

  disconnectedCallback() {
    document.removeEventListener("keydown", this._handleKeydown);
    document.body.style.removeProperty("overflow");
  }

  _handleKeydown(e) {
    if (e.key === "Escape") this.close();
  }

  open() {
    this.setAttribute("open", "");
    this.dispatchEvent(new CustomEvent("ui-open", { bubbles: true }));
  }

  close() {
    this.removeAttribute("open");
    this.dispatchEvent(new CustomEvent("ui-close", { bubbles: true }));
  }

  render() {
    const isOpen = this.hasAttribute("open");
    const title = this.getAttribute("title") || "";

    this.root.innerHTML = `
      <style>
        :host {
          display: ${isOpen ? "block" : "none"};
        }

        .backdrop {
          position: fixed;
          inset: 0;
          background: var(--color-overlay, rgb(31 29 25 / 0.5));
          z-index: 100;
          display: flex;
          align-items: center;
          justify-content: center;
          padding: var(--space-4, 1rem);
          animation: backdrop-in var(--transition-base, 220ms) var(--ease-standard, ease-in-out);
        }

        @keyframes backdrop-in {
          from { opacity: 0; }
          to   { opacity: 1; }
        }

        .dialog {
          position: relative;
          background: var(--color-bg-elevated, #fff);
          border: 1px solid var(--color-border-default, #d7d0c3);
          border-radius: var(--radius-lg, 0.875rem);
          box-shadow: 0 8px 32px var(--shadow-color, rgb(31 29 25 / 0.12));
          width: min(36rem, 95vw);
          max-height: 90vh;
          overflow-y: auto;
          animation: dialog-in var(--transition-base, 220ms) var(--ease-emphasized, cubic-bezier(0.2, 0, 0, 1));
        }

        @keyframes dialog-in {
          from { opacity: 0; transform: scale(0.96) translateY(-0.5rem); }
          to   { opacity: 1; transform: scale(1) translateY(0); }
        }

        .dialog__header {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: var(--space-4, 1rem);
          padding: var(--space-4, 1rem) var(--space-5, 1.25rem);
          border-bottom: 1px solid var(--color-border-subtle, #e6dfd3);
          background: var(--color-bg-surface-muted, #f7f3eb);
          border-radius: var(--radius-lg, 0.875rem) var(--radius-lg, 0.875rem) 0 0;
        }

        .dialog__title {
          margin: 0;
          font-size: var(--font-size-base, 0.96875rem);
          font-weight: var(--font-weight-semibold, 600);
          color: var(--color-fg-primary, #1f1d19);
        }

        .dialog__close {
          appearance: none;
          background: none;
          border: 1px solid transparent;
          padding: var(--space-1, 0.25rem) var(--space-2, 0.5rem);
          margin: 0;
          cursor: pointer;
          color: var(--color-fg-muted, #726b5f);
          font-size: 1rem;
          line-height: 1;
          border-radius: var(--radius-sm, 0.375rem);
          font-family: inherit;
          transition: background var(--transition-fast, 150ms);
        }

        .dialog__close:hover {
          background: var(--color-bg-surface, #fffdfa);
          color: var(--color-fg-primary, #1f1d19);
          border-color: var(--color-border-default, #d7d0c3);
        }

        .dialog__close:focus-visible {
          outline: 2px solid var(--color-focus-ring, #d9e5ef);
          outline-offset: 2px;
        }

        .dialog__body {
          padding: var(--space-5, 1.25rem);
          color: var(--color-fg-primary, #1f1d19);
          font-size: var(--font-size-base, 0.96875rem);
          line-height: var(--line-height-base, 1.55);
        }

        .dialog__footer {
          padding: var(--space-4, 1rem) var(--space-5, 1.25rem);
          border-top: 1px solid var(--color-border-subtle, #e6dfd3);
          background: var(--color-bg-surface-muted, #f7f3eb);
          border-radius: 0 0 var(--radius-lg, 0.875rem) var(--radius-lg, 0.875rem);
          display: flex;
          flex-wrap: wrap;
          gap: var(--space-3, 0.75rem);
          justify-content: flex-end;
        }

        .dialog__footer:empty {
          display: none;
        }
      </style>
      <div class="backdrop" role="presentation">
        <div
          class="dialog"
          role="dialog"
          aria-modal="true"
          ${title ? `aria-labelledby="ui-dialog-title"` : ""}
          tabindex="-1"
        >
          <div class="dialog__header">
            ${title ? `<h2 id="ui-dialog-title" class="dialog__title">${escapeHTML(title)}</h2>` : "<span></span>"}
            <button class="dialog__close" aria-label="Close dialog">✕</button>
          </div>
          <div class="dialog__body">
            <slot></slot>
          </div>
          <div class="dialog__footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    `;

    this.root.querySelector(".backdrop")?.addEventListener("click", (e) => {
      if (e.target === e.currentTarget) this.close();
    });

    this.root.querySelector(".dialog__close")?.addEventListener("click", () => this.close());
  }
}

// ---------------------------------------------------------------------------
// UIQuantity — light DOM quantity stepper (form-participates natively)
// ---------------------------------------------------------------------------

class UIQuantity extends HTMLElement {
  static get observedAttributes() {
    return ["value", "min", "max", "step", "disabled", "name"];
  }

  connectedCallback() {
    this._render();
  }

  attributeChangedCallback() {
    if (this.isConnected) this._render();
  }

  _render() {
    const value = parseInt(this.getAttribute("value") || "1", 10);
    const min = parseInt(this.getAttribute("min") || "1", 10);
    const max = parseInt(this.getAttribute("max") || "999", 10);
    const step = parseInt(this.getAttribute("step") || "1", 10);
    const disabled = this.hasAttribute("disabled");
    const name = this.getAttribute("name") || "quantity";

    this.innerHTML = `
      <div class="quantity-stepper">
        <button
          class="quantity-stepper__btn"
          type="button"
          aria-label="Decrease quantity"
          data-action="dec"
          ${value <= min || disabled ? "disabled" : ""}
        >−</button>
        <input
          class="quantity-stepper__input"
          type="number"
          name="${escapeHTML(name)}"
          value="${value}"
          min="${min}"
          max="${max}"
          step="${step}"
          ${disabled ? "disabled" : ""}
          aria-label="Quantity"
        />
        <button
          class="quantity-stepper__btn"
          type="button"
          aria-label="Increase quantity"
          data-action="inc"
          ${value >= max || disabled ? "disabled" : ""}
        >+</button>
      </div>
    `;

    const input = this.querySelector(".quantity-stepper__input");
    const decBtn = this.querySelector('[data-action="dec"]');
    const incBtn = this.querySelector('[data-action="inc"]');

    const syncButtons = (v) => {
      if (decBtn) decBtn.disabled = v <= min || disabled;
      if (incBtn) incBtn.disabled = v >= max || disabled;
    };

    decBtn?.addEventListener("click", () => {
      let v = parseInt(input.value, 10) - step;
      v = Math.max(min, v);
      input.value = v;
      this.setAttribute("value", String(v));
      syncButtons(v);
      this.dispatchEvent(new CustomEvent("change", { bubbles: true, detail: { value: v } }));
    });

    incBtn?.addEventListener("click", () => {
      let v = parseInt(input.value, 10) + step;
      v = Math.min(max, v);
      input.value = v;
      this.setAttribute("value", String(v));
      syncButtons(v);
      this.dispatchEvent(new CustomEvent("change", { bubbles: true, detail: { value: v } }));
    });

    input?.addEventListener("input", () => {
      let v = parseInt(input.value, 10);
      if (!isNaN(v)) {
        v = Math.max(min, Math.min(max, v));
        input.value = v;
        this.setAttribute("value", String(v));
        syncButtons(v);
        this.dispatchEvent(new CustomEvent("change", { bubbles: true, detail: { value: v } }));
      }
    });
  }
}

// ---------------------------------------------------------------------------
// UIBadge — light DOM badge; applies .badge + variant class to host element
// ---------------------------------------------------------------------------

class UIBadge extends HTMLElement {
  static get observedAttributes() {
    return ["variant", "label"];
  }

  connectedCallback() {
    this._render();
  }

  attributeChangedCallback() {
    if (this.isConnected) this._render();
  }

  _render() {
    const variant = this.getAttribute("variant") || "default";
    const label = this.getAttribute("label");

    const variantClass = variant !== "default" ? `badge--${variant}` : "";
    this.className = ["badge", variantClass].filter(Boolean).join(" ");

    if (label !== null) {
      this.textContent = label;
    }
  }
}

// ---------------------------------------------------------------------------
// UIBreadcrumb — light DOM breadcrumb nav from JSON `items` attribute
// items: [{ label: string, href?: string }]
// ---------------------------------------------------------------------------

class UIBreadcrumb extends HTMLElement {
  static get observedAttributes() {
    return ["items"];
  }

  connectedCallback() {
    this._render();
  }

  attributeChangedCallback() {
    if (this.isConnected) this._render();
  }

  _render() {
    let items = [];
    try {
      items = JSON.parse(this.getAttribute("items") || "[]");
    } catch {
      return;
    }

    const listItems = items
      .map((item, i) => {
        const isLast = i === items.length - 1;
        if (isLast) {
          return `<li class="breadcrumb__item breadcrumb__item--current" aria-current="page">${escapeHTML(item.label)}</li>`;
        }
        return `<li class="breadcrumb__item"><a class="breadcrumb__link" href="${escapeHTML(item.href || "#")}">${escapeHTML(item.label)}</a></li>`;
      })
      .join("");

    this.innerHTML = `
      <nav class="breadcrumb" aria-label="Breadcrumb">
        <ol class="breadcrumb__list">${listItems}</ol>
      </nav>
    `;
  }
}

// ---------------------------------------------------------------------------
// UIPrice — light DOM price display with optional strikethrough original
// ---------------------------------------------------------------------------

class UIPrice extends HTMLElement {
  static get observedAttributes() {
    return ["amount", "currency", "original", "size"];
  }

  connectedCallback() {
    this._render();
  }

  attributeChangedCallback() {
    if (this.isConnected) this._render();
  }

  _render() {
    const amount = parseFloat(this.getAttribute("amount") || "0");
    const currency = this.getAttribute("currency") || "USD";
    const originalAttr = this.getAttribute("original");
    const size = this.getAttribute("size") || "base";

    const fmt = (v) =>
      new Intl.NumberFormat("en-US", { style: "currency", currency }).format(v);

    const hasDiscount =
      originalAttr !== null && parseFloat(originalAttr) > amount;

    this.className = `price price--${size}`;
    this.innerHTML = hasDiscount
      ? `<span class="price__current">${escapeHTML(fmt(amount))}</span><del class="price__original">${escapeHTML(fmt(parseFloat(originalAttr)))}</del>`
      : `<span class="price__current">${escapeHTML(fmt(amount))}</span>`;
  }
}

if (!customElements.get("ui-alert")) {
  customElements.define("ui-alert", UIAlert);
}

if (!customElements.get("ui-metric-card")) {
  customElements.define("ui-metric-card", UIMetricCard);
}

if (!customElements.get("ui-search-form")) {
  customElements.define("ui-search-form", UISearchForm);
}

if (!customElements.get("ui-switch")) {
  customElements.define("ui-switch", UISwitch);
}

if (!customElements.get("ui-toast")) {
  customElements.define("ui-toast", UIToast);
}

if (!customElements.get("ui-dialog")) {
  customElements.define("ui-dialog", UIDialog);
}

if (!customElements.get("ui-quantity")) {
  customElements.define("ui-quantity", UIQuantity);
}

if (!customElements.get("ui-badge")) {
  customElements.define("ui-badge", UIBadge);
}

if (!customElements.get("ui-breadcrumb")) {
  customElements.define("ui-breadcrumb", UIBreadcrumb);
}

if (!customElements.get("ui-price")) {
  customElements.define("ui-price", UIPrice);
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

const prefersReducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

document.addEventListener("DOMContentLoaded", () => {
  initPasswordToggles();
  initNavToggles();
  initFormSubmitStates();
});

function initPasswordToggles() {
  const passwordToggles = document.querySelectorAll("[data-password-toggle]");
  passwordToggles.forEach((toggleButton) => {
    const targetId = toggleButton.getAttribute("data-target");
    if (!targetId) {
      return;
    }
    const input = document.getElementById(targetId);
    if (!input) {
      return;
    }
    toggleButton.addEventListener("click", () => {
      const isPassword = input.getAttribute("type") === "password";
      input.setAttribute("type", isPassword ? "text" : "password");
      toggleButton.textContent = isPassword ? "Hide" : "Show";
      toggleButton.setAttribute("aria-label", isPassword ? "Hide password" : "Show password");
    });
  });
}

function initNavToggles() {
  const navBlocks = document.querySelectorAll("[data-nav]");
  const desktopNav = window.matchMedia("(min-width: 40rem)");

  navBlocks.forEach((nav) => {
    const toggle = nav.querySelector("[data-nav-toggle]");
    const menu = nav.querySelector("[data-nav-menu]");
    if (!toggle || !menu) {
      return;
    }

    const collapsible = nav.dataset.navCollapsible || "mobile";
    let expanded = false;

    toggle.setAttribute("aria-controls", menu.id || undefined);

    const shouldCollapse = () => {
      if (collapsible === "always") {
        return true;
      }
      if (collapsible === "never") {
        return false;
      }
      return !desktopNav.matches;
    };

    const syncNavState = () => {
      const collapsed = shouldCollapse() && !expanded;
      nav.classList.toggle("is-collapsed", collapsed);
      toggle.hidden = !shouldCollapse();
      toggle.setAttribute("aria-expanded", String(!collapsed));

      if (prefersReducedMotion.matches) {
        menu.style.removeProperty("max-height");
        menu.style.removeProperty("opacity");
        menu.style.removeProperty("overflow");
        return;
      }

      menu.style.overflow = "hidden";
      if (collapsed) {
        menu.style.maxHeight = "0";
        menu.style.opacity = "0";
      } else {
        menu.style.maxHeight = menu.scrollHeight + "px";
        menu.style.opacity = "1";
      }
    };

    toggle.addEventListener("click", () => {
      expanded = !expanded;
      syncNavState();
    });

    toggle.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && shouldCollapse() && expanded) {
        expanded = false;
        syncNavState();
      }
    });

    const handleViewportChange = () => {
      expanded = desktopNav.matches;
      syncNavState();
    };

    if (desktopNav.addEventListener) {
      desktopNav.addEventListener("change", handleViewportChange);
    } else {
      desktopNav.addListener(handleViewportChange);
    }

    handleViewportChange();
  });
}

function initFormSubmitStates() {
  const forms = document.querySelectorAll("form");
  forms.forEach((form) => {
    form.addEventListener("submit", () => {
      const method = (form.getAttribute("method") || "get").toLowerCase();
      if (method === "get" || form.hasAttribute("data-no-submit-state")) {
        return;
      }
      if (!form.checkValidity()) {
        return;
      }
      const submitButtons = form.querySelectorAll('button[type="submit"], input[type="submit"]');
      submitButtons.forEach((button) => {
        const originalLabel = button.tagName === "INPUT" ? button.value : button.textContent;
        button.dataset.originalLabel = originalLabel;
        if (button.tagName === "INPUT") {
          button.value = "Submitting...";
        } else {
          button.textContent = "Submitting...";
        }
        button.disabled = true;
      });
    });
  });
}
