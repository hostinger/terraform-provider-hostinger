# hostinger_vps_plans

The `hostinger_vps_plans` data source lists all available VPS plans from Hostinger’s catalog.

Each plan includes the billing model and the unique ID required for VPS creation.

---

## Example Usage

```hcl
data "hostinger_vps_plans" "all" {}

output "plans" {
  value = data.hostinger_vps_plans.all.plans
}
```

---

## Attributes Reference

- `plans` – A list of available plans, each with the following attributes:
  - `id` – The unique plan ID (e.g., `hostingercom-vps-kvm2-usd-1m`).
  - `name` – Human-readable name (e.g., `KVM 2 (billed every month)`).
  - `category` – Plan category, usually `VPS`.

