package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"
)

func (data *LocalData) writeCSV() {
	var rows [][]string

	outputCSV := fmt.Sprintf("alerts_%s.csv", data.AccountId)
	f, err := os.Create(outputCSV)
	if err != nil {
		log.Printf("Error opening csv: %v", err)
	}

	// Make rows
	rows = append(rows, []string{
		"conditionId",
		"conditionName",
		"policyId",
		"policyName",
		"entityGuid",
		"nrqlQuery",
	})
	for _, policyId := range data.PolicyIds {
		policy, ok := data.PolicyMap[policyId]
		if !ok {
			continue
		}
		var conditionIds []int
		for conditionId := range policy.ConditionMap {
			conditionIds = append(conditionIds, conditionId)
		}
		sort.Ints(conditionIds)
		for _, conditionId := range conditionIds {
			condition, ok := policy.ConditionMap[conditionId]
			if !ok {
				continue
			}
			rows = append(rows, []string{
				condition.Id,
				condition.Name,
				policy.Id,
				policy.Name,
				condition.Guid,
				condition.Query,
			})
		}
	}
	log.Printf("Writing csv %s", outputCSV)
	w := csv.NewWriter(f)
	err = w.WriteAll(rows)
	if err != nil {
		log.Printf("Error writing %s: %v", outputCSV, err)
	}
	w.Flush()
	f.Sync()
	f.Close()
}
