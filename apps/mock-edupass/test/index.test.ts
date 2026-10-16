import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { join } from 'node:path';
import { describe, it } from 'node:test';

const INDEX = join(import.meta.dirname, '../src/index.ts');

describe('mock-edupass process', () => {
  it('exits with status 0 on SIGTERM', async () => {
    const port = 9879;

    const child = spawn(process.execPath, [INDEX], {
      env: {
        PATH: process.env.PATH,
        MOCK_EDUPASS_PORT: String(port),
        MOCK_EDUPASS_URL: `http://localhost:${port}`,
        MOCK_EDUPASS_TW_ID: 'teacher-workspace',
        MOCK_EDUPASS_TW_REDIRECT_URI: 'http://localhost:3000/auth/edupass/callback',
        MOCK_EDUPASS_TW_AUTH_METHOD: 'client_secret_post',
        MOCK_EDUPASS_TW_SECRET: 'secret',
      },
    });

    await new Promise<void>((resolve, reject) => {
      child.stdout.on('data', (chunk: Buffer) => {
        if (chunk.toString().includes('listening')) {
          resolve();
        }
      });
      child.once('exit', (code) => reject(new Error(`exited with status ${code}`)));
    });

    const exited = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>(
      (resolve) => {
        child.once('exit', (code, signal) => resolve({ code, signal }));
      },
    );
    child.kill('SIGTERM');

    assert.deepEqual(await exited, { code: 0, signal: null });
  });
});
