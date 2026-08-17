// ok-pi-bridge-v1
/**
 * Open Knowledge bridge for Pi — MANAGED FILE, written by `ok init`.
 * Hand edits are overwritten whenever OK re-syncs this project; remove it
 * with `ok deinit` (or delete the file) to disconnect Pi from OK.
 *
 * On session start it spawns Open Knowledge's MCP stdio server via OK's
 * resilient launcher (bundle, then npx, then version-manager probes), performs
 * the MCP handshake, and registers each MCP tool as a Pi tool under an
 * `ok_` prefix (Pi has no MCP namespacing; the prefix keeps OK's `edit` /
 * `write` from shadowing Pi's built-in tools).
 */
import { spawn } from "node:child_process";
import type { ChildProcessWithoutNullStreams } from "node:child_process";

const LAUNCHERS = {
  "unix": {
    "command": "/bin/sh",
    "args": [
      "-l",
      "-c",
      "# ok-mcp-v2\nUSER_BUNDLE=\"$HOME/Applications/OpenKnowledge.app/Contents/Resources/cli/bin/ok.sh\"\n[ -f \"$USER_BUNDLE\" ] && [ -x \"$USER_BUNDLE\" ] && exec \"$USER_BUNDLE\" mcp\nBUNDLE=\"/Applications/OpenKnowledge.app/Contents/Resources/cli/bin/ok.sh\"\n[ -f \"$BUNDLE\" ] && [ -x \"$BUNDLE\" ] && exec \"$BUNDLE\" mcp\nDEB_BUNDLE=\"/opt/OpenKnowledge/resources/cli/bin/ok.sh\"\n[ -f \"$DEB_BUNDLE\" ] && [ -x \"$DEB_BUNDLE\" ] && exec \"$DEB_BUNDLE\" mcp\ncommand -v npx >/dev/null 2>&1 && exec npx -y @inkeep/open-knowledge@latest mcp\nfor d in \"$HOME/.nvm/versions/node\"/*/bin \"$HOME/.fnm/node-versions\"/*/installation/bin \"$HOME/.asdf/installs/nodejs\"/*/bin /opt/homebrew/bin /usr/local/bin \"$HOME/.local/bin\" \"$HOME/.volta/bin\"; do\n  [ -f \"$d/npx\" ] && [ -x \"$d/npx\" ] && exec \"$d/npx\" -y @inkeep/open-knowledge@latest mcp\ndone\necho \"OpenKnowledge: install OK Desktop or Node.js 24+, then restart your editor\" >&2\nexit 127"
    ]
  },
  "win32": {
    "command": "powershell",
    "args": [
      "-NoProfile",
      "-NonInteractive",
      "-Command",
      "# ok-mcp-win-v1\nif ($env:PATHEXT -notmatch 'CMD') { $env:PATHEXT = '.COM;.EXE;.BAT;.CMD;' + $env:PATHEXT }\nif ($env:APPDATA) {\n  $shim = Join-Path $env:APPDATA 'npm\\ok.cmd'\n  if (Test-Path -LiteralPath $shim -PathType Leaf) { & $shim mcp; exit $LASTEXITCODE }\n}\n$ok = Get-Command ok.cmd -CommandType Application -ErrorAction SilentlyContinue\nif ($ok) { & $ok.Source mcp; exit $LASTEXITCODE }\n$npx = Get-Command npx.cmd -CommandType Application -ErrorAction SilentlyContinue\nif ($npx) { & $npx.Source -y '@inkeep/open-knowledge@latest' mcp; exit $LASTEXITCODE }\n$dirs = @()\nif ($env:ProgramFiles) { $dirs += Join-Path $env:ProgramFiles 'nodejs' }\nif ($env:NVM_SYMLINK) { $dirs += $env:NVM_SYMLINK }\nif ($env:LOCALAPPDATA) {\n  $dirs += Join-Path $env:LOCALAPPDATA 'fnm\\aliases\\default'\n  $dirs += Join-Path $env:LOCALAPPDATA 'Volta\\bin'\n  $dirs += Join-Path $env:LOCALAPPDATA 'pnpm'\n}\nif ($env:USERPROFILE) { $dirs += Join-Path $env:USERPROFILE 'scoop\\shims' }\nforeach ($d in $dirs) {\n  $probe = Join-Path $d 'npx.cmd'\n  if (Test-Path -LiteralPath $probe -PathType Leaf) { & $probe -y '@inkeep/open-knowledge@latest' mcp; exit $LASTEXITCODE }\n}\n[Console]::Error.WriteLine('OpenKnowledge: install Node.js 24+ (npm i -g @inkeep/open-knowledge), then restart your editor')\nexit 127"
    ]
  }
} as const;

const TOOL_PREFIX = "ok_";
// First contact may cold-install the CLI through npx — allow a generous window.
const INIT_TIMEOUT_MS = 120000;
const STDERR_TAIL_LIMIT = 2000;

interface McpContentItem {
  type: string;
  text?: string;
}

interface McpToolInfo {
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

interface PendingRequest {
  resolve: (value: Record<string, unknown>) => void;
  reject: (err: Error) => void;
}

class OkMcpClient {
  private child: ChildProcessWithoutNullStreams;
  private pending = new Map<number, PendingRequest>();
  private nextId = 1;
  private buffer = "";
  private stderrTail = "";
  alive = true;

  constructor(cwd: string, onExit: () => void) {
    const launcher = process.platform === "win32" ? LAUNCHERS.win32 : LAUNCHERS.unix;
    const env: Record<string, string | undefined> = { ...process.env };
    const launcherEnv = (launcher as { env?: Record<string, string> }).env;
    if (launcherEnv) Object.assign(env, launcherEnv);
    this.child = spawn(launcher.command, [...launcher.args], {
      cwd,
      env,
      stdio: ["pipe", "pipe", "pipe"],
      // Inline (not the withHiddenWindowsConsole helper): this spawn lives in the
      // GENERATED Pi extension source, which must stay dependency-free (Pi loads
      // it with jiti; no npm packages resolve). Hide the console on Windows anyway.
      windowsHide: true,
    });
    this.child.stdout.setEncoding("utf8");
    this.child.stdout.on("data", (chunk: string) => this.onStdout(chunk));
    this.child.stderr.setEncoding("utf8");
    this.child.stderr.on("data", (chunk: string) => {
      this.stderrTail = (this.stderrTail + chunk).slice(-STDERR_TAIL_LIMIT);
    });
    // A write buffered before the pipe breaks surfaces as an 'error' on stdin
    // (not on the child); unhandled it would crash Pi's extension host.
    // Swallow it — the exit handler owns teardown.
    this.child.stdin.on("error", () => {});
    const fail = () => {
      if (!this.alive) return;
      this.alive = false;
      const err = new Error(
        "Open Knowledge MCP server exited" +
          (this.stderrTail ? ": " + this.stderrTail.trim() : ""),
      );
      for (const p of this.pending.values()) p.reject(err);
      this.pending.clear();
      onExit();
    };
    this.child.on("exit", fail);
    this.child.on("error", fail);
  }

  private onStdout(chunk: string): void {
    this.buffer += chunk;
    let newline = this.buffer.indexOf("\n");
    while (newline !== -1) {
      const line = this.buffer.slice(0, newline).trim();
      this.buffer = this.buffer.slice(newline + 1);
      if (line.startsWith("{")) {
        try {
          this.onMessage(JSON.parse(line) as Record<string, unknown>);
        } catch {
          // Non-JSON noise on stdout is ignored; framing recovers on the next line.
        }
      }
      newline = this.buffer.indexOf("\n");
    }
  }

  private onMessage(msg: Record<string, unknown>): void {
    const id = msg.id;
    if (typeof msg.method === "string") {
      // Server-initiated request (e.g. roots/list). This bridge declares no
      // client capabilities, so decline rather than leave the server hanging.
      if (id !== undefined && id !== null) {
        this.send({
          jsonrpc: "2.0",
          id,
          error: { code: -32601, message: "Method not supported by the OK Pi bridge" },
        });
      }
      return;
    }
    if (typeof id !== "number") return;
    const pending = this.pending.get(id);
    if (!pending) return;
    this.pending.delete(id);
    const error = msg.error as { message?: string } | undefined;
    if (error) {
      pending.reject(new Error(error.message || "Open Knowledge MCP error"));
      return;
    }
    pending.resolve((msg.result ?? {}) as Record<string, unknown>);
  }

  private send(msg: Record<string, unknown>): void {
    // Buffered stdout can still dispatch onMessage replies after close() ends
    // stdin; writing then would raise ERR_STREAM_WRITE_AFTER_END unhandled.
    if (!this.alive) return;
    this.child.stdin.write(JSON.stringify(msg) + "\n");
  }

  request(
    method: string,
    params: Record<string, unknown>,
    opts: { timeoutMs?: number; signal?: AbortSignal } = {},
  ): Promise<Record<string, unknown>> {
    if (!this.alive) return Promise.reject(new Error("Open Knowledge MCP server is not running"));
    const id = this.nextId++;
    return new Promise<Record<string, unknown>>((resolve, reject) => {
      let timer: ReturnType<typeof setTimeout> | undefined;
      let onAbort: (() => void) | undefined;
      const cleanup = () => {
        if (timer !== undefined) clearTimeout(timer);
        if (onAbort && opts.signal) opts.signal.removeEventListener("abort", onAbort);
        this.pending.delete(id);
      };
      const done: PendingRequest = {
        resolve: (value) => {
          cleanup();
          resolve(value);
        },
        reject: (err) => {
          cleanup();
          reject(err);
        },
      };
      this.pending.set(id, done);
      if (opts.timeoutMs) {
        timer = setTimeout(() => {
          done.reject(new Error(method + " timed out after " + opts.timeoutMs + "ms"));
        }, opts.timeoutMs);
      }
      if (opts.signal) {
        onAbort = () => {
          this.notify("notifications/cancelled", { requestId: id, reason: "aborted" });
          done.reject(new Error("Cancelled"));
        };
        if (opts.signal.aborted) {
          onAbort();
          return;
        }
        opts.signal.addEventListener("abort", onAbort, { once: true });
      }
      this.send({ jsonrpc: "2.0", id, method, params });
    });
  }

  notify(method: string, params: Record<string, unknown>): void {
    if (!this.alive) return;
    this.send({ jsonrpc: "2.0", method, params });
  }

  close(): void {
    this.alive = false;
    for (const p of this.pending.values()) {
      p.reject(new Error("Open Knowledge MCP client closed"));
    }
    this.pending.clear();
    // ok mcp exits on stdin EOF; the delayed kill is a backstop.
    try {
      this.child.stdin.end();
    } catch {
      // Already gone.
    }
    const child = this.child;
    setTimeout(() => {
      try {
        child.kill();
      } catch {
        // Already gone.
      }
    }, 2000).unref();
  }
}

/** JSON Schema pass-through: Pi validates plain JSON Schema parameters. */
function toParameters(inputSchema: Record<string, unknown> | undefined): Record<string, unknown> {
  if (!inputSchema || typeof inputSchema !== "object") {
    return { type: "object", properties: {} };
  }
  const schema = { ...inputSchema };
  delete schema.$schema;
  return schema;
}

function contentText(content: unknown): string {
  if (!Array.isArray(content)) return "";
  return (content as McpContentItem[])
    .filter((item) => item && item.type === "text" && typeof item.text === "string")
    .map((item) => item.text as string)
    .join("\n");
}

export default function okBridge(pi: {
  on(event: string, handler: (event: unknown, ctx: unknown) => unknown): void;
  registerTool(definition: Record<string, unknown>): void;
}) {
  let client: OkMcpClient | null = null;
  let starting: Promise<OkMcpClient> | null = null;
  let sessionCwd = process.cwd();
  const registeredTools = new Set<string>();

  async function ensureClient(): Promise<OkMcpClient> {
    if (client && client.alive) return client;
    if (starting) return starting;
    starting = (async () => {
      const next = new OkMcpClient(sessionCwd, () => {
        if (client === next) client = null;
      });
      try {
        await next.request(
          "initialize",
          {
            protocolVersion: "2025-06-18",
            capabilities: {},
            clientInfo: { name: "pi", version: "1.0.0" },
          },
          { timeoutMs: INIT_TIMEOUT_MS },
        );
      } catch (err) {
        // A timed-out initialize rejects the request while the child may
        // still be running; tear it down so retries don't stack orphans.
        next.close();
        throw err;
      }
      next.notify("notifications/initialized", {});
      client = next;
      return next;
    })();
    try {
      return await starting;
    } finally {
      starting = null;
    }
  }

  async function registerOkTools(): Promise<number> {
    const mcp = await ensureClient();
    const listed = await mcp.request("tools/list", {}, { timeoutMs: INIT_TIMEOUT_MS });
    const tools = Array.isArray(listed.tools) ? (listed.tools as McpToolInfo[]) : [];
    for (const tool of tools) {
      const name = TOOL_PREFIX + tool.name;
      if (registeredTools.has(name)) continue;
      registeredTools.add(name);
      pi.registerTool({
        name,
        label: "OK " + tool.name,
        description: tool.description || "Open Knowledge " + tool.name + " tool.",
        parameters: toParameters(tool.inputSchema),
        execute: async (
          _toolCallId: string,
          params: Record<string, unknown>,
          signal: AbortSignal | undefined,
        ) => {
          const live = await ensureClient();
          const result = await live.request(
            "tools/call",
            { name: tool.name, arguments: params ?? {} },
            { signal },
          );
          const text = contentText(result.content);
          if (result.isError) {
            throw new Error(text || "Open Knowledge tool " + tool.name + " failed");
          }
          return {
            content: [{ type: "text", text }],
            details: result.structuredContent,
          };
        },
      });
    }
    return tools.length;
  }

  pi.on("session_start", async (_event, ctx) => {
    const cwd = (ctx as { cwd?: string } | undefined)?.cwd;
    if (typeof cwd === "string" && cwd.length > 0) sessionCwd = cwd;
    try {
      await registerOkTools();
    } catch (err) {
      const ui = (ctx as { ui?: { notify?: (m: string, level: string) => void } } | undefined)?.ui;
      try {
        ui?.notify?.(
          "Open Knowledge tools unavailable: " + (err instanceof Error ? err.message : String(err)),
          "warning",
        );
      } catch {
        // Non-interactive mode; the failure will resurface on first tool use.
      }
    }
  });

  pi.on("session_shutdown", async () => {
    const pending = starting;
    starting = null;
    const current = client;
    client = null;
    current?.close();
    // An in-flight ensureClient would otherwise assign a live client after
    // shutdown; await it and tear that one down too.
    if (pending) {
      try {
        (await pending).close();
      } catch {
        // Initialization failed — its catch path already closed the client.
      }
    }
  });
}
