# DNS Record Example

This example demonstrates how to create any type of DNS record (A, AAAA, CNAME, TXT, etc.) in a Hostinger-managed DNS zone using the `hostinger_dns_record` Terraform resource.

## Example Usage

```hcl
module "dns" {
  source = "./dns_record"

  hostinger_api_token = "your-api-token"
  dns_zone            = "example.com"
  dns_name            = "api"
  dns_type            = "A"
  dns_value           = "192.0.2.100"
  dns_ttl             = 300
}
```

## Records with extra fields

`MX`, `SRV` and `CAA` records carry their extra fields inside `dns_value`, written the same way
as in a zone file:

```hcl
module "mx" {
  source = "./dns_record"

  hostinger_api_token = "your-api-token"
  dns_zone            = "example.com"
  dns_name            = "send"
  dns_type            = "MX"
  dns_value           = "10 feedback-smtp.ap-northeast-1.amazonses.com"
  dns_ttl             = 900
}
```

## Importing an existing record

```bash
terraform import 'hostinger_dns_record.record' 'example.com|api|A|192.0.2.100'
```

The import ID is `zone|name|type|value`.
