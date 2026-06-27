import { joinSession } from "@github/copilot-sdk/extension";
import { exec } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { promisify } from "node:util";

const execAsync = promisify(exec);
const PROJECT_NAME = "Spool";
const MAX_FIX_ATTEMPTS = 3;
const SCAN_TIMEOUT_MS = 5 * 60 * 1000;

const state = {
    baselineSignatures: null,   // Set<string> captured at session start; null = not yet captured
    fixAttempts: 0,
    scanInFlight: null,
    pendingFixTimer: null,      // setTimeout handle — cleared on session end to prevent late sends
    lastDispatchedSignatures: null, // prevent duplicate fix dispatches for the same finding set
};

/** Best-effort log — a logging failure must never break the security gate. */
async function safeLog(log, message, opts) {
    try {
        await log(message, opts);
    } catch {
        // intentionally swallowed
    }
}

function hookCwd(input) {
    return input?.cwd ?? input?.workingDirectory ?? process.cwd();
}

function readPackageJson(pkgPath) {
    try {
        return JSON.parse(readFileSync(resolve(pkgPath, "package.json"), "utf8"));
    } catch {
        return null;
    }
}

function findWorkspaceRoot(startDir) {
    let current = resolve(startDir);
    while (true) {
        const packageJson = readPackageJson(current);
        if (packageJson?.name === "spool" || existsSync(resolve(current, "pnpm-workspace.yaml"))) {
            return current;
        }
        const parent = dirname(current);
        if (parent === current) return resolve(startDir);
        current = parent;
    }
}

function shellQuote(value) {
    return `'${String(value).replaceAll("'", "'\\''")}'`;
}

/** Resolve the workspace root so semgrep always scans the full Spool project. */
async function getWorkspaceRoot(cwd) {
    try {
        const { stdout } = await execAsync("git rev-parse --show-toplevel", { cwd, timeout: 5000 });
        return stdout.trim();
    } catch {
        return findWorkspaceRoot(cwd);
    }
}

/** Stable identity string for a semgrep finding (used for baseline delta). */
function findingKey(f) {
    return `${f.path}:${f.start?.line}:${f.check_id}`;
}

/**
 * Run semgrep scan and return parsed results.
 * Distinguishes scan failures (tooling/config errors) from actual findings.
 *
 * @returns {{ ok: boolean, findings: object[], error?: string }}
 */
async function runSemgrep(cwd) {
    const workspaceRoot = await getWorkspaceRoot(cwd);
    const run = async (stdout) => {
        const parsed = JSON.parse(stdout);
        return { ok: true, findings: parsed.results ?? [] };
    };
    // Use --config .semgrep.yml when present (avoids network calls to the Semgrep registry
    // which are deprecated for implicit config since Semgrep 1.38 and cause hangs in offline
    // environments). Fall back to the explicit flag so this works even if the file is missing.
    const fs = await import("node:fs/promises");
    const configFile = `${workspaceRoot}/.semgrep.yml`;
    let configArg;
    try {
        await fs.access(configFile);
        configArg = `--config ${shellQuote(configFile)}`;
    } catch {
        configArg = "--config auto";
    }
    try {
        const { stdout } = await execAsync(`semgrep scan ${configArg} --json --quiet --metrics off --exclude node_modules --exclude dist`, {
            cwd: workspaceRoot,
            timeout: SCAN_TIMEOUT_MS,
            maxBuffer: 4 * 1024 * 1024,
        });
        return await run(stdout);
    } catch (err) {
        // semgrep exits non-zero when findings are present — stdout is still valid JSON
        if (err.stdout) {
            try {
                return await run(err.stdout);
            } catch { /* JSON parse failed — fall through to scan error */ }
        }
        return { ok: false, findings: [], error: err.stderr?.trim() || err.message };
    }
}

/** Coalesce concurrent semgrep invocations onto a single in-flight promise. */
function runSemgrepOnce(cwd) {
    if (state.scanInFlight) return state.scanInFlight;
    state.scanInFlight = runSemgrep(cwd).finally(() => { state.scanInFlight = null; });
    return state.scanInFlight;
}

/** Filter findings to only those not present in the session baseline. */
function newFindings(findings) {
    if (!state.baselineSignatures) return findings;
    return findings.filter(f => !state.baselineSignatures.has(findingKey(f)));
}

function formatFinding(f) {
    const msg = f.extra?.message ?? "";
    return `${f.path}:${f.start?.line} [${f.check_id}] ${msg}`;
}

/**
 * Stringify a finding set to a stable key so we can detect whether
 * we've already dispatched a fix prompt for exactly this set.
 */
function findingSetKey(findings) {
    return findings.map(findingKey).sort().join("|");
}

function buildFixPrompt(findings, attempt) {
    const lines = findings.map(f => {
        const formatted = formatFinding(f);
        return formatted.length > 4000 ? formatted.slice(0, 4000) + "\n…(truncated)" : formatted;
    });
    return [
        `🔒 Security Gate: semgrep found ${findings.length} new security issue(s) (fix attempt ${attempt}/${MAX_FIX_ATTEMPTS}).`,
        ``,
        `Dispatch a fresh sub-agent via the \`task\` tool to fix these findings. Use this prompt verbatim:`,
        ``,
        `---`,
        `Fix the following semgrep security findings in the Spool TypeScript/NestJS monorepo. Spool's main workspaces are \`apps/store\` (NestJS knowledge store) and \`apps/mcp\` (MCP server). After applying each fix, verify the issue is resolved by running \`semgrep scan --config .semgrep.yml --json --quiet --metrics off\` from the repository root or on the affected file. Do not introduce new issues, weaken existing security controls, or modify unrelated code.`,
        ``,
        ...lines,
        `---`,
        ``,
        `Do not ask for confirmation — dispatch the fix sub-agent now.`,
    ].join("\n");
}

/** Schedule a fix dispatch, cancelling any already-pending one. */
function scheduleFixDispatch(findings, attempt) {
    if (state.pendingFixTimer !== null) {
        clearTimeout(state.pendingFixTimer);
        state.pendingFixTimer = null;
    }
    const key = findingSetKey(findings);
    if (state.lastDispatchedSignatures === key) return; // already dispatched for this exact set
    state.lastDispatchedSignatures = key;
    state.pendingFixTimer = setTimeout(() => {
        state.pendingFixTimer = null;
        session.send({ prompt: buildFixPrompt(findings, attempt) }).catch(() => {});
    }, 0);
}

const session = await joinSession({
    hooks: {
        onSessionStart: async (input) => {
            const log = session.log.bind(session);
            await safeLog(log, `🔒 Security Gate active for ${PROJECT_NAME} — semgrep scan gates task_complete`, { ephemeral: true });

            // Capture baseline so we only block on findings introduced this session
            const { ok, findings, error } = await runSemgrep(hookCwd(input));
            if (ok) {
                state.baselineSignatures = new Set(findings.map(findingKey));
                await safeLog(log, `🔒 Security Gate: baseline captured (${state.baselineSignatures.size} pre-existing finding(s) excluded)`);
            } else {
                state.baselineSignatures = new Set(); // treat as clean baseline; log the tooling problem
                await safeLog(log, `⚠️  Security Gate: baseline scan failed (${error}) — all findings will be treated as new`, { level: "warning" });
            }

            return {
                additionalContext: [
                    `Security Gate extension is active for ${PROJECT_NAME}.`,
                    "",
                    "Automated behavior — DO NOT bypass:",
                    "1. Before task_complete is allowed, this extension runs `semgrep scan --config .semgrep.yml` from the Spool workspace root to check for new security issues.",
                    "2. If new issues are found, you will receive a new user turn instructing you to dispatch a fix sub-agent. Do so immediately.",
                    `3. The \`task_complete\` tool is gated: it will be denied until semgrep reports no new findings (up to ${MAX_FIX_ATTEMPTS} automated fix attempts).`,
                ].join("\n"),
            };
        },

        onPreToolUse: async (input) => {
            const toolNames = Array.isArray(input.toolCalls)
                ? input.toolCalls.map(tc => tc.name)
                : input.toolName ? [input.toolName] : [];

            if (!toolNames.includes("task_complete")) return;

            const log = session.log.bind(session);
            await safeLog(log, "🔍 Security Gate: running semgrep scan before completion…", { ephemeral: true });

            const { ok, findings, error } = await runSemgrepOnce(hookCwd(input));

            if (!ok) {
                await safeLog(log, `⚠️  Security Gate: semgrep scan failed — ${error}`, { level: "warning" });
                return {
                    permissionDecision: "deny",
                    permissionDecisionReason:
                        `Security Gate: semgrep scan failed and could not verify security posture.\n` +
                        `Error: ${error}\n\n` +
                        `Resolve the scan failure before calling task_complete.`,
                };
            }

            const novel = newFindings(findings);
            if (novel.length === 0) {
                await safeLog(log, `✅ Security Gate: semgrep clean — no new findings`);
                state.fixAttempts = 0;
                state.lastDispatchedSignatures = null;
                return;
            }

            await safeLog(log, `❌ Security Gate: ${novel.length} new finding(s) — blocking task_complete`, { level: "error" });

            if (state.fixAttempts >= MAX_FIX_ATTEMPTS) {
                await safeLog(log, `🛑 Security Gate: ${MAX_FIX_ATTEMPTS} fix attempts exhausted — escalating to user`, { level: "error" });
                return {
                    permissionDecision: "deny",
                    permissionDecisionReason:
                        `Security Gate exhausted ${MAX_FIX_ATTEMPTS} automated fix attempts. ` +
                        `Stop dispatching fix sub-agents and report the remaining findings to the user with recommendations:\n\n` +
                        novel.map(formatFinding).join("\n"),
                };
            }

            state.fixAttempts++;
            scheduleFixDispatch(novel, state.fixAttempts);

            return {
                permissionDecision: "deny",
                permissionDecisionReason:
                    `Security Gate found ${novel.length} new semgrep finding(s). ` +
                    `A fix sub-agent has been dispatched (attempt ${state.fixAttempts}/${MAX_FIX_ATTEMPTS}). ` +
                    `Do not call task_complete until the findings are resolved.`,
            };
        },

        onSessionEnd: async () => {
            // Cancel any pending fix dispatch to prevent sends after teardown
            if (state.pendingFixTimer !== null) {
                clearTimeout(state.pendingFixTimer);
                state.pendingFixTimer = null;
            }
            state.baselineSignatures = null;
            state.fixAttempts = 0;
            state.scanInFlight = null;
            state.lastDispatchedSignatures = null;
        },
    },
    tools: [],
});
