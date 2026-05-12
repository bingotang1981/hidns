# HiDNS

HiDNS is a small **DNS over UDP** server written in Go. It answers **A** queries from a local hosts-style file (suffix match, longest match wins). Anything not listed is **forwarded** to an upstream resolver (default: `114.114.114.114:53`). Non-A queries are **silently ignored** (no UDP reply). See [DESIGN.md](DESIGN.md) for the full design.

## Requirements

- Go **1.22** or newer
- Dependencies managed with `go mod` (`github.com/miekg/dns`)

## Build

```bash
go build -o hidns ./cmd/hidns
```

On Windows, the binary is often named `hidns.exe`.

## Run

```bash
./hidns -listen :5353 -config hosts.txt -v
```

Listening on port **53** usually requires elevated privileges (root on Linux, Administrator on Windows). For local tests, use a high port (for example `:5353`) and point your resolver or `dig @127.0.0.1 -p5353 …` at it.

Stop with **Ctrl+C** (SIGINT / SIGTERM).

## Command-line flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-listen` | `:53` | UDP listen address (`host:port` or `:port`) |
| `-config` | `hosts.txt` | Path to the mapping file |
| `-upstream` | `114.114.114.114:53` | Upstream DNS `host:port` |
| `-timeout` | `5s` | Deadline for each upstream read/write |
| `-v` | off | Enable debug logging |

## Configuration file

- One mapping per line: **`domain;IPv4`** (example: `google.com;127.0.0.1`).
- **Empty lines** are skipped.
- Lines whose first non-CR character is **`#`** are comments.
- Invalid lines are skipped and logged as warnings (unless `-v`, they still appear as `WARN` from the loader).
- **Matching**: the configured name is a **DNS suffix** of the queried name (`a.google.com` matches `google.com`). If several lines match, the **longest** suffix wins.

## Example

`hosts.txt`:

```text
# local override
google.com;127.0.0.1
```

Query (when listening on `5353`):

```bash
dig @127.0.0.1 -p5353 google.com A +short
# expect: 127.0.0.1
```

A name not present in the file is forwarded to `-upstream`; upstream failures produce a **SERVFAIL** response for that A query.

## Security note

Exposing a forwarder on the public internet can be abused as an open resolver or in reflection scenarios. Prefer binding to a trusted network or using host firewalls.

## License

No license file is included in this repository yet; add one if you distribute the project.
