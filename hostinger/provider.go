package hostinger

import (
    "context"

    "github.com/hashicorp/terraform-plugin-sdk/v2/diag"
    "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider returns the *schema.Provider for Hostinger VPS
func Provider() *schema.Provider {
    return &schema.Provider{
        Schema: map[string]*schema.Schema{
            "api_token": {
                Type:        schema.TypeString,
                Required:    true,
                Sensitive:   true,
                Description: "API token for authenticating with Hostinger API.",
                DefaultFunc: schema.EnvDefaultFunc("HOSTINGER_API_TOKEN", nil),
            },
        },
        ResourcesMap: map[string]*schema.Resource{
            "hostinger_vps": resourceHostingerVPS(),
            "hostinger_vps_post_install_script": resourceHostingerVPSPostInstallScript(),
            "hostinger_vps_ssh_key": resourceHostingerVPSSSHKey(),
        },
        DataSourcesMap: map[string]*schema.Resource{
            "hostinger_vps_templates":      dataSourceHostingerVPSTemplates(),
            "hostinger_vps_data_centers":   dataSourceHostingerVPSDataCenters(),
            "hostinger_vps_plans":          dataSourceHostingerVPSPlans(),
        },
        ConfigureContextFunc: providerConfigure,
    }
}

// providerConfigure creates a Hostinger API client using the provided API token
func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
    var diags diag.Diagnostics

    token := d.Get("api_token").(string)
    if token == "" {
        diags = append(diags, diag.Diagnostic{
            Severity: diag.Error,
            Summary:  "API token is required",
            Detail:   "The Hostinger API token must be provided to use this provider.",
        })
        return nil, diags
    }

    // Initialize the Hostinger API client
    client := NewHostingerClient(token, "0.1.0")
    return client, diags
}
