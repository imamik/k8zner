package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

func TestHCloudAPI_Spike(t *testing.T) {
	token := os.Getenv("HCLOUD_TOKEN")
	if token == "" {
		t.Skip("HCLOUD_TOKEN not set, skipping e2e spike")
	}

	client := hcloud.NewClient(hcloud.WithToken(token))
	ctx := context.Background()

	runID := fmt.Sprintf("spike-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Intn(10000))
	fwName := fmt.Sprintf("test-fw-%s", runID)

	t.Logf("Creating firewall: %s", fwName)

	// 1. Create Firewall
	opts := hcloud.FirewallCreateOpts{
		Name: fwName,
		Rules: []hcloud.FirewallRule{
			{
				Direction: hcloud.FirewallRuleDirectionIn,
				Protocol:  hcloud.FirewallRuleProtocolICMP,
				SourceIPs: []net.IPNet{{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}},
			},
		},
	}
	res, _, err := client.Firewall.Create(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to create firewall: %v", err)
	}
	fw := res.Firewall
	defer func() {
		t.Logf("Deleting firewall: %s", fwName)
		if _, err := client.Firewall.Delete(ctx, fw); err != nil {
			t.Errorf("Failed to delete firewall: %v", err)
		}
	}()

	t.Logf("Firewall created with ID: %d", fw.ID)

	// 2. Update Rules (SetRules)
	t.Logf("Updating rules for firewall: %s", fwName)
	newRules := []hcloud.FirewallRule{
		{
			Direction: hcloud.FirewallRuleDirectionIn,
			Protocol:  hcloud.FirewallRuleProtocolTCP,
			Port:      hcloud.Ptr("80"),
			SourceIPs: []net.IPNet{{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}},
		},
	}
	optsSetRules := hcloud.FirewallSetRulesOpts{
		Rules: newRules,
	}

	// Verify SetRules exists and works
	actions, _, err := client.Firewall.SetRules(ctx, fw, optsSetRules)
	if err != nil {
		t.Fatalf("Failed to SetRules: %v", err)
	}

	// Wait for action if needed (SetRules is async-ish but returns action)
	for _, action := range actions {
		t.Logf("SetRules action status: %s", action.Status)
	}

    // 3. Update Metadata (Update)
    t.Logf("Updating metadata (labels)")
    updateOpts := hcloud.FirewallUpdateOpts{
        Labels: map[string]string{"test": "true"},
    }
    updatedFw, _, err := client.Firewall.Update(ctx, fw, updateOpts)
    if err != nil {
        t.Fatalf("Failed to Update: %v", err)
    }
    if updatedFw.Labels["test"] != "true" {
        t.Errorf("Labels not updated")
    }
}
