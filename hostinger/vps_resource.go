package hostinger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

var errPlanRequired = errors.New("plan is required to purchase a new VPS")

func resourceHostingerVPS() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSCreate,
		ReadContext:   resourceHostingerVPSRead,
		DeleteContext: resourceHostingerVPSDelete,
		UpdateContext: resourceHostingerVPSUpdate,
		Importer: &schema.ResourceImporter{
			StateContext: resourceHostingerVPSImport,
		},
		CustomizeDiff: customdiff.All(
			customdiff.ComputedIf("ssh_key_ids_resolved", func(ctx context.Context, d *schema.ResourceDiff, m interface{}) bool {
				return d.HasChange("ssh_key_ids")
			}),
			func(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
				if config := d.GetRawConfig(); d.Id() == "" && !config.IsNull() && config.GetAttr("plan").IsNull() {
					return errPlanRequired
				}
				return nil
			},
		),
		Schema: map[string]*schema.Schema{
			"plan": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				Description:  "VPS plan identifier (e.g., `hostingercom-vps-kvm2-usd-1m`). Required to purchase a new VPS; ignored for an imported one.",
				// The API does not expose the plan identifier a VPS was purchased with, so an imported VPS has none to compare against.
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return d.Id() != "" && old == ""
				},
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"data_center_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				Description:  "Data center location identifier for the VPS.",
				ValidateFunc: validation.IntAtLeast(1),
			},
			"template_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     false,
				Description:  "OS template ID to install (e.g., `1002` for Debian 11).",
				ValidateFunc: validation.IntAtLeast(1),
			},
			"password": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ForceNew:     true,
				Description:  "Root password for the new VPS (will be sent to the server).",
				ValidateFunc: validation.StringLenBetween(8, 100),
			},
			"hostname": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     false,
				Description:  "Hostname to assign to the VPS (FQDN). If not set, a default hostname is generated.",
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}$`), "must be a valid FQDN"),
			},
			"payment_method_id": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Payment method ID to use for the order. If omitted, the default method will be used.",
			},
			"post_install_script_id": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "ID of the post-install script to run after OS setup.",
			},
			"ssh_key_ids": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Deprecated:  "Use ssh_keys instead: account-level public keys are deprecated in the Hostinger API.",
				Description: "List of SSH key IDs to add to the VPS after setup.",
			},
			"ssh_keys": {
				Type:      schema.TypeList,
				Optional:  true,
				Sensitive: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateFunc: validation.StringMatch(
						sshPublicKeyPattern,
						"must be a valid SSH public key (ssh-rsa, ssh-ed25519...)",
					),
				},
				Description: "List of SSH public keys (OpenSSH format) to add to the VPS after setup.",
			},
			"ssh_key_ids_resolved": {
				Type:        schema.TypeList,
				Computed:    true,
				Sensitive:   true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Public keys that `ssh_key_ids` resolved to when last applied, kept so they can still be revoked from the VPS once the account key is deleted.",
			},
			// Output attributes:
			"ipv4_address": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Public IPv4 address assigned to the VPS.",
			},
			"ipv6_address": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Public IPv6 address assigned to the VPS (if available).",
			},
			"vps_id": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The Hostinger VPS instance ID.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current status of the VPS (e.g., running, stopped, installing, reinstalling).",
			},
			"firewall_group_id": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "ID of the firewall currently active on the VPS (0 if none).",
			},
		},
	}
}

func resourceHostingerVPSCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	// Gather required fields for the VPS purchase
	plan := d.Get("plan").(string)
	if plan == "" {
		return diag.FromErr(errPlanRequired)
	}
	dataCenterID := d.Get("data_center_id").(int)
	templateID := d.Get("template_id").(int)

	var passwordPtr *string
	if v, ok := d.GetOk("password"); ok {
		pw := v.(string)
		passwordPtr = &pw
	}
	var hostnamePtr *string
	if v, ok := d.GetOk("hostname"); ok {
		h := v.(string)
		hostnamePtr = &h
	}
	var postInstallScriptIDPtr *int
	if v, ok := d.GetOk("post_install_script_id"); ok {
		id := v.(int)
		postInstallScriptIDPtr = &id
	}

	var paymentMethodIDPtr *int
	if v, ok := d.GetOk("payment_method_id"); ok {
		id := v.(int)
		paymentMethodIDPtr = &id
	}

	// Validate inputs
	ok, err := client.ValidatePlanID(plan)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to validate plan: %w", err))
	}
	if !ok {
		return diag.Errorf("Invalid plan ID: %s", plan)
	}

	ok, err = client.ValidateTemplateID(templateID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to validate template_id: %w", err))
	}
	if !ok {
		return diag.Errorf("Invalid template ID: %d", templateID)
	}

	ok, err = client.ValidateDataCenterID(dataCenterID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to validate data_center_id: %w", err))
	}
	if !ok {
		return diag.Errorf("Invalid data center ID: %d", dataCenterID)
	}

	// Purchase and setup VPS in a single API call
	purchaseReq := PurchaseVPSRequest{
		ItemID:          plan,
		PaymentMethodID: paymentMethodIDPtr,
		Setup: PurchaseVPSSetup{
			DataCenterID:        dataCenterID,
			TemplateID:          templateID,
			Password:            passwordPtr,
			Hostname:            hostnamePtr,
			PostInstallScriptID: postInstallScriptIDPtr,
		},
	}

	purchaseRes, err := client.PurchaseVPS(purchaseReq)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to purchase VPS: %w", err))
	}

	vmID := purchaseRes.VirtualMachine.ID

	// Add SSH keys (optional)
	rawIDs := d.Get("ssh_key_ids").([]interface{})
	rawKeys := d.Get("ssh_keys").([]interface{})
	if len(rawIDs) > 0 || len(rawKeys) > 0 {
		keyStrings := make([]string, 0, len(rawIDs)+len(rawKeys))
		for _, raw := range rawKeys {
			keyStrings = append(keyStrings, strings.TrimSpace(raw.(string)))
		}
		if len(rawIDs) > 0 {
			keyByID, err := client.publicKeyStringsByID()
			if err != nil {
				return diag.FromErr(fmt.Errorf("failed to list public keys: %w", err))
			}
			resolved, err := resolveSSHKeyIDs(keyByID, rawIDs)
			if err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set("ssh_key_ids_resolved", resolved); err != nil {
				return diag.FromErr(fmt.Errorf("failed to set ssh_key_ids_resolved: %w", err))
			}
			keyStrings = append(keyStrings, resolved...)
		}
		if err := client.AddVMSSHKeys(vmID, keyStrings); err != nil {
			return diag.FromErr(fmt.Errorf("failed to add SSH keys: %w", err))
		}
	}

	// Set the resource ID to the VPS instance ID
	d.SetId(strconv.Itoa(vmID))

	if err := d.Set("vps_id", vmID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set vps_id: %w", err))
	}
	if hostnamePtr != nil {
		if err := d.Set("hostname", *hostnamePtr); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set hostname: %w", err))
		}
	}

	// Read and set remaining attributes (IP addresses, etc.)
	return resourceHostingerVPSRead(ctx, d, m)
}

func resourceHostingerVPSRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	vmID, err := strconv.Atoi(d.Id())
	if err != nil {
		// If ID is not valid, remove from state
		d.SetId("")
		return nil
	}

	vm, err := client.GetVirtualMachine(vmID)
	if err != nil {
		if err == ErrNotFound {
			// The VPS no longer exists (possibly cancelled outside Terraform)
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("failed to fetch VPS details (ID %d): %w", vmID, err))
	}

	// Update state with the latest information from the API
	if err := d.Set("hostname", vm.Hostname); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set hostname in read: %w", err))
	}
	if err := d.Set("vps_id", vm.ID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set vps_id: %w", err))
	}
	if err := d.Set("status", vm.State); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set status: %w", err))
	}
	firewallGroupID := 0
	if vm.FirewallGroupID != nil {
		firewallGroupID = *vm.FirewallGroupID
	}
	if err := d.Set("firewall_group_id", firewallGroupID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set firewall_group_id: %w", err))
	}
	if len(vm.IPv4) > 0 {
		if err := d.Set("ipv4_address", vm.IPv4[0].Address); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ipv4_address: %w", err))
		}
	} else {
		if err := d.Set("ipv4_address", ""); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ipv4_address: %w", err))
		}
	}
	if len(vm.IPv6) > 0 {
		if err := d.Set("ipv6_address", vm.IPv6[0].Address); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ipv6_address: %w", err))
		}
	} else {
		if err := d.Set("ipv6_address", ""); err != nil {
			return diag.FromErr(fmt.Errorf("failed to clear ipv6_address: %w", err))
		}
	}

	// State written before ssh_key_ids_resolved existed: backfill it while the account keys still resolve,
	// so the plan refresh captures them before a rotation deletes the account key.
	rawIDs := d.Get("ssh_key_ids").([]interface{})
	if len(rawIDs) > 0 && len(d.Get("ssh_key_ids_resolved").([]interface{})) == 0 {
		keyByID, err := client.publicKeyStringsByID()
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to list public keys: %w", err))
		}
		resolved := make([]string, 0, len(rawIDs))
		for _, raw := range rawIDs {
			if key, ok := keyByID[raw.(int)]; ok {
				resolved = append(resolved, key)
			}
		}
		if err := d.Set("ssh_key_ids_resolved", resolved); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ssh_key_ids_resolved: %w", err))
		}
	}
	// An attribute missing from state (e.g. after import) is planned as "known after apply" on every run.
	if len(d.Get("ssh_key_ids_resolved").([]interface{})) == 0 {
		if err := d.Set("ssh_key_ids_resolved", []string{}); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ssh_key_ids_resolved: %w", err))
		}
	}
	return nil
}

func resourceHostingerVPSDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	vmID := d.Get("vps_id").(int)

	// Always resolve subscription ID from the API
	subscriptionID, err := client.GetSubscriptionIDByVMID(vmID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to find subscription for VPS %d: %w", vmID, err))
	}

	err = client.CancelSubscription(subscriptionID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to cancel subscription: %w", err))
	}

	d.SetId("")
	return nil
}

func resourceHostingerVPSUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	client := m.(*HostingerClient)
	vmID, _ := strconv.Atoi(d.Id())

	// Registered before any API call, so a failed hostname or template update doesn't save the planned keys either.
	var keptOnError map[string][]interface{}
	defer func() {
		revertOnError(d, &diags, "ssh_key_ids", "ssh_keys", "ssh_key_ids_resolved")
		if !diags.HasError() {
			return
		}
		for key, extra := range keptOnError {
			if err := d.Set(key, appendMissing(d.Get(key).([]interface{}), extra)); err != nil {
				diags = append(diags, diag.FromErr(fmt.Errorf("failed to restore %s: %w", key, err))...)
			}
		}
	}()

	if d.HasChange("hostname") {
		newHostname := d.Get("hostname").(string)

		err := client.UpdateHostname(vmID, newHostname)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to update hostname: %w", err))
		}
	}

	if d.HasChange("template_id") {
		vmID := d.Get("vps_id").(int)
		templateID := d.Get("template_id").(int)

		var password *string
		if v, ok := d.GetOk("password"); ok {
			pw := v.(string)
			password = &pw
		}

		var postScriptID *int
		if v, ok := d.GetOk("post_install_script_id"); ok {
			id := v.(int)
			postScriptID = &id
		}

		err := client.RecreateVirtualMachine(vmID, templateID, password, postScriptID)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to recreate VPS: %w", err))
		}
	}

	if d.HasChange("ssh_key_ids") || d.HasChange("ssh_keys") {
		vmID := d.Get("vps_id").(int)

		oldIDRaw, newIDRaw := d.GetChange("ssh_key_ids")
		oldKeyRaw, newKeyRaw := d.GetChange("ssh_keys")
		// Prior state, not the account: an account key deleted since the last apply must still be revoked.
		oldResolvedRaw, _ := d.GetChange("ssh_key_ids_resolved")

		desired := stringSet(newKeyRaw.([]interface{}))
		previous := stringSet(oldKeyRaw.([]interface{}))
		for key := range stringSet(oldResolvedRaw.([]interface{})) {
			previous[key] = true
		}

		var resolved []string
		if len(newIDRaw.([]interface{}))+len(oldIDRaw.([]interface{})) > 0 {
			keyByID, err := client.publicKeyStringsByID()
			if err != nil {
				return diag.FromErr(fmt.Errorf("failed to list public keys: %w", err))
			}
			resolved, err = resolveSSHKeyIDs(keyByID, newIDRaw.([]interface{}))
			if err != nil {
				return diag.FromErr(err)
			}
			for _, key := range resolved {
				desired[key] = true
			}
			// Fallback for state that predates ssh_key_ids_resolved and was never refreshed.
			for _, raw := range oldIDRaw.([]interface{}) {
				if key, ok := keyByID[raw.(int)]; ok {
					previous[key] = true
				}
			}
		}

		vmKeys, err := client.ListVMSSHKeys(vmID)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to check existing SSH keys: %w", err))
		}
		onVM := make(map[string]bool, len(vmKeys))
		for _, k := range vmKeys {
			onVM[strings.TrimSpace(k.Key)] = true
		}

		toAdd, toRemove := diffVMSSHKeys(desired, previous, onVM)

		if len(toAdd) > 0 {
			if err := client.AddVMSSHKeys(vmID, toAdd); err != nil {
				return diag.FromErr(fmt.Errorf("failed to add SSH keys during update: %w", err))
			}
		}
		// The VM now holds the new keys: if a later step fails, keep them in state next to the prior ones,
		// so a config that drops them still revokes them.
		resolvedRaw := make([]interface{}, len(resolved))
		for i, key := range resolved {
			resolvedRaw[i] = key
		}
		keptOnError = map[string][]interface{}{
			"ssh_key_ids": newIDRaw.([]interface{}), "ssh_keys": newKeyRaw.([]interface{}), "ssh_key_ids_resolved": resolvedRaw,
		}
		if len(toRemove) > 0 {
			if err := client.RemoveVMSSHKeys(vmID, toRemove); err != nil {
				return diag.FromErr(fmt.Errorf("failed to remove SSH keys during update: %w", err))
			}
		}
		if err := d.Set("ssh_key_ids_resolved", resolved); err != nil {
			return diag.FromErr(fmt.Errorf("failed to set ssh_key_ids_resolved: %w", err))
		}
	}

	// Always re-read state after update
	return resourceHostingerVPSRead(ctx, d, m)
}

// revertOnError restores the prior state of keys when the update failed. SDKv2 otherwise saves the
// planned values as applied, and Read never refreshes these keys, so a failed change would not be retried.
func revertOnError(d *schema.ResourceData, diags *diag.Diagnostics, keys ...string) {
	if !diags.HasError() {
		return
	}
	for _, key := range keys {
		old, _ := d.GetChange(key)
		if err := d.Set(key, old); err != nil {
			*diags = append(*diags, diag.FromErr(fmt.Errorf("failed to restore %s: %w", key, err))...)
		}
	}
}

// appendMissing appends the values of extra that list doesn't already hold.
func appendMissing(list, extra []interface{}) []interface{} {
	for _, v := range extra {
		found := false
		for _, have := range list {
			found = found || have == v
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
}

func stringSet(raw []interface{}) map[string]bool {
	set := make(map[string]bool, len(raw))
	for _, v := range raw {
		set[strings.TrimSpace(v.(string))] = true
	}
	return set
}

// resolveSSHKeyIDs maps account public key IDs to their key strings, in order.
func resolveSSHKeyIDs(keyByID map[int]string, ids []interface{}) ([]string, error) {
	keys := make([]string, 0, len(ids))
	for _, raw := range ids {
		key, ok := keyByID[raw.(int)]
		if !ok {
			return nil, fmt.Errorf("SSH key with ID %d not found among account public keys", raw.(int))
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// diffVMSSHKeys computes which keys to add (desired but not on the VM) and
// which to remove (previously managed, no longer desired, still on the VM).
func diffVMSSHKeys(desired, previous, onVM map[string]bool) (toAdd, toRemove []string) {
	for key := range desired {
		if !onVM[key] {
			toAdd = append(toAdd, key)
		}
	}
	for key := range previous {
		if !desired[key] && onVM[key] {
			toRemove = append(toRemove, key)
		}
	}
	sort.Strings(toAdd)
	sort.Strings(toRemove)
	return toAdd, toRemove
}

func resourceHostingerVPSImport(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	client := m.(*HostingerClient)
	
	// The import ID should be the VPS ID
	vmID, err := strconv.Atoi(d.Id())
	if err != nil {
		return nil, fmt.Errorf("invalid VPS ID format: %s", d.Id())
	}
	
	vm, err := client.GetVirtualMachineWithFullDetails(vmID)
	if err != nil {
		if err == ErrNotFound {
			return nil, fmt.Errorf("VPS with ID %d not found", vmID)
		}
		return nil, fmt.Errorf("failed to fetch VPS details: %w", err)
	}
	
	// Set all fields
	d.SetId(strconv.Itoa(vmID))
	if err := d.Set("vps_id", vm.ID); err != nil {
		return nil, fmt.Errorf("failed to set vps_id: %w", err)
	}
	if err := d.Set("hostname", vm.Hostname); err != nil {
		return nil, fmt.Errorf("failed to set hostname: %w", err)
	}
	if err := d.Set("status", vm.State); err != nil {
		return nil, fmt.Errorf("failed to set status: %w", err)
	}
	
	// Set data center and template IDs if available
	if vm.DataCenterID > 0 {
		if err := d.Set("data_center_id", vm.DataCenterID); err != nil {
			return nil, fmt.Errorf("failed to set data_center_id: %w", err)
		}
	}
	if vm.TemplateID > 0 {
		if err := d.Set("template_id", vm.TemplateID); err != nil {
			return nil, fmt.Errorf("failed to set template_id: %w", err)
		}
	}
	
	// Set IP addresses
	if len(vm.IPv4) > 0 {
		if err := d.Set("ipv4_address", vm.IPv4[0].Address); err != nil {
			return nil, fmt.Errorf("failed to set ipv4_address: %w", err)
		}
	}
	if len(vm.IPv6) > 0 {
		if err := d.Set("ipv6_address", vm.IPv6[0].Address); err != nil {
			return nil, fmt.Errorf("failed to set ipv6_address: %w", err)
		}
	}
	
	return []*schema.ResourceData{d}, nil
}
