import { Link } from 'react-router';

import illustration from '~/assets/images/remote-load-error.png';
import { Button } from '~/components/ui/button';

export interface RemoteLoadFallbackViewProps {
  onRetry: () => void;
}

export default function RemoteLoadFallbackView({ onRetry }: RemoteLoadFallbackViewProps) {
  return (
    <div className="tw:flex tw:flex-1 tw:flex-col tw:items-center tw:justify-center tw:gap-6 tw:p-8 tw:text-center">
      <img src={illustration} alt="" className="tw:size-64" />
      <div className="tw:flex tw:max-w-sm tw:flex-col tw:gap-2">
        <h2 className="tw:text-2xl tw:font-semibold tw:text-foreground">
          This section didn&apos;t load
        </h2>
        <p className="tw:text-base tw:text-muted-foreground">
          This is usually temporary. Try again, or come back in a few minutes if it keeps happening.
        </p>
      </div>
      <div className="tw:flex tw:gap-3">
        <Button onClick={onRetry}>Try again</Button>
        <Button variant="outline" render={<Link to="/" />}>
          Back to home
        </Button>
      </div>
    </div>
  );
}
