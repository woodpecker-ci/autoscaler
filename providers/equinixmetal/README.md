# Equinix Metal Provider

This provider enables the Woodpecker Autoscaler to deploy agents on Equinix Metal (formerly Packet) bare metal servers.

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `WOODPECKER_EQUINIXMETAL_API_TOKEN` | Yes | Equinix Metal API token |
| `WOODPECKER_EQUINIXMETAL_PROJECT_ID` | Yes | Equinix Metal project ID |
| `WOODPECKER_EQUINIXMETAL_METRO` | Yes | Equinix Metal metro (e.g. `da`, `ny`, `sv`) |
| `WOODPECKER_EQUINIXMETAL_PLAN` | No | Server plan (default: `c3.small.x86`) |
| `WOODPECKER_EQUINIXMETAL_OS` | No | Operating system (default: `ubuntu_22_04`) |
| `WOODPECKER_EQUINIXMETAL_TAGS` | No | Additional tags for servers |

### CLI Flags

| Flag | Description |
|------|-------------|
| `--equinixmetal-api-token` | Equinix Metal API token |
| `--equinixmetal-project-id` | Equinix Metal project ID |
| `--equinixmetal-metro` | Equinix Metal metro |
| `--equinixmetal-plan` | Server plan |
| `--equinixmetal-os` | Operating system |
| `--equinixmetal-tags` | Additional tags |

## Example Usage

```bash
woodpecker-autoscaler \\
  --provider equinixmetal \\
  --equinixmetal-api-token $EQUINIXMETAL_TOKEN \\
  --equinixmetal-project-id $PROJECT_ID \\
  --equinixmetal-metro da \\
  --equinixmetal-plan c3.small.x86 \\
  --equinixmetal-os ubuntu_22_04 \\
  --min-agents 1 \\
  --max-agents 10
```

## Required Permissions

The Equinix Metal API token must have the following permissions:
- Create devices
- Delete devices
- Read device list

## Metros

Common metros:
- `da` - Dallas
- `ny` - New York
- `sv` - Silicon Valley
- `am` - Amsterdam
- `fr` - Frankfurt
- `ld` - London
- `sg` - Singapore
- `sy` - Sydney

See [Equinix Metal Locations](https://deploy.equinix.com/developers/docs/metal/locations/metros/) for more.

## Plans

Common server plans:
- `c3.small.x86` - 4 cores, 8GB RAM (cheapest)
- `c3.medium.x86` - 8 cores, 16GB RAM
- `m3.large.x86` - 16 cores, 32GB RAM

See [Equinix Metal Plans](https://deploy.equinix.com/developers/docs/metal/hardware/plan-types/) for more.

## Operating Systems

Common OS options:
- `ubuntu_22_04` - Ubuntu 22.04 LTS
- `ubuntu_20_04` - Ubuntu 20.04 LTS
- `debian_12` - Debian 12
- `rocky_9` - Rocky Linux 9

See [Equinix Metal Operating Systems](https://deploy.equinix.com/developers/docs/metal/operating-systems/supported/) for more.
