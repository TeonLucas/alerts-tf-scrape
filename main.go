package main

import (
	"context"
	"log"
	"net/http"
	"os"
)

type LocalData struct {
	AccountId      string
	UserKey        string
	Client         *http.Client
	GraphQlHeaders []string
	CDPctx         context.Context
	CDPcancel      context.CancelFunc
	PolicyMap      map[int]Policy
	Dump           string
}

func main() {
	var err error

	// Get required settings
	data := LocalData{
		AccountId: os.Getenv("NEW_RELIC_ACCOUNT"),
		UserKey:   os.Getenv("NEW_RELIC_USER_KEY"),
	}
	if len(data.AccountId) == 0 {
		log.Printf("Please set env var NEW_RELIC_ACCOUNT")
		os.Exit(0)
	}
	if len(data.UserKey) == 0 {
		log.Printf("Please set env var NEW_RELIC_USER_KEY")
		os.Exit(0)
	}
	data.makeClient()

	// Get list of policies
	data.getPolicies()

	// Get conditions for these
	data.getConditions()

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
