import { Tooltip as TooltipPrimitive } from '@base-ui/react/tooltip';

import { cn } from '~/helpers/cn';

function TooltipProvider({ delay = 0, ...props }: TooltipPrimitive.Provider.Props) {
  return <TooltipPrimitive.Provider data-slot="tooltip-provider" delay={delay} {...props} />;
}

function Tooltip({ ...props }: TooltipPrimitive.Root.Props) {
  return (
    <TooltipProvider>
      <TooltipPrimitive.Root data-slot="tooltip" {...props} />
    </TooltipProvider>
  );
}

function TooltipTrigger({ ...props }: TooltipPrimitive.Trigger.Props) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({
  className,
  side = 'top',
  sideOffset = 4,
  align = 'center',
  alignOffset = 0,
  children,
  ...props
}: TooltipPrimitive.Popup.Props &
  Pick<TooltipPrimitive.Positioner.Props, 'align' | 'alignOffset' | 'side' | 'sideOffset'>) {
  return (
    <TooltipPrimitive.Portal
      container={typeof document === 'undefined' ? undefined : document.body}
    >
      <TooltipPrimitive.Positioner
        align={align}
        alignOffset={alignOffset}
        side={side}
        sideOffset={sideOffset}
        positionMethod="fixed"
        className="tw:isolate tw:z-50"
      >
        <TooltipPrimitive.Popup
          data-slot="tooltip-content"
          className={cn(
            'tw:z-50 tw:w-fit tw:max-w-[20rem] tw:origin-(--transform-origin) tw:rounded-lg tw:bg-foreground tw:px-3 tw:py-1.5 tw:text-xs tw:text-background tw:transition-[opacity,scale,translate] tw:duration-150 tw:ease-out tw:data-ending-style:scale-95 tw:data-ending-style:opacity-0 tw:data-ending-style:duration-100 tw:data-instant:transition-none tw:data-starting-style:scale-95 tw:data-starting-style:opacity-0 tw:data-[side=bottom]:data-starting-style:-translate-y-1 tw:data-[side=left]:data-starting-style:translate-x-1 tw:data-[side=right]:data-starting-style:-translate-x-1 tw:data-[side=top]:data-starting-style:translate-y-1 tw:**:data-[slot=kbd]:rounded-4xl tw:motion-reduce:transition-none',
            className,
          )}
          {...props}
        >
          {children}
        </TooltipPrimitive.Popup>
      </TooltipPrimitive.Positioner>
    </TooltipPrimitive.Portal>
  );
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider };
