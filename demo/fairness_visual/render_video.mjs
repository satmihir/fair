#!/usr/bin/env node

import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import net from "node:net";

const __dirname = dirname(fileURLToPath(import.meta.url));
const chromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const demoPath = resolve(__dirname, "index.html");
const outputPath = resolve(__dirname, "fair-demo.mp4");
const framesDir = resolve(tmpdir(), "fairness-demo-frames");
const width = Number(process.env.FAIR_DEMO_WIDTH || 1920);
const height = Number(process.env.FAIR_DEMO_HEIGHT || 1080);
const fps = Number(process.env.FAIR_DEMO_FPS || 16);
const durationSeconds = Number(process.env.FAIR_DEMO_DURATION || 28);
const frameCount = Math.round(fps * durationSeconds);

if (!existsSync(chromePath)) {
  throw new Error(`Chrome not found at ${chromePath}`);
}

function getFreePort() {
  return new Promise((resolvePort, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => resolvePort(address.port));
    });
  });
}

function sleep(ms) {
  return new Promise((resolveSleep) => setTimeout(resolveSleep, ms));
}

async function waitForJSON(url, timeoutMs = 10000) {
  const start = Date.now();
  let lastError;
  while (Date.now() - start < timeoutMs) {
    try {
      const response = await fetch(url);
      if (response.ok) return response.json();
      lastError = new Error(`${response.status} ${response.statusText}`);
    } catch (error) {
      lastError = error;
    }
    await sleep(100);
  }
  throw new Error(`Timed out waiting for ${url}: ${lastError?.message || "unknown error"}`);
}

function connectCDP(webSocketURL) {
  const socket = new WebSocket(webSocketURL);
  let nextID = 1;
  const pending = new Map();
  const eventHandlers = new Map();

  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (message.id && pending.has(message.id)) {
      const { resolve: resolveMessage, reject } = pending.get(message.id);
      pending.delete(message.id);
      if (message.error) reject(new Error(message.error.message));
      else resolveMessage(message.result || {});
      return;
    }

    const handlers = eventHandlers.get(message.method) || [];
    handlers.forEach((handler) => handler(message.params || {}));
  });

  function send(method, params = {}) {
    const id = nextID++;
    socket.send(JSON.stringify({ id, method, params }));
    return new Promise((resolveMessage, reject) => {
      pending.set(id, { resolve: resolveMessage, reject });
      setTimeout(() => {
        if (!pending.has(id)) return;
        pending.delete(id);
        reject(new Error(`CDP command timed out: ${method}`));
      }, 15000);
    });
  }

  function once(method) {
    return new Promise((resolveEvent) => {
      const handler = (params) => {
        const handlers = eventHandlers.get(method) || [];
        eventHandlers.set(method, handlers.filter((candidate) => candidate !== handler));
        resolveEvent(params);
      };
      eventHandlers.set(method, [...(eventHandlers.get(method) || []), handler]);
    });
  }

  return new Promise((resolveSocket, reject) => {
    socket.addEventListener("open", () => resolveSocket({ send, once, close: () => socket.close() }));
    socket.addEventListener("error", reject);
  });
}

function run(command, args) {
  return new Promise((resolveRun, reject) => {
    const child = spawn(command, args, { stdio: "inherit" });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) resolveRun();
      else reject(new Error(`${command} exited with ${code}`));
    });
  });
}

async function stopChrome(chrome) {
  if (chrome.exitCode !== null) return;

  chrome.kill("SIGTERM");
  await new Promise((resolveStop) => {
    const timeout = setTimeout(resolveStop, 3000);
    chrome.once("exit", () => {
      clearTimeout(timeout);
      resolveStop();
    });
  });
}

function removeQuietly(path) {
  try {
    rmSync(path, { force: true, recursive: true, maxRetries: 5, retryDelay: 200 });
  } catch (error) {
    console.warn(`Warning: could not remove ${path}: ${error.message}`);
  }
}

function frameName(index) {
  return resolve(framesDir, `frame-${String(index).padStart(4, "0")}.png`);
}

async function main() {
  const port = await getFreePort();
  const profileDir = mkdtempSync(resolve(tmpdir(), "fair-demo-chrome-"));
  const demoURL = `${pathToFileURL(demoPath).href}?recording=1`;

  rmSync(framesDir, { force: true, recursive: true });
  mkdirSync(framesDir, { recursive: true });

  const chrome = spawn(chromePath, [
    "--headless=new",
    "--disable-gpu",
    "--disable-background-networking",
    "--disable-component-update",
    "--disable-default-apps",
    "--disable-sync",
    "--hide-scrollbars",
    "--no-default-browser-check",
    "--no-first-run",
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${profileDir}`,
    demoURL
  ], { stdio: ["ignore", "ignore", "inherit"] });

  try {
    const targetsURL = `http://127.0.0.1:${port}/json/list`;
    const targets = await waitForJSON(targetsURL);
    const page = targets.find((target) => target.type === "page") || targets[0];
    if (!page?.webSocketDebuggerUrl) {
      throw new Error("Could not find a Chrome page target");
    }

    const cdp = await connectCDP(page.webSocketDebuggerUrl);
    await cdp.send("Page.enable");
    await cdp.send("Runtime.enable");
    await cdp.send("Emulation.setDeviceMetricsOverride", {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: false
    });

    const loaded = cdp.once("Page.loadEventFired");
    await cdp.send("Page.navigate", { url: demoURL });
    await loaded;
    await cdp.send("Runtime.evaluate", {
      expression: "document.fonts ? document.fonts.ready : Promise.resolve()",
      awaitPromise: true
    });

    console.log(`Rendering ${frameCount} frames at ${width}x${height}, ${fps} fps...`);
    for (let frame = 0; frame < frameCount; frame++) {
      const time = frame / fps;
      await cdp.send("Runtime.evaluate", {
        expression: `window.renderFairDemoFrame(${time.toFixed(5)})`,
        awaitPromise: true
      });
      const screenshot = await cdp.send("Page.captureScreenshot", {
        format: "png",
        fromSurface: true
      });
      writeFileSync(frameName(frame), Buffer.from(screenshot.data, "base64"));
      if ((frame + 1) % fps === 0 || frame + 1 === frameCount) {
        process.stdout.write(`\r${Math.round(((frame + 1) / frameCount) * 100)}%`);
      }
    }
    process.stdout.write("\n");

    cdp.close();
  } finally {
    await stopChrome(chrome);
    removeQuietly(profileDir);
  }

  await run("ffmpeg", [
    "-y",
    "-framerate", String(fps),
    "-i", resolve(framesDir, "frame-%04d.png"),
    "-c:v", "libx264",
    "-pix_fmt", "yuv420p",
    "-movflags", "+faststart",
    outputPath
  ]);

  console.log(`Wrote ${outputPath}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
