// Reduced from kubevela/workflow's legacy op package to what reproduces a
// call's result coming back bottom once working out what the calls read has
// evaluated the value they run in. Every line of CUE here is needed to reproduce it.
package op

#MakePlacementDecisions: {
	#provider: "op"
	#do:       "make-placement-decisions"
	inputs: {
		placement: #Placement
	}
}

#Placement: {
}

#LoadEnvBindingEnv: {
	loadPolicies: #LoadPolicies
}

#PrepareEnvBinding: {
	inputs: {
	}
	loadEnv: #LoadEnvBindingEnv & {
	}
	placementDecisions: #MakePlacementDecisions & {
		for decision in inputs.decisions {
		}
	}
}

#LoadPolicies: {
	#provider: "op"
	#do:       "load-policies"
}

#LoadTerraformComponents: {
	#provider: "op"
	#do:       "load-terraform-components"
}

#PrepareTerraformEnvBinding: {
	prepare: #PrepareEnvBinding & {
	}
	loadTerraformComponents: #LoadTerraformComponents
}

#ShareCloudResource: {
	prepareBind: #PrepareTerraformEnvBinding & {
	}
}
