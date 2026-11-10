import { api, busy, csrfToken } from "./core.js";

// The server-rendered forms remain usable without JavaScript. This component
// adds polling and sends replies without replacing the reader's current page.
export class UIMessages extends HTMLElement {
  connectedCallback() {
    this.conversationID = Number(this.getAttribute("conversation-id"));
    this.lastID = Number(this.getAttribute("last-id")) || 0;
    this.list = this.querySelector("[data-message-list]");
    this.form = this.querySelector("[data-message-form]");
    this.status = this.querySelector("[data-chat-status]");
    if (!this.conversationID || !this.list || !this.status) return;
    this.active = true;
    this.delay = 5000;
    this.events = new AbortController();
    const options = { signal: this.events.signal };
    this.form?.addEventListener("submit", (event) => this.send(event), options);
    const textarea = this.form?.querySelector("textarea");
    if (textarea) {
      const resize = () => {
        textarea.style.height = "auto";
        textarea.style.height = Math.min(textarea.scrollHeight, 180) + "px";
      };
      textarea.addEventListener("input", resize, options);
      textarea.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && !e.shiftKey) {
          e.preventDefault();
          this.form.requestSubmit ? this.form.requestSubmit() : this.form.submit();
        }
      }, options);
      resize();
    }
    document.addEventListener("visibilitychange", () => {
      clearTimeout(this.timer);
      if (!document.hidden) { this.poll(); this.markRead(); }
    }, options);
    window.addEventListener("scroll", () => this.markRead(), { ...options, passive: true });
    this.markRead();
    this.scrollToBottom();
    this.schedule();
  }

  disconnectedCallback() {
    this.active = false;
    clearTimeout(this.timer);
    this.events?.abort();
  }

  schedule() {
    clearTimeout(this.timer);
    if (this.active && this.hasAttribute("poll") && !document.hidden) this.timer = setTimeout(() => this.poll(), this.delay);
  }

  async poll() {
    if (!this.active || !this.hasAttribute("poll") || document.hidden || this.polling) return;
    this.polling = true;
    try {
      const data = await api(`/messages/${this.conversationID}?after=${this.lastID}`);
      if (!this.active) return;
      let added = 0;
      for (const message of data.messages || []) {
        if (this.list.querySelector(`[data-message-id="${message.id}"]`)) continue;
        this.list.append(this.messageNode(message));
        this.lastID = Math.max(this.lastID, message.id);
        added++;
      }
      const c = data.conversation;
      if (this.form) this.form.hidden = !c.can_send;
      const state = this.querySelector("[data-conversation-state]");
      state.textContent = c.blocked ? "Messaging is blocked for this contact." : c.closed ? "This conversation is closed." :
        c.incoming_request ? "A new message request. Accept to continue the conversation." : c.state === "pending" ? "Waiting for the recipient to accept." : "";
      const actions = this.querySelector("[data-request-actions]");
      if (actions && !c.incoming_request) actions.hidden = true;
      if (added) {
        this.status.textContent = `${added} new message${added === 1 ? "" : "s"}.`;
        this.scrollToBottom();
      }
      this.delay = data.more ? 250 : 5000;
      this.markRead();
    } catch (error) {
      if (!this.active) return;
      if ([401, 403, 404].includes(error.status)) {
        // A revoked session or reassigned delivery must not keep polling or
        // presenting a conversation that the server no longer authorizes.
        this.list.replaceChildren();
        if (this.form) this.form.hidden = true;
        this.querySelector("[data-request-actions]")?.setAttribute("hidden", "");
        this.querySelector("[data-mark-read]")?.setAttribute("hidden", "");
        this.active = false;
        this.status.textContent = "This conversation is no longer available to your account. Return to the inbox or sign in again.";
      } else {
        this.delay = Math.min(this.delay * 2, 60000);
        this.status.textContent = "Unable to refresh messages. Retrying shortly; your draft is still here.";
      }
    } finally {
      this.polling = false;
      this.schedule();
    }
  }

  async send(event) {
    event.preventDefault();
    if (this.sending || !this.active) return;
    const body = this.form.elements.body;
    const nonce = this.form.elements.nonce;
    const button = this.form.querySelector('[type="submit"]');
    this.sending = true;
    body.disabled = true;
    busy(button, true);
    try {
      const data = await api(`/messages/${this.conversationID}/send`, { method: "POST", body: { body: body.value, nonce: nonce.value } });
      if (!this.active) return;
      body.value = "";
      body.style.height = "auto";
      nonce.value = data.nonce;
      this.status.textContent = "Message sent.";
      await this.poll();
    } catch (error) {
      this.status.textContent = error.message;
      // Keep both draft and nonce: retrying after a lost response is safe.
    } finally {
      this.sending = false;
      body.disabled = false;
      busy(button, false);
      if (this.active && !this.form.hidden) body.focus();
    }
  }

  scrollToBottom() {
    if (!this.list) return;
    const last = this.list.lastElementChild;
    if (last) last.scrollIntoView({ behavior: "smooth", block: "end" });
  }

  async markRead() {
    if (!this.active || document.hidden || this.reading || !this.lastID || this.readID >= this.lastID) return;
    const last = this.list.lastElementChild;
    if (!last) return;
    const rect = last.getBoundingClientRect();
    if (rect.bottom > window.innerHeight || rect.bottom < 0) return;
    const messageID = this.lastID;
    this.reading = true;
    try {
      await api(`/messages/${this.conversationID}/action`, { method: "POST", body: { action: "read", message_id: messageID } });
      this.readID = messageID;
      this.querySelector("[data-mark-read]")?.setAttribute("hidden", "");
    } catch {
      // The explicit form is still available when the automatic update fails.
    } finally { this.reading = false; }
  }

  messageNode(message) {
    const node = document.createElement("li");
    node.className = `chat-message${message.mine ? " chat-message--mine" : ""}`;
    node.dataset.messageId = message.id;
    const meta = document.createElement("div");
    meta.className = "chat-message__meta";
    const sender = document.createElement("strong");
    sender.textContent = `@${message.sender}`;
    const time = document.createElement("time");
    time.dateTime = message.created_at;
    time.textContent = new Date(message.created_at).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
    meta.append(sender, time);
    const body = document.createElement("p");
    body.className = "chat-message__body";
    body.textContent = message.body;
    node.append(meta, body);
    if (message.can_report) node.append(this.reportForm(message.id));
    return node;
  }

  reportForm(id) {
    const details = document.createElement("details");
    details.className = "chat-report";
    const summary = document.createElement("summary");
    summary.textContent = "Report";
    const form = document.createElement("form");
    form.method = "POST";
    form.action = `/messages/${this.conversationID}/action`;
    form.className = "stack-sm";
    for (const [name, value] of Object.entries({ csrf_token: csrfToken(), action: "report", message_id: String(id) })) {
      const input = document.createElement("input");
      input.type = "hidden"; input.name = name; input.value = value;
      form.append(input);
    }
    const label = document.createElement("label");
    label.htmlFor = `report-${id}`; label.textContent = "Reason";
    const select = document.createElement("select");
    select.name = "reason"; select.id = label.htmlFor; select.className = "select";
    for (const [value, text] of Object.entries({ spam: "Spam", harassment: "Harassment", scam: "Scam", personal_information: "Personal information", other: "Other" })) {
      const option = document.createElement("option"); option.value = value; option.textContent = text; select.append(option);
    }
    const notice = document.createElement("p");
    notice.className = "tiny muted"; notice.textContent = "This message will be shared with moderators.";
    const button = document.createElement("button");
    button.type = "submit"; button.className = "btn btn-secondary btn-sm"; button.textContent = "Submit report";
    form.append(label, select, notice, button);
    details.append(summary, form);
    return details;
  }
}
