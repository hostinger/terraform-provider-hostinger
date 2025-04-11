package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceHostingerVPSTemplates() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceHostingerVPSTemplatesRead,
		Schema: map[string]*schema.Schema{
			"templates": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of available OS templates.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "ID of the OS template.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name of the OS template.",
						},
						"description": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Description of the OS template.",
						},
					},
				},
			},
		},
	}
}

func dataSourceHostingerVPSTemplatesRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	req, _ := http.NewRequest("GET", client.BaseURL+"/api/vps/v1/templates", nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return diag.Errorf("failed to fetch templates: %s", resp.Status)
	}

	var result []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		Documentation string `json:"documentation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return diag.FromErr(fmt.Errorf("failed to decode templates: %w", err))
	}

	templates := make([]map[string]interface{}, len(result))
	for i, t := range result {
		templates[i] = map[string]interface{}{
			"id":          t.ID,
			"name":        t.Name,
			"description": t.Description,
		}
	}

	if err := d.Set("templates", templates); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("templates")
	return nil
}

func dataSourceHostingerVPSDataCenters() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceHostingerVPSDataCentersRead,
		Schema: map[string]*schema.Schema{
			"data_centers": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of data center locations available for VPS provisioning.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id":        {Type: schema.TypeInt, Computed: true, Description: "ID of the data center."},
						"name":      {Type: schema.TypeString, Computed: true, Description: "Short code of the data center."},
						"city":      {Type: schema.TypeString, Computed: true, Description: "City where the data center is located."},
						"location":  {Type: schema.TypeString, Computed: true, Description: "Geographical location (region code)."},
						"continent": {Type: schema.TypeString, Computed: true, Description: "Continent of the data center."},
					},
				},
			},
		},
	}
}

func dataSourceHostingerVPSDataCentersRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	req, _ := http.NewRequest("GET", client.BaseURL+"/api/vps/v1/data-centers", nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return diag.FromErr(err)
	}

	d.Set("data_centers", result)
	d.SetId("data_centers")
	return nil
}

func dataSourceHostingerVPSPlans() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceHostingerVPSPlansRead,
		Schema: map[string]*schema.Schema{
			"plans": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of available VPS pricing plans and SKUs.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id":       {Type: schema.TypeString, Computed: true, Description: "SKU ID of the plan."},
						"name":     {Type: schema.TypeString, Computed: true, Description: "Human-readable name of the plan."},
						"category": {Type: schema.TypeString, Computed: true, Description: "Category of the plan (e.g., VPS, Game)."},
					},
				},
			},
		},
	}
}

func dataSourceHostingerVPSPlansRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)

	req, _ := http.NewRequest("GET", client.BaseURL+"/api/billing/v1/catalog", nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	var raw []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return diag.FromErr(err)
	}

	plans := []map[string]interface{}{}
	for _, item := range raw {
		category := item["category"]
		for _, price := range item["prices"].([]interface{}) {
			p := price.(map[string]interface{})
			plans = append(plans, map[string]interface{}{
				"id":       p["id"],
				"name":     p["name"],
				"category": category,
			})
		}
	}

	d.Set("plans", plans)
	d.SetId("plans")
	return nil
}
