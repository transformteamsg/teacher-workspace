'use client';

import { Separator as SeparatorPrimitive } from '@base-ui/react/separator';

import { cn } from '~/helpers/cn';

function Separator({ className, orientation = 'horizontal', ...props }: SeparatorPrimitive.Props) {
  return (
    <SeparatorPrimitive
      data-slot="separator"
      orientation={orientation}
      className={cn(
        'tw:shrink-0 tw:bg-border tw:data-[orientation=horizontal]:h-px tw:data-[orientation=horizontal]:w-full tw:data-[orientation=vertical]:w-px tw:data-[orientation=vertical]:self-stretch',
        className,
      )}
      {...props}
    />
  );
}

export { Separator };
