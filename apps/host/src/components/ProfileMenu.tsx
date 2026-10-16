import { LogOutIcon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

import { Avatar, AvatarFallback } from '~/components/ui/avatar';
import { Button } from '~/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '~/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '~/components/ui/tooltip';
import { preloadedState } from '~/stores/preloaded-state';

/**
 * A single-line label that clips with an ellipsis and shows its full text in
 * a tooltip only while it is clipped, re-measured whenever the label resizes.
 * The trigger renders a `<span>`: the label sits inside the menu, where Base
 * UI's default `<button>` would nest one interactive element in another.
 */
function TruncatedText({ text }: { text: string }) {
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
        className="tw:block tw:truncate tw:text-sm tw:text-muted-foreground"
      >
        {text}
      </TooltipTrigger>
      <TooltipContent>{text}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The signed-in teacher's avatar, opening a menu with their name, role and
 * email, and the Sign out action. Renders nothing when the page has no
 * signed-in user.
 */
export function ProfileMenu() {
  const signOutFormRef = useRef<HTMLFormElement>(null);
  const user = preloadedState.user;

  if (!user) {
    return null;
  }

  return (
    <>
      {/* A real form post rather than fetch, so the browser follows the
          server's redirect to the sign-in page. It sits outside the menu
          because the menu content is portalled out of any enclosing form. */}
      <form ref={signOutFormRef} method="post" action="/logout" hidden />

      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="outline"
              size="icon-sm"
              className="tw:rounded-full"
              aria-label="Account menu"
            />
          }
        >
          <Avatar>
            <AvatarFallback>{user.name.charAt(0).toUpperCase()}</AvatarFallback>
          </Avatar>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="tw:w-auto tw:max-w-[min(24rem,calc(100vw-2rem))] tw:min-w-72"
        >
          <DropdownMenuGroup>
            <DropdownMenuLabel className="tw:whitespace-normal">
              <div className="tw:flex tw:min-w-0 tw:flex-col">
                <span className="tw:truncate tw:text-sm tw:font-semibold tw:text-foreground">
                  {user.name}
                </span>
                <span className="tw:text-sm tw:text-muted-foreground">{user.role}</span>
                {/* The affiliation card. `-mx-3` cancels the label's own
                    padding so the card's edges line up with the menu items'
                    edges instead of sitting 12px in. The school name belongs
                    in here above the email, once the session carries it
                    (follow-up to #161). */}
                <div className="tw:-mx-3 tw:mt-2 tw:flex tw:min-w-0 tw:flex-col tw:rounded-xl tw:bg-muted tw:px-3 tw:py-2">
                  <TruncatedText text={user.email} />
                </div>
              </div>
            </DropdownMenuLabel>
          </DropdownMenuGroup>
          <DropdownMenuSeparator className="tw:mt-2 tw:mb-3" />
          <DropdownMenuItem
            className="tw:py-2 tw:max-sm:py-3"
            onClick={() => signOutFormRef.current?.requestSubmit()}
          >
            <LogOutIcon />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
