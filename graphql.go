package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	GraphQlEndpoint    = "https://api.newrelic.com/graphql"
	PolicyQuery        = `{actor {account(id: %s) {alerts {policiesSearch {policies {id incidentPreference name accountId} nextCursor}}}}}`
	PolicyQueryNext    = `query($cursor: String!) {actor {account(id: %s) {alerts {policiesSearch(cursor: $cursor) {policies {id incidentPreference name accountId} nextCursor}}}}}`
	ConditionQuery     = `query EntitySearchQuery {actor {entitySearch(query: "domain = 'AIOPS' AND type = 'CONDITION' AND accountId = %s", options: {tagFilter: ["id","policyId"]}) {results {entities {guid accountId type name tags {key values}} nextCursor}}}}`
	ConditionQueryNext = `query EntitySearchQuery($cursor: String!) {actor {entitySearch(query: "domain = 'AIOPS' AND type = 'CONDITION' AND accountId = %s", options: {tagFilter: ["id","policyId"]}) {results(cursor: $cursor) {entities {guid accountId type name tags {key values}} nextCursor}}}}`
)

// Alert entities
type Policy struct {
	AccountId          int    `json:"accountId"`
	Id                 string `json:"id"`
	Name               string `json:"name"`
	IncidentPreference string `json:"IncidentPreference"`
	ConditionMap       map[int]Condition
	TF                 string
}
type Condition struct {
	AccountId int    `json:"accountId"`
	PolicyId  string `json:"policyId"`
	Id        string `json:"id"`
	Name      string `json:"name"`
	Guid      string `json:"guid"`
}
type Entity struct {
	AccountId int    `json:"accountId"`
	Guid      string `json:"guid"`
	Name      string `json:"name"`
	Tags      []struct {
		Key    string   `json:"key"`
		Values []string `json:"values"`
	} `json:"tags"`
	Type string `json:"type"`
}

// GraphQl request and result formats
type GraphQlPayload struct {
	Query     string `json:"query"`
	Variables struct {
		Cursor string `json:"cursor"`
	} `json:"variables"`
}
type GraphQlResult struct {
	Errors []interface{} `json:"errors"`
	Data   struct {
		Actor struct {
			EntitySearch struct {
				Results struct {
					Entities   []Entity    `json:"entities"`
					NextCursor interface{} `json:"nextCursor"`
				} `json:"results"`
			} `json:"entitySearch"`
			Account struct {
				Alerts struct {
					PoliciesSearch struct {
						Policies   []Policy    `json:"policies"`
						NextCursor interface{} `json:"nextCursor"`
					} `json:"policiesSearch"`
				} `json:"alerts"`
			} `json:"account"`
		} `json:"actor"`
	} `json:"data"`
}

// Make API request with error retry
func retryQuery(client *http.Client, method, url, data string, headers []string) (b []byte) {
	var res *http.Response
	var err error
	var body io.Reader

	if len(data) > 0 {
		body = strings.NewReader(data)
	}

	// up to 3 retries on API error
	for j := 1; j <= 3; j++ {
		req, _ := http.NewRequest(method, url, body)
		for _, h := range headers {
			params := strings.Split(h, ":")
			req.Header.Set(params[0], params[1])
		}
		res, err = client.Do(req)
		if err != nil {
			log.Println(err)
		}
		if res != nil {
			if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusAccepted {
				break
			}
			log.Printf("Retry %d: http status %d", j, res.StatusCode)
		} else {
			log.Printf("Retry %d: no response", j)
		}
		time.Sleep(500 * time.Millisecond)
	}
	b, err = io.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return
	}
	res.Body.Close()
	return
}

func (data *LocalData) getPolicies() {
	var gQuery GraphQlPayload
	var j []byte
	var err error

	// Get Policies with IncidentPreference
	gQuery.Query = fmt.Sprintf(PolicyQuery, data.AccountId)
	for {
		// make query payload
		j, err = json.Marshal(gQuery)
		if err != nil {
			log.Printf("Error creating GraphQl policies query: %v", err)
		}
		b := retryQuery(data.Client, "POST", GraphQlEndpoint, string(j), data.GraphQlHeaders)

		// parse results
		var graphQlResult GraphQlResult
		log.Printf("Parsing GraphQl policies response %d bytes", len(b))
		err = json.Unmarshal(b, &graphQlResult)
		if err != nil {
			log.Printf("Error parsing GraphQl policies result: %v", err)
		}
		if len(graphQlResult.Errors) > 0 {
			log.Printf("Errors with GraphQl query: %v", graphQlResult.Errors)
		}
		policiesSearch := graphQlResult.Data.Actor.Account.Alerts.PoliciesSearch

		// store policies
		for _, policy := range policiesSearch.Policies {
			var id int
			id, err = strconv.Atoi(policy.Id)
			if err != nil {
				log.Printf("Error parsing policy Id: %v (policy %+v)", err, policy)
				continue
			}
			policy.ConditionMap = make(map[int]Condition)
			data.PolicyMap[id] = policy
		}
		if policiesSearch.NextCursor == nil {
			break
		}

		// get next page of results
		gQuery.Query = fmt.Sprintf(PolicyQueryNext, data.AccountId)
		gQuery.Variables.Cursor = fmt.Sprintf("%s", policiesSearch.NextCursor)
	}
	log.Printf("Found %d policies", len(data.PolicyMap))
}

func parseCondition(entity Entity) (condition Condition, err error) {
	condition.Guid = entity.Guid
	condition.Name = entity.Name
	condition.AccountId = entity.AccountId
	if entity.Type != "CONDITION" {
		err = fmt.Errorf("invalid condition type: %+v", entity)
		return
	}
	if len(entity.Tags) != 2 {
		err = fmt.Errorf("invalid condition tags: %+v", entity)
		return
	}
	for _, tag := range entity.Tags {
		if tag.Key == "policyId" {
			if len(tag.Values) != 1 {
				err = fmt.Errorf("invalid condition entity PolicyId: %+v", entity)
				return
			}
			condition.PolicyId = tag.Values[0]
		}
		if tag.Key == "id" {
			if len(tag.Values) != 1 {
				err = fmt.Errorf("invalid condition entity Id: %+v", entity)
				return
			}
			condition.Id = tag.Values[0]
		}
	}
	return
}

func (data *LocalData) getConditions() {
	var gQuery GraphQlPayload
	var j []byte
	var err error
	var conditionCount int

	// Get conditions, story in Policy map by guid
	gQuery.Query = fmt.Sprintf(ConditionQuery, data.AccountId)
	for {
		// make query payload
		j, err = json.Marshal(gQuery)
		if err != nil {
			log.Printf("Error creating GraphQl conditions query: %v", err)
		}
		b := retryQuery(data.Client, "POST", GraphQlEndpoint, string(j), data.GraphQlHeaders)

		// parse results
		var graphQlResult GraphQlResult
		log.Printf("Parsing GraphQl conditions response %d bytes", len(b))
		err = json.Unmarshal(b, &graphQlResult)
		if err != nil {
			log.Printf("Error parsing GraphQl conditions result: %v", err)
		}
		if len(graphQlResult.Errors) > 0 {
			log.Printf("Errors with GraphQl query: %v", graphQlResult.Errors)
		}
		conditionsSearch := graphQlResult.Data.Actor.EntitySearch.Results

		// store conditions
		for _, entity := range conditionsSearch.Entities {
			var condition Condition
			var policy Policy
			var ok bool
			var id, policyId int

			condition, err = parseCondition(entity)
			if err != nil {
				log.Printf("Error parsing condition: %v", err)
				continue
			}
			policyId, err = strconv.Atoi(condition.PolicyId)
			if err != nil {
				log.Printf("Error parsing condition policyId: %v (condition %+v)", err, condition)
				continue
			}
			policy, ok = data.PolicyMap[policyId]
			if !ok {
				log.Printf("Error locating policy for conditon: %+v", condition)
				continue
			}
			id, err = strconv.Atoi(condition.Id)
			if err != nil {
				log.Printf("Error parsing condition Id: %v (condition %+v)", err, condition)
				continue
			}
			policy.ConditionMap[id] = condition
			data.PolicyMap[policyId] = policy
			conditionCount++
		}
		if conditionsSearch.NextCursor == nil {
			break
		}

		// get next page of results
		gQuery.Query = fmt.Sprintf(ConditionQueryNext, data.AccountId)
		gQuery.Variables.Cursor = fmt.Sprintf("%s", conditionsSearch.NextCursor)
	}
	log.Printf("Found %d conditions", conditionCount)
}

func (data *LocalData) makeClient() {
	data.Client = &http.Client{}
	data.GraphQlHeaders = []string{"Content-Type:application/json", "API-Key:" + data.UserKey}
	data.PolicyMap = make(map[int]Policy)
}
