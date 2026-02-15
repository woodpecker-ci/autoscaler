# DigitalOcean Provider

This provider enables the Woodpecker Autoscaler to deploy agents on DigitalOcean Droplets.

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `WOODPECKER_DIGITALOCEAN_TOKEN` | Yes | DigitalOcean API token |
| `WOODPECKER_DIGITALOCEAN_REGION` | Yes | DigitalOcean region (e.g. `nyc1`, `nyc3`, `sfo3`) |
| `WOODPECKER_DIGITALOCEAN_INSTANCE_TYPE` | No | Droplet size slug (default: `s-1vcpu-1gb`) |
| `WOODPECKER_DIGITALOCEAN_IMAGE` | No | Droplet image (default: `ubuntu-22-04-x64`) |
| `WOODPECKER_DIGITALOCEAN_SSH_KEYS` | No | SSH key fingerprints to add to droplets |
| `WOODPECKER_DIGITALOCEAN_TAGS` | No | Additional tags for droplets |

### CLI Flags

| Flag | Description |
|------|-------------|
| `--digitalocean-token` | DigitalOcean API token |
| `--digitalocean-region` | DigitalOcean region |
| `--digitalocean-instance-type` | Droplet size slug |
| `--digitalocean-image` | Droplet image |
| `--digitalocean-ssh-keys` | SSH key fingerprints |
| `--digitalocean-tags` | Additional tags |

## Example Usage

```bash
woodpecker-autoscaler \\
  --provider digitalocean \\
  --digitalocean-token $DIGITALOCEAN_TOKEN \\
  --digitalocean-region nyc1 \\
  --digitalocean-instance-type s-1vcpu-1gb \\
  --digitalocean-image ubuntu-22-04-x64 \\
  --min-agents 1 \\
  --max-agents 10
```

## Required Permissions

The DigitalOcean API token must have the following permissions:
- Droplet: Create, Read, Delete
- Tags: Create, Read

## Regions

Common regions:
- `nyc1`, `nyc3` - New York
- `sfo3` - San Francisco
- `ams3` - Amsterdam
- `sgp1` - Singapore
- `lon1` - London

See [DigitalOcean Regions](https://docs.digitalocean.com/products/platform/availability-matrix/) for more.

## Instance Types

Common droplet sizes:
- `s-1vcpu-1gb` - 1 vCPU, 1 GB RAM ($6/month)
- `s-1vcpu-2gb` - 1 vCPU, 2 GB RAM ($12/month)
- `s-2vcpu-2gb` - 2 vCPU, 2 GB RAM ($18/month)
- `s-2vcpu-4gb` - 2 vCPU, 4 GB RAM ($24/month)

See [DigitalOcean Droplet Pricing](https://www.digitalocean.com/pricing/droplets) for more.

## Images

Common images:
- `ubuntu-22-04-x64` - Ubuntu 22.04 LTS
- `ubuntu-20-04-x64` - Ubuntu 20.04 LTS
- `debian-12-x64` - Debian 12
- `fedora-39-x64` - Fedora 39

See [DigitalOcean Images](https://docs.digitalocean.com/products/images/) for more.
