# Netbula Security

## Architecture

Netbula uses a two-layer security model for all manager-client communication (workers and the control program).

### Encryption: TLS with self-signed certificate(server-side, stored on manager machine)

All traffic between the manager and its clients is encrypted using TLS with an Ed25519 self-signed certificate generated at manager startup. No external Certificate Authority(CA) is required.

### Manager Identity: Certificate Pinning(client-side, on both controller and worker machine)

Clients verify the manager's identity by pinning the SHA-256 fingerprint of its TLS certificate. During the handshake, the client rejects any certificate whose fingerprint does not match the configured `cert-fingerprint`. This prevents Man-in-the-Middle attacks on subsequent connections.

The pinning is implemented via Go's `tls.Config.VerifyPeerCertificate` callback with `InsecureSkipVerify: true` (CA validation is skipped in favour of the explicit fingerprint check).

### Controller Authorization: Shared Secret Token(between manager and the control program)

Access to the manager's control plane API is gated by an `auth-token`: a 32-byte (hex-encoded) random value (using `crypto/rand`) generated at manager startup.

The control program must supply this token in the `Netbula-Token` HTTP header for every API request.

Because the token is only transmitted inside an established TLS tunnel, it is kept secret between the manager and the control program.

Workers do not use the `auth-token`. They authenticate only via the TLS certificate fingerprint.

## First-Time Setup

When the manager starts for the first time it prints both values to stdout:

```
Connection token(auth-token): <hex string>
# control program only
TLS certificate fingerprint(cert-fingerprint): <hex string>
# used by workers and control program
```

Distribute these values to clients out-of-band (i.e., using another secured network) (e.g., over SSH).

Passing them over an untrusted channel before the first pinned connection is established is the primary attack window (see Limitations section below).

## Limitations

**Trust-on-First-Use (TOFU):** Certificate pinning protects all connections after the first. However, the initial distribution of `cert-fingerprint` and `auth-token` to workers and operators is not protected by Netbula itself. If the channel used for that first exchange is compromised, an attacker can intercept both values.

**No worker-level authentication:**
The manager accepts any TLS connection on the worker port without verifying the connecting host's identity.

Any host with network access to that port of the manager can register as a worker.

**Static credentials at rest:**
`auth-token` is stored in plaintext in `manager_config.json` (on manager server) and `control_config.json` (on controller machines).

Physical or filesystem access to these nodes exposes their credentials.
Users are expected to secure the config files with appropriate filesystem permissions themselves.

**No token expiry:**
The `auth-token` is static for the lifetime of the manager config.

To rotate it, delete `manager_config.json` and restart the manager, then redistribute both values to all clients running `netbula control`.
