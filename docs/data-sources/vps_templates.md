# hostinger_vps_templates

The `hostinger_vps_templates` data source returns all available OS templates supported by Hostinger for VPS creation.

Each template can be used in the `template_id` field of the `hostinger_vps` resource.

---

## Example Usage

```hcl
data "hostinger_vps_templates" "all" {}

output "templates" {
  value = data.hostinger_vps_templates.all.templates
}
```

---

## Attributes Reference

- `templates` – A list of templates, each with:
  - `id` – The template ID (used in `template_id`).
  - `name` – The OS name (e.g., `Debian 11`, `Ubuntu 20.04 LTS`).
  - `description` – Optional extra info.
  - `documentation` – Optional link to the OS documentation.

