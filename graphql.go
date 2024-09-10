package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	GraphQlEndpoint = "https://api.newrelic.com/graphql"
	GrQl_Parallel   = 8
	PolicyQuery     = `query($cursor: String) {actor {account(id: %d) {alerts {policiesSearch(cursor: $cursor) {policies {id incidentPreference name accountId} nextCursor}}}}}`
	ConditionQuery  = `query EntitySearchQuery($cursor: String) {actor {entitySearch(query: "domain = 'AIOPS' AND type = 'CONDITION' AND accountId = %d", options: {tagFilter: ["id","policyId"]}) {results(cursor: $cursor) {entities {guid accountId type name tags {key values}} nextCursor}}}}`
	DetailQuery     = `query getConditionDetail($accountId: Int!, $conditionId: ID!) {actor {account(id: $accountId) {alerts {nrqlCondition(id: $conditionId) {nrql {query} name id}}}}}`
)

// Alert entities
type Policy struct {
	AccountId          int    `json:"accountId"`
	Id                 string `json:"id"`
	Name               string `json:"name"`
	IncidentPreference string `json:"IncidentPreference"`
	ConditionIds       []int
	TF                 string
}
type Condition struct {
	AccountId int    `json:"accountId"`
	PolicyId  string `json:"policyId"`
	Id        string `json:"id"`
	Name      string `json:"name"`
	Guid      string `json:"guid"`
	Query     string
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
type Output struct {
	ConditionId int
	Query       string
}

// GraphQl request and result formats
type GraphQlPayload struct {
	Query     string `json:"query"`
	Variables struct {
		AccountId   int    `json:"accountId,omitempty"`
		ConditionId string `json:"conditionId,omitempty"`
		Cursor      string `json:"cursor,omitempty"`
	} `json:"variables"`
}
type GraphQlResult struct {
	Errors []Error `json:"errors"`
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
					NrqlCondition  NrqlCondition `json:"nrqlCondition"`
					PoliciesSearch struct {
						Policies   []Policy    `json:"policies"`
						NextCursor interface{} `json:"nextCursor"`
					} `json:"policiesSearch"`
				} `json:"alerts"`
			} `json:"account"`
		} `json:"actor"`
	} `json:"data"`
}
type NrqlCondition struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Nrql struct {
		Query string `json:"query"`
	} `json:"nrql"`
}
type Error struct {
	Message string `json:"message"`
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
			data.PolicyMap[id] = policy
		}
		if policiesSearch.NextCursor == nil {
			break
		}

		// get next page of results
		gQuery.Variables.Cursor = fmt.Sprintf("%s", policiesSearch.NextCursor)
	}

	// Sort policy ids
	for policyId := range data.PolicyMap {
		data.PolicyIds = append(data.PolicyIds, policyId)
	}
	sort.Ints(data.PolicyIds)
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

func (data *LocalData) getConditionDetails() {
	inputChan := make(chan int, len(data.ConditionMap)+GrQl_Parallel)
	outputChan := make(chan Output, len(data.ConditionMap)+GrQl_Parallel)

	// Load conditions into channel
	go func() {
		for _, policyId := range data.PolicyIds {
			policy := data.PolicyMap[policyId]
			// Sort condition ids
			sort.Ints(policy.ConditionIds)
			for _, id := range policy.ConditionIds {
				inputChan <- id
			}
		}
		for n := 0; n < GrQl_Parallel; n++ {
			inputChan <- 0
		}
	}()

	for i := 1; i <= GrQl_Parallel; i++ {
		log.Printf("GraphQL - starting condition detail requestor #%d", i)
		go func() {
			var gQuery GraphQlPayload
			var j []byte
			var err error
			gQuery.Query = DetailQuery
			client := &http.Client{}
			for {
				conditionId := <-inputChan
				if conditionId == 0 {
					outputChan <- Output{}
					break
				}
				gQuery.Variables.ConditionId = fmt.Sprintf("%d", conditionId)
				gQuery.Variables.AccountId = data.AccountId
				// make query payload
				j, err = json.Marshal(gQuery)
				if err != nil {
					log.Printf("Error creating GraphQl condition detail query: %v", err)
					continue
				}
				b := retryQuery(client, "POST", GraphQlEndpoint, string(j), data.GraphQlHeaders)
				// parse results
				var graphQlResult GraphQlResult
				err = json.Unmarshal(b, &graphQlResult)
				if err != nil {
					log.Printf("Error parsing GraphQl condition detail result: %v", err)
					continue
				}
				if len(graphQlResult.Errors) > 0 {
					if graphQlResult.Errors[0].Message == "Not Found" {
						continue
					}
					log.Printf("Errors with GraphQl query: %v", graphQlResult.Errors)
					continue
				}
				outputChan <- Output{
					ConditionId: conditionId,
					Query:       graphQlResult.Data.Actor.Account.Alerts.NrqlCondition.Nrql.Query,
				}
			}
		}()
	}

	queries := 0
	for i := 0; i < GrQl_Parallel; i++ {
		for {
			output := <-outputChan
			if output.ConditionId == 0 {
				log.Printf("GraphQL - ending condition detail requestor #%d", i+1)
				break
			}
			condition, ok := data.ConditionMap[output.ConditionId]
			if !ok {
				log.Printf("GraphQL condition detail, no condition for id %d", output.ConditionId)
				continue
			}
			condition.Query = output.Query
			data.ConditionMap[output.ConditionId] = condition
			queries++
		}
	}
	log.Printf("GraphQL - finished condition detail requesters, %d nrql conditions found", queries)
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
			data.ConditionMap[id] = condition
			policy.ConditionIds = append(policy.ConditionIds, id)
			data.PolicyMap[policyId] = policy
			conditionCount++
		}
		if conditionsSearch.NextCursor == nil {
			break
		}
		// get next page of results
		gQuery.Variables.Cursor = fmt.Sprintf("%s", conditionsSearch.NextCursor)
	}
	log.Printf("Found %d conditions", conditionCount)
}

func (data *LocalData) makeClient() {
	data.Client = &http.Client{}
	data.GraphQlHeaders = []string{"Content-Type:application/json", "API-Key:" + data.UserKey}
	data.PolicyMap = make(map[int]Policy)
	data.ConditionMap = make(map[int]Condition)
}
