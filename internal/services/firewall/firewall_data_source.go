package firewall

import (
	"fmt"
	"time"

	"github.com/hashicorp/terraform-provider-azurerm/internal/services/firewall/parse"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-02-01/network"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/firewall/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func firewallDataSource() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Read: firewallDataSourceRead,

		Timeouts: &pluginsdk.ResourceTimeout{
			Read: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: validate.FirewallName,
			},

			"location": azure.SchemaLocationForDataSource(),

			"resource_group_name": azure.SchemaResourceGroupNameForDataSource(),

			"sku_name": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"sku_tier": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"firewall_policy_id": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"ip_configuration": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"name": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"subnet_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"public_ip_address_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"private_ip_address": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
					},
				},
			},

			"management_ip_configuration": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"name": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"subnet_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"public_ip_address_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"private_ip_address": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
					},
				},
			},

			"threat_intel_mode": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"dns_servers": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
				},
			},

			"virtual_hub": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"virtual_hub_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
						"public_ip_count": {
							Type:     pluginsdk.TypeInt,
							Computed: true,
						},
						"public_ip_addresses": {
							Type:     pluginsdk.TypeList,
							Computed: true,
							Elem: &pluginsdk.Schema{
								Type: pluginsdk.TypeString,
							},
						},
						"private_ip_address": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
					},
				},
			},

			"zones": azure.SchemaZonesComputed(),

			"tags": tags.SchemaDataSource(),
		},
	}
}

func firewallDataSourceRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Firewall.AzureFirewallsClient
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id := parse.NewFirewallID(subscriptionId, d.Get("resource_group_name").(string), d.Get("name").(string))
	read, err := client.Get(ctx, id.ResourceGroup, id.AzureFirewallName)
	if err != nil {
		if utils.ResponseWasNotFound(read.Response) {
			return fmt.Errorf("%s was not found", id)
		}

		return fmt.Errorf("retrieving %s: %+v", id, err)
	}

	d.SetId(id.ID())
	d.Set("name", id.AzureFirewallName)
	d.Set("resource_group_name", id.ResourceGroup)

	if location := read.Location; location != nil {
		d.Set("location", azure.NormalizeLocation(*location))
	}

	if props := read.AzureFirewallPropertiesFormat; props != nil {
		if err := d.Set("ip_configuration", flattenFirewallIPConfigurations(props.IPConfigurations)); err != nil {
			return fmt.Errorf("setting `ip_configuration`: %+v", err)
		}
		managementIPConfigs := make([]interface{}, 0)
		if props.ManagementIPConfiguration != nil {
			managementIPConfigs = flattenFirewallIPConfigurations(&[]network.AzureFirewallIPConfiguration{
				*props.ManagementIPConfiguration,
			})
		}
		if err := d.Set("management_ip_configuration", managementIPConfigs); err != nil {
			return fmt.Errorf("setting `management_ip_configuration`: %+v", err)
		}

		d.Set("threat_intel_mode", string(props.ThreatIntelMode))

		if err := d.Set("dns_servers", flattenFirewallDNSServers(props.AdditionalProperties)); err != nil {
			return fmt.Errorf("setting `dns_servers`: %+v", err)
		}

		if policy := props.FirewallPolicy; policy != nil {
			d.Set("firewall_policy_id", policy.ID)
		}

		if sku := props.Sku; sku != nil {
			d.Set("sku_name", string(sku.Name))
			d.Set("sku_tier", string(sku.Tier))
		}

		if err := d.Set("virtual_hub", flattenFirewallVirtualHubSetting(props)); err != nil {
			return fmt.Errorf("setting `virtual_hub`: %+v", err)
		}
	}

	if err := d.Set("zones", azure.FlattenZones(read.Zones)); err != nil {
		return fmt.Errorf("setting `zones`: %+v", err)
	}

	return tags.FlattenAndSet(d, read.Tags)
}
