// Third-party agent skills vendored into .claude/skills, pinned in
// .claude/skills/vendored.json (source repo, commit, and a content hash per
// skill so local edits are noticed before an update overwrites them).
//   node .github/scripts/agent-skills.mjs check    (make skills-check)
//   node .github/scripts/agent-skills.mjs update   (make skills-update)
// "check" compares the pinned commit with the source branch and lists the
// vendored skills that changed upstream (exit 2 when any did). "update"
// downloads the branch head, replaces those skills (minus the excluded paths)
// and moves the pin; review the diff before committing it. Set GITHUB_TOKEN
// to avoid the anonymous GitHub API rate limit.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative } from 'node:path';

const root = join(import.meta.dirname, '../..');
const skillsDir = join(root, '.claude/skills');
const lockPath = join(skillsDir, 'vendored.json');

const lock = JSON.parse(readFileSync(lockPath, 'utf8'));
const excluded = (rel) => lock.exclude.some((e) => (e.endsWith('/') ? rel.startsWith(e) : rel === e));

async function gh(path, accept = 'application/vnd.github+json') {
  const headers = { Accept: accept, 'User-Agent': 'agent-skills-vendor' };
  if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  const res = await fetch(`https://api.github.com/repos/${lock.source}/${path}`, { headers });
  if (!res.ok) throw new Error(`GitHub API ${path}: ${res.status} ${res.statusText}`);
  return accept.endsWith('sha') ? (await res.text()).trim() : res.json();
}

function files(dir) {
  return readdirSync(dir, { recursive: true })
    .map((f) => String(f))
    .filter((f) => statSync(join(dir, f)).isFile())
    .sort();
}

// Hash of a vendored skill directory: every file's path and content.
function hashDir(dir) {
  const h = createHash('sha256');
  for (const f of files(dir)) h.update(f).update('\0').update(readFileSync(join(dir, f))).update('\0');
  return h.digest('hex');
}

function locallyEdited() {
  return Object.entries(lock.skills)
    .filter(([name, s]) => !existsSync(join(skillsDir, name)) || hashDir(join(skillsDir, name)) !== s.hash)
    .map(([name]) => name);
}

async function changedUpstream(head) {
  if (head === lock.commit) return [];
  const cmp = await gh(`compare/${lock.commit}...${head}`);
  const paths = (cmp.files ?? []).map((f) => f.filename);
  return Object.entries(lock.skills)
    .filter(([, s]) => paths.some((p) => p.startsWith(`${s.path}/`) && !excluded(p.slice(s.path.length + 1))))
    .map(([name]) => name);
}

async function check() {
  const head = await gh(`commits/${lock.ref}`, 'application/vnd.github.sha');
  const changed = await changedUpstream(head);
  const edited = locallyEdited();
  if (edited.length) console.log(`edited locally since the pin: ${edited.join(', ')}`);
  if (!changed.length) {
    console.log(`vendored skills are up to date with ${lock.source}@${lock.ref} (${head.slice(0, 7)})`);
    return 0;
  }
  console.log(`updates in ${lock.source}@${lock.ref} ${lock.commit.slice(0, 7)}..${head.slice(0, 7)}: ${changed.join(', ')}`);
  console.log('run `make skills-update`, review the diff and commit it on its own');
  return 2;
}

async function update() {
  const edited = locallyEdited();
  if (edited.length && !process.argv.includes('--force')) {
    throw new Error(`skills edited locally: ${edited.join(', ')} (keep the edits upstream, or rerun with --force to overwrite)`);
  }
  const head = await gh(`commits/${lock.ref}`, 'application/vnd.github.sha');
  const tmp = mkdtempSync(join(tmpdir(), 'agent-skills-'));
  try {
    const res = await fetch(`https://codeload.github.com/${lock.source}/tar.gz/${head}`);
    if (!res.ok) throw new Error(`download ${head}: ${res.status}`);
    writeFileSync(join(tmp, 'src.tgz'), Buffer.from(await res.arrayBuffer()));
    execFileSync('tar', ['xzf', 'src.tgz'], { cwd: tmp });
    const top = readdirSync(tmp).find((d) => d !== 'src.tgz');
    for (const [name, s] of Object.entries(lock.skills)) {
      const from = join(tmp, top, s.path);
      if (!existsSync(join(from, 'SKILL.md'))) throw new Error(`${name}: no SKILL.md at ${s.path} upstream`);
      const to = join(skillsDir, name);
      rmSync(to, { recursive: true, force: true });
      cpSync(from, to, { recursive: true, filter: (src) => src === from || !excluded(relative(from, src)) });
      s.hash = hashDir(to);
    }
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
  const moved = head !== lock.commit;
  lock.commit = head;
  writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n');
  console.log(moved ? `vendored skills updated to ${lock.source}@${head.slice(0, 7)}` : 'already at the branch head; files refreshed');
}

const cmd = process.argv[2];
try {
  if (cmd === 'check') process.exitCode = await check();
  else if (cmd === 'update') await update();
  else throw new Error('usage: agent-skills.mjs check | update [--force]');
} catch (err) {
  console.error(err.message);
  process.exitCode = 1;
}
