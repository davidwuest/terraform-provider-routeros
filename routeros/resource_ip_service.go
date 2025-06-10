package routeros

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

/*
  {
    ".id": "*0",
    "address": "",
    "disabled": "false",
    "invalid": "false",
    "name": "telnet",
    "port": "23",
    "vrf": "main"
  },
  {
    ".id": "*6",
    "address": "",
    "certificate": "https-cert",
    "disabled": "false",
    "invalid": "false",
    "name": "www-ssl",
    "port": "443",
    "tls-version": "any",
    "vrf": "main"
  },
*/

// https://help.mikrotik.com/docs/display/ROS/Services
func ResourceIpService() *schema.Resource {
	resSchema := map[string]*schema.Schema{
		MetaResourcePath: PropResourcePath("/ip/service"),
		MetaId:           PropId(Name),

		"address": {
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "",
			Description: "List of IP/IPv6 prefixes from which the service is accessible.",
			DiffSuppressFunc: func(k, oldValue, newValue string, d *schema.ResourceData) bool {
				if oldValue == "" && newValue == "0.0.0.0/0" {
					return false
				}
				return oldValue == newValue
			},
		},
		"certificate": {
			Type:     schema.TypeString,
			Optional: true,
			Description: "The name of the certificate used by a particular service. Applicable only for services " +
				"that depend on certificates ( www-ssl, api-ssl ).",
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		KeyDisabled: PropDisabledRw,
		KeyDynamic:  PropDynamicRo,
		KeyInvalid:  PropInvalidRo,
		"max_sessions": {
			Type:             schema.TypeInt,
			Optional:         true,
			Description:      "Maximum number of concurrent connections to a particular service. This option is available in RouterOS starting from version 7.16.",
			ValidateFunc:     validation.IntAtLeast(1),
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		"name": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Service name.",
		},
		"numbers": {
			Type:     schema.TypeString,
			Required: true,
			Description: "The name of the service whose settings will be changed ( api, api-ssl, ftp, ssh, telnet, " +
				"winbox, www, www-ssl ).",
			ValidateDiagFunc: ValidationMultiValInSlice([]string{"api", "api-ssl", "ftp", "ssh", "telnet", "winbox",
				"www", "www-ssl"}, false, false),
		},
		"port": {
			Type:         schema.TypeInt,
			Required:     true,
			Description:  "The port particular service listens on.",
			ValidateFunc: validation.IntBetween(1, 65535),
		},
		"proto": {
			Type:     schema.TypeString,
			Computed: true,
		},
		"tls_version": {
			Type:             schema.TypeString,
			Optional:         true,
			Description:      "Specifies which TLS versions to allow by a particular service.",
			ValidateFunc:     validation.StringInSlice([]string{"any", "only-1.2"}, false),
			DiffSuppressFunc: AlwaysPresentNotUserProvided,
		},
		KeyVrf: PropVrfRw,
	}

	resRead := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		path := resSchema[MetaResourcePath].Default.(string)
		filter := map[string]any{"name": d.Get("numbers")}

		ver, err := parseRouterOSVersion(RouterOSVersion)
		if err != nil {
			panic(err)
		}

		// ROS 7.19 => 463616
		if ver >= 463616 {
			filter["dynamic"] = "false"
		}

		res, err := ReadItemsFiltered(buildReadFilter(filter), path, m.(Client))
		if err != nil {
			ColorizedDebug(ctx, fmt.Sprintf(ErrorMsgGet, err))
			return diag.FromErr(err)
		}

		// Resource not found.
		if len(*res) == 0 {
			d.SetId("")
			ColorizedDebug(ctx, fmt.Sprintf(ErrorMsgPatch, err))
			return diag.FromErr(errorNoLongerExists)
		}

		d.SetId((*res)[0].GetID(Id))

		return MikrotikResourceDataToTerraform((*res)[0], resSchema, d)
	}

	resCreateUpdate := func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		item, metadata := TerraformResourceDataToMikrotik(resSchema, d)

		d.SetId(d.Get("numbers").(string))

		var resUrl string
		if m.(Client).GetTransport() == TransportREST {
			// https://router/rest/system/identity/set
			// https://router/rest/caps-man/manager/set
			resUrl = "/set"
		}

		err := m.(Client).SendRequest(crudPost, &URL{Path: metadata.Path + resUrl}, item, nil)
		if err != nil {
			return diag.FromErr(err)
		}

		return resRead(ctx, d, m)
	}

	return &schema.Resource{
		CreateContext: resCreateUpdate,
		ReadContext:   resRead,
		UpdateContext: resCreateUpdate,
		DeleteContext: DefaultSystemDelete(resSchema),

		Importer: &schema.ResourceImporter{
			StateContext: ImportStateCustomContext(resSchema),
		},

		Schema: resSchema,
	}
}
