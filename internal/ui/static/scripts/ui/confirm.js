let nextID = 0;

// <ui-confirm message="…" confirm-label="Delete" cancel-label="Keep"><form>…</form></ui-confirm>
// Asks before a destructive form submits, using a native modal dialog.
export class UIConfirm extends HTMLElement {
  connectedCallback() {
    this.form = this.querySelector("form");
    this.form?.addEventListener("submit", (event) => this.intercept(event));
  }

  intercept(event) {
    if (this.confirmed) {
      this.confirmed = false;
      return;
    }
    event.preventDefault();
    event.stopImmediatePropagation();
    this.dialog().showModal();
  }

  dialog() {
    if (this.modal) return this.modal;
    const id = nextID++;
    const modal = document.createElement("dialog");
    const title = document.createElement("h2");
    title.id = `confirm-title-${id}`;
    title.textContent = this.getAttribute("title") || "Are you sure?";
    modal.setAttribute("aria-labelledby", title.id);

    const message = this.getAttribute("message") || "";
    const text = document.createElement("p");
    text.className = "muted small";
    text.textContent = message;
    if (message.trim()) {
      text.id = `confirm-message-${id}`;
      modal.setAttribute("aria-describedby", text.id);
    }

    const actions = document.createElement("div");
    actions.className = "row-between";
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "btn btn-secondary";
    cancel.textContent = this.getAttribute("cancel-label") || "Cancel";
    cancel.addEventListener("click", () => modal.close());
    const confirm = document.createElement("button");
    confirm.type = "button";
    confirm.className = "btn";
    confirm.textContent = this.getAttribute("confirm-label") || "Confirm";
    confirm.addEventListener("click", () => {
      modal.close();
      this.confirmed = true;
      this.form.requestSubmit();
    });
    actions.append(cancel, confirm);
    modal.append(title, text, actions);
    this.append(modal);
    this.modal = modal;
    return modal;
  }
}
