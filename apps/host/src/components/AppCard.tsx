import type { FunctionComponent, SVGProps } from 'react';
import { Link } from 'react-router';

import { Badge } from '~/components/ui/badge';
import { cn } from '~/helpers/cn';

export interface App {
  name: string;
  description: string;
  icon: FunctionComponent<SVGProps<SVGSVGElement>>;
  href: string;
  badge?: string;
}

function AppBadge({ label }: { label: string }) {
  return <Badge variant="info">{label}</Badge>;
}

function AppIcon({ icon: Icon }: Pick<App, 'icon'>) {
  return (
    <div className="tw:relative tw:flex tw:size-16 tw:shrink-0 tw:items-center tw:justify-center tw:overflow-hidden tw:rounded-[var(--radius-card)] tw:border tw:bg-white tw:p-2">
      <Icon aria-hidden className="tw:size-full" />
      <div className="tw:pointer-events-none tw:absolute tw:inset-0 tw:bg-twblue-9 tw:mix-blend-color tw:transition-opacity tw:duration-200 tw:will-change-[opacity] tw:group-hover:opacity-0" />
    </div>
  );
}

interface AppCardProps extends App {
  className?: string;
}

export function AppCard({ name, description, icon, href, badge, className }: AppCardProps) {
  const cardClassName = cn(
    'tw:group tw:flex tw:min-w-0 tw:flex-col tw:gap-4 tw:rounded-[var(--radius-card)] tw:border tw:border-border tw:bg-card tw:p-4 tw:text-left tw:transition-colors tw:hover:bg-muted/50',
    className,
  );

  const content = (
    <>
      <AppIcon icon={icon} />
      <div className="tw:flex tw:flex-col tw:gap-2">
        <div className="tw:flex tw:items-center tw:gap-2">
          <h3 className="tw:font-semibold tw:text-foreground">{name}</h3>
          {badge && <AppBadge label={badge} />}
        </div>
        <p className="tw:line-clamp-3 tw:text-sm tw:text-muted-foreground">{description}</p>
      </div>
    </>
  );

  if (href.startsWith('http')) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={cardClassName}>
        {content}
      </a>
    );
  }

  return (
    <Link to={href} className={cardClassName}>
      {content}
    </Link>
  );
}

export function FeaturedAppCard({ name, description, icon, href, badge, className }: AppCardProps) {
  const featuredClassName = cn(
    'tw:group tw:flex tw:w-full tw:flex-col tw:gap-4 tw:rounded-[var(--radius-card)] tw:border tw:border-border tw:bg-card tw:p-4 tw:transition-colors tw:hover:bg-muted/50 tw:sm:h-[132px] tw:sm:flex-row tw:sm:items-center',
    className,
  );

  const featuredContent = (
    <>
      <AppIcon icon={icon} />
      <div className="tw:flex tw:flex-1 tw:flex-col tw:gap-2">
        <div className="tw:flex tw:items-center tw:gap-2">
          <h3 className="tw:font-semibold tw:text-foreground">{name}</h3>
          {badge && <AppBadge label={badge} />}
        </div>
        <p className="tw:text-sm tw:text-muted-foreground">{description}</p>
      </div>
    </>
  );

  if (href.startsWith('http')) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={featuredClassName}>
        {featuredContent}
      </a>
    );
  }

  return (
    <Link to={href} className={featuredClassName}>
      {featuredContent}
    </Link>
  );
}
