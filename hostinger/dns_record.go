package hostinger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// dnsRecordCreateTimeout bounds how long a create waits for the new record to
// show up in the zone. It is a variable so tests can shorten it.
var dnsRecordCreateTimeout = 30 * time.Second

// errEmptyDNSRecordID signals that the resource has no ID yet, which callers
// treat as "record not in state" rather than as a malformed ID.
var errEmptyDNSRecordID = errors.New("DNS record ID is empty")

// DNSRecordContent is a single record belonging to a DNS entry
type DNSRecordContent struct {
	Content    string `json:"content"`
	IsDisabled bool   `json:"is_disabled"`
}

// DNSEntry represents a DNS entry from the Hostinger API
type DNSEntry struct {
	Name    string             `json:"name"`
	Type    string             `json:"type"`
	TTL     int                `json:"ttl"`
	Records []DNSRecordContent `json:"records"`
}

// normalizeDNSName normalizes DNS names for comparison by converting to lowercase and removing trailing dots
func normalizeDNSName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// compareTXTContent compares TXT record content, handling quotes appropriately while preserving case sensitivity
func compareTXTContent(content1, content2 string) bool {
	// Remove surrounding quotes for comparison, but preserve case
	clean1 := strings.Trim(content1, "\"")
	clean2 := strings.Trim(content2, "\"")
	return clean1 == clean2
}

// dnsContentMatches reports whether the content returned by the API describes the
// same record as the value configured in terraform.
//
// The API canonicalizes content before storing it, so the two strings rarely match
// byte for byte: hostname targets gain a trailing dot (CNAME, NS, ALIAS, and the
// last field of MX and SRV), TXT values are wrapped in quotes and CAA values are
// re-quoted. Comparing the raw strings makes a freshly created record look missing.
func dnsContentMatches(recordType, apiContent, configValue string) bool {
	switch strings.ToUpper(strings.TrimSpace(recordType)) {
	case "TXT":
		// TXT records are case-sensitive and quoted by the API
		return compareTXTContent(apiContent, configValue)
	case "CNAME", "NS", "ALIAS":
		return normalizeDNSName(apiContent) == normalizeDNSName(configValue)
	case "MX", "SRV":
		// "<priority> <target.>" and "<priority> <weight> <port> <target.>"
		return compareTrailingHostContent(apiContent, configValue)
	case "CAA":
		// "<flag> <tag> \"<value>\""
		return compareCAAContent(apiContent, configValue)
	default:
		return strings.EqualFold(strings.TrimSpace(apiContent), strings.TrimSpace(configValue))
	}
}

// compareTrailingHostContent compares whitespace separated content whose last field
// is a hostname, such as MX ("10 mx.example.com.") and SRV records.
func compareTrailingHostContent(apiContent, configValue string) bool {
	apiFields := strings.Fields(apiContent)
	configFields := strings.Fields(configValue)

	if len(apiFields) != len(configFields) || len(apiFields) == 0 {
		return false
	}

	last := len(apiFields) - 1
	for i := 0; i < last; i++ {
		if !strings.EqualFold(apiFields[i], configFields[i]) {
			return false
		}
	}

	return normalizeDNSName(apiFields[last]) == normalizeDNSName(configFields[last])
}

// compareCAAContent compares CAA content, whose value the API returns quoted.
func compareCAAContent(apiContent, configValue string) bool {
	apiFields := strings.SplitN(strings.TrimSpace(apiContent), " ", 3)
	configFields := strings.SplitN(strings.TrimSpace(configValue), " ", 3)

	if len(apiFields) != 3 || len(configFields) != 3 {
		return strings.EqualFold(strings.TrimSpace(apiContent), strings.TrimSpace(configValue))
	}

	return strings.EqualFold(apiFields[0], configFields[0]) &&
		strings.EqualFold(apiFields[1], configFields[1]) &&
		strings.EqualFold(strings.Trim(apiFields[2], "\""), strings.Trim(configFields[2], "\""))
}

// dnsRecordID builds the synthetic ID used to track a single record.
func dnsRecordID(name, recordType, value string) string {
	return fmt.Sprintf("%s|%s|%s", name, recordType, value)
}

// parseDNSRecordID splits a synthetic resource ID back into its parts. The value is
// the remainder of the ID so that contents containing "|" survive a round trip.
func parseDNSRecordID(id string) (name, recordType, value string, err error) {
	if id == "" {
		return "", "", "", errEmptyDNSRecordID
	}

	parts := strings.SplitN(id, "|", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("unexpected ID format: %s (expected \"name|type|value\")", id)
	}

	return parts[0], parts[1], parts[2], nil
}

func resourceHostingerDNSRecord() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerDNSRecordCreate,
		ReadContext:   resourceHostingerDNSRecordRead,
		DeleteContext: resourceHostingerDNSRecordDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceHostingerDNSRecordImport,
		},

		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"value": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"ttl": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  14400,
				ForceNew: true,
			},
			"overwrite": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
				Description: "If true, existing records matching the same name and type are replaced " +
					"instead of appended to. Required to adopt a name/type that already has records.",
			},
		},
	}
}

func resourceHostingerDNSRecordCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.FromErr(createDNSRecord(ctx, d, meta))
}

func resourceHostingerDNSRecordRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.FromErr(readDNSRecord(ctx, d, meta))
}

func resourceHostingerDNSRecordDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.FromErr(deleteDNSRecord(ctx, d, meta))
}

// resourceHostingerDNSRecordImport adopts an existing record into state from an
// import ID of the form "zone|name|type|value".
func resourceHostingerDNSRecordImport(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "|", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf(
			"unexpected import ID format: %s (expected \"zone|name|type|value\", "+
				"for example \"example.com|staging|CNAME|cname.vercel-dns.com\")",
			d.Id(),
		)
	}

	zone, name, recordType, value := parts[0], parts[1], parts[2], parts[3]

	if err := d.Set("zone", zone); err != nil {
		return nil, fmt.Errorf("error setting zone: %w", err)
	}
	if err := d.Set("name", name); err != nil {
		return nil, fmt.Errorf("error setting name: %w", err)
	}
	if err := d.Set("type", recordType); err != nil {
		return nil, fmt.Errorf("error setting type: %w", err)
	}
	if err := d.Set("value", value); err != nil {
		return nil, fmt.Errorf("error setting value: %w", err)
	}
	d.SetId(dnsRecordID(name, recordType, value))

	return []*schema.ResourceData{d}, nil
}

// findDNSRecord looks up a single record in a zone. It reports whether a matching,
// enabled record exists and returns the entry it belongs to.
func findDNSRecord(ctx context.Context, client *HostingerClient, zone, name, recordType, value string) (*DNSEntry, bool, error) {
	entries, err := listDNSEntries(ctx, client, zone)
	if err != nil {
		return nil, false, err
	}

	for i := range entries {
		entry := &entries[i]

		if normalizeDNSName(entry.Name) != normalizeDNSName(name) || !strings.EqualFold(entry.Type, recordType) {
			continue
		}

		for _, rec := range entry.Records {
			if rec.IsDisabled {
				continue
			}
			if dnsContentMatches(recordType, rec.Content, value) {
				return entry, true, nil
			}
		}
	}

	return nil, false, nil
}

// listDNSEntries fetches every DNS entry of a zone.
func listDNSEntries(ctx context.Context, client *HostingerClient, zone string) ([]DNSEntry, error) {
	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read DNS records: %s", body)
	}

	var entries []DNSEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal read response: %w", err)
	}

	return entries, nil
}

// setDNSRecordState writes the record back to state. The configured value is kept
// as written rather than replaced by the canonicalized content the API returns, so
// that a matching record does not produce a permanent diff.
func setDNSRecordState(d *schema.ResourceData, zone, name, recordType, value string, ttl int) error {
	if err := d.Set("zone", zone); err != nil {
		return fmt.Errorf("error setting zone: %w", err)
	}
	if err := d.Set("name", name); err != nil {
		return fmt.Errorf("error setting name: %w", err)
	}
	if err := d.Set("type", recordType); err != nil {
		return fmt.Errorf("error setting type: %w", err)
	}
	if err := d.Set("value", value); err != nil {
		return fmt.Errorf("error setting value: %w", err)
	}
	if err := d.Set("ttl", ttl); err != nil {
		return fmt.Errorf("error setting ttl: %w", err)
	}
	d.SetId(dnsRecordID(name, recordType, value))

	return nil
}

func createDNSRecord(ctx context.Context, d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)
	name := d.Get("name").(string)
	recordType := d.Get("type").(string)
	value := d.Get("value").(string)
	ttl := d.Get("ttl").(int)
	overwrite := d.Get("overwrite").(bool)

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	payload := map[string]interface{}{
		"overwrite": overwrite,
		"zone": []map[string]interface{}{
			{
				"name": name,
				"type": recordType,
				"ttl":  ttl,
				"records": []map[string]interface{}{
					{
						"content": value,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal create payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create DNS record: %s", respBody)
	}

	// Use synthetic ID to track record uniquely. It is set before verifying so a
	// failed verification does not orphan the record that was just created.
	d.SetId(dnsRecordID(name, recordType, value))

	// The zone is eventually consistent, so poll until the new record shows up.
	// The lookup deliberately does not touch the resource ID: clearing it here
	// used to break the next retry iteration with an ID parsing error.
	var entry *DNSEntry
	err = retry.RetryContext(ctx, dnsRecordCreateTimeout, func() *retry.RetryError {
		found, ok, err := findDNSRecord(ctx, client, zone, name, recordType, value)
		if err != nil {
			return retry.NonRetryableError(err)
		}
		if !ok {
			return retry.RetryableError(fmt.Errorf(
				"waiting for %s record %q to be available in zone %s", recordType, name, zone))
		}
		entry = found
		return nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for DNS record to be created: %w", err)
	}

	return setDNSRecordState(d, zone, name, recordType, value, entry.TTL)
}

func readDNSRecord(ctx context.Context, d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	// Parse synthetic ID
	name, recordType, value, err := parseDNSRecordID(d.Id())
	if err != nil {
		if errors.Is(err, errEmptyDNSRecordID) {
			// Nothing to refresh; the resource is already absent from state.
			d.SetId("")
			return nil
		}
		return err
	}

	zone := d.Get("zone").(string)

	// If zone is empty, this indicates a configuration issue
	if zone == "" {
		return fmt.Errorf("zone is required but not set in resource configuration")
	}

	entry, found, err := findDNSRecord(ctx, client, zone, name, recordType, value)
	if err != nil {
		return err
	}

	if !found {
		// Record not found
		d.SetId("")
		return nil
	}

	return setDNSRecordState(d, zone, name, recordType, value, entry.TTL)
}

func deleteDNSRecord(ctx context.Context, d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)

	// Parse synthetic ID
	name, recordType, valueToDelete, err := parseDNSRecordID(d.Id())
	if err != nil {
		if errors.Is(err, errEmptyDNSRecordID) {
			return nil
		}
		return err
	}

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	// First, fetch all existing records to see if there are other records we need to preserve
	entries, err := listDNSEntries(ctx, client, zone)
	if err != nil {
		return err
	}

	// Collect the records of the same name/type that must survive the delete, since
	// the API can only delete a whole name/type pair at once.
	var recordsToKeep []map[string]interface{}
	for _, entry := range entries {
		if normalizeDNSName(entry.Name) != normalizeDNSName(name) || !strings.EqualFold(entry.Type, recordType) {
			continue
		}

		var keepRecords []map[string]interface{}
		for _, rec := range entry.Records {
			if rec.IsDisabled {
				continue
			}
			if dnsContentMatches(recordType, rec.Content, valueToDelete) {
				continue
			}
			keepRecords = append(keepRecords, map[string]interface{}{
				"content": rec.Content,
			})
		}

		if len(keepRecords) > 0 {
			recordsToKeep = append(recordsToKeep, map[string]interface{}{
				"name":    entry.Name,
				"type":    entry.Type,
				"ttl":     entry.TTL,
				"records": keepRecords,
			})
		}
		break
	}

	// Delete all records of this name/type
	payload := map[string]interface{}{
		"filters": []map[string]interface{}{
			{
				"name": name,
				"type": recordType,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delete payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to delete DNS records: %s", respBody)
	}

	// Recreate the records we want to keep
	if len(recordsToKeep) > 0 {
		// Wait for deletion to propagate
		time.Sleep(2 * time.Second)

		recreatePayload := map[string]interface{}{
			"overwrite": false,
			"zone":      recordsToKeep,
		}

		body, err = json.Marshal(recreatePayload)
		if err != nil {
			return fmt.Errorf("failed to marshal recreate payload: %w", err)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		client.addStandardHeaders(req)

		resp, err = client.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to recreate DNS records: %s", respBody)
		}
	}

	d.SetId("")
	return nil
}
