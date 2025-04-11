# hostinger_vps_data_centers

The `hostinger_vps_data_centers` data source returns a list of all available data centers where you can deploy VPS instances.

---

## Example Usage

```hcl
data "hostinger_vps_data_centers" "all" {}

output "available_data_centers" {
  value = data.hostinger_vps_data_centers.all.data_centers
}
```

---

## Attributes Reference

- `data_centers` – List of data centers, each with the following:
  - `id` – ID of the data center.
  - `name` – Name of the data center.
  - `location` – Region or country.
  - `city` – City location.
  - `continent` – Continent location.

