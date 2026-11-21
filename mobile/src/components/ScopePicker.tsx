import { ActivityIndicator, ScrollView, View } from 'react-native';

import { Chip } from '@/components/Chip';
import { ServiceLogo } from '@/components/ServiceLogo';
import { providerName, setScope, useChartReference } from '@/data/scope';

/** Country and service chips for the Top 10 (the web's per-view ScopePicker). */
export function ScopePicker({ country, provider }: { country: string; provider: string }) {
  const ref = useChartReference();
  if (!ref.data) return <ActivityIndicator />;
  const services = ref.data.providers.filter((p) => p.slug && p.countries?.includes(country));
  return (
    <View style={{ gap: 8 }}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
        {services.map((p) => (
          <Chip
            key={p.slug}
            label={providerName(p.slug!)}
            selected={p.slug === provider}
            onPress={() => setScope({ provider: p.slug })}
            leading={p.logo_path ? <ServiceLogo path={p.logo_path} /> : undefined}
          />
        ))}
      </ScrollView>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
        {ref.data.countries.map((c) => (
          <Chip
            key={c.code}
            label={c.name ?? c.code!}
            selected={c.code === country}
            onPress={() => {
              // Keep the service if it charts there, else take the first that does.
              const keep = ref.data!.providers.find((p) => p.slug === provider)?.countries?.includes(c.code!);
              setScope({ country: c.code, provider: keep ? provider : ref.data!.providers.find((p) => p.countries?.includes(c.code!))?.slug });
            }}
          />
        ))}
      </ScrollView>
    </View>
  );
}

