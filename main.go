package main

import (
    "github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
    "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func main() {
    // Serve the provider plugin
    plugin.Serve(&plugin.ServeOpts{
        ProviderFunc: func() *schema.Provider {
            return Provider()
        },
    })
}
