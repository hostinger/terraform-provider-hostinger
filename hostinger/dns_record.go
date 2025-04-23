package hostinger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s/records", client.BaseURL, zone)

	payload := map[string]interface{}{
		"name":  name,
		"type":  recordType,
		"value": value,
		"ttl":   ttl,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
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
	if resp.StatusCode != 201 {
		return fmt.Errorf("failed to create DNS record: %s", respBody)
	}

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	id := fmt.Sprintf("%v", result["id"])
	d.SetId(id)

	return resourceHostingerDNSRecordRead(d, meta)
}

func resourceHostingerDNSRecordRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)
	id := d.Id()

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s/records", client.BaseURL, zone)

	req, _ := http.NewRequest("GET", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to read DNS records: %s", body)
	}

	var data struct {
		Records []map[string]interface{} `json:"records"`
	}
	json.Unmarshal(body, &data)

	for _, record := range data.Records {
		if fmt.Sprintf("%v", record["id"]) == id {
			d.Set("name", record["name"])
			d.Set("type", record["type"])
			d.Set("value", record["value"])
			d.Set("ttl", int(record["ttl"].(float64)))
			return nil
		}
	}

	d.SetId("")
	return nil
}

func resourceHostingerDNSRecordDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*HostingerClient)

	zone := d.Get("zone").(string)
	id := d.Id()

	url := fmt.Sprintf("%s/api/dns/v1/zones/%s/records/%s", client.BaseURL, zone, id)

	req, _ := http.NewRequest("DELETE", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete DNS record: %s", body)
	}

	d.SetId("")
	return nil
}
