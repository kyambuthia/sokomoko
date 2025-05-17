if (document.readyState === "complete")
{
    console.log("Document is ready");
    console.log("-->");

    const currentTime = Date.now();

    const bannerArea = document.createElement("div");
    bannerArea.className="banner-area";
    bannerArea.innerHTML = currentTime;

    const siteHeader = document.querySelector("header");
    siteHeader.appendChild(bannerArea);
}

document.addEventListener("DOMContentLoaded", () => 
{
    console.log("ready");
});

