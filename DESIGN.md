# HiDNS Design Document

## 1. Overview

HiDNS is a **DNS over UDP** server implemented in **Go**. It listens on a UDP port for client DNS queries. For **A record** queries it resolves locally from configuration or forwards to a public upstream DNS; for **non-A** queries it sends **no response at all**.

## 2. Goals and Non-Goals

### 2.1 Goals

- Provide DNS service **only over UDP** (port is configurable; default 53).
- Support the full path for **A (QTYPE=1)** queries: return the configured IP on a local hit; on a miss, forward to upstream and relay the response to the client.
- Drive local resolution from a configuration file with **suffix matching** and **longest-match** semantics.
- Support **`#` line comments** and blank lines in the configuration file.

### 2.2 Non-Goals

- **Non-A** QTYPEs are not handled: no table lookup, no forwarding, **no reply** (silent discard).
- This document does not specify concrete CLI flags or default paths; those may be defined at implementation time (and documented in the README).

## 3. Configuration File

### 3.1 Format

- One mapping per line: **`domain;IPv4`**.
- Example: `google.com;127.0.0.1`.

### 3.2 Preprocessing Rules

1. Strip a trailing `\r` from each line (Windows line endings).
2. Optional: strip trailing spaces; leading spaces may be trimmed or rejected (implementation choice).
3. **Blank lines**: skip.
4. **Comment lines**: if the line (after stripping trailing `\r`) starts with **`#`**, treat the entire line as a comment and **do not parse** it.
5. **Valid lines**: non-empty, not a comment, and matching `domain;IPv4`. Invalid lines should either be logged and skipped, or cause startup failure (pick one approach and document it in the README).

### 3.3 Domain Matching Semantics

- Let the configured domain be **`suffix`** and the queried name be **`q`** (implementation suggestion: lowercase consistently and normalize the FQDN, e.g. handle a trailing root `.`).
- A **match** if and only if:
  - `q == suffix`, or
  - `q` has `.{suffix}` as a suffix (i.e. there must be a label boundary `.` before `suffix`).
- Example: with `google.com` configured, `google.com`, `a.google.com`, and `a.b.google.com` match; `notgoogle.com` does not match `google.com`.
- **Multiple matches**: use **longest suffix match** (the most specific `suffix`) so that, for example, both `com` and `google.com` do not produce ambiguous wide-domain wins.

## 4. DNS Processing Flow

The following applies only when the payload **successfully parses as a DNS message** and there is **at least one Question**; otherwise silently discard (optional logging).

1. Read **QTYPE** from the **first** Question.
2. If **QTYPE ≠ A**: stop processing and **do not send a UDP reply** to the client.
3. If **QTYPE = A**:
   - Take **QNAME**, normalize to `q`, and perform longest suffix matching per Section 3.3.
   - **On hit**: build a response (`RCODE=NOERROR`) and return an **A record** in the Answer section whose owner name matches the query QNAME, with RDATA set to the configured IPv4; set TTL, AA, etc. according to normal authoritative / local-override semantics (a fixed TTL in code is acceptable).
   - **On miss**: forward the **received UDP payload** to upstream (e.g. `114.114.114.114:53`), wait for the upstream UDP response, then **write it back to the client** (optional length and basic sanity checks). Set a **read deadline** on the upstream socket. Whether to send an error response to the client on upstream timeout is an implementation choice; for consistency with “no reply for non-A”, you may either send **no reply** or return **`SERVFAIL`**—pick one and document it.

### 4.1 Multiple Questions

If the message contains multiple Questions: only the **first Question** should drive behavior; if the first Question is not A, still send no reply. Whether other Questions participate in matching is not mandated; keep behavior simple and predictable.

## 5. Forwarding and Concurrency

- **Correctness**: replies must correspond one-to-one with **(client address, DNS message ID)** so IDs are never mixed up between clients.
- **Simple model**: handle each client request in its own goroutine: send to upstream and block on read, keeping the same DNS ID as the client, which avoids extra ID mapping.
- **Complex model**: if reusing upstream sockets or pools, maintain a **client ↔ upstream** ID mapping (or equivalent), with timeouts and cleanup.

## 6. Implementation Notes (Go)

- Use a mature library to parse and build DNS messages (e.g. `github.com/miekg/dns`) instead of hand-rolling the wire format.
- Configuration: line scan + comment/blank filtering + IPv4 validation; in-memory structure supporting **O(number of labels)** or similar longest-suffix lookup (e.g. strip leftmost labels from `q` and probe a map until the first hit yields the longest configured `suffix`, or use a reversed-name trie).
- Logging: distinguish “non-A dropped”, “invalid message”, “config hit”, “forwarded”, “upstream timeout”, etc., for operations.

## 7. Security and Deployment

- Listening on the public internet risks open-recursive abuse or reflection amplification; in production prefer **source restrictions**, rate limits, binding only on internal interfaces, and firewall policy.
- Configuration file permissions: readable only by the service account to avoid leaking sensitive internal mappings.

## 8. Version History

| Version | Date       | Notes        |
|---------|------------|--------------|
| 1.0     | 2026-05-12 | Initial draft |
