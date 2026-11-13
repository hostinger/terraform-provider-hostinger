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

- `plan` – (Optional) VPS plan identifier. Example: `hostingercom-vps-kvm2-usd-1m`. Required to purchase a new VPS, and changing it replaces a VPS that Terraform purchased. Ignored for an imported VPS.
- `data_center_id` – (Required) ID of the desired data center.
- `template_id` – (Required) OS template ID. Example: `1002` for Debian 11.
- `password` – (Optional, Sensitive) Root password. If not set, one will be auto-generated.
- `hostname` – (Optional) Fully Qualified Domain Name (FQDN). If not set, one will be auto-generated.
- `payment_method_id` – (Optional) Hostinger Payment Method ID. If not set, default will be used.
- `post_install_script_id` – (Optional) ID of a reusable script to run after provisioning.
- `ssh_keys` – (Optional, Sensitive) List of SSH public keys (OpenSSH format) to add to the VPS.
- `ssh_key_ids` – (Optional, **Deprecated**: use `ssh_keys`) List of public SSH key IDs to add to the VPS. Requires account-level public keys, which are deprecated in the Hostinger API.

---

## Attributes Reference

- `id` – Internal Hostinger VPS ID.
- `ipv4_address` – Public IPv4 address of the VPS.
- `ipv6_address` – Public IPv6 address of the VPS.
- `status` – Status of the VPS provisioning.
- `ssh_key_ids_resolved` – (Sensitive) Public keys that `ssh_key_ids` resolved to when last applied. Kept in state so a key removed from `ssh_key_ids` is revoked from the VPS even after its account-level key has been deleted.
- `vps_id` – Same as `id`, retained for convenience and backward compatibility.

---

## Import

Existing VPS instances can be imported using their VPS ID:

```bash
terraform import hostinger_vps.example 123456
```

After importing, run `terraform state show hostinger_vps.example` to see the imported values, then add `data_center_id`, `template_id` and `hostname` to your configuration:

```hcl
resource "hostinger_vps" "example" {
  data_center_id = 22
  template_id    = 1141
  hostname       = "srv123456.hstgr.cloud"
}
```

`plan` is not needed for an imported VPS and is ignored if set: the API does not expose the plan identifier the VPS was purchased with, so there is nothing to compare it against.

