import { mergeProps } from '@base-ui/react/merge-props';
import { useRender } from '@base-ui/react/use-render';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '~/helpers/cn';

/**
 * The one badge. Colour encodes status, and only status:
 * - `complete`   done, live, or answered yes. Lime.
 * - `attention`  not yet, but expected. Amber.
 * - `critical`   something went wrong and needs the teacher. Crimson.
 * - `neutral`    inactive, or finished without an outcome. Slate.
 * - `info`       a true fact worth noticing, not an outcome. Blue.
 * - `category`   what kind of thing this is. Same slate fill as neutral.
 * - `experiment` the shell's non-GA marker. Violet.
 * - `onTint`     `category`, for a chip sitting on a tinted ground. White.
 *
 * Every filled variant is a step-3 fill with no border and text in the same
 * scale. Text is step-11 where that clears 4.5:1 on step-3 at 12px (crimson,
 * twblue, violet, slate). Lime-11 and amber-11 do not (4.29 and 4.25), so
 * those two take step-12. `default`, `destructive`, `ghost` and `link` are
 * shadcn's structural variants and are not status colours.
 */
const badgeVariants = cva(
  'tw:group/badge tw:inline-flex tw:h-5 tw:w-fit tw:shrink-0 tw:items-center tw:justify-center tw:gap-1 tw:overflow-hidden tw:rounded-4xl tw:border tw:border-transparent tw:px-2 tw:py-0.5 tw:text-xs tw:font-medium tw:whitespace-nowrap tw:transition-all tw:transition-colors tw:focus-visible:border-ring tw:focus-visible:ring-[3px] tw:focus-visible:ring-ring/50 tw:has-data-[icon=inline-end]:pr-1.5 tw:has-data-[icon=inline-start]:pl-1.5 tw:aria-invalid:border-destructive tw:aria-invalid:ring-destructive/20 tw:dark:aria-invalid:ring-destructive/40 tw:[&>svg]:pointer-events-none tw:[&>svg]:size-3!',
  {
    variants: {
      variant: {
        default: 'tw:bg-primary tw:text-primary-foreground tw:[a]:hover:bg-primary/80',
        destructive:
          'tw:bg-destructive/10 tw:text-destructive tw:focus-visible:ring-destructive/20 tw:dark:bg-destructive/20 tw:dark:focus-visible:ring-destructive/40 tw:[a]:hover:bg-destructive/20',
        ghost: 'tw:hover:bg-muted tw:hover:text-muted-foreground tw:dark:hover:bg-muted/50',
        link: 'tw:text-primary tw:underline-offset-4 tw:hover:underline',
        complete: 'tw:bg-lime-3 tw:text-lime-12',
        attention: 'tw:bg-amber-3 tw:text-amber-12',
        critical: 'tw:bg-crimson-3 tw:text-crimson-11',
        neutral: 'tw:bg-slate-3 tw:text-slate-11',
        info: 'tw:bg-twblue-3 tw:text-twblue-11',
        category: 'tw:bg-slate-3 tw:text-slate-11',
        experiment: 'tw:bg-violet-3 tw:text-violet-11',
        // Every other fill is step-3, and so is an Alert's wash, so a chip
        // placed on one has nothing to stand against: slate-3 on crimson-3 is
        // two pale fills side by side. This is `category` with the card's
        // white as its fill, which separates from any step-3 ground. It
        // disappears on a white ground, so use `category` there instead.
        onTint: 'tw:bg-card tw:text-slate-11',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

type BadgeVariant = NonNullable<VariantProps<typeof badgeVariants>['variant']>;

const dotVariants = cva('tw:size-2 tw:shrink-0 tw:rounded-full', {
  variants: {
    dot: {
      complete: 'tw:bg-lime-9',
      attention: 'tw:bg-amber-9',
      critical: 'tw:bg-crimson-9',
      neutral: 'tw:bg-slate-9',
      info: 'tw:bg-twblue-9',
    },
  },
});

type BadgeDotColor = NonNullable<VariantProps<typeof dotVariants>['dot']>;

/**
 * `dot` puts a status dot before the label, for a label that is also a status
 * without turning the whole chip into a coloured pill. Only the two slate
 * variants take one: on a coloured fill the dot would compete with the fill
 * for what the colour means. Step-9, each scale's solid accent: step-11 reads
 * muddy at 8px, amber-11 outright brown. Amber-9 is light on slate-3, which is
 * acceptable because the label beside the dot carries the meaning.
 */
type BadgeProps = useRender.ComponentProps<'span'> &
  (
    | { variant: 'neutral' | 'category'; dot?: BadgeDotColor }
    | {
        variant?: Exclude<BadgeVariant, 'neutral' | 'category'> | null;
        dot?: never;
      }
  );

function Badge({ className, variant = 'default', dot, render, children, ...props }: BadgeProps) {
  return useRender({
    defaultTagName: 'span',
    props: mergeProps<'span'>(
      {
        className: cn(badgeVariants({ className, variant })),
        children: (
          <>
            {dot && <span aria-hidden className={dotVariants({ dot })} />}
            {children}
          </>
        ),
      },
      props,
    ),
    render,
    state: {
      slot: 'badge',
      variant,
    },
  });
}

export { Badge, badgeVariants };
export type { BadgeVariant, BadgeDotColor };
