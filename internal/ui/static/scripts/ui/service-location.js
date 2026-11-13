// Progressive enhancement for optional browser geolocation on /services.
// Server-rendered area and radius_km filtering works without JavaScript.
export class UIServiceLocation extends HTMLElement {
  connectedCallback() {
    if (this._ready) return;
    this._ready = true;
    const form = this.querySelector("form");
    const button = this.querySelector("[data-use-location]");
    const latInput = this.querySelector("[data-service-lat]");
    const lngInput = this.querySelector("[data-service-lng]");
    if (!form || !button || !latInput || !lngInput || !("geolocation" in navigator)) return;

    const areaInput = form.querySelector('input[name="area"]');
    const radiusSelect = form.querySelector('select[name="radius_km"]');
    const status = this.querySelector("[data-location-status]");
    button.hidden = false;

    if (areaInput) {
      areaInput.addEventListener("input", () => {
        latInput.removeAttribute("name");
        latInput.value = "";
        lngInput.removeAttribute("name");
        lngInput.value = "";
      });
    }

    button.addEventListener("click", () => {
      button.disabled = true;
      button.setAttribute("aria-busy", "true");
      if (status) status.textContent = "Checking your location…";
      navigator.geolocation.getCurrentPosition(
        (pos) => {
          latInput.name = "lat";
          latInput.value = pos.coords.latitude.toFixed(6);
          lngInput.name = "lng";
          lngInput.value = pos.coords.longitude.toFixed(6);
          if (radiusSelect && !radiusSelect.value) radiusSelect.value = "10";
          if (form.requestSubmit) form.requestSubmit();
          else form.submit();
        },
        () => {
          button.disabled = false;
          button.removeAttribute("aria-busy");
          if (status) status.textContent = "Location unavailable. Enter a neighbourhood or town instead.";
        },
        { enableHighAccuracy: false, timeout: 8000, maximumAge: 300000 },
      );
    });
  }
}
