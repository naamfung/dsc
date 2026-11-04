package main

// 权限范围：与宿主 core/authorization.go 的常量同名同义。
//
// 这里重复一份字符串常量而不是从宿主取（插件拿不到 core）：范围名是**协议**，
// 不是实现细节——宿主按这些名字记账，插件按同样的名字声明，两边字面量必须一致。
const (
	scopeProcessSpawn = "process.spawn"
	scopeNetEgress    = "net.egress"
)

const connectDescription = "Connect to an external MCP server and register its tools. Two transports: " +
	"http — a streamable-HTTP URL (endpoint); stdio — spawn a local command (command + args) and speak JSON-RPC over its stdin/stdout. " +
	"Most MCP servers are meant to be launched, so stdio is the usual choice; use http when something is already listening. " +
	"A stdio server only inherits a small environment baseline (PATH/HOME/TEMP and similar); put any token it needs into env explicitly. " +
	"Registered tools are hidden from your tool list to keep it small — mcp_list shows their names, mcp_load makes them callable."

const connectSchema = `{
  "type": "object",
  "properties": {
    "server_name": {"type": "string", "description": "Short name for this server, [A-Za-z0-9_-] only, max 32 chars. Becomes the tool name prefix: mcp__<server_name>__<tool>."},
    "transport": {"type": "string", "enum": ["http", "stdio"], "description": "http = streamable HTTP to a running endpoint; stdio = launch command and talk over its stdin/stdout. Inferred from endpoint/command when omitted."},
    "endpoint": {"type": "string", "description": "http only: MCP streamable-HTTP URL, e.g. http://127.0.0.1:8765/mcp."},
    "command": {"type": "string", "description": "stdio only: executable to launch, e.g. node, python, uvx, or an absolute path. Resolved via PATH."},
    "args": {"type": "array", "items": {"type": "string"}, "description": "stdio only: arguments passed to command."},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "stdio only: extra environment variables for the server process. Only a small baseline is inherited, so tokens must be listed here."},
    "cwd": {"type": "string", "description": "stdio only: working directory for the server process (defaults to the plugin's)."}
  },
  "required": ["server_name"],
  "additionalProperties": false
}`

const listSchema = `{
  "type": "object",
  "properties": {
    "server": {"type": "string", "description": "Only list tools from this server (default: all connected servers)."},
    "limit": {"type": "integer", "description": "Maximum tools to print (default 40, max 200).", "minimum": 1, "maximum": 200}
  },
  "additionalProperties": false
}`

const loadSchema = `{
  "type": "object",
  "properties": {
    "tools": {"type": "array", "items": {"type": "string"}, "description": "Tool names: full mcp__<server>__<tool> or a unique bare name."},
    "server": {"type": "string", "description": "Instead of names: load every tool this server exposes (use sparingly)."}
  },
  "additionalProperties": false
}`

const unloadSchema = `{
  "type": "object",
  "properties": {
    "tools": {"type": "array", "items": {"type": "string"}, "description": "Tool names to remove from the tool list. Omit everything to unload all."},
    "server": {"type": "string", "description": "Remove every loaded tool of this server."}
  },
  "additionalProperties": false
}`

const disconnectSchema = `{
  "type": "object",
  "properties": {
    "server_name": {"type": "string", "description": "Server to disconnect. Omit to disconnect all servers."}
  },
  "additionalProperties": false
}`

// ---- 资源（resources）：只读内容，不是动作 ----

const listResourcesDescription = "List the resources (documents, datasets, files) that connected MCP servers expose. " +
	"Resources are content, not actions: read one with mcp_read_resource. " +
	"Only servers that advertise the resources capability have any."

const listResourcesSchema = `{
  "type": "object",
  "properties": {
    "server": {"type": "string", "description": "Only list this server's resources (default: all servers that expose them)."},
    "limit": {"type": "integer", "description": "Maximum entries to print (default 40, max 200).", "minimum": 1, "maximum": 200}
  },
  "additionalProperties": false
}`

const readResourceDescription = "Read one resource by URI (resources/read). " +
	"Text comes back verbatim; binary resources are reported as omitted with their type and size. " +
	"When several servers expose the same URI you must pass server=."

const readResourceSchema = `{
  "type": "object",
  "properties": {
    "uri": {"type": "string", "description": "Resource URI exactly as mcp_list_resources printed it."},
    "server": {"type": "string", "description": "Which server to read from. Omit when only one server exposes that URI."}
  },
  "required": ["uri"],
  "additionalProperties": false
}`

// ---- 提示词（prompts）：服务器提供的模板，取回后由调用方决定怎么用 ----

const listPromptsDescription = "List the prompt templates connected MCP servers expose, with their arguments. " +
	"Render one with mcp_get_prompt. Only servers that advertise the prompts capability have any."

const listPromptsSchema = `{
  "type": "object",
  "properties": {
    "server": {"type": "string", "description": "Only list this server's prompts (default: all servers that expose them)."},
    "limit": {"type": "integer", "description": "Maximum entries to print (default 40, max 200).", "minimum": 1, "maximum": 200}
  },
  "additionalProperties": false
}`

const getPromptDescription = "Render one prompt template (prompts/get) into plain text: description plus each message " +
	"prefixed with its role. Non-text content is reported as omitted rather than dropped silently."

const getPromptSchema = `{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "Prompt name exactly as mcp_list_prompts printed it."},
    "server": {"type": "string", "description": "Which server to ask. Omit when only one server exposes that prompt."},
    "arguments": {
      "type": "object",
      "additionalProperties": {"type": "string"},
      "description": "Template arguments as name → value strings."
    }
  },
  "required": ["name"],
  "additionalProperties": false
}`
