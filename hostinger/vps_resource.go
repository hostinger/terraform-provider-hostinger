package hostinger

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceHostingerVPS() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSCreate,
		ReadContext:   resourceHostingerVPSRead,
		DeleteContext: resourceHostingerVPSDelete,
		UpdateContext: resourceHostingerVPSUpdate,
		Schema: map[string]*schema.Schema{
			"plan": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "VPS plan identifier (e.g., `hostingercom-vps-kvm2-usd-1m`).",
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
				Description: "List of SSH key IDs to attach to the VPS after setup.",
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
		},
	}
}

func resourceHostingerVPSCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	// Gather required fields for the VPS order and setup
	plan := d.Get("plan").(string)
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

	var paymentMethodID int
	if v, ok := d.GetOk("payment_method_id"); ok {
		paymentMethodID = v.(int)
	} else {
		var err error
		paymentMethodID, err = client.GetDefaultPaymentMethod()
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to fetch default payment method: %w", err))
		}
	}

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

	// Step 1: Place an order via Hostinger Billing API
	subID, err := client.OrderVPS(plan, paymentMethodID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to create VPS order: %w", err))
	}

	// Step 2: Find the new VPS instance by subscription_id (retry until it appears)
	var vmID int
	found := false
	for i := 0; i < 10; i++ {
		vmID, err = client.FindVirtualMachineBySubscription(subID)
		if err != nil {
			if err == ErrNotFound {
				// Not found yet, wait and retry
				time.Sleep(2 * time.Second)
				continue
			}
			return diag.FromErr(fmt.Errorf("error finding VPS instance (subscription %s): %w", subID, err))
		}
		found = true
		break
	}
	if !found {
		return diag.Errorf("timed out waiting for VPS instance to be created (subscription %s)", subID)
	}

	// Step 3: Call the VPS setup endpoint to activate the server
	setupReq := SetupRequest{
		DataCenterID: dataCenterID,
		TemplateID:   templateID,
		Password:     passwordPtr,
		Hostname:     hostnamePtr,
	}
	_, err = client.SetupVirtualMachine(vmID, setupReq)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to set up VPS (ID %d): %w", vmID, err))
	}

	// Attach SSH keys (optional)
	if v, ok := d.GetOk("ssh_key_ids"); ok {
		rawIDs := v.([]interface{})
		keyIDs := make([]int, len(rawIDs))
		for i, raw := range rawIDs {
			keyIDs[i] = raw.(int)
		}
		err = client.AttachSSHKeysToVM(vmID, keyIDs)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to attach SSH keys: %w", err))
		}
	}

	// Set the resource ID to the VPS instance ID
	d.SetId(strconv.Itoa(vmID))
	// Save outputs
	if err := d.Set("vps_id", vmID); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set vps_id: %w", err))
	}
	// If a hostname was provided, ensure it is saved (otherwise it will be fetched in Read)
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

func resourceHostingerVPSUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	vmID, _ := strconv.Atoi(d.Id())

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

	if d.HasChange("ssh_key_ids") {
		vmID := d.Get("vps_id").(int)
		desiredRaw := d.Get("ssh_key_ids").([]interface{})
		desiredIDs := make(map[int]bool)
		for _, id := range desiredRaw {
			desiredIDs[id.(int)] = true
		}

		currentIDs, err := client.GetSSHKeyIDsForVM(vmID)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to check existing SSH keys: %w", err))
		}

		toAttach := []int{}
		for id := range desiredIDs {
			found := false
			for _, curr := range currentIDs {
				if curr == id {
					found = true
					break
				}
			}
			if !found {
				toAttach = append(toAttach, id)
			}
		}

		if len(toAttach) > 0 {
			err := client.AttachSSHKeysToVM(vmID, toAttach)
			if err != nil {
				return diag.FromErr(fmt.Errorf("failed to attach SSH keys during update: %w", err))
			}
		}
	}

	// Always re-read state after update
	return resourceHostingerVPSRead(ctx, d, m)
}
