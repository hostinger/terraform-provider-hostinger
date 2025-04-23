variable "hostinger_api_token" {
  description = "Hostinger API token with DNS zone access."
  type        = string
}

variable "dns_zone" {
  description = "The DNS zone name (e.g., example.com)."
  type        = string
}

variable "dns_name" {
  description = "Subdomain or record name (e.g., 'api', 'www', or '@')."
  type        = string
}

variable "dns_type" {
  description = "DNS record type (e.g., A, AAAA, CNAME, TXT, MX, etc.)."
  type        = string
}

variable "dns_value" {
  description = "The value of the DNS record (e.g., IP address, CNAME target, TXT value)."
  type        = string
}

variable "dns_ttl" {
  description = "Time to live (TTL) for the record in seconds."
  type        = number
  default     = 14400
}
