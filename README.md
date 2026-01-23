# Vouch: The AI Agent Flight Recorder

> **"If it isnt in the ledger, it didnt happen."**

hit the star if you like the repo ⭐️

Vouch is a **forensic-grade flight recorder** for autonomous AI agents. It passively captures tool execution, cryptographically signs every action, and maintains an immutable, tamper-evident audit trail.

---

## Quick Start

### 1. Build
```bash
go build -o vouch main.go
go build -o vouch-cli cmd/vouch-cli/main.go
```

### 2. Start Recording
```bash
./vouch --target http://localhost:8080 --port 9999
```

### 3. Investigate
```bash
./vouch-cli trace    # Reconstruct timelines
./vouch-cli verify   # Prove integrity
./vouch-cli export   # Sealed evidence bag
```

---

## Why Vouch?

*   **Immutable**: SQLite ledger with SHA-256 chaining. If a single byte is altered, verification fails.
*   **Cryptographic Proof**: Every event is signed with an internal Ed25519 key—proving the record came from Vouch.
*   **Forensic Ready**: Meets [FRE 902(13)](https://www.law.cornell.edu/rules/fre/rule_902) standards for self-authenticating electronic records.
*   **Bitcoin Anchored**: Genesis blocks and periodic state are anchored to the Bitcoin blockchain for external proof-of-existence.
*   **Dynamic Policies**: Hot-reload security rules from `vouch-policy.yaml` without restarting the server.
*   **Production Ready**: Prometheus metrics endpoint with queue depth and latency histograms.
*   **High Performance**: < 2ms overhead with zero-allocation memory pools.

---

## Documentation

You do **not** need all docs to use Vouch. If you only want to record and inspect events locally, the Quick Start above is enough. The guides below are optional and scoped to specific audiences:

- **[ARCHITECTURE.md](ARCHITECTURE.md)**: System design, diagrams, and packet flow (for contributors and reviewers).
- **[INVESTIGATOR_GUIDE.md](INVESTIGATOR_GUIDE.md)**: Incident response workflow (for security/forensics teams).
- **[CLOUD_DEPLOYMENT.md](CLOUD_DEPLOYMENT.md)**: Docker and production ops (for SRE/DevOps).
- **[CONTRIBUTING.md](CONTRIBUTING.md)**: Development workflow and safety standards (for contributors).
- **[Examples](examples/scenario/README.md)**: Live "Rogue Agent" investigation scenario (for demos).

---

## License
Apache 2.0
