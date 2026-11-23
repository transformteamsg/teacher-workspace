import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';

import { cn } from '~/helpers/cn';

const buttonVariants = cva(
  // Focus uses the stock shadcn tinted-ring recipe (`border-ring` plus a 3px
  // `ring-ring/50`), so every focusable surface in the product paints the same
  // indicator. `--color-ring` is twblue-9, so the ring reads as the medium blue
  // halo; on the filled variant the border matches the fill, but the
  // half-opacity ring outside it still carries the state.
  "tw:cursor-pointer tw:focus-visible:border-ring tw:focus-visible:ring-ring/50 tw:focus-visible:ring-[3px] tw:aria-invalid:ring-destructive/20 tw:dark:aria-invalid:ring-destructive/40 tw:aria-invalid:border-destructive tw:dark:aria-invalid:border-destructive/50 tw:rounded-full tw:border tw:border-transparent tw:bg-clip-padding tw:text-sm tw:font-medium tw:aria-invalid:ring-[3px] tw:[&_svg:not([class*='size-'])]:size-4 tw:inline-flex tw:items-center tw:justify-center tw:whitespace-nowrap tw:transition-[transform,background-color,border-color,color,opacity,box-shadow] tw:duration-150 tw:active:scale-[0.96] tw:disabled:pointer-events-none tw:disabled:opacity-50 tw:disabled:active:scale-100 tw:[&_svg]:pointer-events-none tw:shrink-0 tw:[&_svg]:shrink-0 tw:outline-none tw:group/button tw:select-none",
  {
    variants: {
      variant: {
        default:
          'tw:bg-primary tw:text-primary-foreground tw:hover:bg-primary/80 tw:disabled:opacity-100 tw:disabled:bg-slate-3 tw:disabled:text-slate-8',
        outline:
          'tw:border-border tw:bg-card tw:hover:bg-muted tw:hover:text-foreground tw:aria-expanded:bg-muted tw:aria-expanded:text-foreground',
        secondary:
          'tw:border-border tw:bg-card tw:text-secondary-foreground tw:hover:bg-muted tw:aria-expanded:bg-muted tw:aria-expanded:text-secondary-foreground',
        ghost:
          'tw:hover:bg-muted tw:hover:text-foreground tw:dark:hover:bg-muted/50 tw:aria-expanded:bg-muted tw:aria-expanded:text-foreground',
        destructive:
          'tw:bg-destructive/10 tw:hover:bg-destructive/20 tw:dark:bg-destructive/20 tw:text-crimson-12 tw:focus-visible:border-destructive/40 tw:dark:hover:bg-destructive/30',
        link: 'tw:text-primary tw:underline-offset-4 tw:hover:underline',
        // For a surface painted in the inverted pair (`bg-foreground
        // text-background`), such as the coachmark: the page's own ground as
        // the primary, and an outline in the surface's text colour beside it.
        inverse: 'tw:bg-background tw:text-foreground tw:hover:bg-background/85',
        inverseOutline:
          'tw:border-background/40 tw:bg-transparent tw:text-background tw:hover:bg-background/10',
        inverseGhost:
          'tw:text-background/70 tw:hover:bg-background/15 tw:hover:text-background tw:aria-expanded:bg-background/15',
      },
      size: {
        default:
          'tw:h-9 tw:gap-1.5 tw:px-3 tw:has-data-[icon=inline-end]:pr-2.5 tw:has-data-[icon=inline-start]:pl-2.5',
        xs: "tw:h-6 tw:gap-1 tw:px-2.5 tw:text-xs tw:has-data-[icon=inline-end]:pr-2 tw:has-data-[icon=inline-start]:pl-2 tw:[&_svg:not([class*='size-'])]:size-3",
        sm: 'tw:h-8 tw:gap-1 tw:px-3 tw:has-data-[icon=inline-end]:pr-2 tw:has-data-[icon=inline-start]:pl-2',
        lg: 'tw:h-10 tw:gap-1.5 tw:px-4 tw:has-data-[icon=inline-end]:pr-3 tw:has-data-[icon=inline-start]:pl-3',
        icon: 'tw:size-9',
        'icon-xs': "tw:size-6 tw:[&_svg:not([class*='size-'])]:size-3",
        'icon-sm': 'tw:size-8',
        'icon-lg': 'tw:size-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

// A link styled as a button is still a link. Base UI's Button assumes it wraps
// a native <button>: it would stamp `type="button"` on the anchor and warn, and
// opting out via `nativeButton` is worse, since that stamps `role="button"` over
// the link semantics. So links skip the primitive and just take the styling.
// Recognised by tag for a plain <a>, or by `to`/`href` for a router <Link>.
function isLinkRender(
  render: ButtonPrimitive.Props['render'],
): render is React.ReactElement<{ className?: string } & Record<string, unknown>> {
  if (!React.isValidElement(render)) {
    return false;
  }
  if (typeof render.type === 'string') {
    return render.type === 'a';
  }
  const { to, href } = render.props as { to?: unknown; href?: unknown };
  return to !== undefined || href !== undefined;
}

function Button({
  className,
  variant = 'default',
  size = 'default',
  render,
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  const buttonClassName = cn(buttonVariants({ variant, size, className }));

  if (isLinkRender(render)) {
    // An anchor has no `disabled`, so express it the way links can, and no
    // Base UI primitive, so `nativeButton` has nothing to attach to.
    const { disabled, nativeButton, ...linkProps } = props;
    void nativeButton;
    return React.cloneElement(render, {
      'data-slot': 'button',
      ...linkProps,
      ...(disabled ? { 'aria-disabled': true } : {}),
      className: cn(buttonClassName, render.props.className),
    });
  }

  return (
    <ButtonPrimitive data-slot="button" render={render} className={buttonClassName} {...props} />
  );
}

export { Button, buttonVariants };
