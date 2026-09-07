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

resource "hostinger_dns_record" "mx" {
  zone  = "example.com"
  name  = "send"
  type  = "MX"
  value = "10 feedback-smtp.ap-northeast-1.amazonses.com"
  ttl   = 900
}
```

---

## Argument Reference

- `zone` – (Required) DNS zone (domain) that owns the record. Example: `example.com`.
- `name` – (Required) Record name relative to the zone. Use `@` for the zone apex.
- `type` – (Required) Record type, such as `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `SRV`, `CAA` or `ALIAS`.
- `value` – (Required) Record content. For types that carry extra fields, include them in the
  value exactly as they appear in a zone file:
  - `MX` – `"<priority> <host>"`, e.g. `"10 mx.example.com"`
  - `SRV` – `"<priority> <weight> <port> <target>"`, e.g. `"10 5 5060 sip.example.com"`
  - `CAA` – `"<flag> <tag> <value>"`, e.g. `"0 issue letsencrypt.org"`

  Trailing dots and quoting are normalized, so `target.example.com` and `target.example.com.`
  are treated as the same record.
- `ttl` – (Optional) TTL in seconds. Defaults to `14400`.
- `overwrite` – (Optional) Defaults to `false`. When `false`, the record is appended to any
  records that already exist for the same `name` and `type`. Set it to `true` to replace them
  instead, which is what you need to take over a `name`/`type` pair that already has records in
  the zone. Prefer `terraform import` when you want to manage an existing record as-is.

---

## Attributes Reference

- `id` – Synthetic record identifier in the form `name|type|value`.

---

## Import

Existing DNS records can be imported using an ID of the form `zone|name|type|value`, where
`value` is the record content as you want it written in your configuration:

```bash
terraform import hostinger_dns_record.example 'example.com|api.dev|CNAME|target.example.com'
terraform import hostinger_dns_record.mx 'example.com|send|MX|10 feedback-smtp.ap-northeast-1.amazonses.com'
```

The record must already exist in the zone; `ttl` is read back from the API. Quote the ID so your
shell does not interpret the `|` separators.
