import { useEffect, useRef, useState } from 'react';

import { Tooltip, TooltipContent, TooltipTrigger } from '~/components/ui/tooltip';
import { cn } from '~/helpers/cn';

interface TruncatedTextProps {
  /** The full text: the tooltip's content, and the rendering. */
  text: string;
  className?: string;
}

/**
 * A single-line label that clips with an ellipsis and shows its full text in
 * a tooltip only while it is clipped, re-measured whenever the label resizes.
 * The trigger renders a `<span>`: these labels sit inside other interactive
 * elements, where Base UI's default `<button>` would nest one in another.
 */
export function TruncatedText({ text, className }: TruncatedTextProps) {
  // A callback ref: Base UI types the trigger's ref as a button's, which a
  // `useRef` of the rendered span does not satisfy.
  const label = useRef<HTMLElement | null>(null);
  const [truncated, setTruncated] = useState(false);

  useEffect(() => {
    const element = label.current;
    if (!element) return;
    const measure = () => setTruncated(element.scrollWidth > element.clientWidth);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [text]);

  return (
    <Tooltip disabled={!truncated}>
      <TooltipTrigger
        ref={(element) => {
          label.current = element;
        }}
        render={<span />}
        className={cn('tw:block tw:truncate', className)}
      >
        {text}
      </TooltipTrigger>
      <TooltipContent>{text}</TooltipContent>
    </Tooltip>
  );
}
