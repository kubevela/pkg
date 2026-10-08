// Reduced from kubevela/workflow's legacy op package, keeping only what
// reproduces a call's result coming back bottom once working out what the
// calls read has evaluated the value they run in. Each line is needed.
package op

#MakePlacementDecisions: {
	#provider: "op"
	#do:       "make-placement-decisions"
	inputs: {
		placement:  #Placement
	}
	outputs: {
	}
}
#PatchApplication: {
	inputs: {
	}
}
#Placement: {
	clusterSelector?: {
	}
}
#PlacementDecision: {
}
#Component: {
}
#LoadEnvBindingEnv: {
	inputs: {
	}
	loadPolicies: #LoadPolicies
	if inputs.policy == "" && loadPolicies.value != _|_ {
		envBindingPolicies: [for k, v in loadPolicies.value if v.type == "env-binding" {k}]
		if len(envBindingPolicies) > 0 {
		}
	}
	envMap: {
	}
	outputs: {
	}
}
#PrepareEnvBinding: {
	inputs: {
	}
	loadEnv: #LoadEnvBindingEnv & {
		inputs: {
		}
	}
	placementDecisions: #MakePlacementDecisions & {
		inputs: {
		}
	}
	outputs: {
		for decision in inputs.decisions {
			for key, comp in inputs.components {
				"\(decision.cluster)-\(decision.namespace)-\(key)": #ApplyComponent & {
				}
			}
		}
	}
}
#ApplyComponent: {
}
#LoadPolicies: {
	#provider: "op"
	#do:       "load-policies"
}
#LoadTerraformComponents: {
	#provider: "op"
	#do:       "load-terraform-components"
	outputs: {
	}
}
#GetConnectionStatus: {
	inputs: {
	}
}
#PrepareTerraformEnvBinding: {
	inputs: {
	}
	prepare: #PrepareEnvBinding & {
		inputs: {
		}
	}
	loadTerraformComponents: #LoadTerraformComponents
	terraformComponentMap: {
		for _, comp in loadTerraformComponents.outputs.components {
		}
	}
	outputs: {
	}
}
#bindTerraformComponentToCluster: {
	decisions: [...{...}]
	status: #GetConnectionStatus & {
		value: {
			metadata: {
			}
		}
	}
	sync: {
		for decision in decisions {
			"\(decision.cluster)-\(decision.namespace)": #Apply & {
				value: {
				}
			}
		}
	}
}
#ShareCloudResource: {
	prepareBind: #PrepareTerraformEnvBinding & {
		ratelimiter?: {
		}
	}
	tls_config?: {
	}
}
#Apply: {
}