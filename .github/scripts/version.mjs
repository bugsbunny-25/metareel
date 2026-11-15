// The repo's version, kept the same in every package (mobile, the user app,
// the admin UI). The version scheme and build numbers are in mobile/version.ts.
//   node .github/scripts/version.mjs set 0.5.0     (make version V=0.5.0)
//   node .github/scripts/version.mjs check v0.5.0  (release.yml: the tag must match)
// "set" writes the version into each package.json and package-lock.json, and
// mobile/package.json's versionCode (F-Droid reads it at the tag). A release
// tag must be on a commit that already carries its version, so the source at
// the tag says what it is.
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

import { parseVersion, versionCode } from '../../mobile/version.ts';

const root = join(import.meta.dirname, '../..');
const PACKAGES = ['mobile', 'app', 'web'];
const readJSON = (p) => JSON.parse(readFileSync(join(root, p), 'utf8'));
const writeJSON = (p, v) => writeFileSync(join(root, p), JSON.stringify(v, null, 2) + '\n');

const [cmd, arg] = process.argv.slice(2);
try {
  if (!arg || !['set', 'check'].includes(cmd)) throw new Error('usage: version.mjs set X.Y.Z | check vX.Y.Z');
  parseVersion(arg);
  const version = arg.replace(/^v/, '');
  const code = versionCode(version);

  if (cmd === 'set') {
    for (const dir of PACKAGES) {
      const pkg = readJSON(`${dir}/package.json`);
      pkg.version = version;
      if (dir === 'mobile') pkg.versionCode = code;
      writeJSON(`${dir}/package.json`, pkg);
      const lock = readJSON(`${dir}/package-lock.json`);
      lock.version = version;
      if (lock.packages?.['']) lock.packages[''].version = version;
      writeJSON(`${dir}/package-lock.json`, lock);
    }
    console.log(`version ${version} (build ${code}) set in ${PACKAGES.join(', ')}; commit it, then release v${version}`);
  } else {
    const wrong = [];
    for (const dir of PACKAGES) {
      const pkg = readJSON(`${dir}/package.json`);
      if (pkg.version !== version) wrong.push(`${dir}/package.json has ${pkg.version}`);
      if (dir === 'mobile' && pkg.versionCode !== code) wrong.push(`mobile/package.json versionCode is ${pkg.versionCode}, not ${code}`);
    }
    if (wrong.length) throw new Error(`${arg} doesn't match the commit: ${wrong.join('; ')}. Run make version V=${version}, commit, then release.`);
    console.log(`${arg}: every package is at ${version} (build ${code})`);
  }
} catch (e) {
  console.error(`::error::${e.message}`);
  process.exit(1);
}
