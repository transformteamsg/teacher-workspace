import { useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { toast } from 'sonner';

import TeacherIllustration from '~/assets/images/teacher-illustration.png';
import { Button } from '~/components/ui/button';

export function LoginView() {
  const [searchParams] = useSearchParams();
  const showError = searchParams.get('error') === 'oauth2_callback_failed';

  useEffect(() => {
    if (!showError) return;

    const id = toast.error("We couldn't sign you in with Edupass.", {
      description:
        'Choose Sign in with Edupass to try again. If it keeps failing, ask your school admin.',
      duration: Infinity,
      closeButton: true,
    });

    return () => {
      toast.dismiss(id);
    };
  }, [showError]);

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
          className="tw:h-auto tw:w-full tw:max-w-66 tw:shrink-0 tw:object-contain tw:sm:max-w-80 tw:md:w-80"
        />
      </div>
    </main>
  );
}
