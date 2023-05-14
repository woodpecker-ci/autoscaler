# Autoscaler

Scale your woodpecker agents automatically to the moon and back based on the current load.

## Usage

TBD

## Roadmap

- [ ] Add support for multiple providers
  - [x] Hetzner
  - [ ] Amazon AWS
  - [ ] Google Cloud
  - [ ] Azure
  - [ ] Digital Ocean
  - [ ] Linode
  - [ ] Oracle Cloud
  - [ ] Equinix Metal
- [ ] Cleanup agents
  - [x] Remove agents which exist on the provider but are not in the server list (they wont be able to connect to the server anyway as their is no agent token for them)
  - [x] Remove agents from server list which do not exist on the provider
  - [ ] Remove agents which have not connected for a long time
- [ ] Release as container image
- [ ] Add docs
- [ ] Support agent deployment with specific attributes (e.g. platforms, architectures, etc.)
