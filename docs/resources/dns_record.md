# hostinger_dns_record

The `hostinger_dns_record` resource allows you to manage DNS records in a Hostinger DNS zone using their public API.

This resource supports full lifecycle operations: create, read, and delete. Updates are handled by replacing the existing record (via `ForceNew`).

---

## Example Usage

```hcl
provider "hostinger" {
  api_token = var.hostinger_api_token
}

resource "hostinger_dns_record" "example" {
  zone  = "example.com"
  name  = "api.dev"
  type  = "CNAME"
  value = "target.example.com"
  ttl   = 14400
}
```
