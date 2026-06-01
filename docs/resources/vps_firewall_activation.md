# hostinger_vps_firewall_activation

The `hostinger_vps_firewall_activation` resource attaches a [`hostinger_vps_firewall`](vps_firewall.md) to a VPS instance. Creating the resource activates the firewall on the virtual machine; destroying it deactivates the firewall.

Only **one firewall can be active on a VM at a time** — activating a second firewall on the same VM will fail server-side.

---

## Example Usage

```hcl
resource "hostinger_vps_firewall_activation" "web" {
  firewall_id        = hostinger_vps_firewall.web.id
  virtual_machine_id = hostinger_vps.complete.id

  # Re-sync rules to the VM whenever the firewall's rule set changes.
  triggers = {
    rules = sha1(jsonencode(hostinger_vps_firewall.web.rule))
  }
}
```

---

## Argument Reference

- `firewall_id` – (Required, ForceNew) ID of the firewall to activate.
- `virtual_machine_id` – (Required, ForceNew) ID of the virtual machine to activate the firewall on.
- `triggers` – (Optional) An arbitrary map of strings. Whenever any value changes, the firewall is **synced** (re-applied) to the virtual machine on the next `apply`. This is the mechanism for pushing rule changes to a running VM.

> **Sync is manual.** Changing a firewall's rules leaves it out of sync with the VM (`is_synced` becomes `false`); the change does **not** reach the VM until a sync happens. Wire `triggers` to your rule set (as shown above) to sync on apply, or run a sync yourself via the Hostinger API / hPanel.

---

## Attributes Reference

- `id` – Composite ID in the form `firewallId/virtualMachineId`.
- `is_synced` – Whether the firewall is currently in sync with the virtual machine.

> **Limitation:** The Hostinger API does not expose which VM a firewall is active on, so this resource cannot detect if the firewall was deactivated out-of-band. Read only confirms the firewall still exists and reports its sync state.

---

## Import

Existing activations can be imported using a `firewallId/virtualMachineId` composite ID:

```bash
terraform import hostinger_vps_firewall_activation.web 65224/1268054
```
