import { fileURLToPath } from 'node:url';

import express, { type Express, Router } from 'express';
import type Provider from 'oidc-provider';

import { accounts } from './provider.ts';

export interface AppOptions {
  /** Whether a sign-in that names no account asks the user to choose one, instead of signing in as the first account. */
  accountPicker?: boolean;
}

/**
 * Builds the mock Edupass app.
 *
 * @param provider - Handles the OIDC endpoints.
 * @param options - Whether the account picker is on. Off by default.
 * @returns the App.
 */
export function createApp(provider: Provider, options: AppOptions = {}): Express {
  const router = Router();

  router.get('/interaction/:uid', async (req, res) => {
    const details = await provider.interactionDetails(req, res);

    if (details.prompt.name === 'login') {
      const requestedAccount = details.params.account as string | undefined;

      if (!requestedAccount && options.accountPicker) {
        res.render('account-picker', { accounts });
        return;
      }

      const account = requestedAccount
        ? accounts.find((a) => a.id === requestedAccount)
        : accounts[0];

      if (!account) {
        throw new Error(
          `Unknown account '${requestedAccount}'. Available: ${accounts.map((a) => a.id).join(', ')}`,
        );
      }

      await provider.interactionFinished(
        req,
        res,
        { login: { accountId: account.id } },
        { mergeWithLastSubmission: false },
      );
      return;
    }

    if (details.prompt.name === 'consent') {
      const grant = new provider.Grant();
      grant.accountId = details.session!.accountId;
      grant.clientId = details.params.client_id as string;
      grant.addOIDCScope('openid');
      const grantId = await grant.save();

      await provider.interactionFinished(
        req,
        res,
        { consent: { grantId } },
        { mergeWithLastSubmission: true },
      );
      return;
    }

    throw new Error(`Unhandled interaction prompt: ${details.prompt.name}`);
  });

  router.post('/interaction/:uid', express.urlencoded({ extended: false }), async (req, res) => {
    const details = await provider.interactionDetails(req, res);

    if (details.prompt.name !== 'login') {
      throw new Error(`Unhandled interaction prompt: ${details.prompt.name}`);
    }

    const chosenAccount = (req.body as { account?: string }).account;
    const account = accounts.find((a) => a.id === chosenAccount);

    if (!account) {
      throw new Error(`Unknown account '${chosenAccount}'`);
    }

    await provider.interactionFinished(
      req,
      res,
      { login: { accountId: account.id } },
      { mergeWithLastSubmission: false },
    );
  });

  const app = express();

  app.set('views', fileURLToPath(new URL('./views', import.meta.url)));
  app.set('view engine', 'ejs');
  // Parse each template once, as Express otherwise does only when NODE_ENV=production.
  app.set('view cache', true);

  app.get('/health', (_req, res) => {
    res.sendStatus(200);
  });

  app.use(router);
  app.use(provider.callback());

  return app;
}
