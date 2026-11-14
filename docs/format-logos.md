# Format logos checklist

Logos for the premium-format taxonomy (cinema formats, discs and labels). They are
uploaded by an admin in the console under **System → Premium formats**, one at a
time from a format's edit dialog, or all at once with **Upload logos in bulk**,
which matches each file to a format by its name: save each logo as
`<slug>.<ext>`, for example `uhd-blu-ray.svg` or `criterion.png`.

## What a logo file must be

- PNG (transparent background), SVG, WebP, JPEG or GIF; at most 512 KB and
  4096 px a side.
- The apps draw logos on a dark plate. A dark logo is switched to
  **monochrome** (drawn white; the file is unchanged) automatically when it is
  uploaded through the console, if 40% or more of its visible pixels are dark
  and it has transparent corners. The edit dialog shows original, white and
  on-white versions and lets you pick. A logo on an opaque background keeps
  its colours: a transparent version looks better.
- Transparent edges of PNG and GIF uploads are trimmed, so a logo on a big
  empty canvas still fills its badge.
- Keep the files out of git: `data/logos/` (git-ignored) holds the set, named
  by slug, ready for the bulk upload.
- Wider than tall reads best; it is shown about 13–22 px high.
- SVGs may contain shapes, text, gradients, filters and plain CSS. Illustrator
  exports pass, including its namespace entities and "preserve editing" data,
  and so do Inkscape's. Scripts, embedded images, CSS `@`-rules and links
  outside the file are refused.
- These are trademarks of their owners. They label what you watched in a
  private app; don't redistribute them.

## Discs

| Slug | Logo |
|---|---|
| `uhd-blu-ray` | Ultra HD Blu-ray (4K) |
| `blu-ray` | Blu-ray Disc |
| `dvd` | DVD |

## Labels

| Slug | Logo |
|---|---|
| `criterion` | The Criterion Collection |
| `arrow-video` | Arrow Video |
| `kino-lorber` | Kino Lorber |
| `eureka` | Eureka Entertainment (Masters of Cinema) |
| `bfi` | BFI |
| `second-sight` | Second Sight Films |
| `88-films` | 88 Films |
| `vinegar-syndrome` | Vinegar Syndrome |
| `severin` | Severin Films |
| `shout-factory` | Shout! Factory |
| `radiance` | Radiance Films |
| `indicator` | Indicator (Powerhouse Films) |
| `umbrella` | Umbrella Entertainment |
| `imprint` | Imprint Films |
| `studiocanal` | StudioCanal |

More labels are added on the same page (category **Label**), with a logo.

## Cinema formats

| Slug | Logo |
|---|---|
| `imax` | IMAX |
| `imax-3d` | IMAX 3D |
| `imax-laser` | IMAX with Laser |
| `imax-70mm` | IMAX 70mm |
| `dolby-cinema` | Dolby Cinema |
| `screenx` | ScreenX |
| `cinemark-xd` | Cinemark XD |
| `pvr-playhouse` | PVR Playhouse |
| `inox-insignia` | INOX Insignia |
| `rpx` | Regal RPX |
| `prime-at-amc` | PRIME at AMC |
| `superscreen` | Cineworld Superscreen |
| `isense` | ODEON iSense |
| `ultraavx` | Cineplex UltraAVX |
| `vmax` | Event Cinemas Vmax |
| `macro-xe` | Cinépolis Macro XE |
| `pxl` | PVR P[XL] |
| `4dx` | 4DX |
| `d-box` | D-BOX |
| `mx4d` | MX4D |
| `reald-3d` | RealD 3D |
| `dolby-3d` | Dolby 3D |
| `dolby-atmos` | Dolby Atmos |
| `dts-x` | DTS:X |
| `thx` | THX |
| `auro-3d` | Auro 3D |

`plf` (Premium large format), `70mm`, `35mm`, `laser` and `hfr` are generic and
need no logo; they show as text.
