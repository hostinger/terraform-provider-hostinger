package hostinger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// FirewallAction mirrors VPS.V1.Action.ActionResource.
type FirewallAction struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func resourceHostingerVPSFirewallActivation() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSFirewallActivationCreate,
		ReadContext:   resourceHostingerVPSFirewallActivationRead,
		UpdateContext: resourceHostingerVPSFirewallActivationUpdate,
		DeleteContext: resourceHostingerVPSFirewallActivationDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceHostingerVPSFirewallActivationImport,
		},
		Schema: map[string]*schema.Schema{
			"firewall_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				Description:  "ID of the firewall to activate on the virtual machine.",
				ValidateFunc: validation.IntAtLeast(1),
			},
			"virtual_machine_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				Description:  "ID of the virtual machine to activate the firewall on. Only one firewall can be active on a VM at a time.",
				ValidateFunc: validation.IntAtLeast(1),
			},
			"triggers": {
				Type:        schema.TypeMap,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Arbitrary map whose change triggers a firewall sync on the virtual machine. Use it to re-apply rules after they change, e.g. triggers = { rules = sha1(jsonencode(hostinger_vps_firewall.web.rule)) }.",
			},
			"is_synced": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether the firewall is currently in sync with the virtual machine. Changing rules on the firewall sets this to false until a sync is performed.",
			},
		},
	}
}

func resourceHostingerVPSFirewallActivationCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	firewallID := d.Get("firewall_id").(int)
	vmID := d.Get("virtual_machine_id").(int)

	if _, err := client.ActivateFirewall(firewallID, vmID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to activate firewall %d on VM %d: %w", firewallID, vmID, err))
	}

	d.SetId(fmt.Sprintf("%d/%d", firewallID, vmID))
	return resourceHostingerVPSFirewallActivationRead(ctx, d, m)
}

func resourceHostingerVPSFirewallActivationRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	firewallID, vmID, err := parseFirewallActivationID(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	// Detect drift on this specific VM: the VM reports the firewall currently active
	// on it via firewall_group_id. If it no longer points at our firewall (deactivated
	// out-of-band, or another firewall activated on the VM — only one can be active at a
	// time), the activation is gone, so drop it from state.
	vm, err := client.GetVirtualMachine(vmID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("failed to read virtual machine %d: %w", vmID, err))
	}
	if vm.FirewallGroupID == nil || *vm.FirewallGroupID != firewallID {
		d.SetId("")
		return nil
	}

	fw, err := client.GetFirewall(firewallID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("failed to read firewall %d: %w", firewallID, err))
	}

	if err := d.Set("firewall_id", firewallID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set firewall_id: %w", err))
	}
	if err := d.Set("virtual_machine_id", vmID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set virtual_machine_id: %w", err))
	}
	if err := d.Set("is_synced", fw.IsSynced); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set is_synced: %w", err))
	}

	return nil
}

func resourceHostingerVPSFirewallActivationUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	// Only "triggers" is updatable; firewall_id and virtual_machine_id are ForceNew.
	// A change to triggers re-applies (syncs) the firewall to the VM.
	if d.HasChange("triggers") {
		firewallID, vmID, err := parseFirewallActivationID(d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if _, err := client.SyncFirewall(firewallID, vmID); err != nil {
			return diag.FromErr(fmt.Errorf("failed to sync firewall %d on VM %d: %w", firewallID, vmID, err))
		}
	}

	return resourceHostingerVPSFirewallActivationRead(ctx, d, m)
}

func resourceHostingerVPSFirewallActivationDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	firewallID, vmID, err := parseFirewallActivationID(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if _, err := client.DeactivateFirewall(firewallID, vmID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to deactivate firewall %d on VM %d: %w", firewallID, vmID, err))
	}

	d.SetId("")
	return nil
}

func resourceHostingerVPSFirewallActivationImport(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	if _, _, err := parseFirewallActivationID(d.Id()); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}

// parseFirewallActivationID splits a "firewallId/virtualMachineId" composite ID.
func parseFirewallActivationID(id string) (int, int, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid firewall activation ID %q: expected \"firewallId/virtualMachineId\"", id)
	}
	firewallID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid firewall ID in %q: %w", id, err)
	}
	vmID, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid virtual machine ID in %q: %w", id, err)
	}
	return firewallID, vmID, nil
}

// HostingerClient implementations:

func (c *HostingerClient) ActivateFirewall(firewallID, vmID int) (*FirewallAction, error) {
	return c.firewallVMAction("activate", firewallID, vmID)
}

func (c *HostingerClient) DeactivateFirewall(firewallID, vmID int) (*FirewallAction, error) {
	return c.firewallVMAction("deactivate", firewallID, vmID)
}

func (c *HostingerClient) SyncFirewall(firewallID, vmID int) (*FirewallAction, error) {
	return c.firewallVMAction("sync", firewallID, vmID)
}

// firewallVMAction performs one of the activate/deactivate/sync firewall operations,
// which share an identical request/response shape.
func (c *HostingerClient) firewallVMAction(action string, firewallID, vmID int) (*FirewallAction, error) {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d/%s/%d", c.BaseURL, firewallID, action, vmID)
	req, _ := http.NewRequest("POST", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s firewall failed (HTTP %d): %s", action, resp.StatusCode, msg)
	}

	var res FirewallAction
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	// A 200 only means the action was accepted; the action itself can still report
	// failure via its state (success|error|delayed|sent|created). Surface a hard
	// error so a failed activate/deactivate/sync is not reported as a successful apply.
	if res.State == "error" {
		return &res, fmt.Errorf("%s firewall action %d reported state %q", action, res.ID, res.State)
	}
	return &res, nil
}
