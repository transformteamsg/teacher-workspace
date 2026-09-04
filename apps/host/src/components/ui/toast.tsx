import { Toast as ToastPrimitive } from '@base-ui/react/toast';
import { CircleCheck, CircleX, Info, Loader2, TriangleAlert, X } from 'lucide-react';
import type { ReactNode } from 'react';

import { Button } from '~/components/ui/button';
import { cn } from '~/helpers/cn';

const toast = ToastPrimitive.createToastManager();

function ToastProvider({ ...props }: ToastPrimitive.Provider.Props) {
  return <ToastPrimitive.Provider {...props} />;
}

function ToastPortal({ ...props }: ToastPrimitive.Portal.Props) {
  return <ToastPrimitive.Portal data-slot="toast-portal" {...props} />;
}

function ToastViewport({ className, ...props }: ToastPrimitive.Viewport.Props) {
  return (
    <ToastPrimitive.Viewport
      data-slot="toast-viewport"
      className={cn(
        'tw:pointer-events-none tw:fixed tw:inset-x-4 tw:bottom-6 tw:z-50 tw:mx-auto tw:w-auto tw:outline-none tw:sm:right-4 tw:sm:left-auto tw:sm:mx-0 tw:sm:w-[22.5rem]',
        className,
      )}
      {...props}
    />
  );
}

function Toast({ className, ...props }: ToastPrimitive.Root.Props) {
  return (
    <ToastPrimitive.Root
      data-slot="toast"
      className={cn(
        'tw:group/toast tw:pointer-events-auto tw:absolute tw:right-0 tw:bottom-0 tw:z-[calc(1000-var(--toast-index))] tw:w-full tw:origin-bottom tw:rounded-2xl tw:border tw:bg-popover tw:text-popover-foreground tw:shadow-lg tw:will-change-transform tw:outline-none tw:select-none tw:focus-visible:border-ring tw:focus-visible:ring-[3px] tw:focus-visible:ring-ring/50',
        'tw:[--gap:0.75rem] tw:[--height:var(--toast-frontmost-height,var(--toast-height))] tw:[--offset-y:calc(var(--toast-offset-y)*-1+calc(var(--toast-index)*var(--gap)*-1)+var(--toast-swipe-movement-y))] tw:[--peek:0.75rem] tw:[--scale:calc(max(0,1-(var(--toast-index)*0.1)))] tw:[--shrink:calc(1-var(--scale))]',
        'tw:h-(--height) tw:[transform:translateX(var(--toast-swipe-movement-x))_translateY(calc(var(--toast-swipe-movement-y)-(var(--toast-index)*var(--peek))-(var(--shrink)*var(--height))))_scale(var(--scale))] tw:[transition:transform_500ms_cubic-bezier(0.22,1,0.36,1),opacity_500ms,height_150ms]',
        "tw:after:absolute tw:after:top-full tw:after:left-0 tw:after:h-[calc(var(--gap)+1px)] tw:after:w-full tw:after:content-['']",
        'tw:data-expanded:h-(--toast-height) tw:data-expanded:[transform:translateX(var(--toast-swipe-movement-x))_translateY(var(--offset-y))]',
        'tw:data-limited:opacity-0 tw:data-starting-style:[transform:translateY(150%)]',
        'tw:[&[data-ending-style]:not([data-limited]):not([data-swipe-direction])]:[transform:translateY(150%)]',
        'tw:data-ending-style:data-[swipe-direction=down]:[transform:translateY(calc(var(--toast-swipe-movement-y)+150%))]',
        'tw:data-ending-style:data-[swipe-direction=left]:[transform:translateX(calc(var(--toast-swipe-movement-x)-150%))_translateY(var(--offset-y))]',
        'tw:data-ending-style:data-[swipe-direction=right]:[transform:translateX(calc(var(--toast-swipe-movement-x)+150%))_translateY(var(--offset-y))]',
        'tw:data-ending-style:data-[swipe-direction=up]:[transform:translateY(calc(var(--toast-swipe-movement-y)-150%))]',
        'tw:data-expanded:data-ending-style:data-[swipe-direction=down]:[transform:translateY(calc(var(--toast-swipe-movement-y)+150%))]',
        'tw:data-expanded:data-ending-style:data-[swipe-direction=left]:[transform:translateX(calc(var(--toast-swipe-movement-x)-150%))_translateY(var(--offset-y))]',
        'tw:data-expanded:data-ending-style:data-[swipe-direction=right]:[transform:translateX(calc(var(--toast-swipe-movement-x)+150%))_translateY(var(--offset-y))]',
        'tw:data-expanded:data-ending-style:data-[swipe-direction=up]:[transform:translateY(calc(var(--toast-swipe-movement-y)-150%))]',
        className,
      )}
      {...props}
    />
  );
}

function ToastContent({ className, ...props }: ToastPrimitive.Content.Props) {
  return (
    <ToastPrimitive.Content
      data-slot="toast-content"
      className={cn(
        'tw:flex tw:h-full tw:items-start tw:gap-3 tw:overflow-hidden tw:p-4 tw:transition-opacity tw:duration-250 tw:ease-[cubic-bezier(0.22,1,0.36,1)] tw:data-behind:opacity-0 tw:data-expanded:opacity-100',
        className,
      )}
      {...props}
    />
  );
}

function ToastTitle({ className, ...props }: ToastPrimitive.Title.Props) {
  return (
    <ToastPrimitive.Title
      data-slot="toast-title"
      className={cn('tw:text-sm tw:font-medium', className)}
      {...props}
    />
  );
}

function ToastDescription({ className, ...props }: ToastPrimitive.Description.Props) {
  return (
    <ToastPrimitive.Description
      data-slot="toast-description"
      className={cn('tw:text-sm tw:text-muted-foreground', className)}
      {...props}
    />
  );
}

function ToastAction({
  className,
  render = <Button variant="outline" size="sm" />,
  ...props
}: ToastPrimitive.Action.Props) {
  return (
    <ToastPrimitive.Action
      data-slot="toast-action"
      render={render}
      className={cn('tw:shrink-0', className)}
      {...props}
    />
  );
}

function ToastClose({
  className,
  children,
  render = <Button variant="ghost" size="icon-xs" />,
  ...props
}: ToastPrimitive.Close.Props) {
  return (
    <ToastPrimitive.Close
      data-slot="toast-close"
      aria-label="Close toast"
      render={render}
      className={cn(
        "tw:absolute tw:-top-2 tw:-right-2 tw:z-10 tw:shrink-0 tw:cursor-pointer tw:rounded-full tw:border tw:border-border tw:bg-popover tw:text-muted-foreground tw:shadow-sm tw:after:absolute tw:after:-inset-2 tw:after:content-[''] tw:hover:text-foreground",
        'tw:group-hover/toast:opacity-100 tw:focus-visible:opacity-100 tw:sm:[@media(hover:hover)]:opacity-0',
        className,
      )}
      {...props}
    >
      {children ?? <X aria-hidden="true" />}
    </ToastPrimitive.Close>
  );
}

function ToastIcon({ type }: { type: string | undefined }) {
  let icon: ReactNode = null;

  if (type === 'success') {
    icon = <CircleCheck aria-hidden="true" />;
  }

  if (type === 'info') {
    icon = <Info aria-hidden="true" />;
  }

  if (type === 'warning') {
    icon = <TriangleAlert aria-hidden="true" />;
  }

  if (type === 'error') {
    icon = <CircleX className="tw:text-destructive" aria-hidden="true" />;
  }

  if (type === 'loading') {
    icon = <Loader2 className="tw:animate-spin" aria-hidden="true" />;
  }

  if (!icon) {
    return null;
  }

  return (
    <span
      data-slot="toast-icon"
      className="tw:mt-0.5 tw:shrink-0 tw:[&_svg]:pointer-events-none tw:[&_svg:not([class*='size-'])]:size-4"
    >
      {icon}
    </span>
  );
}

function ToastList() {
  const { toasts } = ToastPrimitive.useToastManager();

  return toasts.map((toastItem) => (
    <Toast key={toastItem.id} toast={toastItem}>
      <ToastContent>
        <ToastIcon type={toastItem.type} />
        <div className="tw:flex tw:min-w-0 tw:flex-1 tw:flex-col tw:gap-1">
          <ToastTitle />
          <ToastDescription />
        </div>
        <ToastAction />
      </ToastContent>
      <ToastClose />
    </Toast>
  ));
}

function Toaster({ children, toastManager = toast, ...props }: ToastPrimitive.Provider.Props) {
  return (
    <ToastProvider toastManager={toastManager} {...props}>
      {children}
      <ToastPortal>
        <ToastViewport>
          <ToastList />
        </ToastViewport>
      </ToastPortal>
    </ToastProvider>
  );
}

const createToastManager = ToastPrimitive.createToastManager;
const useToastManager = ToastPrimitive.useToastManager;

export {
  Toaster,
  Toast,
  ToastAction,
  ToastClose,
  ToastContent,
  ToastDescription,
  ToastPortal,
  ToastProvider,
  ToastTitle,
  ToastViewport,
  createToastManager,
  toast,
  useToastManager,
};
