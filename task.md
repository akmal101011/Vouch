# Phase 3: Policy Guard - CLI Tool Implementation

## CLI Infrastructure
- [/] Create `cmd/ael-cli/main.go` with command structure
- [ ] Implement subcommand routing (approve, verify, status)
- [ ] Add flag parsing for common options

## Approval System
- [ ] Implement `ael approve <event-id>` command
- [ ] Add HTTP endpoint in proxy for approval
- [ ] Support approval via HTTP endpoint
- [ ] Add `ael reject <event-id>` command

## Verification Commands
- [/] Implement `ael verify` to validate hash chain
- [ ] Add `ael status` to show current run info
- [ ] Implement `ael events` to list recent events

## Integration
- [ ] Update proxy to expose approval endpoint
- [ ] Test approval flow end-to-end
- [ ] Update documentation with CLI usage
