// Applies the saved colour theme before first paint. Loaded synchronously in
// <head> as an external file because the Content-Security-Policy forbids
// inline scripts.
(() => {
  try {
    const stored = window.localStorage.getItem("sokomoko-theme");
    if (stored === "light" || stored === "dark") {
      document.documentElement.setAttribute("data-theme", stored);
    }
  } catch (e) {
    // Storage can be unavailable (private mode); fall back to the system theme.
  }
})();
