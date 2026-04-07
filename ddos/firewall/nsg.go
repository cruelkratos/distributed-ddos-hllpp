// Package firewall provides Azure NSG-based network lockdown for DDoS mitigation.
// When attack is detected, it applies a deny-all-inbound rule to the NSG.
// When attack subsides, it removes the rule — a binary lockdown toggle that
// requires no per-IP tracking (preserving the HLL++ memory-efficient design).
package firewall

import (
	"context"
	"log"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

const lockdownRuleName = "ddos-lockdown"

// NSGController manages Azure NSG rules for DDoS lockdown/unlock.
type NSGController struct {
	client        *armnetwork.SecurityRulesClient
	resourceGroup string
	nsgName       string
	locked        bool
	mu            sync.Mutex
}

// NewNSGController creates a controller using DefaultAzureCredential.
func NewNSGController(subscriptionID, resourceGroup, nsgName string) (*NSGController, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err := armnetwork.NewSecurityRulesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	return &NSGController{
		client:        client,
		resourceGroup: resourceGroup,
		nsgName:       nsgName,
	}, nil
}

// Lockdown creates a high-priority deny-all-inbound rule on the NSG.
func (c *NSGController) Lockdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locked {
		return nil
	}

	rule := armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Priority:                 to.Ptr[int32](100),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessDeny),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolAsterisk),
			SourceAddressPrefix:      to.Ptr("Internet"),
			SourcePortRange:          to.Ptr("*"),
			DestinationAddressPrefix: to.Ptr("*"),
			DestinationPortRange:     to.Ptr("*"),
			Description:              to.Ptr("DDoS lockdown - auto-applied by aggregator"),
		},
	}

	poller, err := c.client.BeginCreateOrUpdate(ctx, c.resourceGroup, c.nsgName, lockdownRuleName, rule, nil)
	if err != nil {
		log.Printf("[NSG] ERROR creating lockdown rule: %v", err)
		return err
	}
	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		log.Printf("[NSG] ERROR polling lockdown rule creation: %v", err)
		return err
	}

	c.locked = true
	log.Printf("[NSG] LOCKDOWN: deny-all-inbound rule applied to %s/%s", c.resourceGroup, c.nsgName)
	return nil
}

// Unlock removes the lockdown rule, restoring normal traffic.
func (c *NSGController) Unlock(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.locked {
		return nil
	}

	poller, err := c.client.BeginDelete(ctx, c.resourceGroup, c.nsgName, lockdownRuleName, nil)
	if err != nil {
		log.Printf("[NSG] ERROR deleting lockdown rule: %v", err)
		return err
	}
	if _, err = poller.PollUntilDone(ctx, nil); err != nil {
		log.Printf("[NSG] ERROR polling lockdown rule deletion: %v", err)
		return err
	}

	c.locked = false
	log.Printf("[NSG] UNLOCK: lockdown rule removed from %s/%s", c.resourceGroup, c.nsgName)
	return nil
}

// IsLocked returns whether the NSG is currently in lockdown.
func (c *NSGController) IsLocked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.locked
}
