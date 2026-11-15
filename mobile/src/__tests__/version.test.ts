import pkg from '../../package.json';
import { parseVersion, versionCode } from '../../version.ts';

test('build numbers rise with every later version', () => {
  const order = ['0.4.9', '0.5.0', '0.5.1', '0.10.0', '0.999.999', '1.0.0', '1.2.3'];
  const codes = order.map(versionCode);
  for (let i = 1; i < codes.length; i++) expect(codes[i]).toBeGreaterThan(codes[i - 1]);
  expect(versionCode('v0.5.0')).toBe(5000);
  expect(versionCode('1.2.3')).toBe(1_002_003);
});

test('versions outside the scheme are refused', () => {
  for (const bad of ['0.5', '0.5.0-alpha', '0.5.0-rc.1', '0.1000.0', 'mobile-v0.5.0']) expect(() => parseVersion(bad)).toThrow();
});

test("package.json's versionCode (read by F-Droid) matches its version", () => {
  expect(pkg.versionCode).toBe(versionCode(pkg.version));
});
