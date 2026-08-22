package hostinger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// DNSEntry represents a DNS entry from the Hostinger API
type DNSEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	TTL     int    `json:"ttl"`
	Records []struct {
		Content    string `json:"content"`
		IsDisabled bool   `json:"is_disabled"`
	} `json:"records"`
}

// normalizeDNSName normalizes DNS names for comparison by converting to lowercase and removing trailing dots
func normalizeDNSName(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

// compareTXTContent compares TXT record content, handling quotes appropriately while preserving case sensitivity
func compareTXTContent(content1, content2 string) bool {
	// Remove surrounding quotes for comparison, but preserve case
	clean1 := strings.Trim(content1, "\"")
	clean2 := strings.Trim(content2, "\"")
	return clean1 == clean2
}

func resourceHostingerDNSRecord() *schema.Resource {
	return &schema.Resource{
		Create: resourceHostingerDNSRecordCreate,
		Read:   resourceHostingerDNSRecordRead,
		Delete: resourceHostingerDNSRecordDelete,

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
		},
	}
}

func resourceHostingerDNSRecordCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)
	name := d.Get("name").(string)
	recordType := d.Get("type").(string)
	value := d.Get("value").(string)
	ttl := d.Get("ttl").(int)

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	payload := map[string]interface{}{
		"overwrite": false,
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

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(body))
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

	// Use synthetic ID to track record uniquely
	d.SetId(fmt.Sprintf("%s|%s|%s", name, recordType, value))

	// Use retry logic to handle eventual consistency
	err = retry.RetryContext(context.Background(), 30*time.Second, func() *retry.RetryError {
		err := resourceHostingerDNSRecordRead(d, meta)
		if err != nil {
			return retry.NonRetryableError(err)
		}
		// Check if the record was found
		if d.Id() == "" {
			return retry.RetryableError(fmt.Errorf("waiting for DNS record to be available"))
		}
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("error waiting for DNS record to be created: %w", err)
	}
	
	return nil
}

func resourceHostingerDNSRecordRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)
	
	// If zone is empty, this indicates a configuration issue
	if zone == "" {
		return fmt.Errorf("zone is required but not set in resource configuration")
	}

	// Parse synthetic ID
	// SplitN with a limit of 3 keeps everything after the second separator as
	// the value, so a value containing "|" (TXT content, for instance) parses
	// back out intact.
	parts := strings.SplitN(d.Id(), "|", 3)
	if len(parts) != 3 {
		return fmt.Errorf("unexpected ID format: %s", d.Id())
	}
	name, recordType, value := parts[0], parts[1], parts[2]

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	req, _ := http.NewRequest("GET", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to read DNS records: %s", body)
	}

	var entries []DNSEntry

	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal read response: %w", err)
	}

	for _, entry := range entries {
		// Normalize names for comparison (case-insensitive, no trailing dots)
		entryNameNorm := normalizeDNSName(entry.Name)
		searchNameNorm := normalizeDNSName(name)
		
		if entryNameNorm == searchNameNorm && strings.EqualFold(entry.Type, recordType) {
			for _, rec := range entry.Records {
				if rec.IsDisabled {
					continue
				}
				
				// Compare content based on record type
				var contentMatch bool
				if recordType == "TXT" {
					// TXT records: case-sensitive content, handle quotes
					contentMatch = compareTXTContent(rec.Content, value)
				} else {
					// Other records: case-insensitive content comparison
					contentMatch = strings.EqualFold(rec.Content, value)
				}
				
				if contentMatch {
					// Set all fields including zone which was missing
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
					if err := d.Set("ttl", entry.TTL); err != nil {
						return fmt.Errorf("error setting ttl: %w", err)
					}
					d.SetId(fmt.Sprintf("%s|%s|%s", name, recordType, value))
					return nil
				}
			}
		}
	}

	// Record not found
	d.SetId("")
	return nil
}

// resourceHostingerDNSRecordImport brings an existing DNS record under
// management. Records already present in a zone cannot be created -- the create
// path appends with overwrite=false, and Hostinger rejects an append that
// duplicates an existing record -- so import is the only route from a populated
// zone to managed state.
//
// The import ID is "zone|name|type|value". Read derives every attribute from
// the zone plus the "name|type|value" ID it already uses, so import only has to
// seed those two and delegate, which keeps name and content matching in one
// place.
func resourceHostingerDNSRecordImport(_ context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// SplitN with a limit of 4 leaves any "|" in the value as part of the
	// remainder, so values such as TXT content survive the round trip.
	parts := strings.SplitN(d.Id(), "|", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("unexpected import ID format %q: expected \"zone|name|type|value\"", d.Id())
	}
	zone, name, recordType, value := parts[0], parts[1], parts[2], parts[3]

	if zone == "" || name == "" || recordType == "" {
		return nil, fmt.Errorf("unexpected import ID format %q: zone, name and type must all be set", d.Id())
	}

	if err := d.Set("zone", zone); err != nil {
		return nil, fmt.Errorf("error setting zone: %w", err)
	}
	d.SetId(fmt.Sprintf("%s|%s|%s", name, recordType, value))

	if err := resourceHostingerDNSRecordRead(d, meta); err != nil {
		return nil, err
	}

	// Read clears the ID when nothing in the zone matches. Report that as an
	// error rather than handing back an empty resource, which Terraform would
	// otherwise surface as a confusing "resource does not exist" after import.
	if d.Id() == "" {
		return nil, fmt.Errorf("no %s record named %q with value %q found in zone %q", recordType, name, value, zone)
	}

	return []*schema.ResourceData{d}, nil
}

func resourceHostingerDNSRecordDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)

	// Parse synthetic ID  
	// SplitN with a limit of 3 keeps everything after the second separator as
	// the value, so a value containing "|" (TXT content, for instance) parses
	// back out intact.
	parts := strings.SplitN(d.Id(), "|", 3)
	if len(parts) != 3 {
		return fmt.Errorf("unexpected ID format: %s", d.Id())
	}
	name, recordType, valueToDelete := parts[0], parts[1], parts[2]

	// First, fetch all existing records to see if there are other records we need to preserve
	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to read DNS records: %s", body)
	}

	var entries []DNSEntry

	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal records: %w", err)
	}

	// Check if there are other records of the same name/type that we need to preserve
	hasOtherRecords := false
	for _, entry := range entries {
		if normalizeDNSName(entry.Name) == normalizeDNSName(name) && strings.EqualFold(entry.Type, recordType) {
			for _, rec := range entry.Records {
				if !rec.IsDisabled {
					// Compare content appropriately based on record type
					var contentMatch bool
					if recordType == "TXT" {
						contentMatch = compareTXTContent(rec.Content, valueToDelete)
					} else {
						contentMatch = strings.EqualFold(rec.Content, valueToDelete)
					}
					
					if !contentMatch {
						hasOtherRecords = true
						break
					}
				}
			}
			break
		}
	}

	if hasOtherRecords {
		// The Hostinger API doesn't support deleting individual records.
		// We need to delete all and recreate the ones we want to keep.
		
		// First, collect all records we want to keep
		var recordsToKeep []map[string]interface{}
		for _, entry := range entries {
			if normalizeDNSName(entry.Name) == normalizeDNSName(name) && strings.EqualFold(entry.Type, recordType) {
				var keepRecords []map[string]interface{}
				for _, rec := range entry.Records {
					if !rec.IsDisabled {
						// Compare content appropriately based on record type
						var contentMatch bool
						if recordType == "TXT" {
							contentMatch = compareTXTContent(rec.Content, valueToDelete)
						} else {
							contentMatch = strings.EqualFold(rec.Content, valueToDelete)
						}
						
						if !contentMatch {
							keepRecords = append(keepRecords, map[string]interface{}{
								"content": rec.Content,
							})
						}
					}
				}
				if len(keepRecords) > 0 {
					recordsToKeep = append(recordsToKeep, map[string]interface{}{
						"name": entry.Name,
						"type": entry.Type,
						"ttl":  entry.TTL,
						"records": keepRecords,
					})
				}
				break
			}
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

		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal delete payload: %w", err)
		}

		req, err = http.NewRequest("DELETE", url, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		client.addStandardHeaders(req)

		resp, err = client.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to delete DNS records: %s", respBody)
		}
		
		// Recreate the records we want to keep
		if len(recordsToKeep) > 0 {
			// Wait for deletion to propagate
			time.Sleep(2 * time.Second)
			
			recreatePayload := map[string]interface{}{
				"overwrite": false,
				"zone": recordsToKeep,
			}

			body, err = json.Marshal(recreatePayload)
			if err != nil {
				return fmt.Errorf("failed to marshal recreate payload: %w", err)
			}

			req, err = http.NewRequest("PUT", url, bytes.NewBuffer(body))
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

	// Safe to delete - no other records of same type
	payload := map[string]interface{}{
		"filters": []map[string]interface{}{
			{
				"name": name,
				"type": recordType,
			},
		},
	}

	body, err = json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delete payload: %w", err)
	}

	req, err = http.NewRequest("DELETE", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	client.addStandardHeaders(req)

	resp, err = client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to delete DNS record: %s", respBody)
	}

	d.SetId("")
	return nil
}
