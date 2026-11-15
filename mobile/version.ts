// The one version scheme for the server, the image, the web UIs and the apps
// (docs/development.md, "Versions"): X.Y.Z, released as the tag vX.Y.Z. No -alpha or
// other suffixes. Plain TypeScript with no imports, so app.config.ts, the app,
// jest and Node (.github/scripts/version.mjs) all use it as is.
//
// Stores need build numbers that only go up: Apple rejects an upload whose
// CFBundleVersion isn't higher than the last, and Android (sideloaded APKs,
// F-Droid) only installs a higher versionCode. Both come from the version, so
// they are the same on EAS and in an F-Droid build from source:
//   versionCode = X*1,000,000 + Y*1,000 + Z   (0.5.0 = 5000, 1.2.3 = 1002003)

export type Version = { major: number; minor: number; patch: number };

const RE = /^v?(\d+)\.(\d+)\.(\d+)$/;

/** Parses a version or tag; throws on anything outside the scheme. */
export function parseVersion(v: string): Version {
  const m = RE.exec(v.trim());
  if (!m) throw new Error(`"${v}" is not a version like 1.2.3 (no suffixes)`);
  const [major, minor, patch] = [m[1], m[2], m[3]].map(Number);
  if (minor > 999 || patch > 999) throw new Error(`"${v}": minor and patch go up to 999`);
  if (major > 2099) throw new Error(`"${v}": major goes up to 2099`);
  return { major, minor, patch };
}

/** Android versionCode and iOS CFBundleVersion: higher for every later version. */
export function versionCode(v: string): number {
  const p = parseVersion(v);
  return p.major * 1_000_000 + p.minor * 1_000 + p.patch;
}
