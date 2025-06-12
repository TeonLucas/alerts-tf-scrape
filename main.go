package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
)

type LocalData struct {
	AccountId      int
	UserKey        string
	Concurrent     int
	CSVonly        bool
	Disable        bool
	Client         *http.Client
	GraphQlHeaders []string
	CDPctx         context.Context
	CDPcancel      context.CancelFunc
	PolicyIds      []int
	PolicyMap      map[int]Policy
	ConditionMap   map[int]Condition
	Dump           string
}

func main() {
	var err error

	// Get required settings
	data := LocalData{
		UserKey:    os.Getenv("NEW_RELIC_USER_KEY"),
		Concurrent: 1,
	}

	// Get commandline options
	flag.BoolVar(&data.CSVonly, "csv", false, "Generate CSV mode")
	flag.BoolVar(&data.Disable, "disable", false, "Disable all NRQL conditions")
	flag.Parse()
	if data.CSVonly {
		log.Printf("CSV mode enabled")
	}
	if data.Disable {
		log.Printf("Disable all NRQL conditions")
	}

	// Validate settings
	concurrent := os.Getenv("CONCURRENT")
	if len(concurrent) > 0 {
		data.Concurrent, err = strconv.Atoi(concurrent)
		if err != nil {
			log.Printf("Invalid env var CONCURRENT setting: %v", err)
			os.Exit(0)
		}
		if data.Concurrent > 20 {
			data.Concurrent = 20
			log.Printf("Limiting env var CONCURRENT to 20", err)
		}
	}
	accountId := os.Getenv("NEW_RELIC_ACCOUNT")
	if len(accountId) == 0 {
		log.Printf("Please set env var NEW_RELIC_ACCOUNT")
		os.Exit(1)
	}
	data.AccountId, err = strconv.Atoi(accountId)
	if err != nil {
		log.Printf("Please set env var NEW_RELIC_ACCOUNT to an integer")
		os.Exit(1)
	}
	if len(data.UserKey) == 0 {
		log.Printf("Please set env var NEW_RELIC_USER_KEY")
		os.Exit(1)
	}
	data.makeClient()

	// Get list of policies
	data.getPolicies()

	// Get conditions for these
	data.getConditions()
	data.getConditionDetails()

	if data.CSVonly {
		data.writeCSV()
		os.Exit(0)
	}

	// Login for scraper
	err = data.startChromeAndLogin()
	if err != nil {
		log.Printf("Issue loggin into NR1: %v", err)
		os.Exit(1)
	}

	// Generate Terraform and write files
	data.walkPolicies()

	// Exit
	data.logout()
	log.Println("Done")
}
