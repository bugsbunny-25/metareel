import { File } from 'expo-file-system';
import * as ImagePicker from 'expo-image-picker';
import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Field } from '@/components/Field';
import { Icon } from '@/components/Icon';
import { Poster } from '@/components/Poster';
import { DetailScreen, Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { deleteDisc, deleteDiscImage, updateDisc, uploadDiscImage, useDisc } from '@/discs/data';
import { DiscCase, DiscImage } from '@/discs/parts';
import { allImages, type Condition, CONDITIONS, type DiscRelease, HIGHLIGHT, IMAGE_KINDS, type ImageKind, isBoxSet, label, videoLine } from '@/discs/vocab';
import { openTitle } from '@/lib/actions';
import { attempt } from '@/lib/attempt';
import { usePullToRefresh } from '@/lib/refresh';
import { formatMoney, fractionDigits, toMinor } from '@/lib/format';
import { useTheme } from '@/theme';

type DiscSpec = components['schemas']['DiscSpec'];

/**
 * A release in your library, drilled into like the box in your hands: a box
 * set first lists its discs (or the sets inside it), each disc then shows
 * its specs and what is on it, with the film or show a tap away. A single
 * release shows everything at once. Your copy (price, retailer, date,
 * condition) and photos sit on the release's own page. The release's specs
 * are edited on the web, where there is room for the form.
 */
export function DiscView({ id, part, disc }: { id: number; part?: number; disc?: number }) {
  const { colors, radius } = useTheme();
  // The pager pages by the content's width (a rail or side panel takes some of the window).
  const [boxW, setBoxW] = useState(0);
  const query = useDisc(id);
  const pull = usePullToRefresh(() => query.refetch());
  const copy = query.data;
  const root = copy?.release;
  const [kind, setKind] = useState<ImageKind>('front');
  const [target, setTarget] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(false);

  if (query.isPending) return <DetailScreen title="">{<ActivityIndicator color={colors.textMuted} />}</DetailScreen>;
  const r = part != null ? root?.parts.find((p) => p.id === part) : root;
  if (!copy || !root || !r) {
    return (
      <DetailScreen title="">
        <Panel>
          <Text variant="muted">{query.error?.message ?? (root ? 'This disc isn’t in this set any more.' : 'This disc isn’t in your collection.')}</Text>
        </Panel>
      </DetailScreen>
    );
  }
  const top = part == null && disc == null;
  const open = (to: { part?: number; disc?: number }) =>
    router.push({
      pathname: '/disc/[id]',
      params: { id: String(id), ...(to.part != null ? { part: String(to.part) } : {}), ...(to.disc != null ? { disc: String(to.disc) } : {}) },
    });

  // One disc: its specs and what is on it.
  if (disc != null) {
    const d = r.discs.find((x) => x.number === disc);
    if (!d) {
      return (
        <DetailScreen title="">
          <Panel>
            <Text variant="muted">This disc isn’t in this set any more.</Text>
          </Panel>
        </DetailScreen>
      );
    }
    const on = discTitles(r, d);
    const features = r.special_features.filter((f) => f.disc === d.number);
    return (
      <DetailScreen title={`Disc ${d.number}`} refreshing={pull.refreshing} onRefresh={pull.onRefresh}>
        <View style={{ gap: 6 }}>
          <Text variant="caption">
            {r.title.toUpperCase()} · DISC {d.number} OF {r.discs.length}
          </Text>
          <Text variant="display">{on.length === 1 ? on[0].title : d.name || r.title}</Text>
          {d.name && on.length === 1 ? <Text variant="muted">{d.name}</Text> : null}
        </View>
        <OnIt titles={on} title={on.length > 1 ? 'On this disc' : on[0]?.tmdb_kind === 'tv' ? 'The show' : 'The film'} />
        <DiscSpecPanel release={r} disc={d} />
        {features.length ? <Features features={features} /> : null}
      </DetailScreen>
    );
  }

  const images = top ? allImages(r) : allImages(r).filter((im) => im.releaseId === r.id);
  const pageW = boxW || 300;
  // A box set (or a set with several discs) lists them first; a single disc shows everything.
  const choices: Choice[] = r.parts.length
    ? r.parts.map((p) => ({
        key: `p${p.id}`,
        release: p,
        caption: [label(p.media_format), label(p.packaging), p.disc_count > 1 ? `${p.disc_count} discs` : ''].filter(Boolean).join(' · '),
        title: p.title,
        // What is on the part, unless its name already says so.
        sub: p.edition || (p.titles.length === 1 && p.titles[0].title === p.title ? '' : p.titles.map((t) => t.title).join(', ')),
        onPress: () => open({ part: p.id }),
      }))
    : r.discs.length > 1
      ? r.discs.map((d) => ({
          key: `d${d.number}`,
          disc: d.number,
          caption: [`Disc ${d.number}`, label(d.format), d.runtime_minutes ? `${d.runtime_minutes} min` : ''].filter(Boolean).join(' · '),
          title: discTitles(r, d).map((t) => t.title).join(', ') || d.name || `Disc ${d.number}`,
          sub: [d.name && discTitles(r, d).length ? d.name : '', episodesText(d)].filter(Boolean).join(' · '),
          onPress: () => open({ part, disc: d.number }),
        }))
      : [];

  async function addPhoto(from: 'camera' | 'library') {
    // The camera needs permission; the system photo picker doesn't (it hands
    // over only the picture chosen), so the library is never asked for.
    if (from === 'camera') {
      const perm = await ImagePicker.requestCameraPermissionsAsync();
      if (!perm.granted) {
        Alert.alert('No camera access', 'Allow camera access in Settings to photograph your discs.');
        return;
      }
    }
    const opts: ImagePicker.ImagePickerOptions = {
      mediaTypes: ['images'],
      quality: 0.9,
      allowsEditing: true,
      exif: false,
    };
    const res = from === 'camera' ? await ImagePicker.launchCameraAsync(opts) : await ImagePicker.launchImageLibraryAsync(opts);
    const asset = res.canceled ? null : res.assets[0];
    if (!asset) return;
    setBusy(true);
    const id = target ?? root!.id;
    // quality < 1 makes the picker re-encode, so this is a JPEG even for a HEIC photo.
    const mime = asset.mimeType && asset.mimeType !== 'image/heic' ? asset.mimeType : 'image/jpeg';
    const failed = await attempt(async () => uploadDiscImage(id, kind, await new File(asset.uri).arrayBuffer(), mime));
    if (failed) Alert.alert('Couldn’t add the picture', failed);
    setBusy(false);
  }

  function confirmRemove() {
    Alert.alert('Remove from your collection?', `${root!.title} comes off your shelf with what you paid for it.`, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Remove',
        style: 'destructive',
        onPress: () =>
          void deleteDisc(copy!.id)
            .then(() => router.back())
            .catch((err: Error) => Alert.alert('Couldn’t remove it', err.message)),
      },
    ]);
  }

  return (
    <DetailScreen title={top ? r.title : r.title} refreshing={pull.refreshing} onRefresh={pull.onRefresh}>
      <View onLayout={(e) => setBoxW(Math.round(e.nativeEvent.layout.width))} />
      {images.length ? (
        <ScrollView horizontal pagingEnabled showsHorizontalScrollIndicator={false} style={{ width: pageW }}>
          {images.map((im) => (
            <Pressable
              key={im.id}
              style={{ width: pageW, gap: 6, alignItems: 'center' }}
              onLongPress={() =>
                Alert.alert(`${label(im.kind)} picture`, im.part || r.title, [
                  { text: 'Cancel', style: 'cancel' },
                  {
                    text: 'Delete picture',
                    style: 'destructive',
                    onPress: () => void deleteDiscImage(im.releaseId, im.id).catch((e: Error) => Alert.alert('Couldn’t delete it', e.message)),
                  },
                ])
              }
              accessibilityHint="Long press to delete"
            >
              <DiscImage
                url={im.url}
                style={{
                  width: pageW,
                  height: Math.min(420, pageW * 1.2),
                  borderRadius: radius.panel,
                  borderCurve: 'continuous',
                }}
                contentFit="contain"
              />
              <Text variant="caption">
                {label(im.kind)}
                {im.part ? ` · ${im.part}` : ''}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
      ) : (
        <View style={{ alignItems: 'center' }}>
          <DiscCase release={r} width={180} />
        </View>
      )}

      <View style={{ gap: 6 }}>
        <Text variant="caption">
          {!top ? `${root.title.toUpperCase()} · ` : ''}
          {label(r.media_format).toUpperCase()}
          {isBoxSet(r) ? ` · BOX SET OF ${r.disc_count} DISCS` : r.disc_count > 1 ? ` · ${r.disc_count} DISCS` : ''}
        </Text>
        <Text variant="display">{r.title}</Text>
        {r.edition ? <Text variant="muted">{r.edition}</Text> : null}
        {r.highlights.length ? (
          <View style={styles.badges}>
            {r.highlights.map((h) => (
              <Text
                key={h}
                style={[
                  styles.badge,
                  {
                    backgroundColor: colors.bgElevated,
                    color: colors.textPrimary,
                  },
                ]}
              >
                {HIGHLIGHT[h]}
              </Text>
            ))}
          </View>
        ) : null}
      </View>

      {choices.length ? (
        <>
          <ChoiceList title={r.parts.length ? `In this set · ${choices.length}` : `Discs · ${choices.length}`} choices={choices} />
          {/* A set of discs (not a box of sets) also offers its film or show directly. */}
          {!r.parts.length ? <OnIt titles={r.titles} /> : null}
        </>
      ) : (
        <OnIt titles={r.titles} />
      )}

      {top ? <CopyPanel copy={copy} editing={editing} setEditing={setEditing} /> : null}

      {top ? (
        <Panel title="Add a photo">
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 6 }}>
            {IMAGE_KINDS.slice(0, 7).map(([k, l]) => (
              <Chip key={k} label={l} selected={kind === k} onPress={() => setKind(k)} />
            ))}
          </ScrollView>
          {r.parts.length ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 6 }}>
              <Chip label="The box" selected={target === null} onPress={() => setTarget(null)} />
              {r.parts.map((p) => (
                <Chip key={p.id} label={p.title} selected={target === p.id} onPress={() => setTarget(p.id)} />
              ))}
            </ScrollView>
          ) : null}
          <View style={styles.two}>
            <View style={{ flex: 1 }}>
              <Button title="Take photo" busy={busy} onPress={() => void addPhoto('camera')} />
            </View>
            <View style={{ flex: 1 }}>
              <Button title="Choose" kind="quiet" disabled={busy} onPress={() => void addPhoto('library')} />
            </View>
          </View>
        </Panel>
      ) : null}

      <Facts release={r} />
      {/* A single disc's specs show here; a set's are on each disc's page. */}
      {choices.length ? (r.special_features.length ? <Features features={r.special_features} /> : null) : <Discs release={r} />}

      {top ? (
        <>
          <Text variant="muted" style={{ fontSize: 13 }}>
            Disc specs, special features and box-set parts are edited in the web app’s Collection.
          </Text>
          <Button title="Remove from collection" kind="quiet" onPress={confirmRemove} />
        </>
      ) : null}
    </DetailScreen>
  );
}

type DiscTitle = DiscRelease['titles'][number];

/** "Season 1 · Episodes 1-3" for a TV disc. */
function episodesText(d: DiscSpec): string {
  return [d.season_number != null ? `Season ${d.season_number}` : '', d.episodes ? `Episodes ${d.episodes}` : ''].filter(Boolean).join(' · ');
}

/** The titles on one disc: the one it names, else the release's only title. */
function discTitles(r: DiscRelease, d: DiscSpec): DiscTitle[] {
  if (d.title_index != null && r.titles[d.title_index]) return [r.titles[d.title_index]];
  return r.titles.length === 1 ? r.titles : [];
}

type Choice = {
  key: string;
  release?: DiscRelease;
  disc?: number;
  caption: string;
  title: string;
  sub?: string;
  onPress: () => void;
};

/** A set's discs (or the sets inside it), each opening its own page. */
function ChoiceList({ title, choices }: { title: string; choices: Choice[] }) {
  const { colors } = useTheme();
  return (
    <Panel title={title}>
      {choices.map((c) => (
        <Pressable
          key={c.key}
          onPress={c.onPress}
          accessibilityRole="button"
          accessibilityLabel={[c.caption, c.title, c.sub].filter(Boolean).join(', ')}
          style={({ pressed }) => [styles.choice, { borderColor: colors.borderSubtle, opacity: pressed ? 0.7 : 1 }]}
        >
          {c.release ? (
            <DiscCase release={c.release} width={48} />
          ) : (
            <View style={[styles.discDot, { backgroundColor: colors.bgElevated, borderColor: colors.borderSubtle }]}>
              <Text variant="label" num>
                {c.disc}
              </Text>
            </View>
          )}
          <View style={{ flex: 1, gap: 2 }}>
            <Text variant="caption" num>
              {c.caption}
            </Text>
            <Text variant="label" numberOfLines={2}>
              {c.title}
            </Text>
            {c.sub ? (
              <Text variant="muted" numberOfLines={1}>
                {c.sub}
              </Text>
            ) : null}
          </View>
          <Icon name="chevron" size={18} color={colors.textMuted} />
        </Pressable>
      ))}
    </Panel>
  );
}

/** What is on a release or disc, each with a way to its film or show. */
function OnIt({ titles, title = 'On it' }: { titles: DiscTitle[]; title?: string }) {
  const { colors } = useTheme();
  if (!titles.length) return null;
  return (
    <Panel title={title}>
      {titles.map((t) => (
        <Pressable
          key={`${t.tmdb_kind}${t.tmdb_id}${t.season_number ?? ''}`}
          style={({ pressed }) => [styles.row, { opacity: pressed ? 0.7 : 1 }]}
          onPress={() => openTitle({ kind: t.tmdb_kind, id: t.tmdb_id, title: t.title })}
          accessibilityRole="button"
          accessibilityLabel={`${t.title}${t.season_number != null ? `, season ${t.season_number}` : ''}`}
          accessibilityHint={t.tmdb_kind === 'tv' ? 'Opens the show' : 'Opens the film'}
        >
          <Poster path={t.poster_path} title={t.title} width={40} />
          <Text variant="label" style={{ flex: 1 }}>
            {t.title}
            <Text variant="muted">
              {t.year ? ` (${t.year})` : ''}
              {t.season_number != null ? ` · Season ${t.season_number}` : ''}
            </Text>
          </Text>
          <Text variant="caption" style={{ color: colors.textPrimary }}>
            {t.tmdb_kind === 'tv' ? 'View show' : 'View film'}
          </Text>
          <Icon name="chevron" size={16} color={colors.textMuted} />
        </Pressable>
      ))}
    </Panel>
  );
}

function Facts({ release: r }: { release: DiscRelease }) {
  const facts = [
    ['Packaging', [label(r.packaging), r.has_slipcover ? 'slipcover' : ''].filter(Boolean).join(' with ')],
    ['Region', r.region],
    ['Studio', r.studio],
    ['Released', r.release_date ?? ''],
    ['Barcode', r.barcode ?? ''],
  ].filter(([, v]) => v);
  if (!facts.length) return null;
  return (
    <Panel>
      <View style={styles.facts}>
        {facts.map(([k, v]) => (
          <View key={k} style={{ minWidth: 130, gap: 2 }}>
            <Text variant="caption">{k}</Text>
            <Text num>{v}</Text>
          </View>
        ))}
      </View>
    </Panel>
  );
}

function Discs({ release: r }: { release: DiscRelease }) {
  if (!r.discs.length && !r.special_features.length) return null;
  return (
    <>
      {r.discs.map((d: DiscSpec) => (
        <DiscSpecPanel key={d.number} release={r} disc={d} />
      ))}
      {r.special_features.length ? <Features features={r.special_features} /> : null}
    </>
  );
}

/** One disc's picture and sound. */
function DiscSpecPanel({ release: r, disc: d }: { release: DiscRelease; disc: DiscSpec }) {
  const meta = [d.name, d.title_index != null ? r.titles[d.title_index]?.title : '', episodesText(d), d.runtime_minutes ? `${d.runtime_minutes} min` : '', d.region ? `Region ${d.region}` : '']
    .filter(Boolean)
    .join(' · ');
  return (
    <Panel>
      <Text variant="label">
        Disc {d.number} · {label(d.format)}
        {d.capacity ? ` · ${label(d.capacity)}` : ''}
      </Text>
      {meta ? <Text variant="muted">{meta}</Text> : null}
      {videoLine(d) ? <Spec k="Video" v={videoLine(d)} /> : null}
      {d.audio?.length ? <Spec k="Audio" v={d.audio.map((a) => [a.language, label(a.format), a.channels].filter(Boolean).join(' ')).join('\n')} /> : null}
      {d.subtitles?.length ? <Spec k="Subtitles" v={d.subtitles.join(', ')} /> : null}
    </Panel>
  );
}

function Features({ features }: { features: DiscRelease['special_features'] }) {
  const { colors } = useTheme();
  return (
    <Panel title="Special features">
      {features.map((f, i) => (
        <View key={i} style={[styles.feature, { borderColor: colors.borderSubtle }]}>
          <Text style={{ flex: 1 }}>{f.title}</Text>
          <Text variant="muted" num>
            {[f.minutes ? `${f.minutes} min` : '', f.disc ? `disc ${f.disc}` : ''].filter(Boolean).join(' · ')}
          </Text>
        </View>
      ))}
    </Panel>
  );
}

function Spec({ k, v }: { k: string; v: string }) {
  return (
    <View style={styles.spec}>
      <Text variant="caption" style={{ width: 72 }}>
        {k}
      </Text>
      <Text style={{ flex: 1 }}>{v}</Text>
    </View>
  );
}

function CopyPanel({ copy, editing, setEditing }: { copy: components['schemas']['DiscCopy']; editing: boolean; setEditing: (v: boolean) => void }) {
  const { colors } = useTheme();
  const cur0 = copy.currency ?? 'USD';
  const [price, setPrice] = useState(copy.price_minor != null ? (copy.price_minor / 10 ** fractionDigits(cur0)).toFixed(fractionDigits(cur0)) : '');
  const [currency, setCurrency] = useState(copy.currency ?? '');
  const [retailer, setRetailer] = useState(copy.retailer);
  const [bought, setBought] = useState(copy.purchased_on ?? '');
  const [condition, setCondition] = useState<Condition>((copy.condition || '') as Condition);
  const [saving, setSaving] = useState(false);

  async function save() {
    const cur = currency.trim().toUpperCase();
    const minor = toMinor(price, cur || 'USD');
    if (minor === undefined) return Alert.alert('The price isn’t a number.');
    setSaving(true);
    const changes = {
      retailer: retailer.trim(),
      purchased_on: bought.trim() || undefined,
      condition,
      notes: copy.notes ?? '',
      ...(minor !== null ? { price_minor: minor, currency: cur } : {}),
    };
    const failed = await attempt(() => updateDisc(copy.id, changes));
    if (failed) Alert.alert('Couldn’t save', failed);
    else setEditing(false);
    setSaving(false);
  }

  if (!editing) {
    return (
      <Panel title="Your copy">
        <Text variant="display" num style={{ color: colors.accentYours }}>
          {copy.price_minor != null ? formatMoney(copy.price_minor, copy.currency) : '–'}
        </Text>
        <Text variant="muted">{[copy.retailer, copy.purchased_on ?? '', label(copy.condition)].filter(Boolean).join(' · ') || 'No purchase details yet'}</Text>
        <Button title="Edit" kind="quiet" onPress={() => setEditing(true)} />
      </Panel>
    );
  }
  return (
    <Panel title="Your copy">
      <View style={styles.two}>
        <View style={{ flex: 2 }}>
          <Field label="Price" value={price} onChangeText={setPrice} keyboardType="decimal-pad" />
        </View>
        <View style={{ flex: 1 }}>
          <Field label="Currency" value={currency} onChangeText={setCurrency} autoCapitalize="characters" maxLength={3} />
        </View>
      </View>
      <Field label="Retailer" value={retailer} onChangeText={setRetailer} placeholder="Amazon, Best Buy…" />
      <Field label="Bought on" value={bought} onChangeText={setBought} placeholder="YYYY-MM-DD" />
      <View style={{ flexDirection: 'row', gap: 6 }}>
        {CONDITIONS.map(([v, l]) => (
          <Chip key={v} label={l} selected={condition === v} onPress={() => setCondition(condition === v ? '' : v)} />
        ))}
      </View>
      <View style={styles.two}>
        <View style={{ flex: 1 }}>
          <Button title="Cancel" kind="quiet" onPress={() => setEditing(false)} />
        </View>
        <View style={{ flex: 1 }}>
          <Button title="Save" busy={saving} onPress={() => void save()} />
        </View>
      </View>
    </Panel>
  );
}

const styles = StyleSheet.create({
  choice: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 8 },
  discDot: { width: 48, height: 48, borderRadius: 24, borderWidth: StyleSheet.hairlineWidth, alignItems: 'center', justifyContent: 'center' },
  badges: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    borderCurve: 'continuous',
    overflow: 'hidden',
    fontSize: 12,
    fontWeight: '700',
  },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  two: { flexDirection: 'row', gap: 10 },
  facts: { flexDirection: 'row', flexWrap: 'wrap', gap: 14 },
  spec: { flexDirection: 'row', gap: 8 },
  feature: {
    flexDirection: 'row',
    gap: 10,
    paddingVertical: 6,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
});
