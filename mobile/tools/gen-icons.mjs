// Generates every app and web icon from one drawing: the metareel reel, a
// film reel in the lit outline of the chart numerals with one hole in the
// "yours" orange (app/src/tokens.css). On iOS the icon follows the system: a
// light and a dark version, plus a one-colour version for tinted and clear.
// Android has one fixed (dark) icon: launchers cache app icons and don't
// redraw them when the theme changes, so a per-theme icon would show the wrong
// one until the app's next update. Its one-colour layer serves themed icons,
// which do follow the system. Splash screens follow it on both. Run after
// changing the drawing:
//   npm run gen:icons
// Writes (sizes are what each platform asks for; Expo resizes per density):
//   assets/images/icon.png                    1024 opaque: iOS fallback, Android legacy
//   assets/metareel.icon/                     iOS 26 Icon Composer bundle (light, dark, tinted, clear)
//   assets/images/android-icon-*.png          adaptive foreground and background, monochrome (themed)
//   assets/images/notification-icon.png       96, white on transparent (status bar)
//   assets/images/splash-icon{,-light}.png    the mark for the dark and light splash screens
//   ../app/public, ../web/public              favicon.svg (follows the browser), favicon-32.png, apple-touch-icon.png
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

import { Resvg } from '@resvg/resvg-js';

const root = join(import.meta.dirname, '..');
const S = 1024;
const C = S / 2;
const ORANGE = '#ff8a4c';

// The two looks, from the app's dark and light tokens.
const THEMES = {
  dark: { bg: ['#1c1c20', '#0b0b0c'], lit: ['#f4f4f6', '#9a9aa2', '#5a5a62'], hole: '#0b0b0c', glass: '#ececf0', fill: [0.0784, 0.0784, 0.0902] },
  light: { bg: ['#ffffff', '#e6e6ea'], lit: ['#3a3a40', '#5e5e66', '#8a8a92'], hole: '#f2f2f4', glass: '#3a3a40', fill: [0.949, 0.949, 0.957] },
};

// The reel: a ring, five holes on a circle (the one at the left in orange)
// and a hub. Coordinates in a 1024 square, centred.
const RING_R = 330;
const RING_W = 44;
const HOLE_ORBIT = 190;
const HOLE_R = 86;
const HOLE_W = 26;
const HUB_R = 34;
const ACCENT_HOLE = 4; // counted clockwise from the top
const holes = [0, 1, 2, 3, 4].map((k) => {
  const a = ((-90 + k * 72) * Math.PI) / 180;
  return { x: C + HOLE_ORBIT * Math.cos(a), y: C + HOLE_ORBIT * Math.sin(a) };
});

const defs = (t) => `<defs>
  <linearGradient id="lit" x1="0" y1="${C - RING_R}" x2="0" y2="${C + RING_R}" gradientUnits="userSpaceOnUse">
    <stop offset="0" stop-color="${t.lit[0]}"/><stop offset=".55" stop-color="${t.lit[1]}"/><stop offset="1" stop-color="${t.lit[2]}"/>
  </linearGradient>
  <radialGradient id="bg" cx="${C}" cy="${S * 0.35}" r="${S * 0.8}" gradientUnits="userSpaceOnUse">
    <stop offset="0" stop-color="${t.bg[0]}"/><stop offset="1" stop-color="${t.bg[1]}"/>
  </radialGradient>
</defs>`;

/**
 * The mark. kind: 'lit' (gradient outline, orange hole, holes filled with the
 * canvas), 'mono' (one colour, the orange hole filled: themed, tinted, clear
 * and notification icons), 'solid' (the reel without its orange hole in one
 * colour, an Icon Composer layer: the system adds the glass), 'accent' (the
 * orange hole alone, its own Icon Composer layer).
 */
function mark(kind, { theme = THEMES.dark, colour = '#ffffff', scale = 1 } = {}) {
  const t = scale === 1 ? '' : ` transform="translate(${C} ${C}) scale(${scale}) translate(${-C} ${-C})"`;
  const stroke = kind === 'lit' ? 'url(#lit)' : colour;
  const holeFill = kind === 'lit' ? theme.hole : 'none';
  const ring = `<circle cx="${C}" cy="${C}" r="${RING_R}" fill="none" stroke="${stroke}" stroke-width="${RING_W}"/>`;
  const plain = holes
    .filter((_, i) => i !== ACCENT_HOLE)
    .map((h) => `<circle cx="${h.x.toFixed(1)}" cy="${h.y.toFixed(1)}" r="${HOLE_R}" fill="${holeFill}" stroke="${stroke}" stroke-width="${HOLE_W}"/>`)
    .join('');
  const a = holes[ACCENT_HOLE];
  const accentColour = kind === 'lit' || kind === 'accent' ? ORANGE : colour;
  const accent = `<circle cx="${a.x.toFixed(1)}" cy="${a.y.toFixed(1)}" r="${HOLE_R + HOLE_W / 2}" fill="${accentColour}"/>`;
  const hub = `<circle cx="${C}" cy="${C}" r="${HUB_R}" fill="${stroke}"/>`;
  if (kind === 'accent') return `<g${t}>${accent}</g>`;
  if (kind === 'solid') return `<g${t}>${ring}${plain}${hub}</g>`;
  return `<g${t}>${ring}${plain}${accent}${hub}</g>`;
}

const svg = (body, { theme = THEMES.dark, bg = false, round = false } = {}) =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${S} ${S}" width="${S}" height="${S}">${defs(theme)}` +
  (bg ? `<rect width="${S}" height="${S}" ${round ? `rx="${S * 0.225}" ` : ''}fill="url(#bg)"/>` : '') +
  body +
  '</svg>';
// A plain layer for Icon Composer: no gradients, no background.
const layer = (body) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${S} ${S}" width="${S}" height="${S}">${body}</svg>`;

function out(path, data) {
  const p = join(root, path);
  mkdirSync(dirname(p), { recursive: true });
  writeFileSync(p, data);
  console.log('wrote', path);
}
const png = (path, src, size) => out(path, new Resvg(src, { fitTo: { mode: 'width', value: size } }).render().asPng());
const { dark, light } = THEMES;

// iOS fallback and Android legacy (before 8.0) icon: opaque, square.
png('assets/images/icon.png', svg(mark('lit'), { bg: true }), 1024);

// Android adaptive icon: 108dp layers, the mark inside the 66dp safe circle.
// One fixed (dark) icon; monochrome is the themed icon, which follows the system.
png('assets/images/android-icon-foreground.png', svg(mark('lit', { theme: dark, scale: 0.8 })), 1024);
png('assets/images/android-icon-background.png', svg('', { theme: dark, bg: true }), 1024);
png('assets/images/android-icon-monochrome.png', svg(mark('mono', { scale: 0.8 })), 1024);
// Status bar notifications: white on transparent, filling the square.
png('assets/images/notification-icon.png', svg(mark('mono', { scale: 1.3 })), 96);
// Splash screens (app.config.ts): the mark on the dark and the light canvas.
png('assets/images/splash-icon.png', svg(mark('lit', { theme: dark, scale: 1.3 })), 512);
png('assets/images/splash-icon-light.png', svg(mark('lit', { theme: light, scale: 1.3 }), { theme: light }), 512);

// iOS 26: an Icon Composer bundle. Light and dark each get their background
// and reel; the orange hole is shared. Tinted (which also drives Clear) shows
// only the one-colour layer, which the system tints or turns to glass.
const srgb = ([r, g, b]) => `srgb:${r.toFixed(5)},${g.toFixed(5)},${b.toFixed(5)},1.00000`;
out('assets/metareel.icon/Assets/reel-light.svg', layer(mark('solid', { colour: light.glass })));
out('assets/metareel.icon/Assets/reel-dark.svg', layer(mark('solid', { colour: dark.glass })));
out('assets/metareel.icon/Assets/accent.svg', layer(mark('accent')));
out('assets/metareel.icon/Assets/mono.svg', layer(mark('mono')));
const showOnlyIn = (appearance) => [{ value: true }, { appearance, value: false }];
const hideIn = (appearance) => [{ value: false }, { appearance, value: true }];
const glassGroup = (layers, extra = {}) => ({ layers, shadow: { kind: 'neutral', opacity: 0.5 }, translucency: { enabled: true, value: 0.4 }, ...extra });
out(
  'assets/metareel.icon/icon.json',
  JSON.stringify(
    {
      'fill-specializations': [{ value: { solid: srgb(light.fill) } }, { appearance: 'dark', value: { solid: srgb(dark.fill) } }],
      groups: [
        glassGroup([{ name: 'accent', 'image-name': 'accent.svg', 'hidden-specializations': hideIn('tinted') }]),
        glassGroup([
          {
            name: 'reel',
            // No fixed image-name: actool would ignore the per-appearance ones.
            'image-name-specializations': [{ value: 'reel-light.svg' }, { appearance: 'dark', value: 'reel-dark.svg' }],
            'hidden-specializations': hideIn('tinted'),
          },
        ]),
        glassGroup([{ name: 'mono', 'image-name': 'mono.svg', glass: true, 'hidden-specializations': showOnlyIn('tinted') }]),
      ],
      'supported-platforms': { squares: 'shared' },
    },
    null,
    2,
  ) + '\n',
);

// Web user app and admin UI. The SVG favicon follows the browser's light or
// dark setting; the PNG fallback and the iOS home screen icon are the dark one.
const themedFavicon =
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${S} ${S}" width="${S}" height="${S}">` +
  `<style>.l{display:none}@media (prefers-color-scheme: light){.l{display:inline}.d{display:none}}</style>` +
  `<g class="d">${svg(mark('lit'), { bg: true, round: true })}</g>` +
  `<g class="l">${svg(mark('lit', { theme: light }), { theme: light, bg: true, round: true }).replaceAll('id="lit"', 'id="litL"').replaceAll('url(#lit)', 'url(#litL)').replaceAll('id="bg"', 'id="bgL"').replaceAll('url(#bg)', 'url(#bgL)')}</g>` +
  '</svg>';
for (const dir of ['../app/public', '../web/public']) {
  out(`${dir}/favicon.svg`, themedFavicon);
  png(`${dir}/favicon-32.png`, svg(mark('lit'), { bg: true, round: true }), 32);
  png(`${dir}/apple-touch-icon.png`, svg(mark('lit'), { bg: true }), 180);
}
