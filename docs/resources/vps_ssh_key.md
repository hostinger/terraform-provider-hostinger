# hostinger_vps_ssh_key

The `hostinger_vps_ssh_key` resource allows you to manage SSH public keys in your Hostinger account. These keys can be attached to VPS instances at creation or after provisioning.

---

## Example Usage

```hcl
resource "hostinger_vps_ssh_key" "inline_key" {
  name = "My Work Laptop"
  key  = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD..."
}
```

---

## Argument Reference

- `name` – (Required) A friendly name to identify the SSH key.
- `key` – (Required) The actual SSH public key string (usually starts with `ssh-rsa` or `ssh-ed25519`).

---

## Attributes Reference

- `id` – ID of the SSH key in Hostinger’s system.
- `key` – The full SSH public key string.

