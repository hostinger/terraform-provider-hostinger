# Firewall example: a firewall with accept rules, attached to a VPS.
#
# The firewall drops all incoming traffic by default, so each rule opens access
# for a specific protocol/port. Attaching the firewall to a VPS is a separate
# resource; after changing rules you sync them to the VM by bumping `triggers`.

provider "hostinger" {
  api_token = var.api_token
}

resource "hostinger_vps_firewall" "web" {
  name = "web-fw"

  # Allow SSH only from an internal CIDR.
  rule {
    protocol      = "SSH"
    port          = "22"
    source        = "custom"
    source_detail = "10.0.0.0/24"
  }

  # Allow HTTPS from anywhere.
  rule {
    protocol      = "HTTPS"
    port          = "443"
    source        = "any"
    source_detail = "any"
  }
}

# Attach the firewall to a VPS. Create activates it; destroy deactivates it.
resource "hostinger_vps_firewall_activation" "web" {
  firewall_id        = hostinger_vps_firewall.web.id
  virtual_machine_id = hostinger_vps.complete.id

  # Re-sync the firewall to the VM whenever the rule set changes. Sync is manual:
  # without this, rule changes won't take effect on the VM until you sync via the API.
  triggers = {
    rules = sha1(jsonencode(hostinger_vps_firewall.web.rule))
  }
}

output "firewall_id" {
  value = hostinger_vps_firewall.web.id
}

output "firewall_is_synced" {
  value = hostinger_vps_firewall_activation.web.is_synced
}
