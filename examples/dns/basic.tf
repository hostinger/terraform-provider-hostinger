provider "hostinger" {
  api_token = var.hostinger_api_token
}

resource "hostinger_dns_record" "basic" {
  zone  = var.dns_zone
  name  = "api"
  type  = "CNAME"
  value = var.cname_target
  ttl   = 14400
}
