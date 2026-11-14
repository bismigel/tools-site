import { defineConfig, devices } from "@playwright/test";
import { execSync } from "node:child_process";

// Use the system Chromium when there is one (NixOS: the Playwright download has
// no way to satisfy its host-library check), otherwise the bundled browser (CI).
function systemChromium(): string | undefined {
  if (process.env.PLAYWRIGHT_CHROMIUM) return process.env.PLAYWRIGHT_CHROMIUM;
  for (const bin of ["chromium", "chromium-browser", "google-chrome"]) {
    try {
      const out = execSync(`command -v ${bin}`, { stdio: ["ignore", "pipe", "ignore"] })
        .toString()
        .trim();
      if (out) return out;
    } catch {
      /* not installed */
    }
  }
  return undefined;
}

const port = Number(process.env.E2E_PORT || 8092);
const chrome = systemChromium();

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: {
      // Rootless containers and NixOS both need this.
      args: ["--no-sandbox"],
      ...(chrome ? { executablePath: chrome } : {}),
    },
  },
  projects: [
    {
      name: "desktop",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 900 } },
    },
    {
      name: "mobile",
      use: { ...devices["Pixel 5"] },
    },
  ],
  webServer: {
    // A production-mode binary: embedded templates, no dev routes, no SSE.
    command: `make build && ./bin/toolsd -addr=:${port}`,
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: false,
    timeout: 60_000,
    env: { TZ: "UTC" },
  },
});
