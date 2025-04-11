# hostinger_vps

The `hostinger_vps` resource allows you to provision and manage Virtual Private Servers (VPS) on Hostinger using their public API.

It supports full lifecycle operations: create, update (hostname, template, SSH keys), and destroy (via subscription cancelation).

---

## Example Usage

```hcl
resource "hostinger_vps" "box" {
  plan              = "hostingercom-vps-kvm2-usd-1m"
  data_center_id    = 13
  template_id       = 1002
  hostname          = "web-01.example.com"
  password          = "SecureP@ssw0rd!"
  post_install_script_id = hostinger_vps_post_install_script.nginx.id
  ssh_key_ids       = [hostinger_vps_ssh_key.inline_key.id]
}
```

---

## Argument Reference

- `plan` – (Required) VPS plan identifier. Example: `hostingercom-vps-kvm2-usd-1m`.
- `data_center_id` – (Required) ID of the desired data center.
- `template_id` – (Required) OS template ID. Example: `1002` for Debian 11.
- `password` – (Optional, Sensitive) Root password. If not set, one will be auto-generated.
- `hostname` – (Optional) Fully Qualified Domain Name (FQDN). If not set, one will be auto-generated.
- `payment_method_id` – (Optional) Hostinger Payment Method ID. If not set, default will be used.
- `post_install_script_id` – (Optional) ID of a reusable script to run after provisioning.
- `ssh_key_ids` – (Optional) List of public SSH key IDs to attach to the VPS.

---

## Attributes Reference

- `id` – Internal Hostinger VPS ID.
- `ipv4_address` – Public IPv4 address of the VPS.
- `ipv6_address` – Public IPv6 address of the VPS.
- `status` – Status of the VPS provisioning.
- `vps_id` – Same as `id`, retained for convenience and backward compatibility.

