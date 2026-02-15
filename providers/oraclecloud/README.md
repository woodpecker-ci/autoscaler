# Oracle Cloud Provider

This provider enables the Woodpecker Autoscaler to deploy agents on Oracle Cloud Infrastructure (OCI) compute instances.

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `WOODPECKER_ORACLECLOUD_TENANCY_OCID` | Yes | Oracle Cloud tenancy OCID |
| `WOODPECKER_ORACLECLOUD_USER_OCID` | Yes | Oracle Cloud user OCID |
| `WOODPECKER_ORACLECLOUD_FINGERPRINT` | Yes | API key fingerprint |
| `WOODPECKER_ORACLECLOUD_PRIVATE_KEY` | Yes | Path to API private key file |
| `WOODPECKER_ORACLECLOUD_REGION` | Yes | Oracle Cloud region (e.g. `us-ashburn-1`) |
| `WOODPECKER_ORACLECLOUD_COMPARTMENT_OCID` | Yes | Compartment OCID for instances |
| `WOODPECKER_ORACLECLOUD_SHAPE` | No | Instance shape (default: `VM.Standard.E2.1`) |
| `WOODPECKER_ORACLECLOUD_IMAGE_OCID` | No | Image OCID |
| `WOODPECKER_ORACLECLOUD_SUBNET_OCIDS` | No | Subnet OCIDs |
| `WOODPECKER_ORACLECLOUD_SSH_KEYS` | No | SSH public keys |

### CLI Flags

| Flag | Description |
|------|-------------|
| `--oraclecloud-tenancy-ocid` | Tenancy OCID |
| `--oraclecloud-user-ocid` | User OCID |
| `--oraclecloud-fingerprint` | API key fingerprint |
| `--oraclecloud-private-key` | Private key file path |
| `--oraclecloud-region` | Region |
| `--oraclecloud-compartment-ocid` | Compartment OCID |
| `--oraclecloud-shape` | Instance shape |
| `--oraclecloud-image-ocid` | Image OCID |
| `--oraclecloud-subnet-ocids` | Subnet OCIDs |
| `--oraclecloud-ssh-keys` | SSH keys |

## Example Usage

```bash
woodpecker-autoscaler \\
  --provider oraclecloud \\
  --oraclecloud-tenancy-ocid $TENANCY_OCID \\
  --oraclecloud-user-ocid $USER_OCID \\
  --oraclecloud-fingerprint $FINGERPRINT \\
  --oraclecloud-private-key /path/to/private.key \\
  --oraclecloud-region us-ashburn-1 \\
  --oraclecloud-compartment-ocid $COMPARTMENT_OCID \\
  --oraclecloud-shape VM.Standard.E2.1 \\
  --oraclecloud-subnet-ocids $SUBNET_OCID \\
  --min-agents 1 \\
  --max-agents 10
```

## Setup

### 1. Create API Key

```bash
openssl genrsa -out ~/.oci/oci_api_key.pem 2048
chmod 600 ~/.oci/oci_api_key.pem
openssl rsa -pubout -in ~/.oci/oci_api_key.pem -out ~/.oci/oci_api_key_public.pem
```

### 2. Get Key Fingerprint

```bash
openssl rsa -pubout -outform DER -in ~/.oci/oci_api_key.pem | openssl md5 -c
```

### 3. Upload Public Key to OCI Console

Navigate to Identity > Users > Your User > API Keys and add the public key.

### 4. Get Required OCIDs

- **Tenancy OCID**: Profile > Tenancy
- **User OCID**: Identity > Users > Your User
- **Compartment OCID**: Identity > Compartments
- **Subnet OCID**: Networking > Virtual Cloud Networks > Subnets
- **Image OCID**: Compute > Images

## Regions

Common regions:
- `us-ashburn-1` - Ashburn, VA
- `us-phoenix-1` - Phoenix, AZ
- `eu-frankfurt-1` - Frankfurt
- `uk-london-1` - London
- `ap-tokyo-1` - Tokyo

See [Oracle Cloud Regions](https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm) for more.

## Shapes

Common shapes:
- `VM.Standard.E2.1` - 1 OCPU, 1 GB RAM
- `VM.Standard.E2.2` - 2 OCPUs, 2 GB RAM
- `VM.Standard.E3.Flex` - Flexible compute
- `VM.Standard.A1.Flex` - ARM-based compute

See [Oracle Cloud Shapes](https://docs.oracle.com/en-us/iaas/Content/Compute/References/computeshapes.htm) for more.
