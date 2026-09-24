import { generateKeyPairSync } from 'node:crypto';

import type { JWK } from 'oidc-provider';

/**
 * Generates the provider's RSA-2048 signing key, fresh on every call.
 *
 * @returns The private JWK, which oidc-provider needs in order to sign.
 * @throws When key generation fails.
 */
export function generateSigningJwk(): JWK {
  const { privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
  return privateKey.export({ format: 'jwk' }) as JWK;
}
