import assert from 'node:assert/strict';
import { createHash, X509Certificate } from 'node:crypto';
import { rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { after, describe, it } from 'node:test';

import { generate } from 'selfsigned';

import { clientJwk } from '../src/jwks.ts';
import { generateClientKeyPair } from './helpers.ts';

const keyPair = await generateClientKeyPair();

const certificateThumbprint = createHash('sha256')
  .update(new X509Certificate(keyPair.certificatePem).raw)
  .digest('base64url');

after(() => {
  rmSync(keyPair.dir, { recursive: true, force: true });
});

describe('clientJwk', () => {
  describe('accepts', () => {
    it('a raw PEM certificate, with its thumbprint', () => {
      const clientKey = clientJwk(keyPair.certificatePem);

      assert.equal(clientKey.jwk.kty, 'RSA');
      assert.ok(clientKey.jwk.n, 'JWK should carry the modulus');
      assert.equal(clientKey.jwk.d, undefined, 'JWK should not carry the private exponent');
      assert.equal(clientKey.certificateThumbprint, certificateThumbprint);
    });

    it('a path to a PEM certificate', () => {
      const path = join(keyPair.dir, 'from-path.cer');
      writeFileSync(path, keyPair.certificatePem);

      const clientKey = clientJwk(path);

      assert.equal(clientKey.jwk.kty, 'RSA');
      assert.equal(clientKey.certificateThumbprint, certificateThumbprint);
    });
  });

  describe('rejects', () => {
    it('a private key', () => {
      assert.throws(() => clientJwk(keyPair.privateKeyPem), { message: /holds a private key/ });
    });

    it('an empty value', () => {
      assert.throws(() => clientJwk(''), { message: /is not set/ });
      assert.throws(() => clientJwk('   '), { message: /is not set/ });
    });

    it('a bare public key, which carries no certificate to thumbprint', () => {
      assert.throws(() => clientJwk(keyPair.publicKeyPem), {
        message: /must hold an X.509 certificate/,
      });
    });

    it('a non-RSA certificate', async () => {
      const pems = await generate([{ name: 'commonName', value: 'ec-test' }], {
        keyType: 'ec',
        algorithm: 'sha256',
      });

      assert.throws(() => clientJwk(pems.cert), { message: /must hold an RSA key, got EC/ });
    });

    it('an unreadable path, unwrapped from node:fs', () => {
      assert.throws(() => clientJwk('/no/such/tw-client.cer'), { code: 'ENOENT' });
    });

    it('a malformed PEM, unwrapped from node:crypto', () => {
      assert.throws(() => clientJwk('-----BEGIN CERTIFICATE-----\nnope\n'), {
        code: 'ERR_OSSL_PEM_BAD_END_LINE',
      });
    });
  });
});
