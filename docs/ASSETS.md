# Asset pipeline and ownership

This describes the checked-in assets, not the proposed general-purpose face editor. For the `.face` and `.theme` field reference, use [FACES.md](FACES.md); for every asset's role, use [REPOSITORY-MAP.md](REPOSITORY-MAP.md).

## Runtime boundary

The TomTom does not rasterize fonts, parse SVG, or decode PNG/JPEG for its clock UI. Development tools and the modern Go host prepare bounded assets; the C runtime blends grayscale coverage and RGB565 pixels. Artwork supplied to the host's media/notification APIs is decoded and resized there before delivery.

| Format | Producer/source | Consumer and lifetime |
|---|---|---|
| `.face` key/value text | Supplied presets or validated user edits | `faces.h`; selected face geometry and palette |
| PGM digit atlas | Preserved artwork or Pillow generator | `faces.h`; one selected 170,000-byte coverage image, plus a transient replacement during switching |
| `.theme` key/value text | Supplied palette or user save-as | `themes.h`; small selected UI palette, independently capable of recoloring faces |
| `FUI1` UI text atlas | Pre-rasterized Nunito; private owner override allowed | `ui.h`; ASCII text without a device font engine |
| RGB565 artwork | Go host decoding/resizing | Device media player/notification buffers, released when no longer needed |
| SVG/PNG/ICO branding | Editable project SVG and exported copies | Web favicon, native window/tray/executable, installer and SD volume icon; not the procedural device startup animation |
| Candidate BMP | `assets/branding/splash.bmp` | Separate SD-root splash experiment; not part of the proven app animation |

## Presets and catalogs

There are **ten supplied `.face` files**: the nine original reused designs plus `neon.face`. `assets/faces/catalog.json` still lists the original nine. Do not infer that a catalog entry enumerates every available runtime face or that Weather Blue includes a weather application. Neon reuses the Nunito digit atlas rather than adding another atlas.

The six PGM files have the fixed header `P5\n1000 170\n255\n`, followed by 170,000 bytes. Ten 100×170 cells contain digits 0–9. Bytes represent alpha coverage, not a complete colored screenshot. Face settings supply colors and destination rectangles. The parser's strict format is intentional; a generic PGM encoder's comments or alternate whitespace may not be accepted.

The stock Nunito face uses custom stacked geometry, date and seconds; it is not interchangeable with choosing the named `stacked` layout. Named layout commands install their own geometry. The renderer may temporarily adapt geometry to available dock space without persisting that adaptation into the saved face.

`FUI1` contains four magic bytes, 95 one-byte advances for ASCII 32–126, then a 1900×24 alpha image. The public atlas is Nunito. `/mnt/sdcard/fracture-v3/ui-font.bin` is a private override; do not copy the owner's SF Compact font or generated atlas into public packages. The authenticated preview can use that selected local font without making it a release asset.

## Regenerating digit artwork

These are optional development steps, not prerequisites to build or run the supplied assets. They require Python 3 and Pillow; font generation also requires an appropriately licensed local TTF/OTF. Neither Pillow nor font source files are a TomTom dependency.

```sh
# From the repository root; use a disposable output, not an installed preset.
python3 assets/faces/sources/generate_font_atlas.py \
  /path/to/font.ttf /tmp/fracture-font-review.pgm --weight 700 --width 88
```

`generate_font_atlas.py` probes digit bounds, selects a common font size, supersamples at 4×, downsamples with Lanczos and emits the fixed PGM layout. Variable weight/width axes are clamped to supported ranges. The implementation queries font variations; check compatibility with the chosen font and Pillow rather than assuming every static font works unchanged.

`generate_numerals_atlas.py` draws the checked-in cubic Bézier numeral definitions at 4× resolution and writes **both** `digit-atlas-numerals.pgm` and `digit-atlas-numerals.svg` **beside the script in `sources/`**. It has no output-directory CLI. Running it overwrites the SVG there; review or copy the generator into a disposable directory first if preserving the working tree. Its PGM output does not automatically replace `assets/faces/digit-atlas-numerals.pgm`.

After intentional regeneration, compare the rendered face, fixed header/dimensions and license/provenance records, then explicitly promote reviewed output into the asset directory. `provenance.json` records hashes of the six imported atlases; changing an imported file requires explaining whether the record remains historical provenance or is being replaced with new provenance. There is no automatic regeneration step in the normal release pipeline.

## Branding synchronization

`assets/branding/fracture.svg` is the editable project mark. `fracture.png` and `fracture.ico` are exports. `host/branding/fracture.svg` and `host/branding/fracture.ico` are embedding copies because Go embedding stays within the host package tree. Resource generation uses `host/winres/winres.json`; installer and device deployment use the shared asset ICO. Keep these consumers consistent when changing the mark. Exporting the SVG correctly matters: earlier validation records an incorrect raster export despite a correct vector.

The C application's short Fracture → FractureOS startup sequence is procedural (`device/src/startup.h`). It does not consume the ICO, PNG, or BMP. The candidate 320×240 uncompressed 24-bit `splash.bmp` was separately installed and read back, but the owner reported that it did **not** replace the original TomTom startup logo. Do not present that asset as a working firmware-logo replacement. The SD-root `system` file was left untouched.

## Provenance and documentation images

The previous owner-directed face project supplied the original artwork, not its old application/sidebar code. Retain DejaVu, Nunito, Roboto and Ubuntu notices under `assets/faces/licenses/`. `docs/FACES.md` describes the original import and format. No TomTom maps or boot firmware are release assets.

`docs/images/` contains selected public UI illustrations/evidence, not runtime resources or a complete test record. Private deployment evidence, captures with real listening metadata, backups and private fonts belong outside the published source inventory. Existing images show the version/layout at capture time, not a guarantee that every current setting renders identically.
