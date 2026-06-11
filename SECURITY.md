# Security Policy

## Supported versions

This project is pre-1.0. Security fixes are applied to the latest released `v0.x` minor; there are no long-term support branches yet.

| Version | Supported |
| ------- | --------- |
| latest `v0.x` | ✅ |
| older         | ❌ |

## Reporting a vulnerability

Please report security issues **privately** - do not open a public issue for anything exploitable.

- Preferred: [open a private security advisory](https://github.com/wilhelm-murdoch/go-restage/security/advisories/new) ("Security" → "Report a vulnerability" on the repository).
- If that is unavailable, contact the maintainers privately through GitHub.

Please include enough detail to reproduce: affected version, a minimal proof of concept, and the impact you observed. We'll acknowledge the report, investigate, and coordinate a fix and disclosure timeline with you.

## Scope

This is an API-agnostic HTTP client toolkit. It makes outbound requests to whatever API the consuming application configures. The most relevant areas:

- **Credential handling** - `BearerToken` (and any custom `Authenticator`) attaches credentials to outgoing requests; this library never logs them `WithInsecureSkipVerify` disables TLS verification and is intended for local testing only; never use it against a server reachable over an untrusted network.
- **Debug output** - `WithDebug` traces requests and responses. It redacts `Authorization`, `Cookie`, and similar credential headers, but **prints request and response bodies in the clear**, which may contain secrets. Treat its output as sensitive and never enable it against an untrusted network.
- **Response parsing** - untrusted server responses are decoded into caller-supplied Go types and error bodies are truncated (4 KB); report any panic or unbounded allocation triggered by a malformed or hostile response.
