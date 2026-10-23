import { OctagonX } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router';

import TeacherIllustration from '~/assets/images/teacher-illustration.png';
import { Alert, AlertDescription, AlertTitle } from '~/components/ui/alert';
import { Button } from '~/components/ui/button';

// Must match the loginErr* constants in server/internal/handler/auth.go.
type LoginErrorCode =
  | 'no_role'
  | 'multiple_roles'
  | 'unsupported_school_mk'
  | 'oauth2_callback_failed';

const loginErrors: Record<LoginErrorCode, { title: string; description: string }> = {
  no_role: {
    title: "You don't have a role assigned in your Edupass account.",
    description:
      'Ask your Edupass (MIMS) Local Administrator to assign you one for Teacher Workspace.',
  },
  multiple_roles: {
    title: 'You have more than one role or school in your Edupass account.',
    description:
      'You can only sign in with one role at one school. Ask your Edupass (MIMS) Local Administrator to remove the others. If a role is at another school, ask the Local Administrator there.',
  },
  unsupported_school_mk: {
    title: "You can't use Teacher Workspace at an MOE Kindergarten school yet.",
    description:
      'If you also work at another school, ask the Edupass (MIMS) Local Administrator there to assign you a role.',
  },
  oauth2_callback_failed: {
    title: "We couldn't sign you in with Edupass.",
    description:
      'Choose Sign in with Edupass to try again. If it keeps failing, ask the SSOE Administrator.',
  },
};

function toLoginErrorCode(code: string): LoginErrorCode {
  return Object.hasOwn(loginErrors, code) ? (code as LoginErrorCode) : 'oauth2_callback_failed';
}

export default function LoginView() {
  const [searchParams] = useSearchParams();
  const errorParam = searchParams.get('error');
  const returnTo = searchParams.get('return_to');
  const loginError = errorParam === null ? null : loginErrors[toLoginErrorCode(errorParam)];
  const alertRef = useRef<HTMLDivElement>(null);

  const loginHref = returnTo
    ? `/auth/edupass?return_to=${encodeURIComponent(returnTo)}`
    : '/auth/edupass';

  useEffect(() => {
    if (loginError) {
      alertRef.current?.focus();
    }
  }, [loginError]);

  return (
    <main className="tw:flex tw:min-h-dvh tw:flex-col">
      <div className="tw:flex tw:flex-1 tw:flex-col-reverse tw:items-center tw:justify-center tw:gap-8 tw:px-6 tw:py-10 tw:md:flex-row tw:md:gap-16 tw:md:px-8">
        <div className="tw:w-full tw:max-w-sm">
          <div className="tw:rounded-3xl tw:border tw:border-border tw:bg-card tw:p-6 tw:shadow-none tw:sm:p-8">
            <h1 className="tw:text-xl tw:font-semibold tw:text-foreground">
              Sign in to Teacher Workspace
            </h1>
            <p className="tw:mt-2 tw:text-sm tw:leading-normal tw:text-muted-foreground">
              Continue with your Edupass account.
            </p>

            {loginError && (
              <Alert
                ref={alertRef}
                tabIndex={-1}
                className="tw:mt-6 tw:rounded-[14px] tw:border-transparent tw:bg-muted tw:px-4 tw:py-3 tw:has-[>svg]:gap-x-2.5 tw:*:[svg]:text-destructive"
              >
                <OctagonX />
                <AlertTitle>{loginError.title}</AlertTitle>
                <AlertDescription>{loginError.description}</AlertDescription>
              </Alert>
            )}

            <Button
              render={<a href={loginHref} />}
              nativeButton={false}
              className="tw:mt-6 tw:w-full tw:text-white"
            >
              Sign in with Edupass
            </Button>
          </div>
        </div>

        <img
          src={TeacherIllustration}
          alt=""
          aria-hidden
          className="tw:h-auto tw:w-full tw:max-w-66 tw:shrink-0 tw:object-contain tw:sm:max-w-80 tw:md:w-80"
        />
      </div>
    </main>
  );
}
