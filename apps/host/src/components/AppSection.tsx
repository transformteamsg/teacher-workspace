import { type App, AppCard } from '~/components/AppCard';

export interface AppSectionProps {
  title: string;
  apps: App[];
}

export function AppSection({ title, apps }: AppSectionProps) {
  return (
    <section className="tw:flex tw:flex-col tw:gap-4">
      <h2 className="tw:text-lg tw:font-semibold tw:text-foreground">{title}</h2>
      <div className="tw:grid tw:grid-cols-1 tw:gap-4 tw:rounded-2xl tw:sm:grid-cols-3">
        {apps.map((app) => (
          <AppCard key={app.name} {...app} />
        ))}
      </div>
    </section>
  );
}
