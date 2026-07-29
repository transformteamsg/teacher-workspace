import express from 'express';
import type { Application } from 'express';
import type Provider from 'oidc-provider';

import { createInteractionRouter } from './interactions.ts';

/**
 * createApp assembles the Express application. Mount order is load-bearing: the interaction
 * router comes first, and the provider handler is last so it sees every route it owns.
 *
 * No body parser is mounted here. oidc-provider reads request bodies off the raw stream, and
 * while it does fall back to a pre-parsed `req.body`, it warns that an upstream parser is not
 * recommended. The login route brings its own parser, scoped to itself, so the provider always
 * takes the path it prefers.
 */
export function createApp(provider: Provider): Application {
  const app = express();

  app.use(createInteractionRouter(provider));
  app.use(provider.callback());

  return app;
}
