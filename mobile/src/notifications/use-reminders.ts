import * as Notifications from 'expo-notifications';
import { router } from 'expo-router';
import { useEffect } from 'react';
import { AppState } from 'react-native';

import { usePreferences } from '@/data/queries';

import { refreshPushRegistration } from './push';
import { syncEpisodeReminders } from './reminders';

/**
 * Keeps episode reminders in step while signed in (launch and every return
 * to the foreground, throttled), keeps the push token registered when
 * watchlist arrivals are on, and opens what a notification links to (the
 * show, the title) when tapped, including the tap that launched the app.
 */
export function useEpisodeReminderSync(signedIn: boolean) {
  useEffect(() => {
    if (!signedIn) return;
    void syncEpisodeReminders();
    const app = AppState.addEventListener('change', (s) => {
      if (s === 'active') void syncEpisodeReminders();
    });
    return () => app.remove();
  }, [signedIn]);

  // Keep this device's push token current while watchlist arrivals are on.
  const arrivalsOn = usePreferences(signedIn).data?.notify_watchlist_arrivals === true;
  useEffect(() => {
    if (signedIn) void refreshPushRegistration(arrivalsOn);
  }, [signedIn, arrivalsOn]);

  useEffect(() => {
    if (!signedIn) return;
    const open = (n: Notifications.Notification) => {
      const url = n.request.content.data?.url;
      if (typeof url === 'string' && url.startsWith('/')) {
        router.push(url as never);
        Notifications.clearLastNotificationResponse();
      }
    };
    const last = Notifications.getLastNotificationResponse();
    if (last?.notification) open(last.notification);
    const sub = Notifications.addNotificationResponseReceivedListener((r) => open(r.notification));
    return () => sub.remove();
  }, [signedIn]);
}
