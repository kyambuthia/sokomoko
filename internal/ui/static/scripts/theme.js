// Applies a saved light or dark preference before first paint. Loaded as a
// blocking external script because the Content-Security-Policy forbids inline
// scripts. Without a saved choice the page follows the system setting.
(() => {
  try {
    const theme = localStorage.getItem("sokomoko-theme");
    if (theme === "light" || theme === "dark") {
      document.documentElement.dataset.theme = theme;
    }
  } catch {
    // Storage can be unavailable in private mode.
  }
})();
