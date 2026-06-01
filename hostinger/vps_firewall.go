package hostinger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// firewallProtocols are the protocol values accepted by the Hostinger firewall API.
var firewallProtocols = []string{
	"TCP", "UDP", "ICMP", "GRE", "any", "ESP", "AH", "ICMPv6",
	"SSH", "HTTP", "HTTPS", "MySQL", "PostgreSQL",
}

// Firewall mirrors VPS.V1.Firewall.FirewallResource.
type Firewall struct {
	ID        int            `json:"id"`
	Name      string         `json:"name"`
	IsSynced  bool           `json:"is_synced"`
	Rules     []FirewallRule `json:"rules"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// FirewallRule mirrors VPS.V1.Firewall.FirewallRuleResource.
type FirewallRule struct {
	ID           int    `json:"id"`
	Action       string `json:"action"`
	Protocol     string `json:"protocol"`
	Port         string `json:"port"`
	Source       string `json:"source"`
	SourceDetail string `json:"source_detail"`
}

func resourceHostingerVPSFirewall() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSFirewallCreate,
		ReadContext:   resourceHostingerVPSFirewallRead,
		UpdateContext: resourceHostingerVPSFirewallUpdate,
		DeleteContext: resourceHostingerVPSFirewallDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceHostingerVPSFirewallImport,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "Name of the firewall. The Hostinger API has no rename endpoint, so changing this replaces the firewall.",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"rule": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Ordered list of accept rules. The firewall drops all incoming traffic by default, so every rule grants access for a protocol/port. Editing a rule in place updates it via the API (keeping its ID); removing the last rules deletes them and appending new ones creates them.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"protocol": {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "Protocol the rule applies to.",
							ValidateFunc: validation.StringInSlice(firewallProtocols, false),
						},
						"port": {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "Destination port or port range, e.g. \"443\" or \"1024:2048\".",
							ValidateFunc: validateFirewallPort,
						},
						"source": {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "Source kind: \"any\" or \"custom\".",
							ValidateFunc: validation.StringInSlice([]string{"any", "custom"}, false),
						},
						"source_detail": {
							Type:         schema.TypeString,
							Required:     true,
							Description:  "Source detail: \"any\", a single IP, a CIDR, or an IP range.",
							ValidateFunc: validation.StringIsNotEmpty,
						},
						"id": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Server-assigned ID of the rule.",
						},
						"action": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Action applied by the rule. Always \"accept\" (the firewall drops everything else by default).",
						},
					},
				},
			},
			"is_synced": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Whether the firewall is in sync with the VPS instances it is activated on. Changing rules sets this to false until a sync is performed.",
			},
			"created_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp when the firewall was created.",
			},
			"updated_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp when the firewall was last updated.",
			},
		},
	}
}

func resourceHostingerVPSFirewallCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	fw, err := client.CreateFirewall(d.Get("name").(string))
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to create firewall: %w", err))
	}
	// Set the ID immediately so partial failures while creating rules remain tracked.
	d.SetId(strconv.Itoa(fw.ID))

	if v, ok := d.GetOk("rule"); ok {
		for _, raw := range v.([]interface{}) {
			rule := expandFirewallRule(raw.(map[string]interface{}))
			if _, err := client.CreateFirewallRule(fw.ID, rule); err != nil {
				return diag.FromErr(fmt.Errorf("failed to create firewall rule (%s/%s): %w", rule.Protocol, rule.Port, err))
			}
		}
	}

	return resourceHostingerVPSFirewallRead(ctx, d, m)
}

func resourceHostingerVPSFirewallRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid firewall ID %q: %w", d.Id(), err))
	}

	fw, err := client.GetFirewall(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("failed to read firewall: %w", err))
	}

	if err := d.Set("name", fw.Name); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set name: %w", err))
	}
	if err := d.Set("is_synced", fw.IsSynced); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set is_synced: %w", err))
	}
	if err := d.Set("created_at", fw.CreatedAt); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set created_at: %w", err))
	}
	if err := d.Set("updated_at", fw.UpdatedAt); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set updated_at: %w", err))
	}
	if err := d.Set("rule", flattenFirewallRules(fw.Rules)); err != nil {
		return diag.FromErr(fmt.Errorf("failed to set rule: %w", err))
	}

	return nil
}

func resourceHostingerVPSFirewallUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid firewall ID %q: %w", d.Id(), err))
	}

	if d.HasChange("rule") {
		oldRaw, newRaw := d.GetChange("rule")
		updates, creates, deletes := planFirewallRuleChanges(oldRaw.([]interface{}), newRaw.([]interface{}))

		// Update edited rules in place via PUT so they keep their server-assigned ID.
		for _, u := range updates {
			if _, err := client.UpdateFirewallRule(id, u.ID, u.Rule); err != nil {
				return diag.FromErr(fmt.Errorf("failed to update firewall rule %d: %w", u.ID, err))
			}
		}
		// Delete rules removed from the end of the list.
		for _, ruleID := range deletes {
			if err := client.DeleteFirewallRule(id, ruleID); err != nil {
				return diag.FromErr(fmt.Errorf("failed to delete firewall rule %d: %w", ruleID, err))
			}
		}
		// Create rules appended to the list.
		for _, rule := range creates {
			if _, err := client.CreateFirewallRule(id, rule); err != nil {
				return diag.FromErr(fmt.Errorf("failed to create firewall rule (%s/%s): %w", rule.Protocol, rule.Port, err))
			}
		}
	}

	return resourceHostingerVPSFirewallRead(ctx, d, m)
}

func resourceHostingerVPSFirewallDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid firewall ID %q: %w", d.Id(), err))
	}

	if err := client.DeleteFirewall(id); err != nil {
		return diag.FromErr(fmt.Errorf("failed to delete firewall: %w", err))
	}

	d.SetId("")
	return nil
}

func resourceHostingerVPSFirewallImport(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	if _, err := strconv.Atoi(d.Id()); err != nil {
		return nil, fmt.Errorf("invalid firewall import ID %q: expected a numeric firewall ID", d.Id())
	}
	return []*schema.ResourceData{d}, nil
}

// ruleUpdate pairs an existing rule's server-assigned ID with the desired new content,
// for an in-place update via the PUT endpoint.
type ruleUpdate struct {
	ID   int
	Rule FirewallRule
}

// planFirewallRuleChanges diffs the old and new rule lists positionally and returns the
// in-place updates (PUT), appended creations (POST), and removed rule IDs (DELETE) needed
// to converge. Positional diffing lets an edited rule keep its server-assigned ID via the
// update endpoint instead of being destroyed and recreated. Only the four user-supplied
// fields are compared, so the computed id/action attributes never trigger a spurious update.
func planFirewallRuleChanges(oldList, newList []interface{}) (updates []ruleUpdate, creates []FirewallRule, deletes []int) {
	overlap := len(oldList)
	if len(newList) < overlap {
		overlap = len(newList)
	}
	for i := 0; i < overlap; i++ {
		oldMap := oldList[i].(map[string]interface{})
		newRule := expandFirewallRule(newList[i].(map[string]interface{}))
		if expandFirewallRule(oldMap) != newRule {
			updates = append(updates, ruleUpdate{ID: oldMap["id"].(int), Rule: newRule})
		}
	}
	for i := overlap; i < len(oldList); i++ {
		deletes = append(deletes, oldList[i].(map[string]interface{})["id"].(int))
	}
	for i := overlap; i < len(newList); i++ {
		creates = append(creates, expandFirewallRule(newList[i].(map[string]interface{})))
	}
	return updates, creates, deletes
}

func expandFirewallRule(m map[string]interface{}) FirewallRule {
	return FirewallRule{
		Protocol:     m["protocol"].(string),
		Port:         m["port"].(string),
		Source:       m["source"].(string),
		SourceDetail: m["source_detail"].(string),
	}
}

func flattenFirewallRules(rules []FirewallRule) []interface{} {
	// Sort by server-assigned ID so the stored order is stable across reads, regardless
	// of the order the API returns rules in. Combined with positional diffing in Update,
	// this keeps the list order consistent with the configuration after an apply.
	sorted := make([]FirewallRule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	out := make([]interface{}, 0, len(sorted))
	for _, r := range sorted {
		out = append(out, map[string]interface{}{
			"id":            r.ID,
			"action":        r.Action,
			"protocol":      r.Protocol,
			"port":          r.Port,
			"source":        r.Source,
			"source_detail": r.SourceDetail,
		})
	}
	return out
}

// validateFirewallPort accepts a single port or an inclusive "lo:hi" range, each port
// in 1-65535 and lo <= hi, so out-of-range or reversed ports are rejected at plan time
// instead of failing later with a raw HTTP 422 from the API.
func validateFirewallPort(i interface{}, k string) (warnings []string, errs []error) {
	v, ok := i.(string)
	if !ok {
		return nil, []error{fmt.Errorf("expected type of %q to be string", k)}
	}

	parsePort := func(s string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("%q is not a valid port number", s)
		}
		if n < 1 || n > 65535 {
			return 0, fmt.Errorf("port %d is out of range (must be 1-65535)", n)
		}
		return n, nil
	}

	parts := strings.Split(v, ":")
	switch len(parts) {
	case 1:
		if _, err := parsePort(parts[0]); err != nil {
			errs = append(errs, fmt.Errorf("invalid %q: %w", k, err))
		}
	case 2:
		lo, err := parsePort(parts[0])
		if err != nil {
			return nil, []error{fmt.Errorf("invalid %q: %w", k, err)}
		}
		hi, err := parsePort(parts[1])
		if err != nil {
			return nil, []error{fmt.Errorf("invalid %q: %w", k, err)}
		}
		if lo > hi {
			errs = append(errs, fmt.Errorf("invalid %q: range start %d must not exceed end %d", k, lo, hi))
		}
	default:
		errs = append(errs, fmt.Errorf("invalid %q %q: expected a single port (e.g. \"443\") or a range (e.g. \"1024:2048\")", k, v))
	}
	return warnings, errs
}

// firewallRuleRequestBody is the payload the rule create/update endpoints accept.
// action and id are read-only and must not be sent.
func firewallRuleRequestBody(r FirewallRule) map[string]string {
	return map[string]string{
		"protocol":      r.Protocol,
		"port":          r.Port,
		"source":        r.Source,
		"source_detail": r.SourceDetail,
	}
}

// HostingerClient implementations:

func (c *HostingerClient) CreateFirewall(name string) (*Firewall, error) {
	url := c.BaseURL + "/api/vps/v1/firewall"
	data, _ := json.Marshal(map[string]string{"name": name})

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create firewall failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var res Firewall
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *HostingerClient) GetFirewall(id int) (*Firewall, error) {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d", c.BaseURL, id)
	req, _ := http.NewRequest("GET", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("read firewall failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var res Firewall
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *HostingerClient) DeleteFirewall(id int) error {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d", c.BaseURL, id)
	req, _ := http.NewRequest("DELETE", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete firewall failed (HTTP %d): %s", resp.StatusCode, msg)
	}
	return nil
}

func (c *HostingerClient) CreateFirewallRule(firewallID int, r FirewallRule) (*FirewallRule, error) {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d/rules", c.BaseURL, firewallID)
	data, _ := json.Marshal(firewallRuleRequestBody(r))

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create firewall rule failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var res FirewallRule
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *HostingerClient) UpdateFirewallRule(firewallID, ruleID int, r FirewallRule) (*FirewallRule, error) {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d/rules/%d", c.BaseURL, firewallID, ruleID)
	data, _ := json.Marshal(firewallRuleRequestBody(r))

	req, _ := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update firewall rule failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var res FirewallRule
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *HostingerClient) DeleteFirewallRule(firewallID, ruleID int) error {
	url := fmt.Sprintf("%s/api/vps/v1/firewall/%d/rules/%d", c.BaseURL, firewallID, ruleID)
	req, _ := http.NewRequest("DELETE", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete firewall rule failed (HTTP %d): %s", resp.StatusCode, msg)
	}
	return nil
}
