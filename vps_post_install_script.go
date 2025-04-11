package main

import (
	"context"
	"strings"
	"fmt"
	"strconv"
	"encoding/json"
	"bytes"
	"net/http"
	"io"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type PostInstallScript struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func resourceHostingerVPSPostInstallScript() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSPostInstallScriptCreate,
		ReadContext:   resourceHostingerVPSPostInstallScriptRead,
		UpdateContext: resourceHostingerVPSPostInstallScriptUpdate,
		DeleteContext: resourceHostingerVPSPostInstallScriptDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the post-install script.",
			},
			"content": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Shell script content to execute post-install.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
				  return strings.TrimSpace(old) == strings.TrimSpace(new)
				},
			},
		},
	}
}

func resourceHostingerVPSPostInstallScriptCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	name := d.Get("name").(string)
	content := d.Get("content").(string)

	id, err := client.CreatePostInstallScript(name, content)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to create post-install script: %w", err))
	}
	d.SetId(strconv.Itoa(id))
	return resourceHostingerVPSPostInstallScriptRead(ctx, d, m)
}

func resourceHostingerVPSPostInstallScriptRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	id, _ := strconv.Atoi(d.Id())

	script, err := client.GetPostInstallScript(id)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to read post-install script: %w", err))
	}

	d.Set("name", script.Name)
	d.Set("content", script.Content)
	return nil
}

func resourceHostingerVPSPostInstallScriptUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	id, _ := strconv.Atoi(d.Id())
	name := d.Get("name").(string)
	content := d.Get("content").(string)

	err := client.UpdatePostInstallScript(id, name, content)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to update post-install script: %w", err))
	}

	return resourceHostingerVPSPostInstallScriptRead(ctx, d, m)
}

func resourceHostingerVPSPostInstallScriptDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	id, _ := strconv.Atoi(d.Id())

	err := client.DeletePostInstallScript(id)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to delete post-install script: %w", err))
	}

	d.SetId("")
	return nil
}

// HostingerClient implementations:

func (c *HostingerClient) CreatePostInstallScript(name, content string) (int, error) {
	url := c.BaseURL + "/api/vps/v1/post-install-scripts"
	body := map[string]string{"name": name, "content": content}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("create post-install script failed: %s", msg)
	}

	var res PostInstallScript
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}
	return res.ID, nil
}

func (c *HostingerClient) GetPostInstallScript(id int) (*PostInstallScript, error) {
	url := fmt.Sprintf("%s/api/vps/v1/post-install-scripts/%d", c.BaseURL, id)
	req, _ := http.NewRequest("GET", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("read post-install script failed: %s", msg)
	}

	var res PostInstallScript
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *HostingerClient) UpdatePostInstallScript(id int, name, content string) error {
	url := fmt.Sprintf("%s/api/vps/v1/post-install-scripts/%d", c.BaseURL, id)
	body := map[string]string{"name": name, "content": content}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update post-install script failed: %s", msg)
	}
	return nil
}

func (c *HostingerClient) DeletePostInstallScript(id int) error {
	url := fmt.Sprintf("%s/api/vps/v1/post-install-scripts/%d", c.BaseURL, id)
	req, _ := http.NewRequest("DELETE", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete post-install script failed: %s", msg)
	}
	return nil
}
