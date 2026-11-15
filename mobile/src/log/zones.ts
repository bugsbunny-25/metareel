// Wall-clock times in an IANA zone, so a watch at a cinema in another zone is
// entered in that cinema's time (the web app's lib/dates.js).

const pad = (n: number) => String(n).padStart(2, '0');

/** A Date as "YYYY-MM-DDTHH:MM" wall time in a zone (the device's when none). */
export function wallTime(date: Date, zone?: string | null): string {
  if (!zone) return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  const p = Object.fromEntries(
    new Intl.DateTimeFormat('en-CA', {
      timeZone: zone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(date)
      .map((x) => [x.type, x.value]),
  );
  return `${p.year}-${p.month}-${p.day}T${p.hour === '24' ? '00' : p.hour}:${p.minute}`;
}

/**
 * Wall time "YYYY-MM-DDTHH:MM" in a zone → the instant. Two passes settle the
 * offset across a daylight-saving change; a skipped spring-forward time lands
 * an hour later.
 */
export function zonedToDate(wall: string, zone?: string | null): Date {
  const [d, t = '00:00'] = wall.split('T');
  const [y, mo, da] = d.split('-').map(Number);
  const [h, mi] = t.split(':').map(Number);
  if (!zone) return new Date(y, mo - 1, da, h, mi);
  const asUTC = Date.UTC(y, mo - 1, da, h, mi);
  let guess = asUTC;
  for (let i = 0; i < 2; i++) {
    const [sd, st] = wallTime(new Date(guess), zone).split('T');
    const [sy, smo, sda] = sd.split('-').map(Number);
    const [sh, smi] = st.split(':').map(Number);
    guess += asUTC - Date.UTC(sy, smo - 1, sda, sh, smi);
  }
  return new Date(guess);
}

/** Server UTC timestamp ("YYYY-MM-DD HH:MM:SS" or RFC 3339) → Date. */
export function fromUTC(ts?: string | null): Date | null {
  if (!ts) return null;
  const d = new Date(ts.includes('T') ? ts : `${ts.replace(' ', 'T')}Z`);
  return Number.isNaN(d.getTime()) ? null : d;
}

/**
 * The pickers work in the device's clock; a wall time in another zone is
 * shown as the same digits on a device-local Date and read back the same way.
 */
export const wallToPicker = (wall: string) => zonedToDate(wall, null);
export const pickerToWall = (d: Date) => wallTime(d, null);
