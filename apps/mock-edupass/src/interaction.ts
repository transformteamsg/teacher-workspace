import { Router } from 'express';

import { accounts } from './accounts.ts';
import { provider } from './provider.ts';

export const interactionRouter = Router();

interactionRouter.get('/interaction/:uid', async (req, res) => {
  const details = await provider.interactionDetails(req, res);

  if (details.prompt.name === 'login') {
    const requestedAccount = details.params.account as string | undefined;
    const account = accounts.find((a) => a.sub === requestedAccount) ?? accounts[0];

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
  }
});
