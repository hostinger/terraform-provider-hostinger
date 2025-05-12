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
- 🌐 Manage Domain DNS zone: add, update, and remove DNS records

---

## Requirements

- Terraform ≥ 1.3.0
- Hostinger API Token (bearer) - obtainable from your Hostinger account under the API section
- A valid payment method added to your Hostinger account — either Google Pay (Credit Card) or PayPal

---

## Installation

In your terraform config, define `hostinger/hostinger` in your required_providers and set your API key:

```hcl
terraform {
  required_providers {
    hostinger = {
      source = "hostinger/hostinger"
      version = "0.1.6"
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

If you encounter any bugs or unexpected behavior, please [open an issue](https://github.com/hostinger/terraform-provider-hostinger/issues).  
Our team actively monitors reports and strives to address them promptly to ensure a stable and reliable experience for all users.

---

## License

[MPL-2.0](LICENSE)

---

## Maintained by

**Hostinger Engineering**
