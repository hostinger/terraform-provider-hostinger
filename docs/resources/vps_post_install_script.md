# hostinger_vps_post_install_script

The `hostinger_vps_post_install_script` resource allows you to define reusable shell scripts that will be executed on your VPS after provisioning.

Useful for installing packages, setting up firewall rules, or applying configuration automatically.

---

## Example Usage

```hcl
resource "hostinger_vps_post_install_script" "nginx" {
  name    = "Install NGINX"
  content = <<-EOT
    #!/bin/bash
    apt update
    apt install -y nginx
  EOT
}
```

---

## Argument Reference

- `name` – (Required) A unique name for the script.
- `content` – (Required) Shell script contents. Must start with a shebang (e.g., `#!/bin/bash`).

---

## Attributes Reference

- `id` – ID of the post-install script in Hostinger.
- `created_at` – Timestamp when the script was created.
- `updated_at` – Timestamp when the script was last modified.

