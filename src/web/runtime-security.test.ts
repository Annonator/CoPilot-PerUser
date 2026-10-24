// @vitest-environment node

import { spawnSync } from "node:child_process";
import { createServer } from "node:http";
import { brotliCompressSync, brotliDecompressSync, createBrotliDecompress } from "node:zlib";
import { describe, expect, it } from "vitest";

describe("supported Node.js runtime", () => {
  it.each([
    ["20.20.0", false], ["22.23.3", false], ["24.22.0", false],
    ["25.9.0", false], ["26.8.1", false], ["26.12.0", false],
    ["22.23.4", true], ["24.22.1", true], ["26.12.1", true],
    ["26.13.0", true], ["27.0.0", false], ["26.12.1-rc.1", false]
  ])("checks Node %s before starting Next.js", (version, supported) => {
    const result = spawnSync(process.execPath, ["--input-type=module", "-e", `
      Object.defineProperty(process.versions, "node", { value: ${JSON.stringify(version)} });
      await import("./scripts/check-node-version.mjs");
    `], { encoding: "utf8", timeout: 3000 });
    expect(result.error).toBeUndefined();
    expect(result.status).toBe(supported ? 0 : 1);
    if (!supported) expect(result.stderr).toContain("Node2Shell");
  });
});

describe("Node2Shell Brotli regression", () => {
  // Node.js upstream regression input for CVE-2026-111497:
  // https://github.com/nodejs/node/commit/0392385841
  const invalidBlockSwitch = Buffer.from("200010414141fcffff07002228f00b60f7f41f000000", "hex");
  const expectedError = { code: "ERR__ERROR_FORMAT_BLOCK_SWITCH", errno: -17 };

  it("permanently rejects an invalid block switch", () => {
    expect(() => brotliDecompressSync(invalidBlockSwitch)).toThrow(
      expect.objectContaining(expectedError)
    );
  });

  it("rejects the same malformed data split into one-byte chunks", async () => {
    const decoder = createBrotliDecompress();
    const failure = new Promise((resolve, reject) => {
      decoder.once("error", resolve);
      decoder.once("end", () => reject(new Error("Malformed Brotli input was accepted")));
    });
    decoder.resume();
    for (const byte of invalidBlockSwitch) decoder.write(Buffer.from([byte]));
    decoder.end();
    await expect(failure).resolves.toMatchObject(expectedError);
  });

  it("preserves valid Brotli decoding through native fetch", async () => {
    const payload = { status: "ok", includedCredits: 100 };
    const compressed = brotliCompressSync(Buffer.from(JSON.stringify(payload)));
    expect(JSON.parse(brotliDecompressSync(compressed).toString())).toEqual(payload);
    const server = createServer((_request, response) => {
      response.writeHead(200, { "Content-Encoding": "br", "Content-Type": "application/json" });
      response.end(compressed);
    });
    try {
      await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
      const address = server.address();
      if (!address || typeof address === "string") throw new Error("Missing test server address");
      const response = await fetch(`http://127.0.0.1:${address.port}`);
      await expect(response.json()).resolves.toEqual(payload);
    } finally {
      server.closeAllConnections();
      await new Promise<void>((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
    }
  });
});
