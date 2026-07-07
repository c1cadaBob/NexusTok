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
		Resource: "channel",
		LabelKey: "Channel Management",
		Actions: managementActions(
			"Edit sensitive channel settings",
			"Create channels or edit keys, base URLs, credential mode, request overrides, and provider secrets.",
		),
	})

	registerResource(ResourceDefinition{
		Resource: "account_pool",
		LabelKey: "Account Pool",
		Actions: managementActions(
			"Edit sensitive account pool settings",
			"Import, export, delete, refresh, or edit account credentials and native auth files.",
		),
	})

	registerResource(ResourceDefinition{
		Resource: "user",
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
		Resource: "model",
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
		Resource: "subscription",
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
		Resource: "system_setting",
		LabelKey: "System Settings",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "Read system settings",
				DescriptionKey: "View options, system instances, system tasks, logs, performance data, and routing metadata.",
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
