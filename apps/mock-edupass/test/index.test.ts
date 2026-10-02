import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, it } from 'node:test';

const INDEX = join(import.meta.dirname, '../src/index.ts');

describe('mock-edupass process', () => {
  it('exits with status 1 and an error when MOCK_EDUPASS_TW_AUTH_METHOD is unset', () => {
    const result = spawnSync(process.execPath, [INDEX], {
      env: {
        PATH: process.env.PATH,
        MOCK_EDUPASS_URL: 'http://localhost:9000',
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
      },
      encoding: 'utf8',
    });

    assert.equal(result.status, 1);
    assert.match(result.stderr, /MOCK_EDUPASS_TW_AUTH_METHOD/);
  });

  it('starts with the client secret from MOCK_EDUPASS_TW_SECRET_FILE', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'mock-edupass-index-'));
    const file = join(dir, 'client-secret');
    writeFileSync(file, 'secret\n');
    const port = 9878;

    const child = spawn(process.execPath, [INDEX], {
      env: {
        PATH: process.env.PATH,
        MOCK_EDUPASS_PORT: String(port),
        MOCK_EDUPASS_URL: `http://localhost:${port}`,
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET_FILE: file,
      },
    });

    try {
      await new Promise<void>((resolve, reject) => {
        child.stdout.on('data', (chunk: Buffer) => {
          if (chunk.toString().includes('listening')) {
            resolve();
          }
        });
        child.once('exit', (code) => reject(new Error(`exited with status ${code}`)));
      });

      const res = await globalThis.fetch(`http://localhost:${port}/health`);
      assert.equal(res.status, 200);
    } finally {
      child.kill();
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
