import { cn } from '~/helpers/cn';

function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('tw:animate-pulse tw:rounded-xl tw:bg-muted', className)}
      {...props}
    />
  );
}

export { Skeleton };
