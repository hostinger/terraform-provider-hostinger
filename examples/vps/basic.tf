# Minimal VPS provisioning with password login

provider "hostinger" {
  api_token = var.api_token
}

# Creates a simple VPS instance with root password authentication
resource "hostinger_vps" "basic" {
  plan              = "hostingercom-vps-kvm2-usd-1m"  # Plan ID from Hostinger's catalog
  data_center_id    = 13                              # Phoenix DC, for example
  template_id       = 1002                            # Debian 11, Ubuntu, etc.
  hostname          = "basic.example.com"
  password          = "SuperSecurePassword123!"       # Optional — auto-generated if omitted
}

# Output the public IPv4 address
output "basic_ipv4" {
  value = hostinger_vps.basic.ipv4_address
}

