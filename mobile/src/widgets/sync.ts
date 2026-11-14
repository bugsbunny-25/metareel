import { unwrap } from '@/api/client';
import { api, currentSession, subscribeSession } from '@/auth/session';

import BreakEvenWidget from './BreakEvenWidget';
import { breakEvenProps, SIGNED_OUT_BREAK_EVEN, SIGNED_OUT_UP_NEXT, upNextProps } from './props';
import UpNextWidget from './UpNextWidget';

// The widgets show what GET /me/glance says. The app refreshes them on
// launch, on foreground, after the outbox sends something (a log or a tick
// changes Up next) and from the background task. Failures keep what the
// widgets show.

let last = 0;
let running: Promise<void> | null = null;
const MIN_INTERVAL = 60 * 1000;

export function refreshWidgets(opts: { force?: boolean } = {}): Promise<void> {
  if (running) return running;
  if (!opts.force && Date.now() - last < MIN_INTERVAL) return Promise.resolve();
  running = (async () => {
    try {
      if (!currentSession()) return;
      const g = unwrap(await api.GET('/api/v1/me/glance'));
      UpNextWidget.updateSnapshot(upNextProps(g));
      BreakEvenWidget.updateSnapshot(breakEvenProps(g));
      last = Date.now();
    } catch {
      // Offline or signed out mid-way: try again next time.
    } finally {
      running = null;
    }
  })();
  return running;
}

// Signing out blanks the widgets (they would otherwise keep showing the
// account's shows and plan).
subscribeSession(() => {
  if (currentSession()) return;
  try {
    UpNextWidget.updateSnapshot(SIGNED_OUT_UP_NEXT);
    BreakEvenWidget.updateSnapshot(SIGNED_OUT_BREAK_EVEN);
  } catch {
    // Widgets unavailable in this build.
  }
});
