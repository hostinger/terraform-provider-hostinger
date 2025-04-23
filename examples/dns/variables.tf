variable "hostinger_api_token" {
  description = "Hostinger API token with DNS zone access"
  type        = string
}

variable "dns_zone" {
  description = "The DNS zone name (e.g., example.com)"
  type        = string
}

variable "cname_target" {
  description = "The CNAME target value"
  type        = string
}
