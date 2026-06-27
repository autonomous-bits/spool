import { joinSession } from "@github/copilot-sdk/extension";
import { exec } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { promisify } from "node:util";

const execAsync = promisify(exec);

const PROJECT_NAME = "Spool";
const WATCHED_AREAS = ["apps/store", "apps/mcp"];
const CHECKS = [
    ["build", "pnpm build"],
    ["typecheck", "pnpm typecheck"],
    ["test", "pnpm test"],
];
const QA_TIMEOUT_MS = 5 * 60 * 1000;
const MAX_FIX_ATTEMPTS = 3;

// In-memory session state — reset on session end
const state = {
    planPresented: false,
    codeEdited: false,
    fixAttempts: 0,
    qaInFlight: null,            // Promise mutex for concurrent task completions
    lastClaimedSummary: "",      // Most recent sub-agent self-reported summary (for fabrication detection)
    pendingFixTimer: null,       // setTimeout handle — cleared on session end to prevent late sends
    lastDispatchedKey: null,     // prevent duplicate fix dispatches for the same failure set
};

/** Best-effort log — a logging failure must never break the QA gate. */
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

/** Resolve the workspace root so checks run from the Spool checkout root. */
async function getWorkspaceRoot(cwd) {
    try {
        const { stdout } = await execAsync("git rev-parse --show-toplevel", { cwd, timeout: 5000 });
        return stdout.trim();
    } catch {
        return findWorkspaceRoot(cwd);
    }
}

function readPackageJson(pkgPath) {
    try {
        return JSON.parse(readFileSync(resolve(pkgPath, "package.json"), "utf8"));
    } catch {
        return null;
    }
}

function hasUsableScript(packageJson, scriptName) {
    const script = packageJson?.scripts?.[scriptName];
    if (!script) return false;
    return !/echo\s+["']Error:\s+no test specified["']\s*&&\s*exit\s+1/.test(script);
}

/**
 * @param {string} cwd
 * @param {(msg: string, opts?: object) => Promise<void>} log
 * @returns {{ errors: string[], checked: number, skipped: number }}
 */
async function runQA(cwd, log) {
    const workspaceRoot = await getWorkspaceRoot(cwd);
    const errors = [];
    let checked = 0;
    let skipped = 0;

    for (const [label, command] of CHECKS) {
        const packageJson = readPackageJson(workspaceRoot);
        if (!packageJson) {
            skipped++;
            await safeLog(log, `⏭️  workspace — package.json not found, skipping ${label}`);
            continue;
        }
        if (!hasUsableScript(packageJson, label)) {
            skipped++;
            await safeLog(log, `⏭️  workspace — no usable ${label} script, skipping`);
            continue;
        }

        await safeLog(log, `📦 workspace — running ${label}…`, { ephemeral: true });
        try {
            await execAsync(command, {
                cwd: workspaceRoot,
                timeout: QA_TIMEOUT_MS,
                maxBuffer: 4 * 1024 * 1024,
            });
            checked++;
            await safeLog(log, `📦 workspace — ✅ ${label} passed`);
        } catch (err) {
            checked++;
            const output = [err.stdout, err.stderr].filter(Boolean).join("\n");
            errors.push(`=== ${label} failed in ${PROJECT_NAME} workspace ===\n${output}`);
            await safeLog(log, `📦 workspace — ❌ ${label} failed`, { level: "error" });
        }
    }

    return { errors, checked, skipped };
}

/**
 * The CLI sends preToolUse as a batch when the agent calls multiple tools in
 * parallel. In that case input.toolCalls is an array; input.toolName is absent.
 * postToolUse always fires one-at-a-time with input.toolName set.
 */
function resolveToolNames(input) {
    if (Array.isArray(input.toolCalls)) {
        return input.toolCalls.map((tc) => tc.name);
    }
    return input.toolName ? [input.toolName] : [];
}

/** Extract a short string from a tool result for logging/heuristics. */
function toolResultText(result) {
    if (!result) return "";
    if (typeof result === "string") return result;
    if (typeof result.textResultForLlm === "string") return result.textResultForLlm;
    try { return JSON.stringify(result); } catch { return ""; }
}

/** Run QA, but coalesce concurrent invocations on a single in-flight promise. */
function runQAOnce(cwd, log) {
    if (state.qaInFlight) return state.qaInFlight;
    state.qaInFlight = (async () => {
        try {
            return await runQA(cwd, log);
        } finally {
            state.qaInFlight = null;
        }
    })();
    return state.qaInFlight;
}

/** Build a stable key for a failure set to detect duplicate dispatches. */
function errorsKey(errors) {
    return errors.map(e => e.slice(0, 200)).join("|");
}

/**
 * Schedule a fix dispatch, cancelling any already-pending one.
 * Mirrors the security-gate pattern to prevent duplicate sends when multiple
 * parallel sub-agents return at the same time.
 */
function scheduleFixDispatch(errors, attempt) {
    if (state.pendingFixTimer !== null) {
        clearTimeout(state.pendingFixTimer);
        state.pendingFixTimer = null;
    }
    const key = errorsKey(errors);
    if (state.lastDispatchedKey === key) return; // already dispatched for this exact failure set
    state.lastDispatchedKey = key;
    const prompt = buildFixPrompt(errors, attempt);
    state.pendingFixTimer = setTimeout(() => {
        state.pendingFixTimer = null;
        session.send({ prompt }).catch(() => {});
    }, 0);
}

/**
 * Build the prompt that instructs the orchestrator to dispatch a fix sub-agent.
 * The orchestrator receives this as a new user turn (via session.send).
 */
function buildFixPrompt(errors, attempt) {
    const trimmed = errors.map((e) => e.length > 4000 ? e.slice(0, 4000) + "\n…(truncated)" : e);
    return [
        `🛡️ CI Gate: the previous sub-agent's changes failed ${errors.length} check(s) (fix attempt ${attempt}/${MAX_FIX_ATTEMPTS}).`,
        ``,
        `Dispatch a fresh sub-agent via the \`task\` tool to fix these failures. Use this prompt verbatim:`,
        ``,
        `---`,
        `Fix the following QA failures in the Spool TypeScript/NestJS monorepo. Spool's main workspaces are \`apps/store\` (NestJS knowledge store) and \`apps/mcp\` (MCP server). After each fix, re-run the failing workspace command from the repository root. Do not finish your task until all gated checks pass cleanly:`,
        ``,
        `- \`pnpm build\``,
        `- \`pnpm typecheck\``,
        `- \`pnpm test\``,
        ``,
        `Use \`pnpm test:store\` or \`pnpm test:mcp\` for targeted follow-up when appropriate. Run the store locally with Docker Compose, not directly on the host, unless explicitly requested. Do not modify unrelated code or weaken assertions to make tests pass.`,
        ``,
        ...trimmed,
        `---`,
        ``,
        `Do not ask for confirmation — dispatch the fix sub-agent now.`,
    ].join("\n");
}

const session = await joinSession({
    hooks: {
        // Inject a standing CI gate contract so the orchestrator knows the loop is automated.
        onSessionStart: async () => {
            await safeLog(session.log.bind(session), `🛡️  CI Gate active — watching ${PROJECT_NAME}: ${WATCHED_AREAS.join(", ")}`, { ephemeral: true });
            return {
                additionalContext: [
                    `CI Gate extension is active for ${PROJECT_NAME}: ${WATCHED_AREAS.join(", ")}.`,
                    ``,
                    `Automated behavior — DO NOT bypass:`,
                    `1. After every \`task\` tool call returns, this extension automatically runs the Spool workspace checks: \`pnpm build\`, \`pnpm typecheck\`, and \`pnpm test\`.`,
                    `2. If checks fail, you will receive a new user turn instructing you to dispatch a fix sub-agent. Do so immediately, without further confirmation, using the prompt provided.`,
                    `3. The \`task_complete\` tool is gated: it will be denied until the most recent QA run is clean.`,
                    ``,
                    `When dispatching sub-agents via the \`task\` tool, instruct them to run the relevant Spool pnpm checks after completing code changes — do not rely on self-reported pass/fail counts. The store app should be run with Docker Compose for local runtime work.`,
                ].join("\n"),
            };
        },

        // Track plan presentation, code edits, and sub-agent task results.
        onPostToolUse: async (input) => {
            const { toolName, toolArgs, toolResult } = input;
            const cwd = hookCwd(input);
            const log = session.log.bind(session);

            if (toolName === "exit_plan_mode") {
                state.planPresented = true;
                await safeLog(log, "📋 Plan presented — QA gate armed", { ephemeral: true });
                return;
            }

            if (toolName === "create" || toolName === "edit" || toolName === "apply_patch") {
                const filePath = String(toolArgs?.path ?? "");

                // plan.md written to the session workspace signals plan mode exit
                if (/\.copilot[/\\]session-state[/\\][^/\\]+[/\\]plan\.md$/.test(filePath)) {
                    state.planPresented = true;
                    await safeLog(log, "📋 Plan presented — QA gate armed", { ephemeral: true });
                    return;
                }

                if (state.planPresented && !state.codeEdited) {
                    state.codeEdited = true;
                    await safeLog(log, `✏️  Code changes detected — QA gate armed`, { ephemeral: true });
                } else if (state.planPresented) {
                    state.codeEdited = true;
                }
                return;
            }

            // Sub-agent dispatch via the task tool — assume it modified code and run QA now.
            if (toolName === "task") {
                state.codeEdited = true;
                state.lastClaimedSummary = toolResultText(toolResult);

                await safeLog(log, "🔍 CI Gate: sub-agent returned — running build/lint/tests…", { ephemeral: true });
                const { errors, checked, skipped } = await runQAOnce(cwd, log);

                if (errors.length === 0) {
                    state.fixAttempts = 0;
                    state.lastDispatchedKey = null;
                    await safeLog(log, `✅ CI Gate: ${checked} check(s) passed, ${skipped} skipped`);
                    return;
                }

                // Synchronously claim this failure set before any further awaits so that
                // concurrent hooks (parallel sub-agent returns) don't each increment the
                // counter and queue their own dispatch — mirrors the security-gate pattern.
                const key = errorsKey(errors);
                const isNewSet = (state.lastDispatchedKey !== key);
                if (isNewSet) {
                    state.fixAttempts++;
                }

                // Heuristic fabrication check
                const claim = state.lastClaimedSummary.toLowerCase();
                if (/\b(all|\d+\/\d+)\b.*(tests?|passing|passed)/.test(claim) || /tests?\s*:?\s*\d+\s*passed/.test(claim)) {
                    await safeLog(log, "⚠️  Sub-agent claimed checks passed but CI Gate found failures — distrust self-reported counts", { level: "warning" });
                }

                if (state.fixAttempts > MAX_FIX_ATTEMPTS) {
                    await safeLog(log, `🛑 CI Gate: ${state.fixAttempts - 1} fix attempts exhausted — escalating to user`, { level: "error" });
                    return {
                        additionalContext:
                            `CI Gate exhausted ${MAX_FIX_ATTEMPTS} automated fix attempts. ` +
                            `Stop dispatching fix sub-agents and report the remaining failures to the user with recommendations.\n\n` +
                            errors.join("\n\n"),
                    };
                }

                // Only log and dispatch if this is a new failure set — duplicate hooks
                // for the same error set are silently ignored.
                if (isNewSet) {
                    await safeLog(log, `❌ CI Gate: ${errors.length} check(s) failed — dispatching fix sub-agent (attempt ${state.fixAttempts}/${MAX_FIX_ATTEMPTS})`, { level: "warning" });
                    scheduleFixDispatch(errors, state.fixAttempts);
                }
                return;
            }
        },

        onPreToolUse: async (input) => {
            const cwd = hookCwd(input);
            const toolNames = resolveToolNames(input);
            const log = session.log.bind(session);

            // Final safety net: block task_complete if the most recent QA run wasn't clean.
            if (toolNames.includes("task_complete")) {
                if (!state.planPresented || !state.codeEdited) {
                    await safeLog(log, "⏭️  QA gate skipped — no plan/code changes tracked", { ephemeral: true });
                    return;
                }

                await safeLog(log, "🔍 QA gate (final): re-running checks before completion…", { ephemeral: true });
                const { errors, checked, skipped } = await runQAOnce(cwd, log);

                if (errors.length > 0) {
                    await safeLog(log, `❌ QA gate blocked task_complete — ${errors.length} check(s) failed (${checked} run, ${skipped} skipped)`, { level: "error" });
                    return {
                        permissionDecision: "deny",
                        permissionDecisionReason:
                            `QA checks failed. Dispatch a fix sub-agent via the \`task\` tool to address these errors before calling task_complete:\n\n` +
                            errors.join("\n\n"),
                    };
                }

                await safeLog(log, `✅ QA gate passed — ${checked} check(s), ${skipped} skipped`);
                state.planPresented = false;
                state.codeEdited = false;
                state.fixAttempts = 0;
            }
        },

        onSessionEnd: async () => {
            // Cancel any pending fix dispatch to prevent sends after teardown
            if (state.pendingFixTimer !== null) {
                clearTimeout(state.pendingFixTimer);
                state.pendingFixTimer = null;
            }
            state.planPresented = false;
            state.codeEdited = false;
            state.fixAttempts = 0;
            state.qaInFlight = null;
            state.lastClaimedSummary = "";
            state.lastDispatchedKey = null;
        },
    },
    tools: [],
});
