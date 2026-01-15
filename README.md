# Vouch: The AI Agent Black Box

Vouch is a safety-critical flight recorder for AI agents. It intercepts every tool interaction, signs it cryptographically, and persists an immutable audit trail that prevents agents from deleting their tracks if compromised.

---

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)](https://github.com/slyt3/Vouch/actions)

---

## The Problem
As AI agents evolve from simple chat-bots to autonomous action-bots, they are given the power to delete files, move money, and modify production infrastructure. Standard logging is fundamentally broken for this new era:
1. **Lacks Integrity**: If an agent is compromised, it can delete the very logs that explain its behavior.
2. **Missing Context**: Standard logs capture what happened but miss the raw payload and cryptographic proof needed for legal forensics.
3. **No Safety-Brake**: Traditional logs are reactive. They tell you why the system died after the agent deleted the production database.

---

## The Solution
Vouch acts as a transparent interceptor. It sits between your agent (Claude, GPT, AutoGPT) and its tools (CLI, SQL, APIs). Before an action is executed, Vouch records it, verifies it against policy, and signs it.

```text
 [ AI Agent ] <--- (Tool Call) ---> [ Vouch Proxy ] <--- (Execution) ---> [ Tools / APIs ]
                                         |
                                 [ Signed Ledger ]
                                         |
                                 [ SHA-256 Chain ]
```

### Key Features
*   Immutable Ledger: Append-only SQLite store with SHA-256 hash chaining (blockchain-style).
*   Human-in-the-Loop: Stall risky actions (e.g., db.drop) until a human verifies and approves.
*   Cryptographic Proof: Every event is signed with Ed25519 (Hardware-backed support available).
*   Zero-Overhead: High-performance memory pooling ensures < 1.8ms latency impact.
*   Forensic CLI: Reconstruct the agent's complex reasoning chain and link task dependencies.

---

## Quick Start (Demo)

### 1. Installation
```bash
go install github.com/slyt3/Vouch@latest
```

### 2. Configure Safety Policies
Define which tools are risky in vouch-policy.yaml.
```yaml
policies:
  - id: "prevent-deletion"
    match_methods: ["os.remove", "db.drop_root"]
    action: "stall" # Pause the agent and wait for human
    risk_level: "critical"
```

### 3. Start the Proxy
```bash
vouch-proxy --upstream https://api.anthropic.com --port 9999
```

### 4. Interactive Approval
If your agent tries to delete something, Vouch will stop it. You approve it here:
```bash
vouch-cli approve <event_id>
```

---

## How It Works

### 1. Interception Layer
Vouch implements a Transparent Proxy using Go's httputil.ReverseProxy. It synchronously inspects incoming JSON-RPC traffic. It matches the method against your safety policy before the request ever hits the tool.

### 2. Immutable Ledger
Events are stored in a SHA-256 Linked Chain. Every event (Block N) includes the hash of (Block N-1). This ensures that even a single-bit modification to historical logs will invalidate the entire cryptographic chain.

### 3. Policy Engine
The engine evaluates requests in real-time. Action types include:
- allow: Log and forward immediately.
- stall: Block request, alert admin, and wait for cryptographic approval.
- redact: Scrub PII (emails, keys) before persisting to the ledger.

---

## Configuration
The vouch-policy.yaml allows granular control over agent capabilities.

```yaml
# Safety Core Configuration
policies:
  - id: "audit-all"
    match_methods: ["*"] # Log everything
    action: "allow"
    
  - id: "protect-database"
    match_methods: ["sql.execute", "db.query"]
    action: "stall"
    risk_level: "critical"
```

---

## Verification and Forensics

```bash
# Verify the entire cryptographic chain integrity
vouch-cli verify

# View real-time statistics
vouch-cli stats

# Export the forensic audit trail for legal review
vouch-cli export --format json --output audit_trail.json
```

---

## Architecture Deep-Dive

### Cryptographic Chain Design
Vouch uses RFC 8785 (JSON Canonicalization Scheme) to normalize payloads before hashing. This ensures that the hash remains identical regardless of key order, protecting against signature drift when logs are exported between different JSON libraries.

### SEP-1686 Task State Protocol
Vouch tracks agent lifecycle states (working -> stalled -> completed/failed). This allows you to group atomic tool actions into a single high-level Objective, making it easy to see exactly why an agent performed a specific action.

### Stall Intelligence
When a policy triggers a stall, Vouch doesn't just block; it captures the parent_id and task_id. This provides the human approver with the full stack trace of the agent's reasoning chain before they hit Approve.

### PII Redaction
The interceptor performs In-Flight Redaction. If a tool response contains sensitive tokens or emails, Vouch scrubs the data before it is signed and persisted to the local SQLite database.

---

## Performance
Optimized for zero-impact on agent latency.
- Latency Overhead: < 1.8ms per request (local loopback).
- Throughput: ~5,200 events/sec (disk-limited).
- Memory Footprint: < 18MB RSS.

---

## Integration Examples

### Claude / Anthropic
```python
# Simply route your tool calls through the Vouch Proxy
client = anthropic.Client(api_key="...", base_url="http://localhost:9999")
```

### LangChain
```python
from langchain_openai import ChatOpenAI
# secure your agent with one line
llm = ChatOpenAI(openai_api_base="http://localhost:9999/v1")
```

---

## Use Cases
- Financial Services: Trading bot oversight and compliance for automated fund transfers.
- Infrastructure: Prevent hallucinating agents from accidentally deleting AWS/GCP clusters.
- Healthcare: HIPAA-compliant audit trails for AI-assisted clinical diagnosis.
- Threat Hunting: Incident response forensics—see exactly what a remediation agent changed.

---

## Why Not Just Use Alternative?

| Feature | Vouch | LangSmith / LangFuse | Standard Logs |
| :--- | :--- | :--- | :--- |
| **Tamper-Proof** | Yes (Crypto Chain) | No (Cloud-Managed) | No |
| **Blocking Policy**| Yes (Human Stall) | No (Reactive only) | No |
| **Air-Gap Capable** | Yes (Local DB) | No | Yes |
| **Latency** | < 2ms | Network Dependent | Variable |

---

## Common Questions

**Q: Does this slow down my agent?**  
A: No. By utilizing memory pooling (sync.Pool) and a decoupled async ledger worker, the overhead is typically less than 2ms—which is a rounding error for most LLM latencies.

**Q: Can agents bypass this?**  
A: No. Vouch acts as a transparent network gateway. If the agent wants to talk to a tool, it MUST pass through the proxy, which records the raw network bytes.

**Q: What if the ledger fills up?**  
A: Vouch supports periodic snapshots and Cold Storage export, allowing you to move signed ledger segments to long-term storage while maintaining the hash integrity.

---

## Roadmap
- [ ] WebAssembly (Wasm) policy plugins
- [ ] Multi-agent orchestration and linked chains
- [ ] Behavioral anomaly detection (using local ML)
- [ ] Decentralized ledger backup (IPFS/Arweave)

---

## License
[Apache 2.0](LICENSE)

---

## Citation
If you use Vouch in your research or at your company:
```bibtex
@software{vouch2025,
  title = {Vouch: Safety-Critical AI Agent Accountability},
  author = {slyt3},
  year = {2025},
  url = {https://github.com/slyt3/Vouch}
}
```
