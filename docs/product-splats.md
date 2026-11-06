# Product splats

Product detail pages keep their existing image as the default. Products with a
reviewed bundled model get an icon-only toggle in the same media frame. The
model is loaded on demand in a same-origin SuperSplat viewer with its overlay
hidden; the viewer's canvas keeps its touch input for rotating and zooming.

`internal/ui/ProductSplatURL` is an allowlist from product slug to a bundled
`.sog` file in `internal/ui/static/models/products/`. Add a model there and add
its exact product slug to the allowlist before it appears in the storefront.
The `.sog` files are compressed web copies; keep the original PLY source in the
asset-generation workspace.
The current demo seed has no `ceramic-vase` or `leather-backpack` listing, so
the controls appear only when a public product with one of those slugs exists.

The bundled viewer was generated with `@playcanvas/supersplat-viewer` 1.42.0
and is covered by the adjacent MIT and third-party notices. PLY-to-SOG files
were converted with `@playcanvas/splat-transform` 3.10.3.

The asset called `leather-handbag.ply` in `~/Projects/asset-gen/outputs` has a
vase input image. It is bundled here as `ceramic-vase.sog` and only mapped to
the `ceramic-vase` product slug. `leather-backpack.ply` is bundled as
`leather-backpack.sog` and mapped to `leather-backpack`.
