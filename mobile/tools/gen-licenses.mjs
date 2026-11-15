// Writes src/data/licenses.json: every package the app ships (the production
// dependency tree under node_modules, dev tools left out), with its licence
// and project link, for Settings > Open source licences. Run after changing
// dependencies: `npm run gen:licenses`.
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

const root = process.cwd();
const read = (dir) => JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'));

// Fonts bundled from @expo-google-fonts: the packages are MIT, the typefaces OFL.
const FONTS = [
  { name: 'Barlow Condensed (typeface)', license: 'OFL-1.1', url: 'https://github.com/jpt/barlow' },
  { name: 'Source Sans 3 (typeface)', license: 'OFL-1.1', url: 'https://github.com/adobe-fonts/source-sans' },
  { name: 'Source Serif 4 (typeface)', license: 'OFL-1.1', url: 'https://github.com/adobe-fonts/source-serif' },
];

function license(pkg) {
  const l = pkg.license ?? pkg.licenses;
  if (typeof l === 'string') return l;
  if (Array.isArray(l)) return l.map((x) => x.type ?? x).join(' OR ');
  if (l && typeof l === 'object') return l.type ?? 'See project';
  return 'See project';
}

function link(pkg) {
  let r = typeof pkg.repository === 'string' ? pkg.repository : pkg.repository?.url;
  if (r) {
    r = r.replace(/^git\+/, '').replace(/^git:\/\//, 'https://').replace(/\.git$/, '').replace(/^ssh:\/\/git@/, 'https://');
    const short = r.match(/^(github|gitlab|bitbucket):(.+)$/) ?? r.match(/^([\w-]+\/[\w.-]+)$/);
    if (short) r = short.length === 3 ? `https://${short[1]}.com/${short[2]}` : `https://github.com/${short[1]}`;
    if (r.startsWith('https://')) return r;
  }
  if (typeof pkg.homepage === 'string' && pkg.homepage.startsWith('http')) return pkg.homepage;
  return `https://www.npmjs.com/package/${pkg.name}`;
}

// Node's lookup: the package's own node_modules, then each parent's.
function resolve(from, name) {
  for (let dir = from; ; dir = dirname(dir)) {
    const candidate = join(dir, 'node_modules', name);
    if (existsSync(join(candidate, 'package.json'))) return candidate;
    if (dir === root || dirname(dir) === dir) return null;
  }
}

const seen = new Map();
function walk(dir, deps) {
  for (const name of Object.keys(deps ?? {})) {
    const at = resolve(dir, name);
    if (!at) continue; // an optional dependency for another platform
    const pkg = read(at);
    // A prebuilt binary for one OS or CPU (lightningcss-darwin-arm64, …): npm
    // installs a different one per machine, so listing it would make the file
    // differ between a Mac and CI. Its parent package is listed.
    if (pkg.os || pkg.cpu) continue;
    const key = `${pkg.name}@${pkg.version}`;
    if (seen.has(key)) continue;
    seen.set(key, { name: pkg.name, version: pkg.version, license: license(pkg), url: link(pkg) });
    walk(at, { ...pkg.dependencies, ...pkg.optionalDependencies });
  }
}

walk(root, read(root).dependencies);
const packages = [...seen.values()].sort((a, b) => a.name.localeCompare(b.name) || a.version.localeCompare(b.version));
writeFileSync(join(root, 'src/data/licenses.json'), JSON.stringify({ fonts: FONTS, packages }, null, 1) + '\n');
console.log(`wrote src/data/licenses.json: ${packages.length} packages, ${FONTS.length} typefaces`);
