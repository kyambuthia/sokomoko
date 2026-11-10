export class UISplatViewer extends HTMLElement {
  connectedCallback() {
    this.image = this.querySelector("img");
    this.frame = this.querySelector("iframe");
    this.video = this.querySelector("video");
    this.splatButton = this.querySelector(".detail__media-toggle");
    this.videoButton = this.querySelector(".detail__video-toggle");
    if (!this.image || this.onToggle || (!this.splatButton && !this.videoButton)) return;

    this.onToggle = () => this.toggleSplat();
    this.onVideoToggle = () => this.toggleVideo();
    this.splatButton?.addEventListener("click", this.onToggle);
    this.videoButton?.addEventListener("click", this.onVideoToggle);
    // The viewer paints its own scene background, so a theme change has to
    // reload the frame rather than restyle it.
    this.themeObserver = new MutationObserver(() => this.reloadForTheme());
    this.themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  }

  disconnectedCallback() {
    this.splatButton?.removeEventListener("click", this.onToggle);
    this.videoButton?.removeEventListener("click", this.onVideoToggle);
    this.frame?.removeAttribute("src");
    this.stopVideo();
    this.themeObserver?.disconnect();
    this.themeObserver = null;
    this.onToggle = null;
    this.onVideoToggle = null;
  }

  theme() {
    const explicit = document.documentElement.dataset.theme;
    if (explicit === "light" || explicit === "dark") return explicit;
    return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  reloadForTheme() {
    if (this.activeView !== "splat") return;
    const source = this.frame?.getAttribute("src");
    if (!source) return;
    const url = new URL(source, window.location.origin);
    const theme = this.theme();
    if (url.searchParams.get("theme") === theme) return;
    url.searchParams.set("theme", theme);
    this.frame.src = url.pathname + url.search;
  }

  toggleSplat() {
    if (!this.frame || !this.splatButton) return;
    if (this.activeView === "splat") {
      this.showImage();
      return;
    }

    const viewerURL = new URL(this.dataset.viewerUrl, window.location.origin);
    viewerURL.searchParams.set("content", this.dataset.modelUrl);
    viewerURL.searchParams.set("poster", this.dataset.posterUrl);
    viewerURL.searchParams.set("noui", "");
    viewerURL.searchParams.set("theme", this.theme());
    this.frame.src = viewerURL.pathname + viewerURL.search;
    this.frame.hidden = false;
    this.image.hidden = true;
    this.stopVideo();
    this.activeView = "splat";
    this.splatButton.setAttribute("aria-label", "Show product image");
    this.splatButton.setAttribute("aria-pressed", "true");
    this.videoButton?.setAttribute("aria-label", "Play product video");
    this.videoButton?.setAttribute("aria-pressed", "false");
    this.setIcon(this.splatButton, "camera");
  }

  toggleVideo() {
    if (!this.video || !this.videoButton) return;
    if (this.activeView === "video") {
      this.showImage();
      return;
    }

    this.frame?.removeAttribute("src");
    if (this.frame) this.frame.hidden = true;
    this.image.hidden = true;
    this.video.src = this.dataset.videoUrl;
    this.video.hidden = false;
    this.activeView = "video";
    this.splatButton?.setAttribute("aria-label", "Show 3D view");
    this.splatButton?.setAttribute("aria-pressed", "false");
    this.videoButton.setAttribute("aria-label", "Show product image");
    this.videoButton.setAttribute("aria-pressed", "true");
    this.video.play().catch(() => {});
  }

  showImage() {
    this.frame?.removeAttribute("src");
    if (this.frame) this.frame.hidden = true;
    this.stopVideo();
    this.image.hidden = false;
    this.activeView = "image";
    this.splatButton?.setAttribute("aria-label", "Show 3D view");
    this.splatButton?.setAttribute("aria-pressed", "false");
    this.videoButton?.setAttribute("aria-label", "Play product video");
    this.videoButton?.setAttribute("aria-pressed", "false");
    if (this.splatButton) this.setIcon(this.splatButton, "box");
  }

  stopVideo() {
    if (!this.video) return;
    this.video.pause();
    this.video.removeAttribute("src");
    this.video.load();
    this.video.hidden = true;
  }

  setIcon(button, name) {
    const sprite = document.querySelector('meta[name="icon-sprite"]')?.content || "/static/icons/sprite.svg";
    const use = button.querySelector("use");
    use?.setAttribute("href", `${sprite}#i-${name}`);
  }
}
