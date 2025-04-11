# Full-blown example: password login, SSH keys, post-install script

provider "hostinger" {
  api_token = var.api_token
}

# Register public key
resource "hostinger_vps_ssh_key" "my_key" {
  name = "My Laptop"
  key  = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD..."
}

# Script that runs right after VPS creation
resource "hostinger_vps_post_install_script" "install_nginx" {
  name = "Install NGINX"
  content = <<-EOT
    #!/bin/bash
    apt update
    apt install -y nginx
  EOT
}

# Provision a VPS with full setup
resource "hostinger_vps" "complete" {
  plan                   = "hostingercom-vps-kvm2-usd-1m"
  data_center_id         = 13
  template_id            = 1002
  hostname               = "full.example.com"
  password               = "SuperSecurePassword123!"
  ssh_key_ids            = [hostinger_vps_ssh_key.my_key.id]
  post_install_script_id = hostinger_vps_post_install_script.install_nginx.id
}

# Outputs
output "vps_ipv4" {
  value = hostinger_vps.complete.ipv4_address
}

output "vps_ipv6" {
  value = hostinger_vps.complete.ipv6_address
}

output "vps_id" {
  value = hostinger_vps.complete.id
}

