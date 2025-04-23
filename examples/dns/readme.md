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
