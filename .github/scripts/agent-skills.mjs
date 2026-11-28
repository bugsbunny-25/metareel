// Third-party agent skills vendored into .claude/skills, pinned per source in
// .claude/skills/vendored.json (repo, branch, commit, and a content hash per
// skill so local edits are noticed before an update overwrites them).
//   node .github/scripts/agent-skills.mjs check    (make skills-check)
//   node .github/scripts/agent-skills.mjs update   (make skills-update)
// "check" compares each source's pinned commit with its branch and lists the
// vendored skills that changed upstream (exit 2 when any did). "update"
// downloads the branch heads, replaces those skills (minus the excluded paths)
// and moves the pins; review the diff before committing it. Vendored SKILL.md
// files lose any `allowed-tools` frontmatter, so a skill can never pre-approve
// commands: they go through the normal permission prompts. Set GITHUB_TOKEN to
// avoid the anonymous GitHub API rate limit.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative } from 'node:path';

const root = join(import.meta.dirname, '../..');
const skillsDir = join(root, '.claude/skills');
const lockPath = join(skillsDir, 'vendored.json');

const lock = JSON.parse(readFileSync(lockPath, 'utf8'));
const excluded = (src, rel) => src.exclude.some((e) => (e.endsWith('/') ? rel.startsWith(e) : rel === e));

async function gh(src, path, accept = 'application/vnd.github+json') {
  const headers = { Accept: accept, 'User-Agent': 'agent-skills-vendor' };
  if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  const res = await fetch(`https://api.github.com/repos/${src.repo}/${path}`, { headers });
  if (!res.ok) throw new Error(`GitHub API ${src.repo} ${path}: ${res.status} ${res.statusText}`);
  return accept.endsWith('sha') ? (await res.text()).trim() : res.json();
}
const branchHead = (src) => gh(src, `commits/${src.ref}`, 'application/vnd.github.sha');

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

// Drop `allowed-tools` (single or multi-line) from SKILL.md frontmatter.
function stripAllowedTools(skillMd) {
  const text = readFileSync(skillMd, 'utf8');
  const m = text.match(/^---\n([\s\S]*?)\n---\n/);
  if (!m) return false;
  const kept = m[1].replace(/^allowed-tools:.*(\n[ \t]+.*)*\n?/m, '');
  if (kept === m[1]) return false;
  writeFileSync(skillMd, `---\n${kept.replace(/\n$/, '')}\n---\n${text.slice(m[0].length)}`);
  return true;
}

const allSkills = () => lock.sources.flatMap((src) => Object.entries(src.skills).map(([name, s]) => ({ src, name, s })));

function locallyEdited() {
  return allSkills()
    .filter(({ name, s }) => !existsSync(join(skillsDir, name)) || hashDir(join(skillsDir, name)) !== s.hash)
    .map(({ name }) => name);
}

async function changedUpstream(src, head) {
  if (head === src.commit) return [];
  const cmp = await gh(src, `compare/${src.commit}...${head}`);
  const paths = (cmp.files ?? []).map((f) => f.filename);
  return Object.entries(src.skills)
    .filter(([, s]) => paths.some((p) => p.startsWith(`${s.path}/`) && !excluded(src, p.slice(s.path.length + 1))))
    .map(([name]) => name);
}

async function check() {
  let updates = false;
  const edited = locallyEdited();
  if (edited.length) console.log(`edited locally since the pin: ${edited.join(', ')}`);
  for (const src of lock.sources) {
    const head = await branchHead(src);
    const changed = await changedUpstream(src, head);
    if (!changed.length) {
      console.log(`${src.repo}@${src.ref}: up to date (${head.slice(0, 7)})`);
      continue;
    }
    updates = true;
    console.log(`${src.repo}@${src.ref} ${src.commit.slice(0, 7)}..${head.slice(0, 7)}: ${changed.join(', ')}`);
  }
  if (updates) console.log('run `make skills-update`, review the diff and commit it on its own');
  return updates ? 2 : 0;
}

async function update() {
  const edited = locallyEdited();
  if (edited.length && !process.argv.includes('--force')) {
    throw new Error(`skills edited locally: ${edited.join(', ')} (keep the edits upstream, or rerun with --force to overwrite)`);
  }
  for (const src of lock.sources) {
    const head = await branchHead(src);
    const tmp = mkdtempSync(join(tmpdir(), 'agent-skills-'));
    try {
      const res = await fetch(`https://codeload.github.com/${src.repo}/tar.gz/${head}`);
      if (!res.ok) throw new Error(`download ${src.repo}@${head}: ${res.status}`);
      writeFileSync(join(tmp, 'src.tgz'), Buffer.from(await res.arrayBuffer()));
      execFileSync('tar', ['xzf', 'src.tgz'], { cwd: tmp });
      const top = readdirSync(tmp).find((d) => d !== 'src.tgz');
      for (const [name, s] of Object.entries(src.skills)) {
        const from = join(tmp, top, s.path);
        if (!existsSync(join(from, 'SKILL.md'))) throw new Error(`${name}: no SKILL.md at ${src.repo}/${s.path}`);
        const to = join(skillsDir, name);
        rmSync(to, { recursive: true, force: true });
        cpSync(from, to, { recursive: true, filter: (p) => p === from || !excluded(src, relative(from, p)) });
        if (stripAllowedTools(join(to, 'SKILL.md'))) console.log(`${name}: dropped allowed-tools from SKILL.md`);
        s.hash = hashDir(to);
      }
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
    console.log(head !== src.commit ? `${src.repo}: updated to ${head.slice(0, 7)}` : `${src.repo}: already at ${head.slice(0, 7)}; files refreshed`);
    src.commit = head;
  }
  writeFileSync(lockPath, JSON.stringify(lock, null, 2) + '\n');
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
