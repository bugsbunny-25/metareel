// eslint-disable-next-line @typescript-eslint/no-require-imports
const plugin = require('../../plugins/with-home-http.js');

test('cleartext is refused by default and allowed only for the listed hosts', () => {
  const xml: string = plugin.networkSecurityXml(['homelab.local', '192.168.1.20']);
  expect(xml).toContain('<base-config cleartextTrafficPermitted="false" />');
  expect(xml).toContain('<domain includeSubdomains="false">homelab.local</domain>');
  expect(xml).toContain('<domain includeSubdomains="false">192.168.1.20</domain>');
  expect(xml).toContain('<domain includeSubdomains="false">10.0.2.2</domain>'); // emulator dev host
  expect(xml.match(/homelab\.local/g)).toHaveLength(1);
});

test('host names are validated before they reach the XML', () => {
  expect(() => plugin({ name: 'x', slug: 'x' }, { hosts: ['evil"><domain>'] })).toThrow(/invalid host/);
});
