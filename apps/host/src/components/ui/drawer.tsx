import { Drawer as DrawerPrimitive } from '@base-ui/react/drawer';
import { XIcon } from 'lucide-react';
import * as React from 'react';

import { Button } from '~/components/ui/button';
import { cn } from '~/helpers/cn';
import { useIsMobile } from '~/hooks/use-mobile';

interface DrawerContextProps {
  /** Identifies this drawer as the parent of any drawer rendered inside it. */
  id: string;
  hasSnapPoints: boolean;
  modal: DrawerPrimitive.Root.Props['modal'];
  showSwipeHandle: boolean;
  swipeDirection: NonNullable<DrawerPrimitive.Root.Props['swipeDirection']>;
  docked: boolean;
  width: number;
  /** Drag the panel to a new width, clamped to the shared range. */
  resize: (width: number) => void;
}

/** Every docked drawer opens at this width. Any of them can be dragged wider
 * once open, and none of them remembers it: the drag is the teacher's choice
 * about the panel in front of them, not a new default.
 *
 * 440, up from 340. The widest thing a docked panel carries is prose, and at
 * 340 it wrapped every bullet onto a second line. */
export const DOCKED_DRAWER_WIDTH = 440;
type DockedOpenChangeHandler = (active: boolean, isMobile: boolean) => void | (() => void);
const DrawerHostContext = React.createContext<DockedOpenChangeHandler | undefined>(undefined);
export const DrawerHostProvider = DrawerHostContext.Provider;

/** How far the teacher may drag a docked drawer's left edge. Resizing is a
 * property of the dock, not of particular panels, and there is one range, so
 * no two panels resize differently. */
export const DOCKED_DRAWER_MIN_WIDTH = 320;
export const DOCKED_DRAWER_MAX_WIDTH = 1080;
/** A drag never squeezes the page below this. A docked drawer earns its place
 * by being read beside the work; one wide enough to bury the work is a modal
 * that has not said so. */
const MIN_PAGE_WIDTH = 320;
/** The gap a docked drawer keeps from the viewport edge at `lg`, matching the
 * `--drawer-inset` it is rendered with. */
export const DOCKED_DRAWER_INSET = 8;

function clampDockedWidth(width: number) {
  // The viewport can be narrower than the nominal maximum, and on a narrow one
  // the page's floor wins, but never below the panel's own minimum, or the
  // clamp would invert and pin the panel to its widest.
  const max = Math.max(
    DOCKED_DRAWER_MIN_WIDTH,
    Math.min(DOCKED_DRAWER_MAX_WIDTH, window.innerWidth - MIN_PAGE_WIDTH),
  );
  return Math.round(Math.min(max, Math.max(DOCKED_DRAWER_MIN_WIDTH, width)));
}

/** The one duration behind a docked drawer's whole move: the panel's slide and
 * the page's reflow. Every reader falls back to 250ms, so the panel and the
 * page edge can never drift apart.
 *
 * Zeroed inline while a drawer's edge is being dragged, so everything that
 * moves out of the way tracks the pointer instead of easing after it. An eased
 * reflow is right for a panel that opens on its own; under the teacher's hand
 * it reads as lag. */
const SHIFT_DURATION = '--docked-drawer-shift';

// Every open docked drawer that pushes the page registers its width (plus its
// inset, so the page keeps the same gap to the drawer that the drawer keeps to
// the viewport edge) here, and the sum is published as `--docked-drawer-width`
// on <html>, for the app shell to pad its right edge by, so the page reflows
// out of the way instead of lying underneath a `fixed` panel. Below `lg` a
// docked drawer covers the page as a modal instead, so nothing is registered
// there.
const dockedWidths = new Map<string, number>();

function publishDockedWidth() {
  let total = 0;
  for (const width of dockedWidths.values()) total += width;
  document.documentElement.style.setProperty('--docked-drawer-width', `${total}px`);
}

// Only one docked drawer is open at a time under the same parent: each drawer
// owns its own open state, so without this a teacher could stack one docked
// drawer over another. The registry is keyed by the parent drawer's id (top
// level is one key of its own), so opening a drawer inside a docked panel
// closes the other drawers in that panel but never the panel itself, and a
// nested drawer never closes its own parent.
//
// Closing goes through Base UI's imperative `close()`, so the drawer really
// closes whether or not its owner controls it, and the owner sees a normal
// `onOpenChange(false, details)` with reason `imperative-action` that it can
// `details.cancel()` like any other close.
const TOP_LEVEL_PARENT = 'top-level';

type DrawerActionsRef = React.RefObject<DrawerPrimitive.Root.Actions | null>;

const openDockedDrawers = new Map<string, Map<string, DrawerActionsRef>>();

/**
 * One registration for both rules, so the conditions behind them cannot drift:
 * an open docked drawer joins its parent's exclusivity group and, only while
 * it pushes the page at `lg`, contributes its width to the shell's padding.
 */
function useDockedDrawerRegistration({
  id,
  parentId,
  actionsRef,
  isDockedOpen,
  pushesPage,
  width,
}: {
  id: string;
  parentId: string;
  actionsRef: DrawerActionsRef;
  isDockedOpen: boolean;
  pushesPage: boolean;
  width: number;
}) {
  React.useEffect(() => {
    if (!isDockedOpen) return undefined;

    const siblings = openDockedDrawers.get(parentId) ?? new Map<string, DrawerActionsRef>();
    openDockedDrawers.set(parentId, siblings);

    // Register first, then close the others, so two drawers mounting open in
    // the same commit settle the same way whichever order their effects run.
    siblings.set(id, actionsRef);
    for (const [otherId, otherActions] of [...siblings]) {
      if (otherId !== id) otherActions.current?.close();
    }

    return () => {
      siblings.delete(id);
      if (siblings.size === 0) openDockedDrawers.delete(parentId);
    };
  }, [actionsRef, id, isDockedOpen, parentId]);

  // The width is published from an effect of its own because it changes on
  // every frame of a resize drag. Folded into the one above, a single drag
  // across the screen would re-run the loop that closes sibling drawers a few
  // hundred times.
  React.useEffect(() => {
    if (!pushesPage) return undefined;
    dockedWidths.set(id, width + DOCKED_DRAWER_INSET);
    publishDockedWidth();
    return () => {
      dockedWidths.delete(id);
      publishDockedWidth();
    };
  }, [id, pushesPage, width]);
}

/**
 * Base UI writes its imperative actions (`close`, `unmount`) into `actionsRef`.
 * The registry needs them to close a sibling, and a caller may want them too,
 * so writes land in both refs.
 */
function useDrawerActionsRef(callerRef: DrawerActionsRef | undefined) {
  const ownRef = React.useRef<DrawerPrimitive.Root.Actions | null>(null);
  return React.useMemo<DrawerActionsRef>(
    () => ({
      get current() {
        return ownRef.current;
      },
      set current(next) {
        ownRef.current = next;
        if (callerRef) callerRef.current = next;
      },
    }),
    [callerRef],
  );
}

const DrawerContext = React.createContext<DrawerContextProps | null>(null);

function useDrawer() {
  const context = React.useContext(DrawerContext);

  if (!context) {
    throw new Error('useDrawer must be used within a Drawer.');
  }

  return context;
}

function Drawer({
  modal,
  showSwipeHandle = false,
  snapPoints,
  swipeDirection,
  docked = false,
  width = DOCKED_DRAWER_WIDTH,
  push,
  disablePointerDismissal,
  open: openProp,
  defaultOpen,
  onOpenChange,
  onDockedOpenChange,
  actionsRef: callerActionsRef,
  ...props
}: DrawerPrimitive.Root.Props & {
  showSwipeHandle?: boolean;
  onDockedOpenChange?: DockedOpenChangeHandler;
  /**
   * A docked drawer sits flush against the right edge of the viewport, full
   * height, with no overlay, and the page reflows out of its way (`push`).
   * Below `lg` it covers the page as a modal instead. Docked drawers do not
   * dismiss on outside press: the page beside them stays usable.
   */
  docked?: boolean;
  /** The width in px a docked drawer opens at, at `lg` and above. The teacher
   * can drag it wider from there; see `DrawerResizeHandle`. */
  width?: number;
  /** Whether a docked drawer pushes the page. Defaults to true, except for a
   * drawer rendered inside another Drawer: that one covers its parent at the
   * same edge instead of squeezing the page a second time. */
  push?: boolean;
}) {
  const hostDockedOpenChange = React.useContext(DrawerHostContext);
  const dockedOpenChange = onDockedOpenChange ?? hostDockedOpenChange;
  const parent = React.useContext(DrawerContext);
  const id = React.useId();
  const actionsRef = useDrawerActionsRef(callerActionsRef);
  const resolvedPush = push ?? parent === null;
  const isMobile = useIsMobile();
  const hasSnapPoints = snapPoints != null && snapPoints.length > 0;
  const resolvedSwipeDirection = swipeDirection ?? (docked ? 'right' : 'down');
  const resolvedModal = modal ?? (docked ? isMobile : true);

  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(defaultOpen ?? false);
  const isOpen = openProp ?? uncontrolledOpen;
  const handleOpenChange = React.useCallback<
    NonNullable<DrawerPrimitive.Root.Props['onOpenChange']>
  >(
    (nextOpen, details) => {
      // Base UI's own order: the owner hears about the change first and may
      // cancel it, and only an uncancelled change moves the state. Without the
      // guard a cancelled close would leave this shadow copy out of step with
      // the drawer the teacher can still see.
      onOpenChange?.(nextOpen, details);
      if (details.isCanceled) return;
      setUncontrolledOpen(nextOpen);
    },
    [onOpenChange],
  );

  // A width the teacher dragged to. It is deliberately not persisted: a drag
  // is a choice about the panel in front of them right now, not a new default,
  // so every docked panel still opens at `width`.
  const [draggedWidth, setDraggedWidth] = React.useState<number | null>(null);
  const [basis, setBasis] = React.useState({ width, isOpen });
  if (basis.width !== width || basis.isOpen !== isOpen) {
    const reopened = isOpen && !basis.isOpen;
    const callerSetItsOwnWidth = basis.width !== width;
    setBasis({ width, isOpen });
    // Forget the drag when the panel opens again, and when a caller names a
    // width of its own, which is an instruction and should win. Not when it
    // closes: narrowing a panel while it slides out is a wobble on the way to
    // something nobody is looking at any more.
    if ((reopened || callerSetItsOwnWidth) && draggedWidth !== null) {
      setDraggedWidth(null);
    }
  }
  const resolvedWidth = draggedWidth ?? width;

  const resize = React.useCallback((next: number) => {
    setDraggedWidth(clampDockedWidth(next));
  }, []);

  useDockedDrawerRegistration({
    id,
    parentId: parent?.id ?? TOP_LEVEL_PARENT,
    actionsRef,
    isDockedOpen: docked && isOpen,
    pushesPage: docked && resolvedPush && isOpen && !isMobile,
    width: resolvedWidth,
  });

  // The nav gets out of the way for as long as a docked panel is open. Chrome
  // rails on both sides of the page the panel is meant to sit beside make it
  // read as a drawer floating over a cramped page; collapsing the nav buys
  // back room for the work the teacher is still doing. Doing it here means
  // every docked drawer behaves the same way.
  React.useEffect(
    () => dockedOpenChange?.(docked && isOpen, isMobile),
    [dockedOpenChange, docked, isOpen, isMobile],
  );

  const contextValue = React.useMemo(
    () => ({
      id,
      hasSnapPoints,
      modal: resolvedModal,
      showSwipeHandle,
      swipeDirection: resolvedSwipeDirection,
      docked,
      width: resolvedWidth,
      resize,
    }),
    [
      id,
      hasSnapPoints,
      resolvedModal,
      showSwipeHandle,
      resolvedSwipeDirection,
      docked,
      resolvedWidth,
      resize,
    ],
  );

  return (
    <DrawerContext.Provider value={contextValue}>
      <DrawerPrimitive.Root
        data-slot="drawer"
        modal={resolvedModal}
        snapPoints={snapPoints}
        swipeDirection={resolvedSwipeDirection}
        disablePointerDismissal={disablePointerDismissal ?? docked}
        open={openProp}
        defaultOpen={defaultOpen}
        onOpenChange={handleOpenChange}
        actionsRef={actionsRef}
        {...props}
      />
    </DrawerContext.Provider>
  );
}

function DrawerTrigger({ ...props }: DrawerPrimitive.Trigger.Props) {
  return <DrawerPrimitive.Trigger data-slot="drawer-trigger" {...props} />;
}

function DrawerPortal({ ...props }: DrawerPrimitive.Portal.Props) {
  return <DrawerPrimitive.Portal data-slot="drawer-portal" {...props} />;
}

function DrawerClose({ ...props }: DrawerPrimitive.Close.Props) {
  return <DrawerPrimitive.Close data-slot="drawer-close" {...props} />;
}

function DrawerOverlay({ className, ...props }: DrawerPrimitive.Backdrop.Props) {
  return (
    <DrawerPrimitive.Backdrop
      data-slot="drawer-overlay"
      className={cn(
        'tw:fixed tw:inset-0 tw:z-50 tw:min-h-dvh tw:bg-overlay/75 tw:opacity-[max(var(--drawer-overlay-min-opacity,0),calc(1-var(--drawer-swipe-progress)))] tw:transition-opacity tw:duration-[var(--docked-drawer-shift,250ms)] tw:ease-[cubic-bezier(0.22,1,0.36,1)] tw:select-none tw:data-ending-style:pointer-events-none tw:data-ending-style:opacity-0 tw:data-ending-style:duration-[calc(var(--drawer-swipe-strength,1)*var(--docked-drawer-shift,250ms))] tw:data-snap-points:[--drawer-overlay-min-opacity:0.5] tw:data-starting-style:opacity-0 tw:data-swiping:duration-0 tw:supports-backdrop-filter:backdrop-blur-xs tw:supports-[-webkit-touch-callout:none]:absolute',
        className,
      )}
      {...props}
    />
  );
}

function DrawerSwipeHandle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="drawer-swipe-handle"
      aria-hidden="true"
      className={cn(
        'tw:relative tw:z-10 tw:flex tw:shrink-0 tw:cursor-grab tw:transition-opacity tw:duration-200 tw:group-data-nested-drawer-open/drawer-popup:opacity-0 tw:group-data-nested-drawer-swiping/drawer-popup:opacity-100 tw:group-data-[swipe-axis=x]/drawer-popup:h-full tw:group-data-[swipe-axis=x]/drawer-popup:w-3 tw:group-data-[swipe-axis=x]/drawer-popup:items-center tw:group-data-[swipe-axis=y]/drawer-popup:h-3 tw:group-data-[swipe-axis=y]/drawer-popup:w-full tw:group-data-[swipe-axis=y]/drawer-popup:justify-center tw:group-data-[swipe-direction=down]/drawer-popup:items-end tw:group-data-[swipe-direction=left]/drawer-popup:order-last tw:group-data-[swipe-direction=left]/drawer-popup:justify-start tw:group-data-[swipe-direction=right]/drawer-popup:justify-end tw:group-data-[swipe-direction=up]/drawer-popup:order-last tw:group-data-[swipe-direction=up]/drawer-popup:items-start tw:after:block tw:after:shrink-0 tw:after:rounded-full tw:after:bg-muted tw:group-data-[swipe-axis=x]/drawer-popup:after:h-[100px] tw:group-data-[swipe-axis=x]/drawer-popup:after:w-1.5 tw:group-data-[swipe-axis=y]/drawer-popup:after:h-1.5 tw:group-data-[swipe-axis=y]/drawer-popup:after:w-[100px] tw:active:cursor-grabbing',
        className,
      )}
      {...props}
    />
  );
}

/**
 * Drag a docked drawer's left edge to widen it.
 *
 * Pointer-only, and hidden from assistive tech on purpose: it reaches nothing
 * and reveals nothing, and every panel is readable at the width it opens at.
 * Hidden below `lg`, where there is nothing to resize: the drawer covers the
 * page there rather than sitting beside it.
 */
function DrawerResizeHandle() {
  const { width, resize } = useDrawer();
  const from = React.useRef<{
    pointerId: number;
    x: number;
    width: number;
  } | null>(null);
  const [isDragging, setIsDragging] = React.useState(false);

  // The pointer spends the drag out over the page, not on the 12px handle, so
  // the cursor has to be held on the body; otherwise it flickers back to a
  // caret over every paragraph it crosses. The page reflows under the pointer
  // rather than easing after it for the same reason: see SHIFT_DURATION.
  // Undoing this in the cleanup rather than on pointerup means a panel that
  // closes mid-drag still gives the cursor back.
  React.useEffect(() => {
    if (!isDragging) return undefined;
    const { style } = document.body;
    const cursor = style.cursor;
    const userSelect = style.userSelect;
    style.cursor = 'col-resize';
    style.userSelect = 'none';
    document.documentElement.style.setProperty(SHIFT_DURATION, '0s');
    return () => {
      style.cursor = cursor;
      style.userSelect = userSelect;
      document.documentElement.style.removeProperty(SHIFT_DURATION);
    };
  }, [isDragging]);

  function start(event: React.PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.preventDefault();
    from.current = { pointerId: event.pointerId, x: event.clientX, width };
    setIsDragging(true);
    // Capture, because the pointer leaves the handle on the first frame: the
    // panel edge it sits on is what the drag is moving. Best-effort: jsdom
    // has no pointer capture and throws rather than ignoring the call, and a
    // test dispatching on the handle does not need it.
    try {
      event.currentTarget.setPointerCapture(event.pointerId);
    } catch {
      /* empty */
    }
  }

  function move(event: React.PointerEvent<HTMLDivElement>) {
    const origin = from.current;
    if (!origin || origin.pointerId !== event.pointerId) return;
    // The handle is on the panel's left edge, so leftwards is wider. Measured
    // from where the drag began rather than from the viewport edge, so the
    // panel's inset does not offset every width by 8px.
    resize(origin.width + (origin.x - event.clientX));
  }

  function end(event: React.PointerEvent<HTMLDivElement>) {
    if (from.current?.pointerId !== event.pointerId) return;
    from.current = null;
    setIsDragging(false);
    try {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    } catch {
      /* empty */
    }
  }

  return (
    <div
      data-slot="drawer-resize-handle"
      data-dragging={isDragging ? '' : undefined}
      aria-hidden="true"
      className="tw:group/drawer-resize tw:absolute tw:inset-y-0 tw:-left-1.5 tw:z-10 tw:hidden tw:w-3 tw:cursor-col-resize tw:touch-none tw:lg:block"
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
    >
      <div className="tw:mx-auto tw:h-full tw:w-0.5 tw:rounded-full tw:bg-transparent tw:transition-colors tw:group-hover/drawer-resize:bg-border tw:group-data-dragging/drawer-resize:bg-primary" />
    </div>
  );
}

function DrawerContent({
  className,
  children,
  showCloseButton = true,
  showOverlay = true,
  ...props
}: DrawerPrimitive.Popup.Props & {
  showCloseButton?: boolean;
  showOverlay?: boolean;
}) {
  const { hasSnapPoints, modal, showSwipeHandle, swipeDirection, docked, width } = useDrawer();
  const swipeAxis = swipeDirection === 'down' || swipeDirection === 'up' ? 'y' : 'x';
  const { style, ...popupProps } = props;

  return (
    <DrawerPortal>
      {modal === true && showOverlay && (
        <DrawerOverlay
          data-snap-points={hasSnapPoints ? '' : undefined}
          className={docked ? 'tw:bg-black/20' : undefined}
        />
      )}
      <DrawerPrimitive.Viewport
        data-slot="drawer-viewport"
        data-modal={modal}
        className="tw:pointer-events-none tw:fixed tw:inset-0 tw:z-50 tw:select-none tw:data-[modal=true]:pointer-events-auto"
      >
        <DrawerPrimitive.Popup
          data-slot="drawer-popup"
          data-swipe-axis={swipeAxis}
          data-snap-points={hasSnapPoints ? '' : undefined}
          className={cn(
            // Base.
            'tw:group/drawer-popup tw:pointer-events-auto tw:fixed tw:z-50 tw:m-(--drawer-inset,0px) tw:flex tw:h-(--drawer-content-height) tw:max-h-(--drawer-content-max-height,none) tw:min-h-0 tw:w-(--drawer-content-width,auto) tw:transform-[translate3d(var(--translate-x,0px),var(--translate-y,0px),0)_scale(var(--stack-scale))] tw:flex-col tw:rounded-2xl tw:border tw:border-border tw:bg-popover tw:text-sm tw:text-popover-foreground tw:shadow-sm tw:transition-[transform,height,opacity,filter] tw:duration-[var(--docked-drawer-shift,250ms)] tw:ease-[cubic-bezier(0.22,1,0.36,1)] tw:will-change-transform tw:outline-none tw:select-none tw:[--drawer-bleed-background:transparent] tw:[--drawer-inset:--spacing(2)] tw:[interpolate-size:allow-keywords] tw:motion-reduce:transition-none',
            // Nested.
            'tw:data-nested-drawer-open:overflow-hidden tw:data-nested-drawer-open:brightness-95',
            // Bleed.
            'tw:after:pointer-events-none tw:after:absolute tw:after:bg-(--drawer-bleed-background,--theme(--color-popover)) tw:data-[swipe-axis=x]:after:inset-y-0 tw:data-[swipe-axis=x]:after:w-(--bleed) tw:data-[swipe-axis=y]:after:inset-x-0 tw:data-[swipe-axis=y]:after:h-(--bleed) tw:data-[swipe-direction=down]:after:top-full tw:data-[swipe-direction=left]:after:right-full tw:data-[swipe-direction=right]:after:left-full tw:data-[swipe-direction=up]:after:bottom-full',
            // Sizing.
            'tw:[--drawer-content-height:var(--drawer-height,auto)] tw:data-[swipe-axis=x]:[--drawer-content-width:75%] tw:data-[swipe-axis=y]:[--drawer-content-max-height:calc(100dvh-6rem)] tw:data-[swipe-axis=y]:data-snap-points:[--drawer-content-height:100dvh] tw:data-[swipe-axis=x]:sm:[--drawer-content-width:24rem]',
            // Stack.
            'tw:[--bleed:3rem] tw:[--peek:1rem] tw:[--stack-height:var(--drawer-frontmost-height,var(--drawer-height,0px))] tw:[--stack-peek-offset:max(0px,calc((var(--nested-drawers)-var(--stack-progress))*var(--peek)))] tw:[--stack-progress:clamp(0,var(--drawer-swipe-progress),1)] tw:[--stack-scale-base:max(0,calc(1-(var(--nested-drawers)*var(--stack-step))))] tw:[--stack-scale:clamp(0,calc(var(--stack-scale-base)+(var(--stack-step)*var(--stack-progress))),1)] tw:[--stack-shrink:calc(1-var(--stack-scale))] tw:[--stack-step:0.05]',
            // Transitions.
            'tw:data-ending-style:transform-(--closed-transform) tw:data-ending-style:opacity-[0.9999] tw:data-ending-style:duration-[calc(var(--drawer-swipe-strength,1)*var(--docked-drawer-shift,250ms))] tw:data-nested-drawer-swiping:duration-0 tw:data-ending-style:data-nested-drawer-swiping:duration-[calc(var(--drawer-swipe-strength,1)*var(--docked-drawer-shift,250ms))] tw:data-starting-style:transform-(--closed-transform) tw:data-swiping:duration-0 tw:data-ending-style:data-swiping:duration-[calc(var(--drawer-swipe-strength,1)*var(--docked-drawer-shift,250ms))]',
            // Axis: y.
            'tw:data-[swipe-axis=y]:inset-x-0 tw:data-[swipe-axis=y]:data-nested-drawer-open:h-(--stack-height)',
            // Axis: x.
            'tw:data-[swipe-axis=x]:inset-y-0 tw:data-[swipe-axis=x]:flex-row',
            // Direction: down.
            'tw:data-[swipe-direction=down]:bottom-0 tw:data-[swipe-direction=down]:origin-bottom tw:data-[swipe-direction=down]:[--closed-transform:translate3d(0,calc(100%+var(--drawer-inset,0px)+2px),0)] tw:data-[swipe-direction=down]:[--translate-y:calc(var(--drawer-snap-point-offset,0px)+var(--drawer-swipe-movement-y)-var(--stack-peek-offset)-(var(--stack-shrink)*var(--stack-height)))]',
            // Direction: up.
            'tw:data-[swipe-direction=up]:top-0 tw:data-[swipe-direction=up]:origin-top tw:data-[swipe-direction=up]:[--closed-transform:translate3d(0,calc(-100%-var(--drawer-inset,0px)-2px),0)] tw:data-[swipe-direction=up]:[--translate-y:calc(var(--drawer-snap-point-offset,0px)+var(--drawer-swipe-movement-y)+var(--stack-peek-offset)+(var(--stack-shrink)*var(--stack-height)))]',
            // Direction: left.
            'tw:data-[swipe-direction=left]:left-0 tw:data-[swipe-direction=left]:origin-left tw:data-[swipe-direction=left]:[--closed-transform:translate3d(calc(-100%-var(--drawer-inset,0px)-2px),0,0)] tw:data-[swipe-direction=left]:[--translate-x:calc(var(--drawer-swipe-movement-x)+var(--stack-peek-offset)+(var(--stack-shrink)*100%))]',
            // Direction: right.
            'tw:data-[swipe-direction=right]:right-0 tw:data-[swipe-direction=right]:origin-right tw:data-[swipe-direction=right]:[--closed-transform:translate3d(calc(100%+var(--drawer-inset,0px)+2px),0,0)] tw:data-[swipe-direction=right]:[--translate-x:calc(var(--drawer-swipe-movement-x)-var(--stack-peek-offset)-(var(--stack-shrink)*100%))]',
            // Docked: flush to the edge, full height, one dividing border, no
            // stacking effects (a drawer opened from inside a docked drawer
            // covers it at the same edge rather than peeking out behind it).
            // The card-coloured bleed only makes sense while flush; from lg the
            // panel is inset 8px and the strip would fill that gap and swallow
            // the panel's own right border. The top offset clears a 56px
            // header; a host with a different header sets --docked-top-offset
            // on :root instead.
            docked &&
              'tw:rounded-none tw:border-0 tw:border-l tw:bg-card tw:text-foreground tw:shadow-none tw:[--drawer-bleed-background:--theme(--color-card)] tw:[--drawer-inset:0px] tw:[--peek:0px] tw:[--stack-step:0] tw:data-nested-drawer-open:overflow-visible tw:data-nested-drawer-open:brightness-100 tw:data-[swipe-axis=x]:[--drawer-content-width:100%] tw:data-[swipe-axis=x]:sm:[--drawer-content-width:100%] tw:lg:rounded-2xl tw:lg:border tw:lg:border-border tw:lg:[--docked-top:var(--docked-top-offset,3.5rem)] tw:lg:[--drawer-bleed-background:transparent] tw:lg:[--drawer-content-height:calc(100dvh-var(--docked-top)-(2*--spacing(2)))] tw:lg:[--drawer-inset:--spacing(2)] tw:data-[swipe-axis=x]:lg:[--drawer-content-width:var(--docked-width)]',
            className,
          )}
          style={
            docked
              ? ({
                  '--docked-width': `${width}px`,
                  // Under the 56px app header at lg+, where the page reflows
                  // beside it; full height below lg, where it overlays.
                  top: 'var(--docked-top, 0px)',
                  ...style,
                } as React.CSSProperties & Record<'--docked-width', string>)
              : style
          }
          {...popupProps}
        >
          {docked && <DrawerResizeHandle />}
          {showSwipeHandle && <DrawerSwipeHandle />}
          <DrawerPrimitive.Content
            data-slot="drawer-content"
            className={cn(
              'tw:flex tw:min-h-0 tw:flex-1 tw:flex-col tw:overflow-hidden tw:overscroll-contain tw:rounded-[inherit] tw:transition-opacity tw:duration-[var(--docked-drawer-shift,250ms)] tw:ease-[cubic-bezier(0.22,1,0.36,1)] tw:select-text tw:group-data-nested-drawer-open/drawer-popup:opacity-0 tw:group-data-nested-drawer-swiping/drawer-popup:opacity-100 tw:group-data-swiping/drawer-popup:select-none',
              docked && 'tw:group-data-nested-drawer-open/drawer-popup:opacity-100',
            )}
          >
            {children}
          </DrawerPrimitive.Content>
          {showCloseButton && (
            <DrawerPrimitive.Close
              data-slot="drawer-close"
              render={
                <Button
                  variant="ghost"
                  className="tw:absolute tw:top-4 tw:right-4"
                  size="icon-sm"
                />
              }
            >
              <XIcon />
              <span className="tw:sr-only">Close</span>
            </DrawerPrimitive.Close>
          )}
        </DrawerPrimitive.Popup>
      </DrawerPrimitive.Viewport>
    </DrawerPortal>
  );
}

function DrawerHeader({ className, ...props }: React.ComponentProps<'div'>) {
  const { docked } = useDrawer();
  return (
    <div
      data-slot="drawer-header"
      className={cn(
        'tw:flex tw:shrink-0 tw:flex-col tw:gap-0.5 tw:p-4 tw:pb-0 tw:group-data-[swipe-axis=y]/drawer-popup:text-center tw:md:gap-1.5 tw:md:text-left',
        // Docked: one 56px header row, no dividing border. The default
        // close button sits in its right end, so leave room for it.
        docked &&
          'tw:min-h-14 tw:flex-row tw:items-center tw:gap-3 tw:p-4 tw:pr-14 tw:text-left tw:md:gap-3',
        className,
      )}
      {...props}
    />
  );
}

function DrawerFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="drawer-footer"
      className={cn(
        'tw:mt-auto tw:flex tw:shrink-0 tw:flex-col tw:gap-2 tw:p-4 tw:pt-0',
        className,
      )}
      {...props}
    />
  );
}

function DrawerTitle({ className, ...props }: DrawerPrimitive.Title.Props) {
  return (
    <DrawerPrimitive.Title
      data-slot="drawer-title"
      className={cn('tw:text-base tw:font-medium tw:text-foreground', className)}
      {...props}
    />
  );
}

function DrawerDescription({ className, ...props }: DrawerPrimitive.Description.Props) {
  return (
    <DrawerPrimitive.Description
      data-slot="drawer-description"
      className={cn('tw:text-sm tw:text-balance tw:text-muted-foreground', className)}
      {...props}
    />
  );
}

export {
  useDrawer,
  Drawer,
  DrawerPortal,
  DrawerOverlay,
  DrawerResizeHandle,
  DrawerSwipeHandle,
  DrawerTrigger,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerFooter,
  DrawerTitle,
  DrawerDescription,
};
