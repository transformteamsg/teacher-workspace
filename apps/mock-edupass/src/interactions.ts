import express from 'express';
import type { Request, Response, Router } from 'express';
import { errors } from 'oidc-provider';
import type Provider from 'oidc-provider';

import { FAKE_ACCOUNTS, findFakeAccount } from './accounts.ts';
import type { FakeAccount } from './accounts.ts';

/** escapeHtml escapes the five characters that change meaning inside markup. */
function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

/**
 * page wraps body markup in a minimal document. It references no stylesheet, script, image,
 * or font: a page built from markup alone is one a Content-Security-Policy cannot break.
 */
function page(title: string, body: string): string {
  return [
    '<!DOCTYPE html>',
    '<html lang="en">',
    '<head>',
    '<meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    `<title>${escapeHtml(title)}</title>`,
    '</head>',
    '<body>',
    body,
    '</body>',
    '</html>',
  ].join('\n');
}

/** accountEntry renders one account as its own form, so picking it is a single click. */
function accountEntry(uid: string, account: FakeAccount): string {
  const label = account.name ?? account.sub;
  const detail =
    account.name === undefined
      ? `${account.sub}, ${account.email}, no name claim`
      : `${account.sub}, ${account.email}`;

  return [
    '<li>',
    `<form method="post" action="/interaction/${encodeURIComponent(uid)}/login">`,
    `<input type="hidden" name="sub" value="${escapeHtml(account.sub)}">`,
    `<button type="submit">Sign in as ${escapeHtml(label)}</button>`,
    `<span>${escapeHtml(detail)}</span>`,
    '</form>',
    '</li>',
  ].join('\n');
}

function renderPicker(uid: string): string {
  const entries = FAKE_ACCOUNTS.map((account) => accountEntry(uid, account)).join('\n');
  return page(
    'Mock Edupass sign-in',
    [
      '<h1>Mock Edupass</h1>',
      '<p>This is a local stand-in for Edupass. Pick an account to sign in as.</p>',
      `<ul>\n${entries}\n</ul>`,
    ].join('\n'),
  );
}

function renderMessage(message: string): string {
  return page('Mock Edupass', `<h1>Mock Edupass</h1>\n<p>${escapeHtml(message)}</p>`);
}

function sendHtml(res: Response, status: number, html: string): void {
  res.status(status).type('html').send(html);
}

const EXPIRED_MESSAGE = 'This sign-in request is unknown or has expired.';

/**
 * isExpiredInteraction reports whether the error is the expected "no such interaction" case.
 * Anything else is rethrown rather than reported as an expired sign-in: this is a debugging
 * tool, so turning a real failure into a misleading message is the costliest thing it can do.
 */
function isExpiredInteraction(err: unknown): boolean {
  return err instanceof errors.SessionNotFound;
}

/**
 * createInteractionRouter serves the login interaction: an account picker and the form post
 * that completes it. It is the only hand-written HTTP surface; oidc-provider owns the rest.
 *
 * The urlencoded parser is scoped to the login route alone. Mounted application-wide it would
 * drain the request stream oidc-provider reads from, pushing it onto a fallback path it warns
 * against rather than the raw-stream read it expects.
 */
export function createInteractionRouter(provider: Provider): Router {
  const router = express.Router();

  router.get('/interaction/:uid', async (req: Request, res: Response) => {
    let uid: string;
    try {
      ({ uid } = await provider.interactionDetails(req, res));
    } catch (err: unknown) {
      if (!isExpiredInteraction(err)) {
        throw err;
      }
      sendHtml(res, 400, renderMessage(EXPIRED_MESSAGE));
      return;
    }

    sendHtml(res, 200, renderPicker(uid));
  });

  router.post(
    '/interaction/:uid/login',
    express.urlencoded({ extended: false }),
    async (req: Request, res: Response) => {
      let clientId: string;
      try {
        const details = await provider.interactionDetails(req, res);
        clientId = String(details.params.client_id);
      } catch (err: unknown) {
        if (!isExpiredInteraction(err)) {
          throw err;
        }
        sendHtml(res, 400, renderMessage(EXPIRED_MESSAGE));
        return;
      }

      const submitted: unknown = (req.body as Record<string, unknown> | undefined)?.sub;
      const account = typeof submitted === 'string' ? findFakeAccount(submitted) : undefined;
      if (account === undefined) {
        sendHtml(res, 400, renderMessage('That is not one of the accounts on offer.'));
        return;
      }

      // No consent screen, so the grant the provider would otherwise collect is created here.
      // Without it the provider raises a consent prompt and never reaches the redirect URI.
      const grant = new provider.Grant({ accountId: account.sub, clientId });
      grant.addOIDCScope('openid');
      const grantId = await grant.save();

      await provider.interactionFinished(
        req,
        res,
        { login: { accountId: account.sub }, consent: { grantId } },
        { mergeWithLastSubmission: false },
      );
    },
  );

  return router;
}
