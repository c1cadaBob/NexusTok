package authz

var registry []ResourceDefinition

func registerResource(resource ResourceDefinition) {
	registry = append(registry, resource)
}

func managementActions(sensitiveLabel string, sensitiveDescription string) []ActionDefinition {
	return []ActionDefinition{
		{
			Action:         ActionRead,
			LabelKey:       "Read",
			DescriptionKey: "View lists, details, and non-secret metadata.",
			DefaultRoles:   []string{BuiltInRoleAdmin},
		},
		{
			Action:         ActionOperate,
			LabelKey:       "Operate",
			DescriptionKey: "Run checks, tests, syncs, refreshes, and other operational actions.",
			DefaultRoles:   []string{BuiltInRoleAdmin},
		},
		{
			Action:         ActionWrite,
			LabelKey:       "Write",
			DescriptionKey: "Create or edit non-sensitive configuration.",
			DefaultRoles:   []string{BuiltInRoleAdmin},
		},
		{
			Action:         ActionSensitiveWrite,
			LabelKey:       sensitiveLabel,
			DescriptionKey: sensitiveDescription,
		},
		{
			Action:         ActionSecretView,
			LabelKey:       "View secrets",
			DescriptionKey: "View complete keys, credentials, or other secret material after secure verification.",
		},
	}
}

func init() {
	registerResource(ResourceDefinition{
		Resource: ResourceChannel,
		LabelKey: "Channel Management",
		Actions: managementActions(
			"Edit sensitive channel settings",
			"Create channels or edit keys, base URLs, credential mode, request overrides, and provider secrets.",
		),
	})

	registerResource(ResourceDefinition{
		Resource: ResourceChannelAccount,
		LabelKey: "Channel Accounts",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read channel accounts",
				DescriptionKey: "View channel-owned account lists, details, masked keys, and routing metadata.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate channel accounts",
				DescriptionKey: "Enable, disable, clear cooldowns, and run safe lifecycle operations for channel-owned accounts.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit channel account credentials",
				DescriptionKey: "Create, import, update, or delete channel-owned upstream credentials.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceAccountPool,
		LabelKey: "Account Pool",
		Actions: managementActions(
			"Edit sensitive account pool settings",
			"Delete groups, refresh account credentials, or edit account lifecycle settings.",
		),
	})

	registerResource(ResourceDefinition{
		Resource: ResourceAccountPoolAuthFile,
		LabelKey: "Account Pool Auth Files",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read account pool auth files",
				DescriptionKey: "View imported credential files and non-secret metadata.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit account pool auth files",
				DescriptionKey: "Import, update, delete, or replace credential JSON for account pool auth files.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceUser,
		LabelKey: "User Management",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read users",
				DescriptionKey: "View user lists, profiles, quota, and subscription summaries.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate users",
				DescriptionKey: "Reset passkeys, disable 2FA, bind subscriptions, or run user-level maintenance actions.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit users",
				DescriptionKey: "Edit ordinary user profile, quota, group, and status fields below the operator role.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit privileged users",
				DescriptionKey: "Create, delete, promote, demote, or edit administrators and root-sensitive identity settings.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceModel,
		LabelKey: "Model Management",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read models",
				DescriptionKey: "View model, vendor, deployment, and pricing metadata.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate models",
				DescriptionKey: "Preview or run upstream model sync and deployment connection checks.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit models",
				DescriptionKey: "Create or edit model metadata, vendor metadata, pricing, prefill groups, and deployments.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit sensitive model settings",
				DescriptionKey: "Delete model or vendor records, change provider-level pricing, or edit deployment lifecycle settings.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceSubscription,
		LabelKey: "Subscription Management",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read subscriptions",
				DescriptionKey: "View subscription plans, user subscriptions, and payment-related summaries.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate subscriptions",
				DescriptionKey: "Bind plans, reset subscription quota, invalidate user subscriptions, or run payment maintenance actions.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit subscription plans",
				DescriptionKey: "Create or edit plan title, duration, amount, purchase limits, and non-secret payment mappings.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit sensitive subscription settings",
				DescriptionKey: "Delete subscriptions, change payment compliance, or edit root-owned payment provider settings.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceRedemption,
		LabelKey: "Redemption Codes",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read redemption codes",
				DescriptionKey: "View redemption code lists, details, status, quota, and redemption metadata.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate redemption codes",
				DescriptionKey: "Enable, disable, or clean up redemption codes through maintenance actions.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit redemption codes",
				DescriptionKey: "Create redemption codes or edit ordinary redemption code metadata, quota, expiry, and status.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Delete redemption codes",
				DescriptionKey: "Delete redemption code records or perform bulk cleanup that removes redemption audit data.",
			},
			{
				Action:         ActionSecretView,
				LabelKey:       "View redemption codes",
				DescriptionKey: "View complete redemption code values after secure verification.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceUsageLog,
		LabelKey: "Usage Logs",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read usage logs",
				DescriptionKey: "View cross-user request logs, usage statistics, audit entries, and routing cache usage details.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Delete usage logs",
				DescriptionKey: "Delete historical request logs, audit logs, or other usage evidence that may be needed for investigation.",
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceUsageData,
		LabelKey: "Usage Data",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read usage data",
				DescriptionKey: "View cross-user quota trends, user usage rankings, and traffic flow analytics.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})

	registerResource(ResourceDefinition{
		Resource: ResourceSystemSetting,
		LabelKey: "System Settings",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read system settings",
				DescriptionKey: "View options, system instances, system tasks, performance data, and routing metadata.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Operate system",
				DescriptionKey: "Run cleanup, cache clear, model ratio reset, performance maintenance, and system task actions.",
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit system settings",
				DescriptionKey: "Edit global options, OAuth providers, payment integrations, pricing settings, and navigation settings.",
			},
			{
				Action:         ActionSensitiveWrite,
				LabelKey:       "Edit sensitive system settings",
				DescriptionKey: "Change secrets, root-only options, callbacks, webhooks, and security-critical global configuration.",
			},
			{
				Action:         ActionSecretView,
				LabelKey:       "View system secrets",
				DescriptionKey: "View complete system secrets, OAuth credentials, webhook secrets, or payment provider keys.",
			},
		},
	})
}
