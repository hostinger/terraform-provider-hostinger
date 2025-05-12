package hostinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceHostingerDNSRecord() *schema.Resource {
	return &schema.Resource{
		Create: resourceHostingerDNSRecordCreate,
		Read:   resourceHostingerDNSRecordRead,
		Delete: resourceHostingerDNSRecordDelete,

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
		"overwrite": true,
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
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create DNS record: %s", respBody)
	}

	// Use synthetic ID to track record uniquely
	d.SetId(fmt.Sprintf("%s|%s|%s", name, recordType, value))

	return resourceHostingerDNSRecordRead(d, meta)
}

func resourceHostingerDNSRecordRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)

	// Parse synthetic ID
	parts := strings.Split(d.Id(), "|")
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
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to read DNS records: %s", body)
	}

	var entries []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		TTL     int    `json:"ttl"`
		Records []struct {
			Content    string `json:"content"`
			IsDisabled bool   `json:"is_disabled"`
		} `json:"records"`
	}

	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal read response: %w", err)
	}

	// Normalize helper
	normalize := func(s string) string {
		return strings.TrimSuffix(strings.ToLower(s), ".")
	}

	for _, entry := range entries {
		if normalize(entry.Name) == normalize(name) && strings.EqualFold(entry.Type, recordType) {
			for _, rec := range entry.Records {
				if !rec.IsDisabled && normalize(rec.Content) == normalize(value) {
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


func resourceHostingerDNSRecordDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)

	// Parse synthetic ID
	parts := strings.Split(d.Id(), "|")
	if len(parts) != 3 {
		return fmt.Errorf("unexpected ID format: %s", d.Id())
	}
	name, recordType, value := parts[0], parts[1], parts[2]

	// First fetch zone data to find record index
	url := fmt.Sprintf("%s/api/dns/v1/zones/%s", client.BaseURL, zone)

	req, _ := http.NewRequest("GET", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch DNS zone to find record for deletion: %s", body)
	}

	var entries []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		TTL     int    `json:"ttl"`
		Records []struct {
			Content    string `json:"content"`
			IsDisabled bool   `json:"is_disabled"`
		} `json:"records"`
	}

	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("failed to parse DNS zone during delete: %w", err)
	}

	// Rebuild the zone data minus the record to be deleted
	var updated []map[string]interface{}
	for _, entry := range entries {
		if entry.Name == name && entry.Type == recordType {
			newRecs := []map[string]interface{}{}
			for _, rec := range entry.Records {
				if rec.Content != value {
					newRecs = append(newRecs, map[string]interface{}{
						"content": rec.Content,
					})
				}
			}
			if len(newRecs) > 0 {
				updated = append(updated, map[string]interface{}{
					"name":    entry.Name,
					"type":    entry.Type,
					"ttl":     entry.TTL,
					"records": newRecs,
				})
			}
		} else {
			// keep untouched
			records := []map[string]interface{}{}
			for _, rec := range entry.Records {
				records = append(records, map[string]interface{}{
					"content": rec.Content,
				})
			}
			updated = append(updated, map[string]interface{}{
				"name":    entry.Name,
				"type":    entry.Type,
				"ttl":     entry.TTL,
				"records": records,
			})
		}
	}

	// Overwrite full zone without the deleted record
	delPayload := map[string]interface{}{
		"overwrite": true,
		"zone":      updated,
	}
	delBody, _ := json.Marshal(delPayload)

	delReq, _ := http.NewRequest("PUT", url, bytes.NewBuffer(delBody))
	client.addStandardHeaders(delReq)

	delResp, err := client.HTTPClient.Do(delReq)
	if err != nil {
		return err
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(delResp.Body)
		return fmt.Errorf("failed to update DNS zone during delete: %s", respBody)
	}

	d.SetId("")
	return nil
}
