package hostinger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func newVPSServer(t *testing.T) *HostingerClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/vps/v1/virtual-machines/123456" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 123456, "plan": "KVM 4", "subscription_id": "sub-kvm4", "data_center_id": 18, "template": {"id": 1077}, "hostname": "srv123456.hstgr.cloud", "state": "running"}`))
	}))
	t.Cleanup(server.Close)
	return &HostingerClient{BaseURL: server.URL, HTTPClient: http.DefaultClient, Token: "test-token"}
}

func TestVPSImportLeavesPlanUnset(t *testing.T) {
	client := newVPSServer(t)
	d := schema.TestResourceDataRaw(t, resourceHostingerVPS().Schema, map[string]interface{}{})
	d.SetId("123456")

	if _, err := resourceHostingerVPSImport(context.Background(), d, client); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := d.Get("plan").(string); got != "" {
		t.Errorf("expected no plan, got %q", got)
	}
	if got := d.Get("data_center_id").(int); got != 18 {
		t.Errorf("expected data_center_id 18, got %d", got)
	}
}

func vpsPlanDiff(t *testing.T, statePlan string, config map[string]interface{}) *terraform.InstanceDiff {
	t.Helper()
	state := &terraform.InstanceState{ID: "123456", Attributes: map[string]string{
		"plan": statePlan, "data_center_id": "18", "template_id": "1077",
	}}
	config["data_center_id"] = 18
	config["template_id"] = 1077
	diff, err := resourceHostingerVPS().Diff(context.Background(), state, terraform.NewResourceConfigRaw(config), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	return diff
}

func TestVPSImportedIgnoresPlanInConfig(t *testing.T) {
	for name, config := range map[string]map[string]interface{}{
		"omitted":   {},
		"plan name": {"plan": "KVM 4"},
		"plan ID":   {"plan": "hostingercom-vps-kvm4-usd-1y"},
	} {
		t.Run(name, func(t *testing.T) {
			diff := vpsPlanDiff(t, "", config)
			if diff != nil && (diff.RequiresNew() || diff.Attributes["plan"] != nil) {
				t.Errorf("expected no plan change, got %v", diff.Attributes["plan"])
			}
		})
	}
}

func TestVPSPurchasedReplacesOnPlanChange(t *testing.T) {
	diff := vpsPlanDiff(t, "hostingercom-vps-kvm2-usd-1y", map[string]interface{}{"plan": "hostingercom-vps-kvm4-usd-1y"})
	if diff == nil || !diff.RequiresNew() {
		t.Errorf("expected a plan change to replace the VPS, got %v", diff)
	}

	diff = vpsPlanDiff(t, "hostingercom-vps-kvm2-usd-1y", map[string]interface{}{})
	if diff != nil && (diff.RequiresNew() || diff.Attributes["plan"] != nil) {
		t.Errorf("expected no plan change when plan is omitted, got %v", diff.Attributes["plan"])
	}
}

func TestVPSCreateWithoutPlanFailsBeforeAnyRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	client := &HostingerClient{BaseURL: server.URL, HTTPClient: http.DefaultClient, Token: "test-token"}
	d := schema.TestResourceDataRaw(t, resourceHostingerVPS().Schema, map[string]interface{}{
		"data_center_id": 18,
		"template_id":    1077,
	})

	diags := resourceHostingerVPSCreate(context.Background(), d, client)
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "plan is required") {
		t.Errorf("expected a plan is required error, got %v", diags)
	}
}

func TestVPSReadSetsEmptyResolvedSSHKeysAfterImport(t *testing.T) {
	client := newVPSServer(t)
	d := schema.TestResourceDataRaw(t, resourceHostingerVPS().Schema, map[string]interface{}{
		"data_center_id": 18,
		"template_id":    1077,
	})
	d.SetId("123456")

	if diags := resourceHostingerVPSRead(context.Background(), d, client); diags.HasError() {
		t.Fatalf("expected no error, got %v", diags)
	}
	if got := d.State().Attributes["ssh_key_ids_resolved.#"]; got != "0" {
		t.Errorf("expected ssh_key_ids_resolved to be an empty list in state, got count %q", got)
	}
}
