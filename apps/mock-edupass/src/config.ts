import { X509Certificate } from 'node:crypto';
import { readFileSync } from 'node:fs';

export type ClientAuth =
  | { method: 'client_secret_post'; clientSecret: string }
  | { method: 'private_key_jwt'; certificate: X509Certificate };

export interface Client {
  id: string;
  redirectUri: string;
  auth: ClientAuth;
}

export interface Config {
  port: number;
  url: string;
  tw: Client;
}

/**
 * Reads the mock-edupass configuration from the environment.
 *
 * @param env - Environment to read, usually `process.env`.
 * @returns Port, URL, and the tw configuration.
 * @throws When a setting is missing, conflicting, unreadable, or invalid.
 */
export function loadConfig(env: NodeJS.ProcessEnv): Config {
  const port = Number(env.MOCK_EDUPASS_PORT) || 9000;

  if (!env.MOCK_EDUPASS_URL) {
    throw new Error('MOCK_EDUPASS_URL is required');
  }
  if (!env.MOCK_EDUPASS_TW_ID) {
    throw new Error('MOCK_EDUPASS_TW_ID is required');
  }
  if (!env.MOCK_EDUPASS_TW_REDIRECT_URI) {
    throw new Error('MOCK_EDUPASS_TW_REDIRECT_URI is required');
  }

  switch (env.MOCK_EDUPASS_TW_AUTH_METHOD) {
    case 'client_secret_post': {
      if (env.MOCK_EDUPASS_TW_SECRET && env.MOCK_EDUPASS_TW_SECRET_FILE) {
        throw new Error(
          'MOCK_EDUPASS_TW_SECRET and MOCK_EDUPASS_TW_SECRET_FILE are both set: set only one',
        );
      }

      let clientSecret = env.MOCK_EDUPASS_TW_SECRET;
      if (env.MOCK_EDUPASS_TW_SECRET_FILE) {
        try {
          clientSecret = readFileSync(env.MOCK_EDUPASS_TW_SECRET_FILE, 'utf8').replace(
            /[\r\n]+$/,
            '',
          );
        } catch (err) {
          throw new Error('MOCK_EDUPASS_TW_SECRET_FILE cannot be read', { cause: err });
        }
      }

      if (!clientSecret) {
        throw new Error(
          'MOCK_EDUPASS_TW_SECRET or MOCK_EDUPASS_TW_SECRET_FILE is required for client_secret_post',
        );
      }

      return {
        port,
        url: env.MOCK_EDUPASS_URL,
        tw: {
          id: env.MOCK_EDUPASS_TW_ID,
          redirectUri: env.MOCK_EDUPASS_TW_REDIRECT_URI,
          auth: { method: env.MOCK_EDUPASS_TW_AUTH_METHOD, clientSecret },
        },
      };
    }

    case 'private_key_jwt': {
      if (env.MOCK_EDUPASS_TW_CERTIFICATE && env.MOCK_EDUPASS_TW_CERTIFICATE_FILE) {
        throw new Error(
          'MOCK_EDUPASS_TW_CERTIFICATE and MOCK_EDUPASS_TW_CERTIFICATE_FILE are both set: set only one',
        );
      }

      let pem = env.MOCK_EDUPASS_TW_CERTIFICATE;
      if (env.MOCK_EDUPASS_TW_CERTIFICATE_FILE) {
        try {
          pem = readFileSync(env.MOCK_EDUPASS_TW_CERTIFICATE_FILE, 'utf8');
        } catch (err) {
          throw new Error('MOCK_EDUPASS_TW_CERTIFICATE_FILE cannot be read', { cause: err });
        }
      }

      if (!pem) {
        throw new Error(
          'MOCK_EDUPASS_TW_CERTIFICATE or MOCK_EDUPASS_TW_CERTIFICATE_FILE is required for private_key_jwt',
        );
      }

      if (pem.includes('PRIVATE KEY')) {
        throw new Error(
          'MOCK_EDUPASS_TW_CERTIFICATE holds a private key: expected the X.509 certificate',
        );
      }

      let certificate: X509Certificate;
      try {
        certificate = new X509Certificate(pem);
      } catch (err) {
        throw new Error('MOCK_EDUPASS_TW_CERTIFICATE is not an X.509 certificate', {
          cause: err,
        });
      }

      if (certificate.publicKey.asymmetricKeyType !== 'rsa') {
        throw new Error(
          `MOCK_EDUPASS_TW_CERTIFICATE must hold an RSA public key, got ${certificate.publicKey.asymmetricKeyType}`,
        );
      }

      const now = new Date();
      if (now < certificate.validFromDate || now > certificate.validToDate) {
        throw new Error(
          `MOCK_EDUPASS_TW_CERTIFICATE is not valid at ${now.toISOString()}: valid from ${certificate.validFrom} to ${certificate.validTo}`,
        );
      }

      return {
        port,
        url: env.MOCK_EDUPASS_URL,
        tw: {
          id: env.MOCK_EDUPASS_TW_ID,
          redirectUri: env.MOCK_EDUPASS_TW_REDIRECT_URI,
          auth: { method: env.MOCK_EDUPASS_TW_AUTH_METHOD, certificate },
        },
      };
    }

    default:
      throw new Error(
        `MOCK_EDUPASS_TW_AUTH_METHOD must be client_secret_post or private_key_jwt, got ${JSON.stringify(env.MOCK_EDUPASS_TW_AUTH_METHOD)}`,
      );
  }
}
