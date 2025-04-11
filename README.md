# Terraform Provider: Hostinger VPS

Provision and manage Hostinger VPS instances effortlessly using Terraform.

This provider integrates with Hostinger's Public API to manage VPS servers, post-install scripts, and SSH keys. It supports in-place updates, advanced validation, and optional configuration for security and automation.

> ⚡ Production-ready and developer-focused — built to meet the standards of modern cloud provisioning.

---

## Features

- ✅ Create and delete VPS servers
- 🔄 In-place updates for hostname, SSH keys, OS template
- 🔐 Attach SSH keys (inline or existing)
- 📜 Upload post-install scripts
- 🔎 Validate `plan`, `template_id`, and `data_center_id` before provisioning
- 🧠 Auto-detect default payment method
- 💥 Cancellation triggers actual subscription deletion via Hostinger Billing API

---

## Requirements

- Terraform ≥ 1.3.0
- Hostinger API Token (bearer)

---

## Installation

Until released to the Terraform Registry, install the provider locally:

```bash
mkdir -p ~/.terraform.d/plugins/local/hostinger/hostinger/0.1.0/darwin_arm64
mv terraform-provider-hostinger ~/.terraform.d/plugins/local/hostinger/hostinger/0.1.0/darwin_arm64/terraform-provider-hostinger_v0.1.0
```

---

## Example Usage

```hcl
terraform {
  required_providers {
    hostinger = {
      source  = "local/hostinger/hostinger"
      version = "0.1.0"
    }
  }
}

provider "hostinger" {
  api_token = "YOUR_SECRET_TOKEN"
}

resource "hostinger_vps_ssh_key" "inline_key" {
  name = "My SSH Key"
  key  = "ssh-rsa AAAAB3... user@host"
}

resource "hostinger_vps_post_install_script" "nginx" {
  name    = "NGINX Script"
  content = <<-EOT
    #!/bin/bash
    apt-get update
    apt-get install -y nginx
  EOT
}

resource "hostinger_vps" "web" {
  plan                  = "hostingercom-vps-kvm2-usd-1m"
  data_center_id        = 13
  template_id           = 1002
  hostname              = "web01.example.com"
  ssh_key_ids           = [hostinger_vps_ssh_key.inline_key.id]
  post_install_script_id = hostinger_vps_post_install_script.nginx.id
}

output "vps_ip" {
  value = hostinger_vps.web.ipv4_address
}
```

---

## Data Sources

```hcl
data "hostinger_vps_templates" "all" {}
data "hostinger_vps_data_centers" "all" {}
data "hostinger_vps_plans" "all" {}

output "available_templates" {
  value = data.hostinger_vps_templates.all.templates
}
```

---

## Resource Types

- `hostinger_vps` — Provisions a VPS
- `hostinger_vps_ssh_key` — Manages account-level SSH keys
- `hostinger_vps_post_install_script` — Upload post-install scripts

---

## Environment Variables

- `HOSTINGER_API_TOKEN` *(optional)* – Alternative to passing `api_token` in the provider block.

---

## Development

This provider uses the legacy Terraform SDK (v2) for now, and may be upgraded to Terraform Plugin Framework in future versions.

To build:

```bash
go build -o terraform-provider-hostinger
```

To test locally:

```bash
terraform init
terraform plan
terraform apply
```

---

## Contributing

Pull requests and issues are welcome. We aim to build the most user-friendly Hostinger provider on the Terraform Registry.

---

## License

[MIT](LICENSE)

---

## Maintained by

**Hostinger Engineering**
