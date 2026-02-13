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

if (!customElements.get("ui-alert")) {
  customElements.define("ui-alert", UIAlert);
}

if (!customElements.get("ui-metric-card")) {
  customElements.define("ui-metric-card", UIMetricCard);
}

document.addEventListener("DOMContentLoaded", () => {
  const searchForm = document.querySelector("#searchForm");

  if (searchForm) {
    searchForm.addEventListener("submit", (e) => {
      e.preventDefault();

      const formData = new FormData(searchForm);
      const queryString = formData.get("q")?.toString().trim();

      if (!queryString) {
        return;
      }

      window.location.href = `/search?q=${encodeURIComponent(queryString)}`;
    });
  }

  const searchInput = document.querySelector("#searchInput");
  if (searchInput) {
    let debounceTimer;

    searchInput.addEventListener("input", (e) => {
      clearTimeout(debounceTimer);
      const query = e.target.value.trim();

      if (query.length < 2) {
        return;
      }

      debounceTimer = setTimeout(async () => {
        try {
          const response = await fetch("/search", {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ queryString: query }),
          });

          if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
          }

          await response.json();
        } catch (error) {
          console.error("Search failed:", error);
        }
      }, 300);
    });
  }
});
