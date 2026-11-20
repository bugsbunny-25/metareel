import { File } from 'expo-file-system';
import * as ImagePicker from 'expo-image-picker';
import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Field } from '@/components/Field';
import { Poster } from '@/components/Poster';
import { DetailScreen, Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { deleteDisc, deleteDiscImage, updateDisc, uploadDiscImage, useDisc } from '@/discs/data';
import { DiscCase, DiscImage } from '@/discs/parts';
import { allImages, type Condition, CONDITIONS, type DiscRelease, HIGHLIGHT, IMAGE_KINDS, type ImageKind, isBoxSet, label, videoLine } from '@/discs/vocab';
import { openTitle } from '@/lib/actions';
import { formatMoney, fractionDigits, toMinor } from '@/lib/format';
import { useTheme } from '@/theme';

type DiscSpec = components['schemas']['DiscSpec'];

/**
 * One disc in your collection: its pictures (swipe), what is on it, each
 * disc's picture and sound, the special features, a box set's parts, and your
 * copy (price, retailer, date, condition). Photograph the front, back,
 * slipcover or the discs themselves from here. The release's specs are
 * edited on the web, where there is room for the form.
 */
export function DiscView({ id }: { id: number }) {
  const { colors, radius } = useTheme();
  // The pager pages by the content's width (a rail or side panel takes some of the window).
  const [boxW, setBoxW] = useState(0);
  const disc = useDisc(id);
  const copy = disc.data;
  const r = copy?.release;
  const [kind, setKind] = useState<ImageKind>('front');
  const [target, setTarget] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(false);

  if (disc.isPending) return <DetailScreen title="">{<ActivityIndicator color={colors.textMuted} />}</DetailScreen>;
  if (!copy || !r) {
    return (
      <DetailScreen title="">
        <Panel>
          <Text variant="muted">{disc.error?.message ?? 'This disc isn’t in your collection.'}</Text>
        </Panel>
      </DetailScreen>
    );
  }
  const images = allImages(r);
  const pageW = boxW || 300;

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
    try {
      // quality < 1 makes the picker re-encode, so this is a JPEG even for a HEIC photo.
      const bytes = await new File(asset.uri).arrayBuffer();
      await uploadDiscImage(target ?? r!.id, kind, bytes, asset.mimeType && asset.mimeType !== 'image/heic' ? asset.mimeType : 'image/jpeg');
    } catch (err) {
      Alert.alert('Couldn’t add the picture', err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  function confirmRemove() {
    Alert.alert('Remove from your collection?', `${r!.title} comes off your shelf with what you paid for it.`, [
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
    <DetailScreen title={r.title} refreshing={disc.isRefetching} onRefresh={() => void disc.refetch()}>
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

      {r.titles.length ? (
        <Panel title="On it">
          {r.titles.map((t) => (
            <Pressable
              key={`${t.tmdb_kind}${t.tmdb_id}${t.season_number ?? ''}`}
              style={styles.row}
              onPress={() => openTitle({ kind: t.tmdb_kind, id: t.tmdb_id })}
              accessibilityRole="button"
            >
              <Poster path={t.poster_path} title={t.title} width={40} />
              <Text variant="label" style={{ flex: 1 }}>
                {t.title}
                <Text variant="muted">
                  {t.year ? ` (${t.year})` : ''}
                  {t.season_number != null ? ` · Season ${t.season_number}` : ''}
                </Text>
              </Text>
            </Pressable>
          ))}
        </Panel>
      ) : null}

      <CopyPanel copy={copy} editing={editing} setEditing={setEditing} />

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

      <Facts release={r} />
      <Discs release={r} />
      {r.parts.map((p) => (
        <View key={p.id} style={{ gap: 12 }}>
          <View style={styles.row}>
            <DiscCase release={p} width={64} />
            <View style={{ flex: 1 }}>
              <Text variant="caption">{[label(p.media_format), label(p.packaging)].filter(Boolean).join(' · ')}</Text>
              <Text variant="heading">{p.title}</Text>
              {p.edition ? <Text variant="muted">{p.edition}</Text> : null}
            </View>
          </View>
          <Discs release={p} />
        </View>
      ))}

      <Text variant="muted" style={{ fontSize: 13 }}>
        Disc specs, special features and box-set parts are edited in the web app’s Collection.
      </Text>
      <Button title="Remove from collection" kind="quiet" onPress={confirmRemove} />
    </DetailScreen>
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
  const { colors } = useTheme();
  if (!r.discs.length && !r.special_features.length) return null;
  return (
    <>
      {r.discs.map((d: DiscSpec) => {
        const meta = [
          d.name,
          d.title_index != null ? r.titles[d.title_index]?.title : '',
          d.runtime_minutes ? `${d.runtime_minutes} min` : '',
          d.region ? `Region ${d.region}` : '',
        ]
          .filter(Boolean)
          .join(' · ');
        return (
          <Panel key={d.number}>
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
      })}
      {r.special_features.length ? (
        <Panel title="Special features">
          {r.special_features.map((f, i) => (
            <View key={i} style={[styles.feature, { borderColor: colors.borderSubtle }]}>
              <Text style={{ flex: 1 }}>{f.title}</Text>
              <Text variant="muted" num>
                {[f.minutes ? `${f.minutes} min` : '', f.disc ? `disc ${f.disc}` : ''].filter(Boolean).join(' · ')}
              </Text>
            </View>
          ))}
        </Panel>
      ) : null}
    </>
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
    try {
      await updateDisc(copy.id, {
        retailer: retailer.trim(),
        purchased_on: bought.trim() || undefined,
        condition,
        notes: copy.notes ?? '',
        ...(minor !== null ? { price_minor: minor, currency: cur } : {}),
      });
      setEditing(false);
    } catch (err) {
      Alert.alert('Couldn’t save', err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
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
  badges: { flexDirection: 'row', flexWrap: 'wrap', gap: 6 },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
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
