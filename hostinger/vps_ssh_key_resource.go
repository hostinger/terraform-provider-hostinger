package hostinger

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"bytes"
	"net/http"
	"io"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type SSHKey struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

func resourceHostingerVPSSSHKey() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceHostingerVPSSSHKeyCreate,
		ReadContext:   resourceHostingerVPSSSHKeyRead,
		DeleteContext: resourceHostingerVPSSSHKeyDelete,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the SSH public key.",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				ForceNew:    true,
				Description: "The actual SSH public key string.",
				ValidateFunc: validation.StringMatch(
					regexp.MustCompile(`^(ssh-(rsa|ed25519|ecdsa)) `),
					"must be a valid SSH public key (ssh-rsa, ssh-ed25519...)",
				  ),
			},
		},
	}
}

func resourceHostingerVPSSSHKeyCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	name := d.Get("name").(string)
	key := d.Get("key").(string)

	url := client.BaseURL + "/api/vps/v1/public-keys"
	payload := map[string]string{"name": name, "key": key}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return diag.FromErr(fmt.Errorf("create ssh key failed: %s", msg))
	}

	var keyResp SSHKey
	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(strconv.Itoa(keyResp.ID))
	return resourceHostingerVPSSSHKeyRead(ctx, d, m)
}

func resourceHostingerVPSSSHKeyRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	id := d.Id()
	url := client.BaseURL + "/api/vps/v1/public-keys"

	req, _ := http.NewRequest("GET", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return diag.FromErr(fmt.Errorf("read ssh key failed: %s", msg))
	}

	var result struct {
		Data []SSHKey `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return diag.FromErr(err)
	}

	keyID, _ := strconv.Atoi(id)
	for _, k := range result.Data {
		if k.ID == keyID {
			d.Set("name", k.Name)
			d.Set("key", k.Key)
			return nil
		}
	}
	d.SetId("") // not found
	return nil
}

func resourceHostingerVPSSSHKeyDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*HostingerClient)
	id := d.Id()
	url := fmt.Sprintf("%s/api/vps/v1/public-keys/%s", client.BaseURL, id)

	req, _ := http.NewRequest("DELETE", url, nil)
	client.addStandardHeaders(req)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return diag.FromErr(fmt.Errorf("delete ssh key failed: %s", msg))
	}
	d.SetId("")
	return nil
}

// Call from the VPS resource after creation or update
func (c *HostingerClient) AttachSSHKeysToVM(vmID int, keyIDs []int) error {
	url := fmt.Sprintf("%s/api/vps/v1/public-keys/attach/%d", c.BaseURL, vmID)
	payload := map[string]interface{}{
		"ids": keyIDs,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("attach ssh keys failed: %s", msg)
	}
	return nil
}
