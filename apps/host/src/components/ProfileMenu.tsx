import { LogOutIcon, UserIcon } from 'lucide-react';
import { useRef } from 'react';

import { Button } from '~/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '~/components/ui/dropdown-menu';

export function ProfileMenu() {
  const signOutFormRef = useRef<HTMLFormElement>(null);

  return (
    <>
      {/* A real form post rather than fetch, so the browser follows the
          server's redirect to the sign-in page. It sits outside the menu
          because the menu content is portalled out of any enclosing form. */}
      <form ref={signOutFormRef} method="post" action="/auth/logout" hidden />

      <DropdownMenu>
        <DropdownMenuTrigger render={<Button size="icon" aria-label="Profile" />}>
          <UserIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="tw:w-auto">
          <DropdownMenuItem
            className="tw:cursor-pointer"
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
