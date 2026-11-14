import { breakEvenProps, upNextProps } from '@/widgets/props';

describe('upNextProps', () => {
  it('lists up to three shows, links the first, and names what airs next', () => {
    const p = upNextProps({
      up_next: [
        { tmdb_id: '1416', show_title: "Grey's Anatomy", season_number: 1, episode_number: 1, name: "A Hard Day's Night" },
        { tmdb_id: '2', show_title: 'B', season_number: 2, episode_number: 3, name: '' },
        { tmdb_id: '3', show_title: 'C', season_number: 1, episode_number: 9 },
        { tmdb_id: '4', show_title: 'D', season_number: 1, episode_number: 1 },
      ],
      next_airing: { tmdb_id: '1416', show_title: "Grey's Anatomy", season_number: 23, episode_number: 6, air_date: '2026-11-19' },
    });
    expect(p.state).toBe('ok');
    expect(p.items).toHaveLength(3);
    expect(p.items[0]).toEqual({ title: "Grey's Anatomy", line: "S1E1 · A Hard Day's Night" });
    expect(p.items[1].line).toBe('S2E3');
    expect(p.url).toBe('metareel:///show/1416');
    expect(p.airing).toMatch(/^Grey's Anatomy S23E6 · /);
  });

  it('is empty without shows', () => {
    expect(upNextProps({ up_next: [], next_airing: null })).toMatchObject({ state: 'empty', items: [], url: 'metareel:///up-next' });
  });
});

describe('breakEvenProps', () => {
  it('shows the month against the break-even count and the balance', () => {
    const p = breakEvenProps({ subscription: { plan: 'A-List', currency: 'USD', watches_this_month: 3, break_even_watches: 4, saved_minor: 4210 } });
    expect(p).toMatchObject({ state: 'ok', plan: 'A-List', progressLabel: '3 of 4 films this month', progress: 0.75 });
    expect(p.balance).toMatch(/^Saved .*42\.10 so far$/);
    const over = breakEvenProps({ subscription: { plan: 'A-List', currency: 'USD', watches_this_month: 6, break_even_watches: 4, saved_minor: -500 } });
    expect(over.progress).toBe(1);
    expect(over.balance).toMatch(/5\.00 more than tickets so far$/);
  });

  it('is empty without a plan', () => {
    expect(breakEvenProps({ subscription: null }).state).toBe('empty');
  });
});
