(function () {
  function csrfToken() {
    return document.querySelector('meta[name="csrf-token"]')?.getAttribute("content") || "";
  }

  async function request(path, options = {}) {
    const method = (options.method || "GET").toUpperCase();
    const headers = new Headers(options.headers || {});
    headers.set("Accept", "application/json");
    if (options.body !== undefined && options.body !== null) {
      headers.set("Content-Type", "application/json");
    }
    if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
      const token = csrfToken();
      if (token) headers.set("X-CSRF-Token", token);
    }

    const response = await fetch(`/api/v1${path}`, {
      ...options,
      method,
      headers,
      credentials: "same-origin",
      body: options.body === undefined || options.body === null ? undefined : JSON.stringify(options.body),
    });

    let payload = null;
    try {
      payload = await response.json();
    } catch (_) {
      payload = null;
    }

    if (!response.ok) {
      const error = payload?.error || {};
      const failure = new Error(error.message || `Request failed (${response.status})`);
      failure.status = response.status;
      failure.code = error.code || "request_failed";
      throw failure;
    }

    return payload?.data;
  }

  window.SokomokoAPI = { request, csrfToken };
})();
