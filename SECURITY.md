# Security Policy

## Reporting a Vulnerability

Witness handles secrets and runs reconciliation on target hosts, so we take
security reports seriously. Please use **responsible disclosure**:

- Do **not** open a public issue for a vulnerability.
- Report privately to: `security@lyssar.github.com`

> **Note:** `security@lyssar.github.com` is a placeholder. The maintainer must
> replace it with the real security contact before public release.

Please include:

- A description of the vulnerability and its impact.
- Steps to reproduce, if possible.
- Affected versions and any suggested fix.

We aim to acknowledge reports within a few business days and will coordinate a
disclosure timeline with you.

## Secret Handling

Witness encrypts secrets at rest with [age](https://age-encryption.org/). The
observer's `age.key` is the identity used to decrypt secrets during reconcile.

- Never commit plaintext secrets or age private keys to the repository.
- Protect `age.key` with restrictive file permissions (e.g. `0600`).
- Rotate keys and secrets if a key is ever exposed.