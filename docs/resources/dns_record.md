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

---

## Import

Records that already exist in a zone cannot be created — the create path
appends with `overwrite: false`, and Hostinger rejects an append that duplicates
an existing record with `[DNS:4008] DNS resource record is not valid or
conflicts with another resource record`. Import is the way to bring a populated
zone under management.

The import ID is `zone|name|type|value`:

```sh
terraform import hostinger_dns_record.example 'example.com|api.dev|CNAME|target.example.com'
```

Or with an `import` block:

```hcl
import {
  to = hostinger_dns_record.example
  id = "example.com|api.dev|CNAME|target.example.com"
}
```

Notes:

- A name/type holding several values (two `MX` records, say) is several
  resources, each imported by its own `value`.
- `TXT` values may be given with or without the surrounding quotes the API
  stores; state keeps whichever form the import ID used.
- Only the `value` may contain `|`; it is taken as everything after the third
  separator.
