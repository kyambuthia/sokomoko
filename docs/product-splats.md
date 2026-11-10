# Product splats

Product detail pages keep their existing image as the default. Products with a
reviewed bundled model get an icon-only toggle in the same media frame. The
model is loaded on demand in a same-origin SuperSplat viewer with its overlay
hidden; the viewer's canvas keeps its touch input for rotating and zooming.

`internal/ui/ProductSplatURL` is an allowlist from product slug to a bundled
`.sog` file in `internal/ui/static/models/products/`. Add a model there and add
its exact product slug to the allowlist before it appears in the storefront.

## Theme

The storefront owns the light and dark choice, and the viewer follows it. The
storefront passes `?theme=light` or `?theme=dark` to the viewer; the viewer
also reads the theme from the parent page and from the system preference, in
that order, and applies it before the first paint so nothing flashes the wrong
colour.

The scene background is the viewer's camera clear colour, not the page
background, so the viewer sets it from the theme: white in light mode and
black in dark mode. The model itself has no backdrop baked in — the black
around it was the viewer's clear colour.

The viewer paints its own background, so a theme change while the model is
open reloads the frame with the new theme. The model and the viewer are
cached, so the reload costs a re-initialisation and no download.

## Assets

The bundled viewer is generated with `@playcanvas/supersplat-viewer` 1.42.0
and is covered by the adjacent MIT and third-party notices. PLY-to-SOG files
are converted with `@playcanvas/splat-transform` 3.10.3.

`web/splat-viewer/splat-viewer.source.html` is the unminified viewer shell and
the source of the bundled `internal/ui/static/splat-viewer.html`, which is the
same shell with its module script minified. Only the served copy is embedded
in the binary; the source is not.

The `.sog` files are compressed web copies; keep the full-fidelity originals
in the asset-generation workspace. They are the source of truth now: the PLY
files they were converted from are gone, so nothing can be re-derived from
them. Decimation reduces a model to a fixed share of its gaussians.

```bash
make splat-assets        # rebuild the models and the viewer
make splat-assets-check  # verify the checked-in assets match their sources
```

`make splat-assets-check` verifies the viewer everywhere, and the models only
where the full-fidelity originals are available. Set `SPLAT_SOURCE_DIR` to
point at them, and `SPLAT_DECIMATE` to choose a different share.

A model is compared member by member rather than byte for byte: the SOG
container stores a timestamp, so two builds of the same model differ in the
archive even when the gaussians are identical.

## Demo models

The asset called `leather-handbag.ply` in the asset-generation workspace has a
vase input image. It is bundled here as `ceramic-vase.sog` and only mapped to
the `ceramic-vase` product slug. `leather-backpack.ply` is bundled as
`leather-backpack.sog` and mapped to `leather-backpack`.
