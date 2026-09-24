import assert from 'node:assert/strict';
import { createPrivateKey, createPublicKey, X509Certificate } from 'node:crypto';
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, describe, it } from 'node:test';

import { createKeyPair, ensureKeyPair } from '../lib/certificate.ts';

const tempDirs: string[] = [];

after(() => {
  for (const dir of tempDirs) {
    rmSync(dir, { recursive: true, force: true });
  }
});

function tempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), 'mock-edupass-generate-'));
  tempDirs.push(dir);
  return dir;
}

describe('createKeyPair', () => {
  it('writes an RSA-2048 pair', async () => {
    const dir = tempDir();
    const { privateKeyPath, certificatePath } = await createKeyPair(dir);

    assert.equal(privateKeyPath, join(dir, 'private.key'));
    assert.equal(certificatePath, join(dir, 'public.cer'));

    const privateKeyPem = readFileSync(privateKeyPath, 'utf8');
    assert.match(privateKeyPem, /^-----BEGIN PRIVATE KEY-----/);

    const jwk = createPrivateKey(privateKeyPem).export({ format: 'jwk' });
    assert.equal(jwk.kty, 'RSA');
    assert.equal(Buffer.from(jwk.n ?? '', 'base64url').length, 256);

    const certificate = new X509Certificate(readFileSync(certificatePath, 'utf8'));
    assert.equal(certificate.subject, 'CN=teacher-workspace');
    assert.equal(
      certificate.publicKey.export({ type: 'spki', format: 'pem' }).toString(),
      createPublicKey(privateKeyPem).export({ type: 'spki', format: 'pem' }).toString(),
    );
  });

  it('writes a self-signed certificate for the key', async () => {
    const { privateKeyPath, certificatePath } = await createKeyPair(tempDir());
    const privateKey = createPrivateKey(readFileSync(privateKeyPath, 'utf8'));

    const certificate = new X509Certificate(readFileSync(certificatePath, 'utf8'));

    assert.ok(certificate.checkPrivateKey(privateKey), 'certificate should match the private key');
    assert.ok(
      certificate.verify(certificate.publicKey),
      'certificate should verify against itself',
    );
  });

  it('writes a certificate valid 365 days', async () => {
    const { certificatePath } = await createKeyPair(tempDir());

    const certificate = new X509Certificate(readFileSync(certificatePath, 'utf8'));

    assert.equal(
      Date.parse(certificate.validTo) - Date.parse(certificate.validFrom),
      365 * 24 * 60 * 60 * 1000,
    );
  });

  it('writes the private key 0600', async () => {
    const { privateKeyPath } = await createKeyPair(tempDir());

    assert.equal(statSync(privateKeyPath).mode & 0o777, 0o600);
  });

  it('tightens a loose private key mode', async () => {
    const dir = tempDir();
    const { privateKeyPath } = await createKeyPair(dir);
    chmodSync(privateKeyPath, 0o644);

    await createKeyPair(dir);

    assert.equal(statSync(privateKeyPath).mode & 0o777, 0o600);
  });

  it('replaces an existing pair', async () => {
    const dir = tempDir();
    const paths = await createKeyPair(dir);
    const first = readFileSync(paths.privateKeyPath, 'utf8');

    await createKeyPair(dir);

    assert.notEqual(readFileSync(paths.privateKeyPath, 'utf8'), first);
  });
});

describe('ensureKeyPair', () => {
  it('generates into a missing directory', async () => {
    const paths = await ensureKeyPair(join(tempDir(), 'nested', 'mock-edupass'));

    assert.ok(existsSync(paths.privateKeyPath), 'private key should exist');
    assert.ok(existsSync(paths.certificatePath), 'certificate should exist');
  });

  it('returns an existing pair unchanged', async () => {
    const dir = tempDir();
    const created = await createKeyPair(dir);
    const privateKeyPem = readFileSync(created.privateKeyPath, 'utf8');
    const certificatePem = readFileSync(created.certificatePath, 'utf8');
    const mtimeMs = statSync(created.privateKeyPath).mtimeMs;

    const ensured = await ensureKeyPair(dir);

    assert.deepEqual(ensured, created);
    assert.equal(readFileSync(ensured.privateKeyPath, 'utf8'), privateKeyPem);
    assert.equal(readFileSync(ensured.certificatePath, 'utf8'), certificatePem);
    assert.equal(statSync(ensured.privateKeyPath).mtimeMs, mtimeMs);
  });

  it('regenerates when the certificate is missing', async () => {
    const dir = tempDir();
    const created = await createKeyPair(dir);
    const privateKeyPem = readFileSync(created.privateKeyPath, 'utf8');
    rmSync(created.certificatePath);

    const ensured = await ensureKeyPair(dir);

    assert.ok(existsSync(ensured.certificatePath), 'certificate should exist');
    assert.notEqual(readFileSync(ensured.privateKeyPath, 'utf8'), privateKeyPem);
  });

  it('regenerates when the private key is missing', async () => {
    const dir = tempDir();
    const created = await createKeyPair(dir);
    const certificatePem = readFileSync(created.certificatePath, 'utf8');
    rmSync(created.privateKeyPath);

    const ensured = await ensureKeyPair(dir);

    assert.ok(existsSync(ensured.privateKeyPath), 'private key should exist');
    assert.notEqual(readFileSync(ensured.certificatePath, 'utf8'), certificatePem);
  });
});
