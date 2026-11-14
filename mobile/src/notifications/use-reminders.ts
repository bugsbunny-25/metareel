import * as Notifications from 'expo-notifications';
import { router } from 'expo-router';
import { useEffect } from 'react';
import { AppState } from 'react-native';

import { syncEpisodeReminders } from './reminders';

/**
 * Keeps episode reminders in step while signed in (launch and every return
 * to the foreground, throttled), and opens the show when a reminder is
 * tapped, including the tap that launched the app.
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
