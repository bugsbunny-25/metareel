import { MAX_REMINDERS, planReminders, REMINDER_HOUR } from '@/notifications/plan';

const ep = (o: Partial<{ tmdb_id: string; show_title: string; air_date: string; season_number: number; episode_number: number; name: string; episode_type: string }>) => ({
  tmdb_id: '100',
  show_title: 'Slow Horses',
  air_date: '2026-11-20',
  season_number: 5,
  episode_number: 3,
  name: 'Return',
  ...o,
});

const now = new Date(2026, 10, 14, 9, 0, 0); // 14 Nov 2026, 09:00 local

describe('planReminders', () => {
  it('schedules one reminder per show and day at the reminder hour, linking to the show', () => {
    const r = planReminders([ep({})], now);
    expect(r).toHaveLength(1);
    expect(r[0].id).toBe('episode:100:2026-11-20');
    expect(r[0].at).toEqual(new Date(2026, 10, 20, REMINDER_HOUR, 0, 0));
    expect(r[0].title).toBe('Slow Horses');
    expect(r[0].body).toBe('New episode today: S5E3 · Return');
    expect(r[0].url).toBe('/show/100');
  });

  it('folds a season dropped on one day into one reminder', () => {
    const r = planReminders(
      [ep({ episode_number: 3 }), ep({ episode_number: 1, name: 'Pilot' }), ep({ episode_number: 2 })],
      now,
    );
    expect(r).toHaveLength(1);
    expect(r[0].body).toBe('3 new episodes today: S5E1–S5E3');
  });

  it('names premieres and finales', () => {
    expect(planReminders([ep({ episode_number: 1, name: '' })], now)[0].body).toBe('Premiere today: S5E1');
    expect(planReminders([ep({ episode_type: 'finale', name: 'End' })], now)[0].body).toBe('Finale today: S5E3 · End');
  });

  it('leaves out past times, undated episodes and anything beyond two weeks', () => {
    const r = planReminders(
      [
        ep({ air_date: '2026-11-14' }), // today, 18:00 still ahead
        ep({ tmdb_id: '200', air_date: '2026-11-13' }), // yesterday
        ep({ tmdb_id: '300', air_date: '2026-12-01' }), // 17 days out
        ep({ tmdb_id: '400', air_date: undefined as unknown as string }),
        ep({ tmdb_id: '500', air_date: 'soon' }),
      ],
      now,
    );
    expect(r.map((x) => x.id)).toEqual(['episode:100:2026-11-14']);
    const evening = new Date(2026, 10, 14, 19, 0, 0);
    expect(planReminders([ep({ air_date: '2026-11-14' })], evening)).toEqual([]);
  });

  it('orders by time and caps the count for the iOS pending limit', () => {
    const many = Array.from({ length: 70 }, (_, i) => ep({ tmdb_id: String(i), air_date: `2026-11-${String(15 + (i % 10)).padStart(2, '0')}` }));
    const r = planReminders(many, now);
    expect(r).toHaveLength(MAX_REMINDERS);
    for (let i = 1; i < r.length; i++) expect(r[i].at.getTime()).toBeGreaterThanOrEqual(r[i - 1].at.getTime());
  });
});
