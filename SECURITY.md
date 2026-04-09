# Security

NVIDIA is dedicated to the security and trust of our software products and services, including all source code repositories.

Please do not report security vulnerabilities through GitHub.

## Reporting Security Vulnerabilities

To report a potential security vulnerability in any NVIDIA product:

- Web: [Security Vulnerability Submission Form](https://www.nvidia.com/en-us/security/)
- Email: psirt@nvidia.com
  - Use [NVIDIA PGP Key](https://www.nvidia.com/en-us/security/pgp-key/) for secure communication

Include in your report:

- Product name and version
- Type of vulnerability
- Affected deployment model or operating system
- Steps to reproduce
- Proof-of-concept or exploit code, if available
- Potential impact and exploitation method

NVIDIA offers acknowledgement for externally reported security issues under our coordinated vulnerability disclosure policy. Visit [PSIRT Policies](https://www.nvidia.com/en-us/security/psirt/) for details.

## Product Security Resources

For all security-related concerns: <https://www.nvidia.com/en-us/security/>

## Project Security Notes

This repository contains the `fleetint` host agent. The notes below are intended to help maintainers and automated reviewers understand the primary attack surface and security-sensitive code paths in this project.

### Key Security Assumptions

- The local HTTP API is intended to be reachable only from the local host unless an operator explicitly changes the listen address.
- Backend communication is expected to use HTTPS endpoints.
- The local state directory is trusted local storage and must remain readable only by the local service account or root.
- The host running the agent is part of the trusted computing base. A local privileged attacker can generally bypass the agent's own controls.

### Security-Sensitive Code Locations

- Enrollment CLI and persisted credentials: [`cmd/fleetint/enroll.go`](cmd/fleetint/enroll.go)
- Metadata inspection and updates: [`cmd/fleetint/metadata.go`](cmd/fleetint/metadata.go)
- Default bind address and filesystem permissions: [`internal/config/default.go`](internal/config/default.go)
- Backend and local URL validation: [`internal/endpoint/endpoint.go`](internal/endpoint/endpoint.go)
- Enrollment HTTP flow: [`internal/enrollment/enrollment.go`](internal/enrollment/enrollment.go)
- Exporter token refresh and endpoint reload: [`internal/exporter/exporter.go`](internal/exporter/exporter.go)
- Local HTTP server routes: [`internal/server/server.go`](internal/server/server.go)
- Optional fault injection handler: [`internal/server/handlers_inject_fault.go`](internal/server/handlers_inject_fault.go)
- Remote attestation and `nvattest` invocation: [`internal/attestation/attestation.go`](internal/attestation/attestation.go)

### Threat Model

In scope:

- Exposure of the local HTTP API beyond loopback
- Mishandling of stored enrollment credentials or refreshed JWTs
- SSRF-style abuse through attacker-controlled backend or nonce endpoints
- Unsafe subprocess execution during attestation
- Fault injection features being enabled or exposed unexpectedly

Out of scope:

- Full compromise by a privileged local attacker
- Vulnerabilities in third-party services operated outside this repository
- Kernel, driver, firmware, or hardware issues outside the agent's control

### Trust Boundaries

- Local API boundary: the agent exposes local HTTP endpoints for metrics, health, state, event, and info access.
- Backend boundary: enrollment, metrics export, log export, and nonce retrieval cross from the local host to remote NVIDIA-managed or operator-configured services.
- Local persistence boundary: tokens and endpoint metadata are stored in the local SQLite-backed state directory.
- Subprocess boundary: attestation shells out to the external `nvattest` binary and treats its output as security-relevant evidence.

### Authentication Detail

- Initial enrollment sends the SAK token as a bearer token to the configured enrollment endpoint.
- Successful enrollment persists the SAK token, JWT, and service endpoints for later reuse.
- The exporter reloads persisted metadata, validates remote endpoints, and can re-enroll with the stored SAK token when JWT refresh is required.
- The local HTTP server does not implement request authentication. Its primary protection is the default loopback bind address.
- The fault injection endpoint is disabled by default and additionally checks that requests originate from loopback.

### Input Validation

- Remote backend, metrics, logs, and nonce endpoints are validated before use; backend URLs must use HTTPS.
- Local server URLs are constrained to loopback and reject userinfo, query, fragment, and non-empty path components.
- Metadata displayed via CLI redacts sensitive token values before printing.
- Reviewers should pay particular attention to any future changes that broaden accepted endpoint formats, alter state-file permissions, or expose the local API on non-loopback interfaces by default.

### Security-Sensitive Dependencies

- `nvattest` is executed as an external binary and should be treated as part of the trusted runtime for attestation flows.
- SQLite state storage contains enrollment material and should be protected by host filesystem permissions.
- TLS validation for remote endpoints depends on the host trust store and any operator-provided CA configuration.
