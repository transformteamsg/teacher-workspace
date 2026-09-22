import { chmodSync, existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

import { generate } from 'selfsigned';

interface KeyPair {
  privateKeyPath: string;
  certificatePath: string;
}

const VALIDITY_MS = 365 * 24 * 60 * 60 * 1000;

/**
 * Writes an RSA-2048 private key and a self-signed certificate, replacing any pair already there.
 *
 * `algorithm` is named because `selfsigned` still defaults to SHA-1. Both `mode` options are
 * followed by a `chmodSync` because `mode` is ignored when the entry already exists.
 *
 * @param dir - Directory to write into, created `0700` when missing.
 * @returns The `private.key` and `public.cer` paths just written.
 * @throws When generation, directory creation, or either write fails.
 */
export async function createKeyPair(dir: string): Promise<KeyPair> {
  const privateKeyPath = join(dir, 'private.key');
  const certificatePath = join(dir, 'public.cer');

  mkdirSync(dir, { recursive: true, mode: 0o700 });
  chmodSync(dir, 0o700);

  const notBeforeDate = new Date();
  const notAfterDate = new Date(notBeforeDate.getTime() + VALIDITY_MS);
  const pems = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
    keyType: 'rsa',
    keySize: 2048,
    algorithm: 'sha256',
    notBeforeDate,
    notAfterDate,
  });

  writeFileSync(privateKeyPath, pems.private, { mode: 0o600 });
  chmodSync(privateKeyPath, 0o600);
  writeFileSync(certificatePath, `${pems.cert}\n`);

  return { privateKeyPath, certificatePath };
}

/**
 * Returns the key pair in a directory, generating one only when either file is missing.
 *
 * A pair found intact is returned as is, so its permissions are never re-tightened.
 *
 * @param dir - Directory the pair lives in.
 * @returns The paths of the private key and the certificate.
 * @throws When a pair has to be generated and that generation or write fails.
 */
export async function ensureKeyPair(dir: string): Promise<KeyPair> {
  const privateKeyPath = join(dir, 'private.key');
  const certificatePath = join(dir, 'public.cer');

  if (existsSync(privateKeyPath) && existsSync(certificatePath)) {
    return { privateKeyPath, certificatePath };
  }

  return createKeyPair(dir);
}
