import express, { Router } from 'express';

import { accounts, createProvider } from './provider.ts';

export function createApp(port: number) {
  const provider = createProvider(port);

  const router = Router();

  router.get('/interaction/:uid', async (req, res) => {
    const details = await provider.interactionDetails(req, res);

    if (details.prompt.name === 'login') {
      const requestedAccount = details.params.account as string | undefined;
      const account = requestedAccount
        ? accounts.find((a) => a.sub === requestedAccount)
        : accounts[0];

      if (!account) {
        throw new Error(
          `Unknown account '${requestedAccount}'. Available: ${accounts.map((a) => a.sub).join(', ')}`,
        );
      }

      await provider.interactionFinished(
        req,
        res,
        { login: { accountId: account.sub } },
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

  const app = express();

  app.get('/health', (_req, res) => {
    res.sendStatus(200);
  });

  app.use(router);
  app.use(provider.callback());

  return { app, provider };
}
