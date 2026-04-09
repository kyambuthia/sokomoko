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
