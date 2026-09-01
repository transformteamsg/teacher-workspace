/**
 * Entry manifest for the `pg` Module Federation remote, which provides the
 * Posts and Groups views.
 *
 * Set `PUBLIC_PG_REMOTE_ENTRY` (see `.env.development.local`) to develop
 * against a local pg dev server; unset, the deployed remote is used.
 */
export const PG_REMOTE_ENTRY =
  import.meta.env.PUBLIC_PG_REMOTE_ENTRY ??
  'https://d390008ekba73v.cloudfront.net/mf-manifest.json';

/** Reports whether the app is pointed at a local pg dev server. */
export const isLocalPgRemote = Boolean(import.meta.env.PUBLIC_PG_REMOTE_ENTRY);
