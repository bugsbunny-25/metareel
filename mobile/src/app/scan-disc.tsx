import { type BarcodeScanningResult, CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { Image } from 'expo-image';
import { router } from 'expo-router';
import { useRef, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Field } from '@/components/Field';
import { Poster } from '@/components/Poster';
import { Text } from '@/components/Text';
import { usePreferences } from '@/data/queries';
import { addDisc, lookupDisc, searchTitles } from '@/discs/data';
import { DiscCase } from '@/discs/parts';
import { type Condition, CONDITIONS, type DiscLookup, label, MEDIA_FORMATS, type MediaFormat, type Packaging } from '@/discs/vocab';
import { currencyFor, toMinor } from '@/lib/format';
import { useTheme } from '@/theme';

type TitleHit = components['schemas']['TitleSearchResult'];

const BARCODES = ['ean13', 'upc_a', 'upc_e', 'ean8'] as const;

/**
 * Scan a disc's barcode (or type it) and add it to your collection. A
 * release metareel knows is one tap (another copy); a product the barcode
 * service knows comes back as a draft with its TMDB matches and the
 * retailer's pictures; otherwise name it and pick the film. What you paid and
 * where goes with it. Then scan the next one: collections are added in piles.
 */
export default function ScanDisc() {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const [perm, requestPerm] = useCameraPermissions();
  const prefs = usePreferences();
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [found, setFound] = useState<DiscLookup | null>(null);
  const [added, setAdded] = useState<{ id: number; title: string } | null>(null);
  const scanning = useRef(false);

  async function look(raw: string) {
    if (scanning.current || !raw.trim()) return;
    scanning.current = true;
    setBusy(true);
    setErr('');
    try {
      setFound(await lookupDisc(raw.trim()));
      setCode(raw.trim());
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      scanning.current = false;
    } finally {
      setBusy(false);
    }
  }

  function onScan(r: BarcodeScanningResult) {
    if (scanning.current || found) return;
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    void look(r.data);
  }

  function next() {
    setFound(null);
    setAdded(null);
    setCode('');
    setErr('');
    scanning.current = false;
  }

  const country = (prefs.data as { country?: string } | undefined)?.country;
  return (
    <View
      style={{
        flex: 1,
        backgroundColor: colors.bgCanvas,
        paddingTop: insets.top,
        paddingLeft: insets.left,
        paddingRight: insets.right,
      }}
    >
      <View style={[styles.head, { paddingHorizontal: gutter.compact }]}>
        <Text variant="title">Add a disc</Text>
        <Pressable onPress={() => router.back()} accessibilityRole="button" hitSlop={12}>
          <Text variant="label">Done</Text>
        </Pressable>
      </View>

      <ScrollView
        contentContainerStyle={{
          padding: gutter.compact,
          gap: 14,
          paddingBottom: insets.bottom + 24,
        }}
        keyboardShouldPersistTaps="handled"
      >
        {added ? (
          <View style={{ gap: 12 }}>
            <Text variant="heading">Added {added.title}</Text>
            <Button title="Scan the next one" onPress={next} />
            <Button
              title="Open it"
              kind="quiet"
              onPress={() => {
                router.back();
                router.push({
                  pathname: '/disc/[id]',
                  params: { id: String(added.id) },
                });
              }}
            />
          </View>
        ) : found ? (
          <Result lookup={found} currency={currencyFor(country)} onAdded={setAdded} onRescan={next} />
        ) : (
          <>
            <View style={[styles.camera, { backgroundColor: colors.bgSurface }]}>
              {perm?.granted ? (
                <CameraView style={StyleSheet.absoluteFill} facing="back" barcodeScannerSettings={{ barcodeTypes: [...BARCODES] }} onBarcodeScanned={onScan} />
              ) : (
                <View style={styles.permission}>
                  <Text variant="muted" style={{ textAlign: 'center' }}>
                    Point the camera at the barcode on the back of the case. metareel uses the camera only while this screen is open.
                  </Text>
                  {perm && !perm.canAskAgain ? (
                    <Button title="Open Settings" kind="quiet" onPress={() => void Linking.openSettings()} />
                  ) : (
                    <Button title="Use the camera" onPress={() => void requestPerm()} />
                  )}
                </View>
              )}
              {perm?.granted ? <View style={[styles.target, { pointerEvents: 'none', borderColor: colors.accentPrimary }]} /> : null}
              {busy ? <ActivityIndicator style={StyleSheet.absoluteFill} color={colors.accentPrimary} /> : null}
            </View>
            <Field
              label="Or type the barcode"
              value={code}
              onChangeText={setCode}
              keyboardType="number-pad"
              placeholder="UPC or EAN"
              returnKeyType="search"
              onSubmitEditing={() => void look(code)}
            />
            <Button title="Look up" busy={busy} disabled={!code.trim()} onPress={() => void look(code)} />
          </>
        )}
        {err ? <Text style={{ color: colors.deltaDown }}>{err}</Text> : null}
      </ScrollView>
    </View>
  );
}

const STATUS: Record<string, string> = {
  not_found: 'The barcode service doesn’t know this one. Name it and pick the film; the next person to scan it gets your release.',
  rate_limited: 'The free barcode lookup is used up for today. Name it and pick the film, or try again tomorrow.',
  unavailable: 'Barcode lookups are off or failed. Name it and pick the film.',
};

function Result({
  lookup,
  currency: defaultCurrency,
  onAdded,
  onRescan,
}: {
  lookup: DiscLookup;
  currency: string;
  onAdded: (a: { id: number; title: string }) => void;
  onRescan: () => void;
}) {
  const { colors } = useTheme();
  const sg = lookup.suggestion;
  const known = lookup.status === 'known' ? lookup.release : undefined;
  const [title, setTitle] = useState(sg?.title || sg?.product_title || '');
  const [edition, setEdition] = useState(sg?.edition ?? '');
  const [format, setFormat] = useState<MediaFormat>((sg?.media_format as MediaFormat) || 'bluray');
  const [titles, setTitles] = useState<TitleHit[]>(() => {
    if (!lookup.candidates.length || sg?.box_set) return [];
    return [lookup.candidates[0]];
  });
  const [hits, setHits] = useState<TitleHit[]>(lookup.candidates);
  const [front, setFront] = useState<string | null>(sg?.images[0] ?? null);
  const [price, setPrice] = useState('');
  const [currency, setCurrency] = useState(defaultCurrency);
  const [retailer, setRetailer] = useState('');
  const [condition, setCondition] = useState<Condition>('new');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const has = (t: TitleHit) => titles.some((x) => x.tmdb_kind === t.tmdb_kind && x.tmdb_id === t.tmdb_id);
  const toggle = (t: TitleHit) => setTitles(has(t) ? titles.filter((x) => !(x.tmdb_kind === t.tmdb_kind && x.tmdb_id === t.tmdb_id)) : [...titles, t]);

  async function search() {
    try {
      setHits((await searchTitles(title)).results ?? []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  async function add() {
    const cur = currency.trim().toUpperCase();
    const minor = toMinor(price, cur || 'USD');
    if (minor === undefined) return setErr('The price isn’t a number.');
    setBusy(true);
    setErr('');
    try {
      const copy = {
        retailer: retailer.trim(),
        condition,
        ...(minor !== null ? { price_minor: minor, currency: cur } : {}),
      };
      const out = known
        ? await addDisc({ ...copy, release_id: known.id })
        : await addDisc({
            ...copy,
            release: {
              barcode: lookup.barcode,
              title: title.trim(),
              edition: edition.trim(),
              media_format: format,
              packaging: (sg?.packaging ?? '') as Packaging,
              has_slipcover: sg?.has_slipcover ?? false,
              studio: sg?.studio ?? '',
              label: sg?.label ?? '',
              titles: titles.map((t) => ({
                tmdb_kind: t.tmdb_kind ?? 'movie',
                tmdb_id: t.tmdb_id ?? '',
              })),
            },
            import_images: front ? [{ url: front, kind: 'front' }] : [],
          });
      onAdded({ id: out.id, title: out.release?.title ?? title });
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={{ gap: 14 }}>
      <Text variant="caption" num>
        Barcode {lookup.barcode}
      </Text>
      {known ? (
        <View style={styles.known}>
          <DiscCase release={known} width={110} />
          <View style={{ flex: 1, gap: 4 }}>
            <Text variant="heading">{known.title}</Text>
            <Text variant="muted">{[label(known.media_format), known.edition].filter(Boolean).join(' · ')}</Text>
            {lookup.owned_copy_ids.length ? (
              <Text style={{ color: colors.accentYours }}>
                You have {lookup.owned_copy_ids.length === 1 ? 'a copy' : `${lookup.owned_copy_ids.length} copies`} already.
              </Text>
            ) : null}
          </View>
        </View>
      ) : (
        <>
          {STATUS[lookup.status] ? <Text variant="muted">{STATUS[lookup.status]}</Text> : null}
          {sg ? <Text variant="muted">From UPCitemdb: “{sg.product_title}”</Text> : null}
          {sg?.images.length ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
              {sg.images.map((u) => (
                <Pressable key={u} onPress={() => setFront(front === u ? null : u)} accessibilityRole="button" accessibilityState={{ selected: front === u }}>
                  <Image
                    source={{ uri: u }}
                    style={[
                      styles.pic,
                      {
                        borderColor: front === u ? colors.accentYours : 'transparent',
                      },
                    ]}
                    contentFit="contain"
                  />
                </Pressable>
              ))}
            </ScrollView>
          ) : null}
          <Field label="Title as printed" value={title} onChangeText={setTitle} />
          <Field label="Edition" value={edition} onChangeText={setEdition} placeholder="Steelbook, Collector’s Edition…" />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 6 }}>
            {MEDIA_FORMATS.map(([f, l]) => (
              <Chip key={f} label={l} selected={format === f} onPress={() => setFormat(f)} />
            ))}
          </ScrollView>
          <View style={{ gap: 8 }}>
            <View
              style={{
                flexDirection: 'row',
                alignItems: 'center',
                justifyContent: 'space-between',
              }}
            >
              <Text variant="label">What’s on it</Text>
              <Pressable onPress={() => void search()} accessibilityRole="button" hitSlop={8}>
                <Text variant="label" style={{ color: colors.accentYours }}>
                  Search TMDB
                </Text>
              </Pressable>
            </View>
            {lookup.collections.map((c) => (
              <Chip key={c.id} label={`+ ${c.name} (${c.parts.length})`} onPress={() => setTitles([...titles, ...c.parts.filter((p) => !has(p))])} />
            ))}
            {[...titles, ...hits.filter((h) => !has(h))].slice(0, 10).map((h) => (
              <Pressable
                key={`${h.tmdb_kind}${h.tmdb_id}`}
                style={styles.hit}
                onPress={() => toggle(h)}
                accessibilityRole="checkbox"
                accessibilityState={{ checked: has(h) }}
              >
                <Poster path={h.poster_path} title={h.title} width={32} />
                <Text style={{ flex: 1 }} numberOfLines={1}>
                  {h.title}
                  <Text variant="muted">{h.year ? ` (${h.year})` : ''}</Text>
                </Text>
                <Text
                  style={{
                    color: has(h) ? colors.accentYours : colors.textMuted,
                    fontSize: 18,
                  }}
                >
                  {has(h) ? '✓' : '+'}
                </Text>
              </Pressable>
            ))}
          </View>
        </>
      )}

      <Text variant="heading">Your copy</Text>
      <View style={{ flexDirection: 'row', gap: 10 }}>
        <View style={{ flex: 2 }}>
          <Field label="Price" value={price} onChangeText={setPrice} keyboardType="decimal-pad" placeholder="24.99" />
        </View>
        <View style={{ flex: 1 }}>
          <Field label="Currency" value={currency} onChangeText={setCurrency} autoCapitalize="characters" maxLength={3} />
        </View>
      </View>
      <Field label="Retailer" value={retailer} onChangeText={setRetailer} placeholder="Amazon, Best Buy, HMV…" />
      <View style={{ flexDirection: 'row', gap: 6 }}>
        {CONDITIONS.map(([v, l]) => (
          <Chip key={v} label={l} selected={condition === v} onPress={() => setCondition(v)} />
        ))}
      </View>
      {err ? <Text style={{ color: colors.deltaDown }}>{err}</Text> : null}
      <Button title={known ? 'Add another copy' : 'Add to collection'} busy={busy} disabled={!known && !title.trim()} onPress={() => void add()} />
      <Button title="Scan a different disc" kind="quiet" onPress={onRescan} />
    </View>
  );
}

const styles = StyleSheet.create({
  head: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 12,
  },
  camera: {
    height: 260,
    borderRadius: 18,
    overflow: 'hidden',
    alignItems: 'center',
    justifyContent: 'center',
  },
  permission: { padding: 20, gap: 14, alignItems: 'center' },
  target: {
    position: 'absolute',
    left: '12%',
    right: '12%',
    top: '38%',
    height: '24%',
    borderWidth: 2,
    borderRadius: 10,
    opacity: 0.8,
  },
  known: { flexDirection: 'row', gap: 14, alignItems: 'center' },
  pic: {
    width: 90,
    height: 110,
    borderRadius: 8,
    borderWidth: 2,
    backgroundColor: '#fff',
  },
  hit: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    paddingVertical: 4,
  },
});
