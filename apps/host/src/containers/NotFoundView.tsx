import { Link } from 'react-router';

import illustration from '~/assets/images/not-found.png';
import { Button } from '~/components/ui/button';

const suggestedLinks = [
  { label: 'Student Insights', to: '/students' },
  { label: 'Posts', to: '/posts' },
];

export function NotFoundView() {
  return (
    <main className="tw:flex tw:min-h-svh tw:flex-col-reverse tw:items-center tw:justify-center tw:gap-8 tw:bg-background tw:px-6 tw:py-10 tw:lg:flex-row tw:lg:gap-16 tw:lg:px-8">
      <div className="tw:w-full tw:max-w-sm">
        <h1 className="tw:text-2xl tw:font-semibold tw:text-foreground">
          Sorry, we couldn&apos;t find that page
        </h1>
        <p className="tw:mt-3 tw:text-sm tw:text-muted-foreground">
          The page you were looking for doesn&apos;t exist, or it may have moved. Check the URL, or
          head back to a familiar place.
        </p>
        <Button className="tw:mt-8" render={<Link to="/" />}>
          Back to home
        </Button>
        <div className="tw:mt-10">
          <p className="tw:text-sm tw:text-muted-foreground">Or try:</p>
          <ul className="tw:mt-3 tw:flex tw:flex-col tw:gap-1.5">
            {suggestedLinks.map((link) => (
              <li key={link.to}>
                <Link
                  to={link.to}
                  className="tw:text-sm tw:font-medium tw:text-primary tw:hover:underline"
                >
                  {link.label}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </div>
      <img
        src={illustration}
        alt="A person surrounded by scattered papers and screens, looking lost"
        width={640}
        height={640}
        decoding="async"
        className="tw:h-auto tw:w-64 tw:max-w-full tw:object-contain tw:lg:w-80"
      />
    </main>
  );
}
