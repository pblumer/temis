---
name: temis-dmn-engine
version: 1.0.0
description: >
  Betrieb und Deployment von temis — der Go-basierten DMN 1.5
  Decision Engine mit FEEL-Support. Build from source, Deploy via
  Ansible, Model-Management, MCP-Nutzung.
triggers:
  - "temis deploy"
  - "temis build"
  - "temis health"
  - "temis mcp"
  - "temis model"
  - "dmn engine"
---

# temis DMN Engine — Operations Skill

## Quick Reference

| Ressource | URL |
|---|---|
| Repo | `https://github.com/pblumer/temis.git` |
| Prod HTTP API | `https://temis.blumer.cloud` |
| Prod MCP | `https://temis-mcp.blumer.cloud/mcp` |
| Deploy aus | `cloud.blumer.home` Ansible |

## Was ist temis?

- **DMN 1.5** Decision Engine in Go
- **FEEL** (Friendly Enough Expression Language) vollständig unterstützt
- Nutzbar als **Library** oder **HTTP/gRPC Service**
- Seit PR #54: MCP-Endpunkt ist in temisd co-located
- Models werden als `.dmn`-Dateien persistiert (SHA256-gehasht)

## Auth

| Layer | Mechanismus |
|---|---|
| HTTP API | Optional BasicAuth (htpasswd) + Bearer Token |
| MCP | Kein separater Auth; API-Token auf `/v1` |
| Clio Audit | `kid.secret`-Format via `vault_temis_clio_token` |

## Build (Source)

Die Ansible-Rolle baut automatisch aus dem Git-Repo:

```bash
cd /path/to/temis
go build ./cmd/temisd
```

## Deploy (Ansible)

Aus `cloud.blumer.home` Repo:

```bash
ansible-playbook -i inventory.yaml ansible/playbooks/vps.yml \
  --limit server01 --tags temis --diff --ask-vault-pass
```

**Config in** `host_vars/server01.yml`:
```yaml
temis_enabled: true
temis_domain: "temis.blumer.cloud"
temis_network: "server01_edge"
```

**Bekannter Bug:** Docker Compose V1 auf server01 →
`KeyError: 'ContainerConfig'` beim Recreate. Empfohlene Lösung:
Docker Compose V2 installieren + `temis_compose_cmd: "docker compose"`.

## Endpoints

```bash
# Health
curl -s https://temis.blumer.cloud/healthz    # 200
curl -s https://temis.blumer.cloud/readyz     # 200

# Docs UI
curl -s https://temis.blumer.cloud/docs       # 200

# MCP (JSON-RPC 2.0)
curl -s -X POST https://temis-mcp.blumer.cloud/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

## Model Management

Models werden im Container nach `/data` geschrieben
(bind-mount `/opt/temis/data`). Persistente Dateien:

```
/opt/temis/data/
  <sha256>.dmn      # Persistiertes DMN-Model
```

Upload über HTTP API (siehe `/docs`).

## Auth

| Layer | Mechanismus |
|---|---|
| HTTP API | Optional BasicAuth (htpasswd) + Bearer Token |
| MCP | Kein separater Auth; API-Token auf `/v1` |
| Clio Audit | `kid.secret`-Format via `vault_temis_clio_token` |

## MCP Nutzung (bevorzugt via `mcp-bridge`)

Das Tool `mcp-bridge` abstrahiert JSON-RPC 2.0 in einfache CLI-Befehle.
Config liegt unter `~/.mcp-servers.json`.

```bash
# Tools listen
mcp-bridge list temis

# Tool ausführen
mcp-bridge exec temis evaluate '{"model": "<sha256>", "variables": {"input": 42}}'
mcp-bridge exec temis info '{}'

# Raw JSON-RPC call
mcp-bridge call temis tools/list
mcp-bridge call temis tools/call '{"name": "evaluate", "arguments": {"model": "<sha256>"}}'
```

## MCP Nutzung (direkt / Fallback)

Falls `mcp-bridge` nicht verfügbar ist, direkt via `curl`:

```bash
# Tools listen
curl -s -X POST https://temis-mcp.blumer.cloud/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Tool aufrufen
curl -s -X POST https://temis-mcp.blumer.cloud/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "evaluate",
      "arguments": {"model": "<sha256>", "variables": {"input": 42}}
    }
  }'
```

## Clio Audit-Sink (WP-54)

Entscheidungen können als Events nach Clio gestreamt werden:

```yaml
# vault_temis_clio_token: "kid.secret"
temis_clio_url: "http://clio:7374"  # internes Docker-Netz
```

**Wichtig:** `http://clio:7374` (nicht `https://clio.blumer.cloud`),
da Hairpin-NAT sonst blockiert.

## Troubleshooting

| Problem | Lösung |
|---|---|
| `ContainerConfig` beim Deploy | docker-compose V1 Bug → V2 installieren |
| Model nicht gefunden | SHA256 prüfen; `/data` bind-mount OK? |
| MCP 404 | `temis_mcp_enabled: true` prüfen |
| Audit-Events fehlen | `temis_clio_url` auf internes Netz setzen |

## Links

- [DMN Spec 1.5](https://www.omg.org/spec/DMN/1.5/)
- [FEEL Primer](https://docs.camunda.io/docs/components/modeler/feel/what-is-feel/)
