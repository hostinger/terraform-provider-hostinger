# DNS Record Example

This example demonstrates how to create a CNAME record in a Hostinger-managed DNS zone using the `hostinger_dns_record` resource.

## Example

```hcl
module "dns" {
  source = "./dns_record"

  hostinger_api_token = "your-api-token"
  dns_zone            = "example.com"
  cname_target        = "target.example.com"
}
```
