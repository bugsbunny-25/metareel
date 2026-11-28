// Lists the components and hooks React Compiler skips (it bails out silently
// at build time, leaving them unoptimized). Run: npm run check:compiler
// Exits 1 when any function is skipped, with the reason for each.
const fs = require('fs');
const path = require('path');
const babel = require('@babel/core');

const root = path.join(__dirname, '..');
const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? (e.name === '__tests__' ? [] : walk(path.join(d, e.name))) : [path.join(d, e.name)]));

let compiled = 0;
const skipped = [];
for (const filename of walk(path.join(root, 'src')).filter((f) => /\.tsx?$/.test(f) && !f.endsWith('.d.ts'))) {
  babel.transformSync(fs.readFileSync(filename, 'utf8'), {
    filename,
    babelrc: false,
    configFile: false,
    presets: [[require.resolve('@babel/preset-typescript'), { isTSX: true, allExtensions: true }]],
    plugins: [
      [
        require.resolve('babel-plugin-react-compiler'),
        {
          target: '19',
          panicThreshold: 'none',
          logger: {
            logEvent(file, e) {
              if (e.kind === 'CompileSuccess') compiled++;
              else if (e.kind !== 'CompileDiagnostic') {
                const reason = e.detail?.reason ?? e.detail?.options?.reason ?? e.reason ?? '';
                skipped.push(`${path.relative(root, file)}:${e.fnLoc?.start?.line ?? '?'} ${e.kind} ${String(reason).slice(0, 160)}`);
              }
            },
          },
        },
      ],
    ],
  });
}
console.log(`React Compiler: ${compiled} functions compiled, ${skipped.length} skipped`);
if (skipped.length) {
  console.log(skipped.join('\n'));
  process.exitCode = 1;
}
