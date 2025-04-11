package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/hostinger/terraform-provider-hostinger/hostinger"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: hostinger.Provider,
	})
}
