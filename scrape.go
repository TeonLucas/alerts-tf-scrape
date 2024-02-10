package main

import (
	"context"
	"fmt"
	"log"
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
			fmt.Println("New Relic login page, please log in")
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

func (data *LocalData) scrapeConditionTF(policy *Policy, condition Condition) (err error) {
	// Scrape condition and update policy TF
	err = chromedp.Run(data.CDPctx, policy.doScrapeCondition(condition.Name, condition.Guid))
	return
}
