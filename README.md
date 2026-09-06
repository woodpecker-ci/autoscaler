# Autoscaler

Scale your woodpecker agents automatically to the moon and back based on the current load.

## Usage

If you are using docker-compose you can add the following to your `docker-compose.yml` file:

```yml
# docker-compose.yml
version: '3'

services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:next
    [...]

  woodpecker-autoscaler:
    image: woodpeckerci/autoscaler:next
    restart: always
    depends_on:
      - woodpecker-server
    environment:
      - WOODPECKER_SERVER=https://your-woodpecker-server.tld # the url of your woodpecker server / could also be a public url
      - WOODPECKER_TOKEN=${WOODPECKER_TOKEN} # the Personal Access Token you can get from the UI https://your-woodpecker-server.tld/user/cli-and-api
      - WOODPECKER_MIN_AGENTS=0
      - WOODPECKER_MAX_AGENTS=3
      - WOODPECKER_WORKFLOWS_PER_AGENT=2 # the number of workflows each agent can run at the same time
      - WOODPECKER_GRPC_ADDR=https://grpc.your-woodpecker-server.tld # the grpc address of your woodpecker server, publicly accessible from the agents
      - WOODPECKER_GRPC_SECURE=true
      - WOODPECKER_AGENT_ENV= # optional environment variables to pass to the agents
      - WOODPECKER_PROVIDER=hetznercloud # set the provider, you can find all the available ones down below
      - WOODPECKER_HETZNERCLOUD_API_TOKEN=${WOODPECKER_HETZNERCLOUD_API_TOKEN} # your api token for the Hetzner cloud
```

The agents will use `WOODPECKER_GRPC_ADDR` and an agent token automatically created on the server by the autoscaler to connect to the server. Therefore the `WOODPECKER_GRPC_ADDR` has to be publicly accessible from the newly created agents. Check for example how you could use [caddy](https://woodpecker-ci.org/docs/administration/configuration/server#caddy) to expose the grpc connection.

## Equinix Metal

Set `WOODPECKER_PROVIDER=equinixmetal` and configure at least:

- `WOODPECKER_EQUINIXMETAL_API_TOKEN`
- `WOODPECKER_EQUINIXMETAL_PROJECT_ID`
- `WOODPECKER_EQUINIXMETAL_PLAN`
- exactly one of `WOODPECKER_EQUINIXMETAL_METRO` or `WOODPECKER_EQUINIXMETAL_FACILITY`

Equinix Metal support is currently experimental: it has not been tested by the project maintainers, as none of them have real provider access.

Useful optional settings:

- `WOODPECKER_EQUINIXMETAL_OPERATING_SYSTEM` (default: `ubuntu_24_04`)
- `WOODPECKER_EQUINIXMETAL_BILLING_CYCLE` (default: `hourly`)
- `WOODPECKER_EQUINIXMETAL_TAGS`
- `WOODPECKER_EQUINIXMETAL_PROJECT_SSH_KEYS`
- `WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE`
- `WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX`

## DigitalOcean

Set `WOODPECKER_PROVIDER=digitalocean` and configure at least:

- `WOODPECKER_DIGITALOCEAN_API_TOKEN` (or `WOODPECKER_DIGITALOCEAN_API_TOKEN_FILE`)

DigitalOcean support is currently experimental: it has not been tested by the project maintainers, as none of them have real provider access.

Useful optional settings:

- `WOODPECKER_DIGITALOCEAN_REGION` (default: `nyc1`)
- `WOODPECKER_DIGITALOCEAN_SIZE` (default: `s-1vcpu-1gb`)
- `WOODPECKER_DIGITALOCEAN_IMAGE` (default: `ubuntu-24-04-x64`, slug or name)
- `WOODPECKER_DIGITALOCEAN_SSH_KEYS` (names or fingerprints; if unset, a key named `random-autoscaler-key` is created and reused, its private key is discarded)
- `WOODPECKER_DIGITALOCEAN_TAGS`
- `WOODPECKER_DIGITALOCEAN_PUBLIC_IPV4_ENABLE` (default: `true`; set to `false` to create private droplets without a public network interface, which requires `WOODPECKER_DIGITALOCEAN_NAT_GATEWAY`)
- `WOODPECKER_DIGITALOCEAN_NAT_GATEWAY` (name or ID of an existing [VPC NAT gateway](https://docs.digitalocean.com/products/vpc-nat-gateway/); the agents are placed in the VPC it serves as default gateway so they can reach the server and pull images)
- `WOODPECKER_DIGITALOCEAN_PUBLIC_IPV6_ENABLE` (default: `true`, requires public IPv4)

## OpenStack

Set `WOODPECKER_PROVIDER=openstack`. The prefix for all the following environment variables is `WOODPECKER_OPENSTACK_`.

You have to supply the `AUTH_URL` pointing to your Keystone. If necessary, you can also specifiy the `DOMAIN_NAME`, `REGION` and `PROJECT_NAME`.

Both `USERNAME`/`PASSWORD` authentication and application credentials via `APPLICATION_CREDENTIAL_ID` and `APPLICATION_CREDENTIAL_SECRET` are supported.
Credentials can also be read from files, to do so append `_FILE` to the appropriate variable name and set it to the file path.

You can select the flavor and image for the agent instances via `FLAVOR/IMAGE_NAME` or UUID reference (`FLAVOR/IMAGE_REF`).
If you set `VOLUME_SIZE`, block storage volumes are used.

You can add your OpenStack SSH keypair via `KEYPAIR`.

## Yandex Cloud

Set `WOODPECKER_PROVIDER=yandexcloud` and configure at least:

- `WOODPECKER_YANDEXCLOUD_FOLDER_ID`
- `WOODPECKER_YANDEXCLOUD_SUBNET_ID`
- exactly one of `WOODPECKER_YANDEXCLOUD_IMAGE_ID` or `WOODPECKER_YANDEXCLOUD_IMAGE_FAMILY`
- exactly one credential mode:
  - `WOODPECKER_YANDEXCLOUD_SERVICE_ACCOUNT_KEY_FILE` (path to an authorized key JSON file)
  - `WOODPECKER_YANDEXCLOUD_IAM_TOKEN` (or `WOODPECKER_YANDEXCLOUD_IAM_TOKEN_FILE`)
  - `WOODPECKER_YANDEXCLOUD_USE_INSTANCE_SERVICE_ACCOUNT=true` when the autoscaler runs on a Yandex Cloud VM

The provider creates one Compute Cloud VM per agent, waits for create/delete operations to finish, and uses the `wp.autoscaler/pool` label to isolate pools. It uses per-second billing behavior.

Useful optional settings:

- `WOODPECKER_YANDEXCLOUD_IMAGE_FOLDER_ID` (default: `standard-images`, used with image families)
- `WOODPECKER_YANDEXCLOUD_PLATFORM_ID` (default: `standard-v3`)
- `WOODPECKER_YANDEXCLOUD_CORES` (default: `2`)
- `WOODPECKER_YANDEXCLOUD_MEMORY` (default: `4GiB`)
- `WOODPECKER_YANDEXCLOUD_CORE_FRACTION` (default: `100`)
- `WOODPECKER_YANDEXCLOUD_DISK_TYPE` (default: `network-hdd`)
- `WOODPECKER_YANDEXCLOUD_DISK_SIZE` (default: `20GiB`)
- `WOODPECKER_YANDEXCLOUD_SECURITY_GROUP_IDS`
- `WOODPECKER_YANDEXCLOUD_PUBLIC_IPV4_ENABLE` (default: `true`; when disabled, the subnet must provide egress through NAT or another route)
- `WOODPECKER_YANDEXCLOUD_LABELS` (key=value pairs)
- `WOODPECKER_YANDEXCLOUD_OPERATION_TIMEOUT` (default: `5m`)

The service account needs `compute.editor` on the folder and access to the selected image. Creating VMs with public IPv4 additionally requires the corresponding VPC public address permissions. If public IPv4 is disabled, agents still need a route to the Woodpecker gRPC endpoint and container registries.

Yandex Cloud exposes VM metadata at `169.254.169.254`. The provider places a route to that address in the default cloud-init bootstrap before the agent container starts so jobs cannot read the agent token from metadata. Custom cloud-init templates must render and execute `.PreExec`, as the built-in template does.

Yandex Cloud support is currently experimental and should be validated against a real folder before production use.

## Teardown policy

How idle agents are torn down depends on how the selected provider bills:

- **Per-second billing** (e.g. AWS, Scaleway): an idle agent is drained and removed once it has been idle for `WOODPECKER_AGENT_IDLE_TIMEOUT`. Holding an idle agent open buys nothing.
- **Hourly-rounded-up billing** (e.g. Linode, Hetzner Cloud, Vultr): a partial hour costs the same as a full one, so an idle agent is kept schedulable for the rest of the hour that has already been paid for and is only torn down just before its next hour boundary (anchored at its creation time). A busy agent simply rolls into the next paid hour; you never pay for an idle hour.

  The teardown window is `WOODPECKER_AGENT_BILLING_TEARDOWN_MARGIN` (default `2m`) plus `WOODPECKER_RECONCILIATION_INTERVAL`, so a reconciliation can never tick straight past the boundary. With the defaults (`2m` margin, `1m` interval) an idle agent becomes eligible for teardown in the last 3 minutes of each paid hour.

The billing model is selected automatically by the provider, so no extra configuration is required to benefit from this.

## Roadmap

- [ ] Add support for multiple providers
  - [x] Hetzner Cloud
  - [x] Amazon AWS
  - [ ] Google Cloud
  - [ ] Azure
  - [x] Digital Ocean **[experimental]** (untested by the maintainers against real provider access, see [above](#digitalocean))
  - [x] Linode
  - [x] OpenStack **[experimental]**
  - [x] Yandex Cloud **[experimental]**
  - [ ] Oracle Cloud
  - [x] Equinix Metal **[experimental]** (untested by the maintainers against real provider access, see [above](#equinix-metal))
  - [x] Vultr
  - [x] Scaleway
- [ ] Cleanup agents
  - [x] Remove agents which exist on the provider but are not in the server list (they wont be able to connect to the server anyway as their is no agent token for them)
  - [x] Remove agents from server list which do not exist on the provider
  - [ ] Remove agents which have not connected for a long time
- [x] Release as container image
- [x] Add docs
- [ ] Support agent deployment with specific attributes (e.g. platforms, architectures, etc.)
