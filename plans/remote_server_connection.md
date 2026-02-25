# Plan: Add Remote Server Connection Support to repl/exec

## Goal

Enable `dh repl` and `dh exec` to connect to existing/remote Deephaven servers instead of always starting an embedded server.

**Examples:**
```bash
# Connect to remote server in REPL mode
dh repl --host myserver.com --port 10000

# Execute script on remote server
dh exec script.py --host remote.example.com

# With authentication
dh exec script.py --host secure.example.com --auth-type Basic --auth-token user:pass

# With TLS
dh repl --host prod.example.com --tls --tls-ca-cert /path/to/ca.pem
```

## Current Behavior

- `--port` exists but only controls the **embedded** server port
- All commands start an embedded DeephavenServer before connecting
- DeephavenClient only accepts `host` and `port` parameters

## Proposed Changes

### 1. Add `--host` Flag to Detect Remote Mode

When `--host` is provided (and not "localhost"), skip embedded server startup and connect directly.

| Flag | Default | Behavior |
|------|---------|----------|
| `--host` | None | If set, connect to remote server instead of starting embedded |
| `--port` | 10000 | Server port (works for both embedded and remote) |

**Decision logic:**
```python
if args.host:
    # Remote mode: connect directly to existing server
    client = DeephavenClient(host=args.host, port=args.port, ...)
else:
    # Embedded mode: start server, then connect (current behavior)
    with DeephavenServer(port=args.port, ...) as server:
        client = DeephavenClient(port=server.actual_port, ...)
```

### 2. Add Authentication Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--auth-type` | "Anonymous" | Authentication type: Anonymous, Basic, or custom handler |
| `--auth-token` | "" | Token/credentials (format depends on auth-type) |

**Environment variable support:**
- `DH_AUTH_TYPE` - default auth type
- `DH_AUTH_TOKEN` - default auth token (avoids command-line exposure)

### 3. Add TLS Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--tls` | False | Enable TLS/SSL encryption |
| `--tls-ca-cert` | None | Path to CA certificate PEM file |
| `--tls-client-cert` | None | Path to client certificate PEM (mutual TLS) |
| `--tls-client-key` | None | Path to client private key PEM (mutual TLS) |

### 4. Update DeephavenClient

Extend to pass through all pydeephaven.Session parameters:

```python
class DeephavenClient:
    def __init__(
        self,
        host: str = "localhost",
        port: int = 10000,
        auth_type: str = "Anonymous",
        auth_token: str = "",
        use_tls: bool = False,
        tls_root_certs: bytes | None = None,
        client_cert_chain: bytes | None = None,
        client_private_key: bytes | None = None,
    ):
        ...
```

### 5. Update CLI Argument Parsing (cli.py)

Add shared arguments to both `repl` and `exec` subparsers:

```python
def add_connection_args(parser):
    """Add common connection arguments to a parser."""
    parser.add_argument("--host", help="Remote server hostname (skips embedded server)")
    parser.add_argument("--auth-type", default="Anonymous", help="Auth type")
    parser.add_argument("--auth-token", default="", help="Auth token")
    parser.add_argument("--tls", action="store_true", help="Enable TLS")
    parser.add_argument("--tls-ca-cert", help="CA certificate file")
    parser.add_argument("--tls-client-cert", help="Client certificate file")
    parser.add_argument("--tls-client-key", help="Client private key file")

add_connection_args(repl_parser)
add_connection_args(exec_parser)
```

### 6. Update run_repl() and run_exec()

**Before (embedded only):**
```python
def run_repl(port, jvm_args, verbose, vi_mode):
    with DeephavenServer(port=port, ...) as server:
        with DeephavenClient(port=server.actual_port) as client:
            # ... repl logic
```

**After (embedded or remote):**
```python
def run_repl(port, jvm_args, verbose, vi_mode, host=None, auth_type="Anonymous", ...):
    if host:
        # Remote mode - connect directly
        with DeephavenClient(host=host, port=port, auth_type=auth_type, ...) as client:
            # ... repl logic
    else:
        # Embedded mode - start server first
        with DeephavenServer(port=port, ...) as server:
            with DeephavenClient(port=server.actual_port) as client:
                # ... repl logic
```

## Implementation Order

1. **Update DeephavenClient** (`client.py`)
   - Add all connection parameters to __init__
   - Pass parameters through to Session()

2. **Add helper function** (`cli.py`)
   - `add_connection_args()` to add shared flags
   - `read_cert_file()` to load PEM files as bytes

3. **Update repl command** (`cli.py`)
   - Add connection arguments
   - Update run_repl() signature and logic
   - Handle remote vs embedded mode

4. **Update exec command** (`cli.py`)
   - Add connection arguments
   - Update run_exec() signature and logic
   - Handle remote vs embedded mode

5. **Add tests**
   - Test argument parsing
   - Test remote connection (mock or integration)
   - Test TLS cert file loading
   - Test auth token from environment variable

6. **Update documentation**
   - Help text examples
   - README updates

## Flags NOT Included (for simplicity)

These pydeephaven options are omitted for the initial implementation:

- `--session-type` - Always "python" for dh-cli
- `--never-timeout` - Always True (keep alive)
- `--client-opts` - Advanced gRPC options (edge case)
- `--extra-headers` - Custom HTTP headers (edge case)

Can be added later if users request them.

## Error Handling

1. **Connection failures** - Clear message: "Failed to connect to {host}:{port}: {error}"
2. **Auth failures** - "Authentication failed. Check --auth-type and --auth-token"
3. **TLS failures** - "TLS error: {details}. Verify certificates with --tls-ca-cert"
4. **Missing cert files** - "Certificate file not found: {path}"

## Test Scenarios

1. `dh repl` - Embedded server (unchanged behavior)
2. `dh repl --host localhost --port 10000` - Remote to local
3. `dh exec script.py --host remote` - Remote execution
4. `dh exec script.py --host secure --tls --auth-type Basic --auth-token u:p` - Full auth
5. `DH_AUTH_TOKEN=secret dh exec script.py --host api` - Token from env

## Design Decisions

1. **`--host localhost` goes remote** - Any use of `--host` means remote mode, even localhost. Omit `--host` for embedded server.

2. **No URL format** - pydeephaven.Session uses separate `host`/`port` params, not URLs. Keep it simple with `--host` and `--port` flags.

3. **`app` command unchanged** - `dh app` runs a local web UI, remote mode doesn't make sense for it.
