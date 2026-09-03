import { CircleX, X } from 'lucide-react';
import { useState } from 'react';
import { useSearchParams } from 'react-router';

import TeacherIllustration from '~/assets/illustrations/teacher-illustration.png';
import { Button } from '~/components/ui/button';

export function LoginView() {
  const [searchParams] = useSearchParams();
  const showError = searchParams.get('error') === 'oauth2_callback_failed';
  const [toastDismissed, setToastDismissed] = useState(false);

  return (
    <div className="tw:flex tw:min-h-dvh tw:flex-col tw:bg-background">
      <main className="tw:flex tw:flex-1 tw:flex-col-reverse tw:items-center tw:justify-center tw:gap-8 tw:px-6 tw:py-10 tw:lg:flex-row tw:lg:gap-16 tw:lg:px-8">
        <div className="tw:w-full tw:max-w-sm">
          <div className="tw:rounded-3xl tw:border tw:border-border tw:bg-card tw:p-6 tw:shadow-none tw:sm:p-8">
            <h1 className="tw:text-xl tw:font-semibold tw:text-foreground">
              Sign in to Teacher Workspace
            </h1>
            <p className="tw:mt-2 tw:text-sm tw:leading-normal tw:text-muted-foreground">
              Continue with your Edupass account.
            </p>

            <Button
              render={<a href="/auth/edupass" />}
              className="tw:mt-6 tw:h-9 tw:w-full tw:text-white"
            >
              Sign in with Edupass
            </Button>
          </div>
        </div>

        <img
          src={TeacherIllustration}
          alt=""
          aria-hidden
          className="tw:h-auto tw:w-full tw:max-w-66 tw:shrink-0 tw:object-contain tw:sm:max-w-80 tw:lg:w-80"
        />
      </main>

      {showError && !toastDismissed && (
        <div
          role="alert"
          className="tw:group tw:fixed tw:right-4 tw:bottom-4 tw:left-4 tw:z-50 tw:flex tw:items-start tw:gap-1.5 tw:rounded-[14px] tw:border tw:border-[#ededed] tw:bg-popover tw:p-4 tw:shadow-[0px_4px_12px_#0000001a] tw:sm:right-6 tw:sm:bottom-6 tw:sm:left-auto tw:sm:w-[calc(100%-2rem)] tw:sm:max-w-[356px] tw:dark:border-[#333333]"
          style={{
            fontFamily:
              'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif',
          }}
        >
          <div data-icon="" className="tw:mt-0.5 tw:mr-1 tw:self-start tw:text-muted-foreground">
            <CircleX className="tw:size-4 tw:text-destructive" aria-hidden="true" />
          </div>
          <div data-content="" className="tw:flex tw:min-w-0 tw:flex-1 tw:flex-col tw:gap-0.5">
            <div
              data-title=""
              className="tw:text-sm tw:leading-normal tw:font-medium tw:text-wrap tw:text-foreground"
            >
              We couldn&apos;t sign you in with Edupass.
            </div>
            <div
              data-description=""
              className="tw:text-sm tw:leading-[1.4] tw:font-normal tw:text-wrap tw:text-[#3f3f3f] tw:dark:text-[#e8e8e8]"
            >
              Choose Sign in with Edupass to try again. If it keeps failing, ask your school admin.
            </div>
          </div>
          <button
            type="button"
            aria-label="Close toast"
            onClick={() => setToastDismissed(true)}
            className="tw:absolute tw:-top-2 tw:-right-2 tw:flex tw:size-6 tw:cursor-pointer tw:items-center tw:justify-center tw:rounded-full tw:border tw:border-[#ededed] tw:bg-popover tw:text-muted-foreground tw:shadow-sm tw:transition-opacity tw:group-focus-within:opacity-100 tw:group-hover:opacity-100 tw:hover:text-foreground tw:dark:border-[#333333] tw:[@media(hover:hover)]:opacity-0"
          >
            <X className="tw:size-3" strokeWidth={1.5} />
          </button>
        </div>
      )}
    </div>
  );
}
