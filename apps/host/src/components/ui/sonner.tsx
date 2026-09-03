import { CircleCheck, CircleX, Info, Loader2, TriangleAlert } from 'lucide-react';
import type { CSSProperties } from 'react';
import { Toaster as SonnerToaster, type ToasterProps } from 'sonner';

function Toaster({ ...props }: ToasterProps) {
  return (
    <SonnerToaster
      className="toaster group"
      icons={{
        success: <CircleCheck className="tw:size-4" />,
        info: <Info className="tw:size-4" />,
        warning: <TriangleAlert className="tw:size-4" />,
        error: <CircleX className="tw:size-4 tw:text-destructive" />,
        loading: <Loader2 className="tw:size-4 tw:animate-spin" />,
      }}
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
          '--border-radius': 'var(--radius)',
          '--toast-close-button-start': 'unset',
          '--toast-close-button-end': '0',
          '--toast-close-button-transform': 'translate(35%, -35%)',
        } as CSSProperties
      }
      toastOptions={{
        style: { fontSize: '14px' },
        classNames: {
          toast: 'tw:group/toast',
          icon: 'tw:mt-0.5',
          closeButton:
            'tw:sm:[@media(hover:hover)]:opacity-0 tw:group-hover/toast:opacity-100 tw:focus-visible:opacity-100',
        },
      }}
      {...props}
    />
  );
}

export { Toaster };
