import { api } from "./core.js";

// Enhances the manual local-time field with advisory start-time suggestions.
export class UIBookingAvailability extends HTMLElement {
  connectedCallback() {
    if (this.ready) return;
    this.ready = true;
    this.slug = this.dataset.serviceSlug || "";
    this.dateInput = this.querySelector("[data-availability-date]");
    this.slotSelect = this.querySelector("[data-availability-slots]");
    this.slotLabel = this.querySelector("[data-availability-label]");
    this.manualInput = this.querySelector("[data-manual-start]");
    this.status = this.querySelector("[data-availability-status]");
    // Optional: suggested times as tappable chips instead of the select.
    this.chips = this.querySelector("[data-availability-chips]");
    if (!this.slug || !this.dateInput || !this.slotSelect || !this.manualInput) return;

    this.sequence = 0;
    this.dateInput.addEventListener("change", () => {
      this.manualInput.value = "";
      this.load();
    });
    this.manualInput.addEventListener("input", () => {
      if (this.slotSelect.value) this.slotSelect.value = "";
      this.syncChips();
    });
    this.slotSelect.addEventListener("change", () => {
      if (this.slotSelect.value) this.manualInput.value = this.slotSelect.value;
      this.syncChips();
    });
    if (this.dateInput.value) void this.load();
  }

  async load() {
    const date = this.dateInput.value;
    const sequence = ++this.sequence;
    this.slotSelect.disabled = true;
    this.slotSelect.hidden = !date;
    if (this.slotLabel) this.slotLabel.hidden = true;
    if (this.chips) {
      this.chips.replaceChildren();
      this.chips.hidden = true;
      this.slotSelect.hidden = true;
    }
    this.slotSelect.replaceChildren();
    this.addOption("", date ? "Loading suggested times…" : "Choose a date to load suggestions", true);
    if (!date) {
      this.setStatus("Choose a date to see suggested start times.");
      return;
    }

    this.setStatus("Loading suggested times in the service timezone…");
    try {
      const result = await api(
        "/catalog/services/" + encodeURIComponent(this.slug) +
          "/availability?date=" + encodeURIComponent(date)
      );
      if (sequence !== this.sequence) return;
      this.slotSelect.replaceChildren();
      this.addOption("", "Choose a suggested start time", true);
      for (const slot of result?.slots || []) {
        const option = document.createElement("option");
        option.value = slot.local_start;
        option.textContent = slot.local_start.slice(11, 16);
        this.slotSelect.append(option);
      }
      this.slotSelect.disabled = !result?.slots?.length;
      this.slotSelect.hidden = !result?.slots?.length || !!this.chips;
      this.renderChips(result?.slots || []);
      if (this.slotLabel) this.slotLabel.hidden = !result?.slots?.length;
      if (result?.slots?.length) {
        this.setStatus("Choose a suggested start time or enter another time below.");
      } else {
        this.setStatus("No suggested times for this date. You can still enter a time below.");
      }
    } catch {
      if (sequence !== this.sequence) return;
      this.slotSelect.replaceChildren();
      this.slotSelect.hidden = true;
      this.addOption("", "Suggestions unavailable", true);
      this.setStatus("Suggestions could not be loaded. Enter a local start time below.");
    }
  }

  renderChips(slots) {
    if (!this.chips) return;
    this.chips.replaceChildren();
    for (const slot of slots) {
      const chip = document.createElement("button");
      chip.type = "button";
      chip.className = "chip";
      chip.value = slot.local_start;
      chip.textContent = slot.local_start.slice(11, 16);
      chip.setAttribute("aria-pressed", "false");
      chip.addEventListener("click", () => {
        this.manualInput.value = chip.value;
        this.slotSelect.value = chip.value;
        this.syncChips();
      });
      this.chips.append(chip);
    }
    this.chips.hidden = !slots.length;
    this.syncChips();
  }

  syncChips() {
    if (!this.chips) return;
    for (const chip of this.chips.querySelectorAll("button")) {
      chip.setAttribute("aria-pressed", String(chip.value === this.manualInput.value));
    }
  }

  addOption(value, label, disabled) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    option.disabled = disabled;
    option.selected = true;
    this.slotSelect.append(option);
  }

  setStatus(message) {
    if (this.status) this.status.textContent = message;
  }
}
