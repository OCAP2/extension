# Changelog

## Unreleased

### Breaking Changes

**Consistent response format for all commands**

All extension commands now return responses in a consistent `["status", "data"]` format.

| Command | Before | After |
|---------|--------|-------|
| `:TIMESTAMP:` | `1770051536876299200` | `["ok", "1770051536876299200"]` |
| `:INIT:`, `:LOG:`, etc. | `["ok", "ok"]` | `["ok"]` |

**Addon migration:**

```sqf
// Before (inconsistent handling)
private _timestamp = "ocap_recorder" callExtension ":TIMESTAMP:";
// _timestamp = "1770051536876299200"

// After (consistent handling)
private _result = parseSimpleArray ("ocap_recorder" callExtension ":TIMESTAMP:");
if (_result select 0 == "ok") then {
    private _timestamp = _result select 1;
    // _timestamp = "1770051536876299200"
};

// For commands that returned ["ok", "ok"], now return ["ok"]
private _result = parseSimpleArray ("ocap_recorder" callExtension ":INIT:");
// Before: ["ok", "ok"]
// After:  ["ok"]
// Check: (_result select 0) == "ok"
```

**Response format reference:**

| Status | Format | Example |
|--------|--------|---------|
| Success (no data) | `["ok"]` | `:INIT:`, `:LOG:` |
| Success (with data) | `["ok", data]` | `:TIMESTAMP:`, `:VERSION:` |
| Error | `["error", "message"]` | Any command on failure |
