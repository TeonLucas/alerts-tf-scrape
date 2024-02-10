package main

import (
	"fmt"
	"log"
	"os"
	"sort"
)

// Generate the policy Terraform code
func (policy *Policy) makePolicyTF() {
	policy.TF = fmt.Sprintf(`resource "newrelic_alert_policy" "policy_%s" {
  account_id = %d
  policy_id = %s
  name = %q
  incident_preference = %q
}`+"\n\n", policy.Id, policy.AccountId, policy.Id, policy.Name, policy.IncidentPreference)
	return
}

// Walk the policies to scrape each condition Terraform code
func (data *LocalData) walkPolicies() {
	var policyId, i int
	var err error

	// Sort policy ids
	policyIds := make([]int, len(data.PolicyMap))
	for policyId = range data.PolicyMap {
		policyIds[i] = policyId
		i++
	}
	sort.Ints(policyIds)

	// Traverse policies in order
	log.Printf("Walking %d policies to generate Terraform", len(policyIds))
	for _, policyId = range policyIds {
		var conditionId, j int
		var policy Policy

		// Start the TF code with the policy definiton
		policy = data.PolicyMap[policyId]
		policy.makePolicyTF()

		// Sort condition ids
		conditionIds := make([]int, len(policy.ConditionMap))
		for conditionId = range policy.ConditionMap {
			conditionIds[j] = conditionId
			j++
		}
		sort.Ints(conditionIds)

		// Traverse conditions in order
		for _, conditionId = range conditionIds {
			condition := policy.ConditionMap[conditionId]

			// Do scrape
			data.scrapeConditionTF(&policy, condition)
			if err != nil {
				log.Println("Scrape condition TF error:", err)
			}
		}
		data.PolicyMap[policyId] = policy

		policy.writeTF()
	}
}

func (policy *Policy) writeTF() {
	filename := fmt.Sprintf("policy_%s.tf", policy.Id)
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("Error opening alert policy terraform: %v", err)
	}

	log.Printf("Writing alert policy terraform to %s", filename)
	f.WriteString(policy.TF)
	f.WriteString("\n")
	f.Sync()
	f.Close()
}
