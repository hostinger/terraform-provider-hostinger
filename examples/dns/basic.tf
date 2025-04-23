provider "hostinger" {
  api_token = var.hostinger_api_token
}

resource "hostinger_dns_record" "record" {
  zone  = var.dns_zone
  name  = var.dns_name
  type  = var.dns_type
  value = var.dns_value
  ttl   = var.dns_ttl
}
