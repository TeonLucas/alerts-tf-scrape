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

	// Sort policy ids
	policyIds := make([]int, len(data.PolicyMap))
	for policyId = range data.PolicyMap {
		policyIds[i] = policyId
		i++
	}
	sort.Ints(policyIds)

	// Traverse policies concurrently
	log.Printf("Walking %d policies to generate Terraform", len(policyIds))
	log.Printf("Using concurrency=%d", data.Concurrent)
	data.concurrentScrape(policyIds)
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
