# Provision a VPS using SSH keys instead of password login (recommended for production)

provider "hostinger" {
  api_token = var.api_token
}

# Register an SSH public key with your Hostinger account
resource "hostinger_vps_ssh_key" "my_key" {
  name = "My Laptop"
  key  = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD..."
}

# Provision VPS with that key — login via SSH only
resource "hostinger_vps" "with_ssh" {
  plan           = "hostingercom-vps-kvm2-usd-1m"
  data_center_id = 13
  template_id    = 1002
  hostname       = "ssh.example.com"
  ssh_key_ids    = [hostinger_vps_ssh_key.my_key.id]
}

