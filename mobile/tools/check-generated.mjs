// Fails when a generated file is out of date with its source: the API types
// (internal/openapi/spec/public.yaml) or the theme tokens (app/src/tokens.css).
// Used by CI; run `npm run gen:api` / `npm run gen:theme` to fix.
import { execSync } from 'node:child_process';

const files = ['src/api/schema.ts', 'src/theme/tokens.ts'];
execSync('npm run --silent gen:api', { stdio: 'inherit' });
execSync('npm run --silent gen:theme', { stdio: 'inherit' });
try {
  execSync(`git diff --exit-code -- ${files.join(' ')}`, { stdio: 'inherit' });
} catch {
  console.error('\nGenerated files are out of date. Run `npm run gen:api` and `npm run gen:theme`, then commit.');
  process.exit(1);
}
console.log('generated files are up to date');
