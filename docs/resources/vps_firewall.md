# hostinger_vps_firewall

The `hostinger_vps_firewall` resource manages a VPS firewall and its rules on Hostinger.

A firewall **drops all incoming traffic by default**, so every rule you add grants access for a specific protocol and port — rules are always `accept`. Rules are declared as an ordered list of nested `rule` blocks. Editing a rule in place updates it via the API and preserves its rule ID; appending blocks creates rules and removing trailing blocks deletes them.

A firewall on its own does nothing until it is activated on a VPS. Use [`hostinger_vps_firewall_activation`](vps_firewall_activation.md) to attach it to a virtual machine.

---

## Example Usage

```hcl
resource "hostinger_vps_firewall" "web" {
  name = "web-fw"

  rule {
    protocol      = "SSH"
    port          = "22"
    source        = "custom"
    source_detail = "10.0.0.0/24"
  }

  rule {
    protocol      = "HTTPS"
    port          = "443"
    source        = "any"
    source_detail = "any"
  }
}
```

---

## Argument Reference

- `name` – (Required, ForceNew) Name of the firewall. The Hostinger API has no rename endpoint, so changing the name **replaces** the firewall. Because a replaced firewall is deleted (and automatically deactivated from any attached VPS), there is a brief gap in protection during the replace.
- `rule` – (Optional) An ordered list of accept rules. Each `rule` block supports:
  - `protocol` – (Required) One of `TCP`, `UDP`, `ICMP`, `GRE`, `any`, `ESP`, `AH`, `ICMPv6`, `SSH`, `HTTP`, `HTTPS`, `MySQL`, `PostgreSQL`.
  - `port` – (Required) A single port (e.g. `"443"`) or a range (e.g. `"1024:2048"`).
  - `source` – (Required) `any` or `custom`.
  - `source_detail` – (Required) `any`, a single IP, a CIDR, or an IP range.

> **Note:** Adding, changing, or removing a rule causes the firewall to lose sync with any VPS it is activated on (`is_synced` becomes `false`). Re-apply the rules to the VM with a sync — see [`hostinger_vps_firewall_activation`](vps_firewall_activation.md).

---

## Attributes Reference

- `id` – ID of the firewall in Hostinger's system.
- `is_synced` – Whether the firewall is in sync with the VPS instances it is activated on.
- `created_at` – Timestamp when the firewall was created.
- `updated_at` – Timestamp when the firewall was last updated.
- Each `rule` additionally exports:
  - `id` – Server-assigned ID of the rule.
  - `action` – Always `accept` (the firewall drops everything else by default).

---

## Import

Existing firewalls can be imported using their numeric firewall ID:

```bash
terraform import hostinger_vps_firewall.web 65224
```

After importing, run `terraform state show hostinger_vps_firewall.web` to view the imported rules, then add matching `name` and `rule` blocks to your configuration.
