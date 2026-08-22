import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { createCanvas, joinSession } from "@github/copilot-sdk/extension";

const execFileAsync = promisify(execFile);
const servers = new Map();

async function readGraph(branch) {
    const { stdout } = await execFileAsync("go", ["run", "./cmd/spl", "graph", "--branch", branch], {
        cwd: process.cwd(),
        maxBuffer: 32 * 1024 * 1024,
    });
    return JSON.parse(stdout);
}

function sendJson(response, status, value) {
    response.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
    response.end(JSON.stringify(value));
}

function renderHtml() {
    return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Spool graph</title>
<style>
* { box-sizing: border-box; }
body { margin: 0; overflow: hidden; background: var(--background-color-default, #fff); color: var(--text-color-default, #1f2328); font: var(--text-body-medium, 14px)/var(--leading-body-medium, 20px) var(--font-sans, system-ui); }
#graph { display: block; width: 100vw; height: 100vh; cursor: grab; touch-action: none; }
#graph:active { cursor: grabbing; }
#overlay { position: fixed; inset: 12px auto auto 12px; max-width: 310px; padding: 10px 12px; border: 1px solid var(--border-color-default, #d0d7de); border-radius: 8px; background: color-mix(in srgb, var(--background-color-default, #fff) 92%, transparent); box-shadow: 0 3px 12px #0002; }
#tooltip { position: fixed; display: none; max-width: 320px; padding: 6px 8px; border: 1px solid var(--border-color-default, #d0d7de); border-radius: 6px; background: var(--background-color-default, #fff); color: var(--text-color-default, #1f2328); box-shadow: 0 3px 12px #0003; font-size: 12px; pointer-events: none; }
#title { font-weight: var(--font-weight-semibold, 600); }
#meta, #hint { color: var(--text-color-muted, #57606a); font-size: 12px; }
#error { color: var(--true-color-red, #cf222e); white-space: pre-wrap; }
button { margin-top: 8px; padding: 5px 9px; color: inherit; background: var(--background-color-default, #fff); border: 1px solid var(--border-color-default, #d0d7de); border-radius: 6px; cursor: pointer; }
</style>
</head>
<body>
<canvas id="graph"></canvas>
<div id="overlay"><div id="title">Spool graph</div><div id="meta">Loading…</div><div id="hint">Drag to rotate · scroll to zoom · hover a node for details</div><button id="refresh">Refresh</button><div id="error"></div></div>
<div id="tooltip"></div>
<script>
const canvas = document.querySelector("#graph"), context = canvas.getContext("2d");
const meta = document.querySelector("#meta"), error = document.querySelector("#error"), tooltip = document.querySelector("#tooltip");
const darkMode = document.body.dataset.colorMode === "dark";
let graph = { nodes: [], edges: [] }, yaw = -0.55, pitch = 0.45, distance = 900, dragging, hovered;
function hue(value) {
  let result = 0;
  for (let index = 0; index < value.length; index++) result = ((result << 5) - result + value.charCodeAt(index)) | 0;
  return Math.abs(result) % 360;
}
function color(value, active = false) {
  return "hsl(" + hue(value) + " " + (active ? "92%" : "70%") + " " + (darkMode ? (active ? "72%" : "62%") : (active ? "40%" : "46%")) + ")";
}
function resize() { canvas.width = innerWidth * devicePixelRatio; canvas.height = innerHeight * devicePixelRatio; context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0); draw(); }
addEventListener("resize", resize);
function position(index, count) {
  const golden = Math.PI * (3 - Math.sqrt(5)), y = 1 - (index / Math.max(1, count - 1)) * 2, radius = Math.sqrt(1 - y * y), angle = golden * index;
  return { x: Math.cos(angle) * radius * 280, y: y * 280, z: Math.sin(angle) * radius * 280 };
}
function project(point) {
  const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), sp = Math.sin(pitch);
  const x = point.x * cy - point.z * sy, z = point.x * sy + point.z * cy;
  const y = point.y * cp - z * sp, depth = z * cp + point.y * sp + distance, scale = distance / Math.max(1, depth);
  return { x: innerWidth / 2 + x * scale, y: innerHeight / 2 - y * scale, depth, scale };
}
function draw() {
  context.clearRect(0, 0, innerWidth, innerHeight);
  const nodes = graph.nodes.map((node, index) => ({ ...node, point: position(index, graph.nodes.length) }));
  const byId = new Map(nodes.map(node => [node.id, node]));
  const projected = new Map(nodes.map(node => [node.id, project(node.point)]));
  const degrees = new Map(nodes.map(node => [node.id, 0]));
  const connected = new Set();
  for (const edge of graph.edges) {
    degrees.set(edge.source, (degrees.get(edge.source) || 0) + 1);
    degrees.set(edge.target, (degrees.get(edge.target) || 0) + 1);
    if (edge.source === hovered || edge.target === hovered) { connected.add(edge.source); connected.add(edge.target); }
  }
  context.lineWidth = 1;
  for (const edge of graph.edges) {
    const source = projected.get(edge.source), target = projected.get(edge.target);
    if (!source || !target) continue;
    const active = edge.source === hovered || edge.target === hovered;
    context.strokeStyle = color(edge.type || edge.id, active);
    context.globalAlpha = active ? .95 : .42;
    context.lineWidth = active ? 2.5 : 1.25;
    context.beginPath(); context.moveTo(source.x, source.y); context.lineTo(target.x, target.y); context.stroke();
  }
  context.globalAlpha = 1;
  for (const node of nodes.sort((a, b) => projected.get(b.id).depth - projected.get(a.id).depth)) {
    const point = projected.get(node.id);
    const radius = Math.max(3, Math.min(16, (3 + Math.sqrt(degrees.get(node.id)) * 1.8) * point.scale));
    point.radius = radius;
    context.fillStyle = color(node.id, node.id === hovered || connected.has(node.id));
    context.beginPath(); context.arc(point.x, point.y, radius, 0, Math.PI * 2); context.fill();
  }
  canvas._projected = projected;
}
function setGraph(value) {
  graph = value; hovered = undefined;
  meta.textContent = value.nodes.length + " nodes · " + value.edges.length + " edges · " + value.snapshot.branch;
  error.textContent = ""; draw();
}
async function load() {
  try { const response = await fetch("/graph"); const value = await response.json(); if (!response.ok) throw new Error(value.error); setGraph(value); }
  catch (reason) { error.textContent = String(reason.message || reason); }
}
canvas.addEventListener("pointerdown", event => { dragging = { x: event.clientX, y: event.clientY, yaw, pitch }; canvas.setPointerCapture(event.pointerId); });
function hitNode(x, y) {
  let closest, distance = Infinity;
  for (const node of graph.nodes) { const point = canvas._projected?.get(node.id); if (!point) continue; const gap = Math.hypot(point.x - x, point.y - y); if (gap < Math.max(14, point.radius + 5) && gap < distance) { closest = node; distance = gap; } }
  return closest;
}
canvas.addEventListener("pointermove", event => {
  if (dragging) { yaw = dragging.yaw + (event.clientX - dragging.x) * .008; pitch = Math.max(-1.5, Math.min(1.5, dragging.pitch + (event.clientY - dragging.y) * .008)); draw(); return; }
  const node = hitNode(event.clientX, event.clientY);
  if (node?.id !== hovered) { hovered = node?.id; draw(); }
  canvas.style.cursor = node ? "pointer" : "grab";
  if (node) { tooltip.textContent = node.title || node.id; tooltip.style.display = "block"; tooltip.style.left = (event.clientX + 14) + "px"; tooltip.style.top = (event.clientY + 14) + "px"; }
  else tooltip.style.display = "none";
});
canvas.addEventListener("pointerup", () => { dragging = undefined; });
canvas.addEventListener("pointerleave", () => { if (!dragging) { hovered = undefined; tooltip.style.display = "none"; draw(); } });
canvas.addEventListener("wheel", event => { event.preventDefault(); distance = Math.max(300, Math.min(1800, distance + event.deltaY)); draw(); }, { passive: false });
document.querySelector("#refresh").addEventListener("click", async () => { await fetch("/refresh", { method: "POST" }); await load(); });
resize(); load();
</script>
</body>
</html>`;
}

async function startServer(branch) {
    const entry = { graph: null, error: null, branch };
    const load = async () => {
        try {
            entry.graph = await readGraph(branch);
            entry.error = null;
        } catch (reason) {
            entry.error = reason instanceof Error ? reason.message : String(reason);
        }
    };
    const server = createServer(async (request, response) => {
        const url = new URL(request.url ?? "/", "http://127.0.0.1");
        if (url.pathname === "/graph") {
            if (!entry.graph && !entry.error) await load();
            return entry.error ? sendJson(response, 500, { error: entry.error }) : sendJson(response, 200, entry.graph);
        }
        if (url.pathname === "/refresh" && request.method === "POST") {
            await load();
            return entry.error ? sendJson(response, 500, { error: entry.error }) : sendJson(response, 200, entry.graph);
        }
        response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
        response.end(renderHtml());
    });
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    entry.server = server;
    entry.url = `http://127.0.0.1:${address.port}/`;
    entry.load = load;
    return entry;
}

await joinSession({
    canvases: [
        createCanvas({
            id: "spool-graph-3d",
            displayName: "Spool graph 3D",
            description: "Interactive 3D view of every node and edge in a Spool branch.",
            inputSchema: {
                type: "object",
                properties: { branch: { type: "string", pattern: "^[A-Za-z0-9._/-]+$" } },
                additionalProperties: false,
            },
            actions: [{
                name: "refresh",
                description: "Reload graph data from the selected Spool branch.",
                handler: async (ctx) => {
                    const entry = servers.get(ctx.instanceId);
                    if (!entry) throw new Error("Canvas instance is not open");
                    await entry.load();
                    if (entry.error) throw new Error(entry.error);
                    return { nodes: entry.graph.nodes.length, edges: entry.graph.edges.length, branch: entry.branch };
                },
            }],
            open: async (ctx) => {
                const branch = ctx.input?.branch ?? "main";
                let entry = servers.get(ctx.instanceId);
                if (!entry) {
                    entry = await startServer(branch);
                    servers.set(ctx.instanceId, entry);
                }
                return { title: `Spool graph: ${entry.branch}`, url: entry.url };
            },
            onClose: async (ctx) => {
                const entry = servers.get(ctx.instanceId);
                if (entry) {
                    servers.delete(ctx.instanceId);
                    await new Promise((resolve) => entry.server.close(resolve));
                }
            },
        }),
    ],
});
