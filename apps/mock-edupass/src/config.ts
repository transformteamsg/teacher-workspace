export const config = {
  port: Number(process.env.MOCK_EDUPASS_PORT) || 9000,
  clientPublicKey: process.env.MOCK_EDUPASS_TW_PUBLIC_KEY ?? '',
};
