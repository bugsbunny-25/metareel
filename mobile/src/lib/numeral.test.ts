import { numeralLayout } from './numeral';

describe('numeralLayout', () => {
  it('gives a lone 1 a foot centred on its stem, and nothing else one', () => {
    const one = numeralLayout(1);
    expect(one.foot).not.toBeNull();
    expect(one.foot!.x + one.foot!.w / 2).toBeCloseTo(one.x + 18, 1);
    expect(one.width).toBeGreaterThanOrEqual(one.foot!.x + one.foot!.w);
    expect(numeralLayout(10).foot).toBeNull();
    expect(numeralLayout(7).foot).toBeNull();
  });

  it('is as wide as the digits it holds', () => {
    expect(numeralLayout(10).width).toBeGreaterThan(numeralLayout(2).width);
    expect(numeralLayout(2).width).toBeGreaterThan(numeralLayout(1).width - 1);
    expect(numeralLayout(1).height).toBe(numeralLayout(10).height);
  });
});
