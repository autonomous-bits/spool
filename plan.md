# Plan for SG3: Docker end-to-end verification

## Approach
1. Build code: `pnpm build`
2. Start docker: `docker compose up --build -d spoolstore`
3. Wait for spoolstore to become healthy/ready.
4. Obtain session token: `pnpm dev:session-token`
5. Test GET `/chunks?limit=5` with auth and workspace header. Confirm 200 and `{ chunks, nextCursor }`.
6. Test GET `/chunks/:id/neighbourhood` on a chunk with an edge. Confirm 200 and `{ chunk, neighbours }`.
7. Test negative cases:
   - Missing Authorization -> 401
   - Missing X-Workspace-Id -> 403
8. Test MCP tools `search-chunks` and `get-neighbourhood` via curl to the MCP server.
9. If everything succeeds, update `goal.html` to mark SG3 as done, and `docs/goals/README.md` to mark G12 as done.
