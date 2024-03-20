package main

import (
	"context"
	"fmt"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"log"
	"sort"
	"time"

	"github.com/chromedp/chromedp"
)

// For Chrome web driver
func overrideHeadless() []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,

		// After Puppeteer's default behavior.
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-features", "site-per-process,TranslateUI,BlinkGenPropertyTrees"),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("force-color-profile", "srgb"),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
	}
}

func doLogin() chromedp.Tasks {
	return chromedp.Tasks{
		// Navigate to NR ui
		chromedp.Navigate("https://login.newrelic.com/login"),

		// Ask for user input
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("New Relic login page, please log in")
			time.Sleep(3 * time.Second)
			return nil
		}),

		// Wait for login complete
		chromedp.WaitNotPresent("form#login"),

		// Wait for login complete
		chromedp.WaitVisible("div[id='root']"),

		// Ask for user input
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Login complete")
			time.Sleep(3 * time.Second)
			return nil
		}),
	}
}

func (data *LocalData) startChromeAndLogin() (err error) {
	// Launch scraper
	log.Println("Launching Chrome web scraper")
	opts := overrideHeadless()
	ctx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	data.CDPctx, data.CDPcancel = chromedp.NewContext(ctx, chromedp.WithLogf(log.Printf))

	// Do login
	err = chromedp.Run(data.CDPctx, doLogin())
	return
}

func (data *LocalData) logout() {
	var err error

	// Logout
	if err = chromedp.Run(data.CDPctx, chromedp.Navigate("https://rpm.newrelic.com/logout")); err != nil {
		log.Println("Login error:", err)
	}
}

func (policy *Policy) doScrapeCondition(name, guid string) chromedp.Tasks {
	var text string
	return chromedp.Tasks{
		// Navigate to alert condition builder page
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Printf("Navigate to condition builder for %q", name)
			return nil
		}),
		chromedp.Navigate(fmt.Sprintf("https://one.newrelic.com/nr1-core/condition-builder/entity/%s?account=%d",
			guid, policy.AccountId)),
		chromedp.WaitVisible("div[class*='SelfEnd']>button[type='button']"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			log.Printf("Click [View as code] button")
			return nil
		}),
		chromedp.Click("div[class*='SelfEnd']>button[type='button']"),
		chromedp.WaitVisible("div[class*='StackItem']:first-child>div[role='button']"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			log.Printf("Click [Terraform] Code preview")
			return nil
		}),
		chromedp.Click("div[class*='StackItem']:first-child>div[role='button']"),
		chromedp.WaitVisible("div[class*='multiline-code']"),
		chromedp.Text("div[class*='multiline-code']", &text),
		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			log.Printf("Copied %d bytes of TF code", len(text))
			policy.TF += text
			return nil
		}),
	}
}

func (data *LocalData) concurrentScrape(policyIds []int) {
	// make channels
	outputChan := make(chan bool, data.Concurrent)
	inputChan := make(chan int, len(policyIds)+data.Concurrent)

	for _, id := range policyIds {
		inputChan <- id
	}
	for i := 0; i < data.Concurrent; i++ {
		inputChan <- 0
	}

	// Start concurrent scrapers
	for i := 1; i <= data.Concurrent; i++ {
		log.Printf("Opening new Chrome window %d\n", i)
		var scraperCtx context.Context
		var scraperCancel context.CancelFunc
		go func() {
			var err error
			var res *runtime.RemoteObject
			if err = chromedp.Run(data.CDPctx, chromedp.Evaluate(`window.open("about:blank", "", "resizable,scrollbars,status")`, &res)); err != nil {
				log.Printf("Error opening new Chrome window %d: %v\n", i, err)
			}

			var targets []*target.Info
			targets, err = chromedp.Targets(data.CDPctx)
			if err != nil {
				log.Printf("Error accessing new Chrome window %d targets: %v\n", i, err)
			}

			for x, t := range targets {
				if !t.Attached {
					scraperCtx, scraperCancel = chromedp.NewContext(data.CDPctx, chromedp.WithTargetID(t.TargetID))
					fmt.Printf("New Chrome window #%d, target #%d\n", i, x+1)
					defer scraperCancel()
					break
				}
			}

			for {
				var policyId int
				policyId = <-inputChan
				if policyId == 0 {
					// exit concurrent
					outputChan <- true
					break
				}

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
					err = chromedp.Run(scraperCtx, policy.doScrapeCondition(condition.Name, condition.Guid))
					if err != nil {
						log.Println("Scrape condition TF error:", err)
					}
				}
				data.PolicyMap[policyId] = policy
				policy.writeTF()
			}
		}()
	}

	// Complete scrapers
	for i := 1; i <= data.Concurrent; i++ {
		<-outputChan
		log.Printf("Completed scrape on Chrome window %d", i)
	}
}
