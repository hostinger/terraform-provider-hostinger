package hostinger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type SSHKey struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type VMSSHKey struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	Data string `json:"data"`
	Name string `json:"name"`
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
	defer func() { _ = resp.Body.Close() }()

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

	keys, err := client.ListPublicKeys()
	if err != nil {
		return diag.FromErr(fmt.Errorf("read ssh key failed: %w", err))
	}

	keyID, _ := strconv.Atoi(id)
	for _, k := range keys {
		if k.ID == keyID {
			if err := d.Set("name", k.Name); err != nil {
				return diag.FromErr(fmt.Errorf("failed to set name: %w", err))
			}
			if err := d.Set("key", k.Key); err != nil {
				return diag.FromErr(fmt.Errorf("failed to set key: %w", err))
			}
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return diag.FromErr(fmt.Errorf("delete ssh key failed: %s", msg))
	}
	d.SetId("")
	return nil
}

func (c *HostingerClient) ListPublicKeys() ([]SSHKey, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/api/vps/v1/public-keys", nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list public keys failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var result struct {
		Data []SSHKey `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *HostingerClient) ListVMSSHKeys(vmID int) ([]VMSSHKey, error) {
	url := fmt.Sprintf("%s/api/vps/v1/virtual-machines/%d/ssh-keys", c.BaseURL, vmID)

	req, _ := http.NewRequest("GET", url, nil)
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch SSH keys for VM (HTTP %d): %s", resp.StatusCode, msg)
	}

	var keys []VMSSHKey
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (c *HostingerClient) AddVMSSHKeys(vmID int, keys []string) error {
	return c.updateVMSSHKeys(http.MethodPost, vmID, keys)
}

func (c *HostingerClient) RemoveVMSSHKeys(vmID int, keys []string) error {
	return c.updateVMSSHKeys(http.MethodDelete, vmID, keys)
}

func (c *HostingerClient) updateVMSSHKeys(method string, vmID int, keys []string) error {
	url := fmt.Sprintf("%s/api/vps/v1/virtual-machines/%d/ssh-keys", c.BaseURL, vmID)
	body, _ := json.Marshal(map[string][]string{"keys": keys})

	req, _ := http.NewRequest(method, url, bytes.NewBuffer(body))
	c.addStandardHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s ssh keys on VM failed (HTTP %d): %s", method, resp.StatusCode, msg)
	}
	return nil
}
func (c *HostingerClient) publicKeyStringsByID() (map[int]string, error) {
	keys, err := c.ListPublicKeys()
	if err != nil {
		return nil, err
	}
	byID := make(map[int]string, len(keys))
	for _, k := range keys {
		byID[k.ID] = strings.TrimSpace(k.Key)
	}
	return byID, nil
}
