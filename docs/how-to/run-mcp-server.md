# Run the MCP server

Ingest the approved repository mirrors first, then configure an MCP client with an absolute executable path:

```json
{"mcpServers":{"aios":{"command":"/absolute/path/to/bin/aios","args":["serve","--config","/path/to/catalog.json","--data-dir","/path/to/data"]}}}
```

The server uses newline-delimited JSON-RPC frames over stdio. Inbound frames are capped at 1 MiB. Malformed/oversized frames return `-32700`; valid JSON that is not a request returns `-32600`; the connection remains available for subsequent frames. stdout must contain protocol frames only.

The server opens SQLite read-only and exposes only the tools in the [MCP reference](../reference/mcp-tools.md). It has no HTTP endpoint, arbitrary file-read tool, shell execution, network listener, or write-capable tool.
