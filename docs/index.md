# Hostinger Provider

The **Hostinger Terraform Provider** lets you provision and manage Virtual Private Servers (VPS) on Hostinger via Terraform.

Easily automate server provisioning, attach SSH keys, run post-install scripts, and manage compute across multiple data centers — all backed by Hostinger’s public API.

---

## Example Usage

```hcl
provider "hostinger" {
  api_token = "your_hostinger_api_token"
}

resource "hostinger_vps" "web" {
  plan              = "hostingercom-vps-kvm2-usd-1m"
  data_center_id    = 13
  template_id       = 1002
  hostname          = "example.hostinger-vps.com"
  password          = "SecurePassword123!"
}
```

---

## Resources

| Name | Description |
|------|-------------|
| `hostinger_vps` | Provision and manage a VPS instance |
| `hostinger_vps_post_install_script` | Create a reusable post-install script |
| `hostinger_vps_ssh_key` | Add and attach an SSH key to a VPS |

---

## Data Sources

| Name | Description |
|------|-------------|
| `hostinger_vps_templates` | List all available OS templates |
| `hostinger_vps_data_centers` | List all available data centers |
| `hostinger_vps_plans` | List all available VPS plans |

