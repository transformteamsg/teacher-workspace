import assert from 'node:assert/strict';
import { X509Certificate } from 'node:crypto';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, before, describe, it } from 'node:test';

import { generate } from 'selfsigned';

import { loadConfig } from '../src/config.ts';

let dir: string;
let rsaCertificate: string;
let ecCertificate: string;
let expiredCertificate: string;
let rsaPrivateKey: string;

before(async () => {
  dir = mkdtempSync(join(tmpdir(), 'mock-edupass-config-'));

  const rsa = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
    keyType: 'rsa',
    keySize: 2048,
    algorithm: 'sha256',
  });
  const ec = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
    keyType: 'ec',
    algorithm: 'sha256',
  });
  const expired = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
    keyType: 'rsa',
    keySize: 2048,
    algorithm: 'sha256',
    notBeforeDate: new Date('2020-01-01T00:00:00Z'),
    notAfterDate: new Date('2020-01-02T00:00:00Z'),
  });

  rsaCertificate = rsa.cert;
  ecCertificate = ec.cert;
  expiredCertificate = expired.cert;
  rsaPrivateKey = rsa.private;
  writeFileSync(join(dir, 'rsa.cer'), rsa.cert);
});

after(() => {
  rmSync(dir, { recursive: true, force: true });
});

describe('loadConfig', () => {
  describe('port', () => {
    it('defaults to 9000', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.equal(config.port, 9000);
    });

    it('reads MOCK_EDUPASS_PORT', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_PORT: '9100',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.equal(config.port, 9100);
    });
  });

  describe('URL', () => {
    it('throws when MOCK_EDUPASS_URL is unset or empty', () => {
      for (const url of [undefined, '']) {
        assert.throws(
          () =>
            loadConfig({
              MOCK_EDUPASS_URL: url,
              MOCK_EDUPASS_TW_ID: 'teacher-workspace',
              MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
              MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
              MOCK_EDUPASS_TW_SECRET: 'secret',
            }),
          /MOCK_EDUPASS_URL is required/,
        );
      }
    });

    it('reads MOCK_EDUPASS_URL', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'https://mock-edupass.example.com/edupass',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.equal(config.url, 'https://mock-edupass.example.com/edupass');
    });
  });

  describe('client ID', () => {
    it('throws when MOCK_EDUPASS_TW_ID is unset or empty', () => {
      for (const id of [undefined, '']) {
        assert.throws(
          () =>
            loadConfig({
              MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
              MOCK_EDUPASS_URL: 'http://localhost:9000',
              MOCK_EDUPASS_TW_ID: id,
              MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
              MOCK_EDUPASS_TW_SECRET: 'secret',
            }),
          /MOCK_EDUPASS_TW_ID is required/,
        );
      }
    });

    it('reads MOCK_EDUPASS_TW_ID', () => {
      const config = loadConfig({
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace-staging',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.equal(config.tw.id, 'teacher-workspace-staging');
    });
  });

  describe('redirect URI', () => {
    it('throws when MOCK_EDUPASS_TW_REDIRECT_URI is unset or empty', () => {
      for (const redirectUri of [undefined, '']) {
        assert.throws(
          () =>
            loadConfig({
              MOCK_EDUPASS_URL: 'http://localhost:9000',
              MOCK_EDUPASS_TW_ID: 'teacher-workspace',
              MOCK_EDUPASS_TW_REDIRECT_URI: redirectUri,
              MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
              MOCK_EDUPASS_TW_SECRET: 'secret',
            }),
          /MOCK_EDUPASS_TW_REDIRECT_URI is required/,
        );
      }
    });

    it('reads MOCK_EDUPASS_TW_REDIRECT_URI', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3100/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.equal(config.tw.redirectUri, 'http://localhost:3100/auth/edupass/callback');
    });
  });

  describe('client authentication method', () => {
    it('throws when MOCK_EDUPASS_TW_AUTH_METHOD is unset', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
          }),
        /MOCK_EDUPASS_TW_AUTH_METHOD/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_AUTH_METHOD is empty', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: '',
          }),
        /MOCK_EDUPASS_TW_AUTH_METHOD/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_AUTH_METHOD is unknown', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_basic',
          }),
        /MOCK_EDUPASS_TW_AUTH_METHOD/,
      );
    });
  });

  describe('client_secret_post', () => {
    it('returns the client secret from MOCK_EDUPASS_TW_SECRET', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      });

      assert.deepEqual(config.tw.auth, { method: 'client_secret_post', clientSecret: 'secret' });
    });

    it('returns the client secret from MOCK_EDUPASS_TW_SECRET_FILE without trailing newlines', () => {
      const file = join(dir, 'client-secret');
      writeFileSync(file, 'secret\r\n\n');

      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET_FILE: file,
      });

      assert.deepEqual(config.tw.auth, { method: 'client_secret_post', clientSecret: 'secret' });
    });

    it('throws when the client secret is unset or empty', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
            MOCK_EDUPASS_TW_SECRET: '',
          }),
        /MOCK_EDUPASS_TW_SECRET or MOCK_EDUPASS_TW_SECRET_FILE is required/,
      );
    });

    it('throws when both MOCK_EDUPASS_TW_SECRET and MOCK_EDUPASS_TW_SECRET_FILE are set', () => {
      const file = join(dir, 'client-secret-both');
      writeFileSync(file, 'secret');

      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
            MOCK_EDUPASS_TW_SECRET: 'secret',
            MOCK_EDUPASS_TW_SECRET_FILE: file,
          }),
        /both set/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_SECRET_FILE cannot be read', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
            MOCK_EDUPASS_TW_SECRET_FILE: join(dir, 'missing'),
          }),
        /MOCK_EDUPASS_TW_SECRET_FILE cannot be read/,
      );
    });

    it('throws when neither MOCK_EDUPASS_TW_SECRET nor MOCK_EDUPASS_TW_SECRET_FILE is set', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
          }),
        /MOCK_EDUPASS_TW_SECRET or MOCK_EDUPASS_TW_SECRET_FILE is required/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_SECRET_FILE holds only newlines', () => {
      const file = join(dir, 'client-secret-newlines');
      writeFileSync(file, '\n\r\n');

      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
            MOCK_EDUPASS_TW_SECRET_FILE: file,
          }),
        /MOCK_EDUPASS_TW_SECRET or MOCK_EDUPASS_TW_SECRET_FILE is required/,
      );
    });

    it('does not need a certificate', () => {
      assert.doesNotThrow(() =>
        loadConfig({
          MOCK_EDUPASS_URL: 'http://localhost:9000',
          MOCK_EDUPASS_TW_ID: 'teacher-workspace',
          MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
          MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
          MOCK_EDUPASS_TW_SECRET: 'secret',
        }),
      );
    });
  });

  describe('private_key_jwt', () => {
    it('returns the certificate from MOCK_EDUPASS_TW_CERTIFICATE', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
        MOCK_EDUPASS_TW_CERTIFICATE: rsaCertificate,
      });

      assert.equal(config.tw.auth.method, 'private_key_jwt');
      assert.equal(
        config.tw.auth.method === 'private_key_jwt' && config.tw.auth.certificate.fingerprint256,
        new X509Certificate(rsaCertificate).fingerprint256,
      );
    });

    it('returns the certificate from MOCK_EDUPASS_TW_CERTIFICATE_FILE', () => {
      const config = loadConfig({
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
        MOCK_EDUPASS_TW_CERTIFICATE_FILE: join(dir, 'rsa.cer'),
      });

      assert.equal(config.tw.auth.method, 'private_key_jwt');
    });

    it('throws when the certificate is unset or empty', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
          }),
        /MOCK_EDUPASS_TW_CERTIFICATE or MOCK_EDUPASS_TW_CERTIFICATE_FILE is required/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_CERTIFICATE is empty', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE: '',
          }),
        /MOCK_EDUPASS_TW_CERTIFICATE or MOCK_EDUPASS_TW_CERTIFICATE_FILE is required/,
      );
    });

    it('throws when both MOCK_EDUPASS_TW_CERTIFICATE and MOCK_EDUPASS_TW_CERTIFICATE_FILE are set', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE: rsaCertificate,
            MOCK_EDUPASS_TW_CERTIFICATE_FILE: join(dir, 'rsa.cer'),
          }),
        /both set/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_CERTIFICATE_FILE cannot be read', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE_FILE: join(dir, 'missing.cer'),
          }),
        /MOCK_EDUPASS_TW_CERTIFICATE_FILE cannot be read/,
      );
    });

    it('throws when the certificate has expired', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE: expiredCertificate,
          }),
        /not valid at/,
      );
    });

    it('throws when the certificate does not hold an RSA public key', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE: ecCertificate,
          }),
        /RSA public key/,
      );
    });

    it('throws when MOCK_EDUPASS_TW_CERTIFICATE holds a private key', () => {
      assert.throws(
        () =>
          loadConfig({
            MOCK_EDUPASS_URL: 'http://localhost:9000',
            MOCK_EDUPASS_TW_ID: 'teacher-workspace',
            MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
            MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
            MOCK_EDUPASS_TW_CERTIFICATE: rsaPrivateKey,
          }),
        /holds a private key/,
      );
    });

    it('does not need a client secret', () => {
      assert.doesNotThrow(() =>
        loadConfig({
          MOCK_EDUPASS_URL: 'http://localhost:9000',
          MOCK_EDUPASS_TW_ID: 'teacher-workspace',
          MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
          MOCK_EDUPASS_TW_AUTH_METHOD: 'private_key_jwt',
          MOCK_EDUPASS_TW_CERTIFICATE: rsaCertificate,
        }),
      );
    });
  });
});
