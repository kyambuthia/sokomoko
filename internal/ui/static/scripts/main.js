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
      info: { border: "#b8c8d8", bg: "#f5f8fb", text: "#1f1e1a" },
      success: { border: "#9fc7ad", bg: "#f2f9f5", text: "#2c6a44" },
      danger: { border: "#d7abab", bg: "#fcf3f3", text: "#8b2e2e" },
      warning: { border: "#d8c49a", bg: "#faf6eb", text: "#6b561d" },
    };

    const palette = palettes[variant] || palettes.info;

    this.root.innerHTML = `
      <style>
        :host {
          display: block;
          margin: 0;
        }

        .alert {
          border: 1px solid ${palette.border};
          background: ${palette.bg};
          color: ${palette.text};
          border-radius: 10px;
          padding: 0.85rem 1rem;
          line-height: 1.45;
          font: inherit;
        }

        .title {
          display: block;
          margin: 0 0 0.35rem;
          font-weight: 700;
          color: inherit;
        }

        ::slotted(p) {
          margin: 0;
        }
      </style>
      <section class="alert" role="status" aria-live="polite">
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
          border: 1px solid #beb7a8;
          background: #fff;
          border-radius: 10px;
          padding: 1rem;
        }

        .label {
          margin: 0 0 0.4rem;
          font-size: 0.9rem;
          letter-spacing: 0.04em;
          text-transform: uppercase;
          color: #666258;
          font-weight: 600;
        }

        .value {
          margin: 0;
          font-size: 1.7rem;
          line-height: 1.2;
          color: #1f1e1a;
          font-weight: 700;
          word-break: break-word;
        }

        .note {
          margin: 0.35rem 0 0;
          color: #666258;
          font-size: 0.92rem;
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
    const inputID = `ui-search-input-${UISearchForm.nextID++}`;

    this.classList.add("search-form-component");
    this.innerHTML = `
      <form action="${escapeHTML(action)}" method="GET" class="search-bar">
        <div class="search-bar__field">
          <label class="search-bar__label" for="${inputID}">${escapeHTML(label)}</label>
          <input
            type="search"
            id="${inputID}"
            name="q"
            placeholder="${escapeHTML(placeholder)}"
            value="${escapeHTML(queryValue)}"
            class="search-bar__input"
            required
            minlength="2"
          />
          ${hint ? `<p class="search-bar__hint">${escapeHTML(hint)}</p>` : ""}
        </div>
        <div class="search-bar__actions">
          <button type="submit" class="btn search-bar__button">${escapeHTML(submitLabel)}</button>
          ${showClear ? `<a href="${escapeHTML(clearURL)}" class="btn btn--secondary search-bar__button">Clear</a>` : ""}
        </div>
      </form>
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

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

document.addEventListener("DOMContentLoaded", () => {
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
    });
  });

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
        button.dataset.originalLabel = button.tagName === "INPUT" ? button.value : button.textContent;
        if (button.tagName === "INPUT") {
          button.value = "Submitting...";
        } else {
          button.textContent = "Submitting...";
        }
        button.disabled = true;
      });
    });
  });

  const navBlocks = document.querySelectorAll("[data-nav]");
  navBlocks.forEach((nav) => {
    const toggle = nav.querySelector("[data-nav-toggle]");
    const menu = nav.querySelector("[data-nav-menu]");
    if (!toggle || !menu) {
      return;
    }

    nav.classList.add("is-collapsed");
    toggle.setAttribute("aria-expanded", "false");

    toggle.addEventListener("click", () => {
      const isExpanded = toggle.getAttribute("aria-expanded") === "true";
      toggle.setAttribute("aria-expanded", String(!isExpanded));
      nav.classList.toggle("is-collapsed", isExpanded);
    });
  });

});
