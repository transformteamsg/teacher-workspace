import { useEffect, useState } from 'react';

import { Tooltip, TooltipContent, TooltipTrigger } from '~/components/ui/tooltip';
import { cn } from '~/helpers/cn';

/**
 * One `ResizeObserver` for every label on the page rather than one each.
 *
 * Several labels on one page resize together: a list or a table renders one
 * per row. An observer apiece then delivers a callback apiece, and React
 * commits between them, so every read finds the layout dirty again and
 * flushes it. A single observer hands the whole page over in one callback
 * instead, and reading every label before notifying any of them keeps that
 * to one flush, because a notification re-renders.
 */
const subscribers = new WeakMap<Element, (truncated: boolean) => void>();
let sharedObserver: ResizeObserver | null = null;

function measureBatch(entries: ResizeObserverEntry[]) {
  const measured: [(truncated: boolean) => void, boolean][] = [];
  for (const { target } of entries) {
    const notify = subscribers.get(target);
    if (notify) measured.push([notify, target.scrollWidth > target.clientWidth]);
  }
  for (const [notify, truncated] of measured) notify(truncated);
}

/** Measure `element` now and on every resize. Returns an unsubscribe. */
function observeLabel(element: Element, notify: (truncated: boolean) => void) {
  const observer = (sharedObserver ??= new ResizeObserver(measureBatch));
  subscribers.set(element, notify);
  // `observe` delivers a first callback of its own, before the next paint.
  // That callback is the initial measurement; there is no separate one to
  // make, and making one anyway is what read every label twice.
  observer.observe(element);
  return () => {
    observer.unobserve(element);
    subscribers.delete(element);
  };
}

interface TruncatedTextProps {
  /** The full text: the tooltip's content, and the rendering. */
  text: string;
  className?: string;
}

/**
 * A single-line label that clips with an ellipsis and shows its full text in
 * a tooltip only when it is actually clipped: measured on the rendered
 * element (`scrollWidth > clientWidth`) and re-measured whenever the element
 * resizes, so narrowing the container turns the tooltip on.
 *
 * The trigger renders a plain `<span>` rather than Base UI's default
 * `<button>`: these labels sit in or beside interactive elements, where a
 * button would add a second focus stop and nest one interactive element
 * inside another. `Tooltip` takes `disabled` instead of the trigger being
 * swapped out, so the DOM shape survives a resize.
 */
export function TruncatedText({ text, className }: TruncatedTextProps) {
  // A state ref, not `useRef`: the measurement has to re-run once the
  // element exists, and Base UI types the trigger's own ref as a button's.
  const [element, setElement] = useState<HTMLElement | null>(null);
  const [truncated, setTruncated] = useState(false);

  useEffect(() => {
    if (!element) return;
    // Without a ResizeObserver the first observe callback never arrives, so
    // the measurement has to be made here.
    if (typeof ResizeObserver === 'undefined') {
      setTruncated(element.scrollWidth > element.clientWidth);
      return;
    }
    // New text re-observes, which re-measures: a longer string clips where
    // the one it replaced did not.
    return observeLabel(element, setTruncated);
  }, [element, text]);

  return (
    <Tooltip disabled={!truncated}>
      <TooltipTrigger
        ref={setElement}
        render={<span />}
        className={cn('tw:block tw:truncate', className)}
      >
        {text}
      </TooltipTrigger>
      <TooltipContent>{text}</TooltipContent>
    </Tooltip>
  );
}
