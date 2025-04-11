# Automatically install software on VPS after creation using a post-install script

provider "hostinger" {
  api_token = var.api_token
}

# Script to run on the VPS after it’s provisioned
resource "hostinger_vps_post_install_script" "install_nginx" {
  name = "Install NGINX"
  content = <<-EOT
    #!/bin/bash
    apt update
    apt install -y nginx
  EOT
}

# Attach script to VPS provisioning
resource "hostinger_vps" "with_script" {
  plan                   = "hostingercom-vps-kvm2-usd-1m"
  data_center_id         = 13
  template_id            = 1002
  hostname               = "script.example.com"
  post_install_script_id = hostinger_vps_post_install_script.install_nginx.id
}

