import { useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router } from 'expo-router';
import { type ReactNode, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Switch, View } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Chip } from '@/components/Chip';
import { DateTimeField } from '@/components/DateTimeField';
import { Field } from '@/components/Field';
import { FormatLogo } from '@/components/FormatLogo';
import { Icon } from '@/components/Icon';
import { ServiceLogo } from '@/components/ServiceLogo';
import { Stars } from '@/components/Stars';
import { Text } from '@/components/Text';
import { findNearby, type NearbyResult } from '@/data/nearby';
import { useFormats, useServices, useSubscriptions } from '@/data/queries';
import { deviceTimeZone } from '@/lib/format';
import { useTheme } from '@/theme';

import {
  ASPECTS,
  AUDIO,
  DEVICES,
  DIMENSIONS,
  formDay,
  HDRS,
  type LogForm,
  moneyShown,
  planOnDay,
  OTHER_MEDIA,
  type Place,
  PLANS,
  type Precision,
  type Quick,
  RESOLUTIONS,
  SCREENINGS,
  type Venue,
  zoneOf,
} from './form';

type Format = components['schemas']['WatchFormat'];
export type Set = (patch: Partial<LogForm>) => void;

// --- layout pieces

export function Section({ title, children, hint }: { title: string; children: ReactNode; hint?: string }) {
  return (
    <View style={{ gap: 8 }}>
      <Text variant="caption">{title}</Text>
      {children}
      {hint ? (
        <Text variant="muted" style={{ fontSize: 13 }}>
          {hint}
        </Text>
      ) : null}
    </View>
  );
}

export function Wrap({ children }: { children: ReactNode }) {
  return <View style={styles.wrap}>{children}</View>;
}

function Pair({ children }: { children: ReactNode }) {
  return <View style={styles.pair}>{children}</View>;
}

/** One choice of several as chips; tapping the chosen one clears it. */
function Choice<T extends string>({
  options,
  value,
  onChange,
  clearable = true,
}: {
  options: readonly (readonly [T, string])[];
  value: string;
  onChange: (v: T | '') => void;
  clearable?: boolean;
}) {
  return (
    <Wrap>
      {options.map(([v, label]) => (
        <Chip key={v} label={label} selected={value === v} onPress={() => onChange(value === v && clearable ? '' : v)} />
      ))}
    </Wrap>
  );
}

/** Suggestions as chips plus a field for anything else. */
function Suggest({
  label,
  value,
  onChange,
  options,
  placeholder,
  leading,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: string[];
  placeholder?: string;
  /** An icon before an option's label (a service's logo). */
  leading?: (option: string) => ReactNode;
}) {
  return (
    <View style={{ gap: 8 }}>
      <Field label={label} value={value} onChangeText={onChange} placeholder={placeholder} autoCorrect={false} />
      <ScrollView horizontal showsHorizontalScrollIndicator={false} keyboardShouldPersistTaps="handled" contentContainerStyle={{ gap: 8 }}>
        {options.map((o) => (
          <Chip key={o} label={o} selected={value === o} onPress={() => onChange(value === o ? '' : o)} leading={leading?.(o)} />
        ))}
      </ScrollView>
    </View>
  );
}

function Toggle({ label, value, onChange }: { label: string; value: boolean; onChange: (v: boolean) => void }) {
  const { colors } = useTheme();
  return (
    <View style={styles.toggle}>
      <Text style={{ flex: 1 }}>{label}</Text>
      <Switch value={value} onValueChange={onChange} trackColor={{ true: colors.accentYours }} accessibilityLabel={label} />
    </View>
  );
}

/** A format (or disc, or label) as a chip with its logo. */
function FormatChip({ f, selected, onPress }: { f: Format; selected: boolean; onPress: () => void }) {
  return (
    <Chip
      label={f.label ?? f.slug ?? ''}
      selected={selected}
      onPress={onPress}
      leading={f.logo_url ? <FormatLogo url={f.logo_url} mono={f.logo_display === 'mono'} height={14} maxWidth={56} /> : undefined}
    />
  );
}

// --- when

const QUICK: { value: Quick; label: string }[] = [
  { value: 'now', label: 'Just now' },
  { value: 'hour', label: 'An hour ago' },
  { value: 'today', label: 'Earlier today' },
  { value: 'yesterday', label: 'Yesterday' },
  { value: 'exact', label: 'Pick a date' },
];
const PRECISIONS: [Precision, string][] = [
  ['minute', 'Exact time'],
  ['day', 'Day'],
  ['month', 'Month'],
  ['year', 'Year'],
];
const MONTHS = Array.from(
  { length: 12 },
  (_, i) => [String(i + 1).padStart(2, '0'), new Date(2026, i, 15).toLocaleDateString(undefined, { month: 'short' })] as const,
);

export function WhenSection({ f, set, isNew }: { f: LogForm; set: Set; isNew: boolean }) {
  const zone = zoneOf(f);
  const away = zone !== deviceTimeZone() ? zone : '';
  return (
    <Section
      title="When"
      hint={
        away && f.quick === 'exact' ? `In ${away.replace(/_/g, ' ')} time${f.place === 'theater' && f.venue?.timezone ? ", the cinema's" : ''}.` : undefined
      }
    >
      {isNew ? (
        <Wrap>
          {QUICK.map((w) => (
            <Chip key={w.value} label={w.label} selected={f.quick === w.value} onPress={() => set({ quick: w.value })} />
          ))}
        </Wrap>
      ) : null}
      {f.quick === 'exact' ? (
        <>
          <Choice options={PRECISIONS} value={f.precision} clearable={false} onChange={(precision) => precision && set({ precision })} />
          {f.precision === 'minute' ? <DateTimeField value={f.wall} onChange={(wall) => set({ wall, day: wall.slice(0, 10) })} max={new Date()} /> : null}
          {f.precision === 'day' ? <DateTimeField value={f.day} withTime={false} onChange={(day) => set({ day })} max={new Date()} /> : null}
          {f.precision === 'month' ? (
            <>
              <Field
                label="Year"
                keyboardType="number-pad"
                maxLength={4}
                value={f.month.slice(0, 4)}
                onChangeText={(y) => set({ month: `${y}-${f.month.slice(5, 7) || '01'}` })}
              />
              <Wrap>
                {MONTHS.map(([m, label]) => (
                  <Chip key={m} label={label} selected={f.month.slice(5, 7) === m} onPress={() => set({ month: `${f.month.slice(0, 4)}-${m}` })} />
                ))}
              </Wrap>
            </>
          ) : null}
          {f.precision === 'year' ? <Field label="Year" keyboardType="number-pad" maxLength={4} value={f.year} onChangeText={(year) => set({ year })} /> : null}
        </>
      ) : null}
    </Section>
  );
}

// --- your take

export function TakeSection({ f, set }: { f: LogForm; set: Set }) {
  const { colors } = useTheme();
  return (
    <Section title="Rating">
      <View style={styles.ratingRow}>
        <Stars value={f.rating} onChange={(rating) => set({ rating })} />
        <Pressable
          onPress={() => {
            void Haptics.selectionAsync();
            set({ liked: !f.liked });
          }}
          accessibilityRole="switch"
          accessibilityState={{ checked: f.liked }}
          accessibilityLabel="Liked"
          style={[styles.like, { backgroundColor: f.liked ? colors.accentYours : colors.bgElevated }]}
        >
          <Icon name={f.liked ? 'heart-fill' : 'heart'} size={22} color={f.liked ? colors.accentOn : colors.textPrimary} />
        </Pressable>
      </View>
    </Section>
  );
}

// --- where

const PLACES: [Place, string][] = [
  ['theater', 'Cinema'],
  ['streaming', 'Streaming'],
  ['physical', 'Disc or media'],
  ['broadcast', 'TV broadcast'],
  ['other', 'Elsewhere'],
];

export function WhereSection({ f, set, country }: { f: LogForm; set: Set; country?: string }) {
  return (
    <>
      <Section title="Where">
        <Choice options={PLACES} value={f.place} onChange={(place) => set({ place })} />
      </Section>
      {f.place === 'theater' ? <CinemaFields f={f} set={set} /> : null}
      {f.place === 'streaming' ? <StreamingFields f={f} set={set} country={country} /> : null}
      {f.place === 'physical' ? <PhysicalFields f={f} set={set} /> : null}
      {f.place === 'broadcast' ? (
        <>
          <Field label="Channel" placeholder="BBC One" value={f.channel} onChangeText={(channel) => set({ channel })} />
          <Suggest label="Device" value={f.device} onChange={(device) => set({ device })} options={DEVICES} placeholder="TV" />
        </>
      ) : null}
      {moneyShown(f) && f.place !== 'theater' ? <Money f={f} set={set} /> : null}
    </>
  );
}

const FORMAT_GROUPS: [string, string][] = [
  ['brand', 'Screen'],
  ['projection', 'Projection'],
  ['sound', 'Sound'],
  ['motion', 'Motion'],
  ['dimension', '3D'],
];

function CinemaFields({ f, set }: { f: LogForm; set: Set }) {
  const formats = useFormats();
  const all = (formats.data?.formats ?? []).filter((x) => x.active !== false);
  const toggle = (slug: string) => set({ formats: f.formats.includes(slug) ? f.formats.filter((s) => s !== slug) : [...f.formats, slug] });
  return (
    <>
      <VenuePicker venue={f.venue} onPick={(venue) => set({ venue, screenId: null, subTouched: f.orig ? f.subTouched : false })} />
      <Pair>
        <View style={{ flex: 1 }}>
          <Field label="Seat" placeholder="H12" maxLength={40} value={f.seat} onChangeText={(seat) => set({ seat })} autoCapitalize="characters" />
        </View>
        <View style={{ flex: 1 }}>
          <Field label="Tickets" keyboardType="number-pad" value={f.tickets} onChangeText={(tickets) => set({ tickets })} />
        </View>
      </Pair>
      <Section title="Screening">
        <Choice options={SCREENINGS} value={f.screening} clearable={false} onChange={(screening) => set({ screening })} />
        {f.screening ? (
          <Field label="Event" placeholder="MAMI Film Festival" maxLength={120} value={f.eventName} onChangeText={(eventName) => set({ eventName })} />
        ) : null}
        <Toggle label="With a Q&A or introduction" value={f.withQA} onChange={(withQA) => set({ withQA })} />
      </Section>
      {formats.isPending ? <ActivityIndicator /> : null}
      {FORMAT_GROUPS.map(([cat, name]) => {
        const list = all.filter((x) => x.category === cat);
        if (!list.length) return null;
        return (
          <Section key={cat} title={name}>
            <Wrap>
              {list.map((x) => (
                <FormatChip key={x.slug} f={x} selected={f.formats.includes(x.slug!)} onPress={() => toggle(x.slug!)} />
              ))}
            </Wrap>
          </Section>
        );
      })}
      <PlanPicker f={f} set={set} />
      <Money f={f} set={set} />
      <Field label="Booked through" placeholder="BookMyShow, Fandango…" maxLength={60} value={f.booking} onChangeText={(booking) => set({ booking })} />
    </>
  );
}

/**
 * Your cinema plans that ran on the watch's day. Always shown at a cinema,
 * with a way to add one when there is none, so a covered ticket can always
 * be marked.
 */
function PlanPicker({ f, set }: { f: LogForm; set: Set }) {
  const subs = useSubscriptions();
  const { colors } = useTheme();
  const day = formDay(f);
  const plans = (subs.data?.subscriptions ?? []).filter((s) => planOnDay(s, day) || s.id === f.subId);
  return (
    <Section title="Covered by a plan" hint={f.subId ? 'What you paid on top goes under Paid on top; the face value counts towards your savings.' : undefined}>
      {subs.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {plans.length ? (
        <Wrap>
          <Chip label="No, paid" selected={!f.subId} onPress={() => set({ subId: null, subTouched: true })} />
          {plans.map((s) => (
            <Chip
              key={s.id}
              label={s.plan ?? ''}
              selected={f.subId === s.id}
              onPress={() =>
                set({ subId: s.id!, subTouched: true, currency: !f.price && !f.faceValue && !f.fees && !f.snacks ? (s.currency ?? f.currency) : f.currency })
              }
            />
          ))}
        </Wrap>
      ) : subs.data ? (
        <Pressable onPress={() => router.push('/subscriptions')} accessibilityRole="link" style={[styles.empty, { backgroundColor: colors.bgElevated }]}>
          <Text variant="muted" style={{ flex: 1 }}>
            {(subs.data.subscriptions ?? []).length
              ? 'None of your plans ran on this day.'
              : 'No cinema plans yet. Add one (A-List, Unlimited…) to track what it saves you.'}
          </Text>
          <Text variant="label">Plans</Text>
        </Pressable>
      ) : null}
    </Section>
  );
}

function Money({ f, set }: { f: LogForm; set: Set }) {
  const cinema = f.place === 'theater';
  return (
    <View style={{ gap: 10 }}>
      <Pair>
        {cinema && f.subId ? (
          <View style={{ flex: 1 }}>
            <Field
              label="Face value"
              keyboardType="decimal-pad"
              placeholder="Usual price"
              value={f.faceValue}
              onChangeText={(faceValue) => set({ faceValue })}
            />
          </View>
        ) : null}
        <View style={{ flex: 1 }}>
          <Field
            label={cinema ? (f.subId ? 'Paid on top' : 'Paid') : 'Price'}
            keyboardType="decimal-pad"
            placeholder="0.00"
            value={f.price}
            onChangeText={(price) => set({ price })}
          />
        </View>
        <View style={{ width: 92 }}>
          <Field label="Currency" autoCapitalize="characters" maxLength={3} value={f.currency} onChangeText={(currency) => set({ currency })} />
        </View>
      </Pair>
      {cinema ? (
        <Pair>
          <View style={{ flex: 1 }}>
            <Field label="Booking fees" keyboardType="decimal-pad" placeholder="0.00" value={f.fees} onChangeText={(fees) => set({ fees })} />
          </View>
          <View style={{ flex: 1 }}>
            <Field label="Snacks" keyboardType="decimal-pad" placeholder="0.00" value={f.snacks} onChangeText={(snacks) => set({ snacks })} />
          </View>
        </Pair>
      ) : null}
    </View>
  );
}

function VenuePicker({ venue, onPick }: { venue: Venue | null; onPick: (v: Venue | null) => void }) {
  const { colors } = useTheme();
  const [nearby, setNearby] = useState<NearbyResult | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [query, setQuery] = useState('');
  const search = useQuery({
    queryKey: ['venue-search', query.trim()],
    enabled: query.trim().length >= 2,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/me/venues', { params: { query: { q: query.trim(), include_closed: true, limit: 8 } }, signal })),
  });
  useEffect(() => {
    if (venue) return;
    let live = true;
    void findNearby().then((r) => live && setNearby(r));
    return () => {
      live = false;
    };
  }, [venue, attempt]);

  if (venue) {
    return (
      <Section title="Cinema">
        <Pressable
          onPress={() => onPick(null)}
          style={[styles.venuePicked, { backgroundColor: colors.bgElevated }]}
          accessibilityRole="button"
          accessibilityHint="Choose a different cinema"
        >
          <Icon name="pin" color={colors.textPrimary} />
          <View style={{ flex: 1 }}>
            <Text variant="label">{venue.name}</Text>
            {venue.city ? <Text variant="muted">{venue.city}</Text> : null}
          </View>
          <Text variant="caption">Change</Text>
        </Pressable>
      </Section>
    );
  }
  return (
    <Section title="Cinema">
      {!nearby ? <ActivityIndicator color={colors.textMuted} /> : null}
      {nearby?.status === 'denied' ? <Text variant="muted">Location is off for metareel. Search for the cinema instead.</Text> : null}
      {nearby?.status === 'unavailable' ? (
        <Pressable
          onPress={() => {
            setNearby(null);
            setAttempt((a) => a + 1);
          }}
          accessibilityRole="button"
        >
          <Text variant="muted">
            {nearby.message} <Text variant="label">Try again</Text>
          </Text>
        </Pressable>
      ) : null}
      {nearby?.status === 'ok' && nearby.offline ? <Text variant="caption">Offline: from saved cinemas near here</Text> : null}
      {nearby?.status === 'ok' ? nearby.venues.map((v) => <VenueRow key={v.id} v={v} onPress={() => onPick(v)} />) : null}
      <Field label="Search cinemas" placeholder="PVR Phoenix" value={query} onChangeText={setQuery} autoCorrect={false} />
      {(search.data?.venues ?? []).map((v) => (
        <VenueRow
          key={v.id}
          v={{ id: v.id!, name: v.name ?? '', chain: v.chain, city: v.city, timezone: v.timezone }}
          onPress={() => onPick({ id: v.id!, name: v.name ?? '', chain: v.chain, city: v.city, timezone: v.timezone })}
        />
      ))}
    </Section>
  );
}

function VenueRow({ v, onPress }: { v: Venue; onPress: () => void }) {
  const { colors } = useTheme();
  const dist = v.distance_m == null ? '' : v.distance_m < 1000 ? `${v.distance_m} m` : `${(v.distance_m / 1000).toFixed(1)} km`;
  return (
    <Pressable onPress={onPress} style={({ pressed }) => [styles.venue, { opacity: pressed ? 0.7 : 1 }]} accessibilityRole="button">
      <View style={{ flex: 1 }}>
        <Text variant="label" numberOfLines={1}>
          {v.name}
          {v.your_watches ? (
            <Text variant="label" style={{ color: colors.accentYours }}>
              {' '}
              · yours
            </Text>
          ) : null}
        </Text>
        {v.city ? (
          <Text variant="muted" numberOfLines={1}>
            {v.city}
          </Text>
        ) : null}
      </View>
      {dist ? (
        <Text variant="caption" num>
          {dist}
        </Text>
      ) : null}
    </Pressable>
  );
}

function StreamingFields({ f, set, country }: { f: LogForm; set: Set; country?: string }) {
  const services = useServices(country);
  const names = [...new Set((services.data?.services ?? []).map((s) => s.name).filter(Boolean))].slice(0, 16);
  const logos = new Map((services.data?.services ?? []).filter((s) => s.logo_path).map((s) => [s.name, s.logo_path] as const));
  return (
    <>
      <Suggest
        label="Service"
        value={f.provider}
        onChange={(provider) => set({ provider })}
        options={names}
        placeholder="Netflix"
        leading={(o) => (logos.get(o) ? <ServiceLogo path={logos.get(o)} /> : undefined)}
      />
      <Section title="Plan">
        <Choice options={PLANS} value={f.monetization} clearable={false} onChange={(monetization) => set({ monetization })} />
      </Section>
      <Suggest label="Device" value={f.device} onChange={(device) => set({ device })} options={DEVICES} placeholder="TV" />
    </>
  );
}

function PhysicalFields({ f, set }: { f: LogForm; set: Set }) {
  const formats = useFormats();
  const all = (formats.data?.formats ?? []).filter((x) => x.active !== false);
  const discs = all.filter((x) => x.category === 'disc');
  const labels = all.filter((x) => x.category === 'label');
  return (
    <>
      <Section title="Disc">
        {formats.isPending ? <ActivityIndicator /> : null}
        <Wrap>
          {discs.map((d) => (
            <FormatChip key={d.slug} f={d} selected={f.disc === d.slug} onPress={() => set({ disc: f.disc === d.slug ? '' : d.slug!, media: '' })} />
          ))}
          {OTHER_MEDIA.map(([v, label]) => (
            <Chip key={v} label={label} selected={!f.disc && f.media === v} onPress={() => set({ media: f.media === v && !f.disc ? '' : v, disc: '' })} />
          ))}
        </Wrap>
      </Section>
      {labels.length ? (
        <Section title="Label (who released it)">
          <Wrap>
            {labels.map((l) => (
              <FormatChip key={l.slug} f={l} selected={f.label === l.slug} onPress={() => set({ label: f.label === l.slug ? '' : l.slug! })} />
            ))}
          </Wrap>
        </Section>
      ) : null}
      <Field label="Edition" placeholder="Director's cut, 4K restoration" maxLength={80} value={f.edition} onChangeText={(edition) => set({ edition })} />
      <Suggest label="Device" value={f.device} onChange={(device) => set({ device })} options={DEVICES} placeholder="Projector" />
    </>
  );
}

// --- picture and sound (tucked away; open when the watch has any)

export function PictureSection({ f, set }: { f: LogForm; set: Set }) {
  const { colors } = useTheme();
  const filled = !!(
    f.dimension ||
    f.resolution ||
    f.hdr ||
    f.audioFormat ||
    f.frameRate ||
    f.audioLang ||
    f.subLang ||
    f.aspect ||
    f.screenAspect ||
    f.screenW ||
    f.screenH
  );
  const [open, setOpen] = useState(filled);
  return (
    <View style={{ gap: 12 }}>
      <Pressable
        onPress={() => setOpen((o) => !o)}
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        style={[styles.disclosure, { backgroundColor: colors.bgElevated }]}
      >
        <Text variant="label" style={{ flex: 1 }}>
          Picture and sound
        </Text>
        {filled && !open ? <Text variant="caption">Set</Text> : null}
        <Icon name={open ? 'chevron-down' : 'chevron'} size={16} color={colors.textMuted} />
      </Pressable>
      {open ? (
        <>
          <Section title="3D">
            <Choice options={DIMENSIONS} value={f.dimension} onChange={(dimension) => set({ dimension })} />
          </Section>
          <Section title="Resolution">
            <Choice options={RESOLUTIONS} value={f.resolution} onChange={(resolution) => set({ resolution })} />
          </Section>
          <Section title="HDR">
            <Choice options={HDRS} value={f.hdr} onChange={(hdr) => set({ hdr })} />
          </Section>
          <Suggest label="Sound" value={f.audioFormat} onChange={(audioFormat) => set({ audioFormat })} options={AUDIO} placeholder="Dolby Atmos" />
          <Pair>
            <View style={{ flex: 1 }}>
              <Field label="Audio language" placeholder="English" value={f.audioLang} onChangeText={(audioLang) => set({ audioLang })} />
            </View>
            <View style={{ flex: 1 }}>
              <Field label="Subtitles" placeholder="None" value={f.subLang} onChangeText={(subLang) => set({ subLang })} />
            </View>
          </Pair>
          <Field label="Frame rate (fps)" keyboardType="decimal-pad" placeholder="24" value={f.frameRate} onChangeText={(frameRate) => set({ frameRate })} />
          <Suggest label="Aspect ratio of the film" value={f.aspect} onChange={(aspect) => set({ aspect })} options={ASPECTS} placeholder="2.39:1" />
          <Toggle label="Changes ratio part way (e.g. IMAX scenes)" value={f.aspectVariable} onChange={(aspectVariable) => set({ aspectVariable })} />
          <Field label="Screen aspect ratio" placeholder="1.90:1" value={f.screenAspect} onChangeText={(screenAspect) => set({ screenAspect })} />
          <Pair>
            <View style={{ flex: 1 }}>
              <Field label="Screen width" keyboardType="decimal-pad" value={f.screenW} onChangeText={(screenW) => set({ screenW })} />
            </View>
            <View style={{ flex: 1 }}>
              <Field label="Height" keyboardType="decimal-pad" value={f.screenH} onChangeText={(screenH) => set({ screenH })} />
            </View>
            <View style={{ justifyContent: 'flex-end', paddingBottom: 6 }}>
              <Wrap>
                <Chip label="m" selected={f.dimUnit === 'm'} onPress={() => set({ dimUnit: 'm' })} />
                <Chip label="ft" selected={f.dimUnit === 'ft'} onPress={() => set({ dimUnit: 'ft' })} />
              </Wrap>
            </View>
          </Pair>
        </>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  pair: { flexDirection: 'row', gap: 10 },
  toggle: { flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 44 },
  ratingRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  like: { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center' },
  venue: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 8 },
  venuePicked: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 12, borderRadius: 12 },
  empty: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 12, borderRadius: 12 },
  disclosure: { flexDirection: 'row', alignItems: 'center', gap: 8, paddingHorizontal: 14, minHeight: 48, borderRadius: 12 },
});
