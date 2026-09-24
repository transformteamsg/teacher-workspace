import { createHash, generateKeyPairSync, type KeyObject, X509Certificate } from 'node:crypto';
import { readFileSync } from 'node:fs';

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

export interface ClientKey {
  jwk: JWK;
  certificateThumbprint: string;
}

/**
 * Parses the configured client certificate into the JWK registered as the client's `jwks`.
 *
 * A bare public key is refused rather than accepted: `x5t#S256` is a hash of the certificate's
 * DER, so a key alone leaves nothing to compare and the check would pass for any header.
 *
 * @param value - A PEM X.509 certificate, or a path to one, distinguished by a `-----BEGIN` prefix.
 * @returns The public JWK and the base64url SHA-256 of the certificate DER.
 * @throws When the value is unset, holds a private key, is not a certificate, or is not RSA.
 */
export function clientJwk(value: string): ClientKey {
  const pem = readPem(value);

  if (pem.includes('PRIVATE KEY')) {
    throw new Error('client public key holds a private key: expected an X.509 certificate');
  }

  if (!pem.includes('BEGIN CERTIFICATE')) {
    throw new Error('client public key must hold an X.509 certificate');
  }

  const certificate = new X509Certificate(pem);

  return {
    jwk: publicJwk(certificate.publicKey),
    certificateThumbprint: createHash('sha256').update(certificate.raw).digest('base64url'),
  };
}

function readPem(value: string): string {
  const trimmed = value.trim();

  if (!trimmed) {
    throw new Error(
      'client public key is not set: expected a PEM public key, an X.509 certificate, or a path',
    );
  }

  if (trimmed.startsWith('-----BEGIN')) {
    return trimmed;
  }

  return readFileSync(trimmed, 'utf8');
}

function publicJwk(key: KeyObject): JWK {
  const jwk = key.export({ format: 'jwk' }) as JWK;

  if (jwk.kty !== 'RSA') {
    throw new Error(`client public key must hold an RSA key, got ${jwk.kty}`);
  }

  return jwk;
}
